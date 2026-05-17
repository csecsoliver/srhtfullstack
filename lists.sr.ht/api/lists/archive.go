package lists

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
	apiErr "git.sr.ht/~sircmpwn/lists.sr.ht/api/errors"
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model"
	"github.com/emersion/go-mbox"
	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/lib/pq"
)

type Archiver struct {
	ctx      context.Context
	listID   int
	isImport bool
}

// Create a new message archiver.
//
// Note: the archiver does not attempt to verify access controls and will
// unconditionally complete the requested operation. The user is expected to
// verify the necessary permissions are available before use.
func NewArchiver(ctx context.Context, tx *sql.Tx, listID int) *Archiver {
	return &Archiver{ctx: ctx, listID: listID, isImport: false}
}

type ImportResult struct {
	Dropped   []error
	Imported  int
	Duplicate int
}

// Import an mbox spool into a mailing list.
//
// Does not enforce access controls. If error is non-nil, the import was aborted
// and rolled back.
func (ar *Archiver) ImportSpool(spool io.Reader) (ImportResult, error) {
	ar.isImport = true
	defer func() { ar.isImport = false }()

	var (
		result ImportResult
		input  int // used to track the source message in the mbox file
	)

	r := mbox.NewReader(spool)
	for {
		input += 1
		select {
		case <-ar.ctx.Done():
			return result, errors.New("mailing list spool import timed out")
		default:
		}

		msg, err := r.NextMessage()
		if err == io.EOF {
			break
		} else if err != nil {
			return result, fmt.Errorf("error reading message %d from spool: %v", input, err)
		}

		if _, err = ar.ArchiveMessage(msg); err != nil {
			if errors.Is(err, apiErr.ErrDuplicateEmail) {
				result.Duplicate += 1
				continue
			}
			ie := fmt.Errorf("error importing message %d: %v", input, err)
			log.Println(err.Error())
			result.Dropped = append(result.Dropped, ie)
		} else {
			result.Imported += 1
		}
	}

	return result, nil
}

// Import a single email (RFC 2045 MIME message) into a mailing list archive.
//
// Does not enforce access controls.
func (ar *Archiver) ArchiveMessage(r io.Reader) (int, error) {
	var (
		emailID int
		err_    error
	)
	err := database.WithTx(ar.ctx, nil, func(tx *sql.Tx) error {
		emailID, err_ = ar.archiveMessage(tx, r)
		return err_
	})
	return emailID, err
}

func (ar *Archiver) archiveMessage(tx *sql.Tx, r io.Reader) (int, error) {
	var rawMessage bytes.Buffer

	mr, err := mail.CreateReader(io.TeeReader(r, &rawMessage))
	if err != nil {
		return 0, err
	}
	subject, err := mr.Header.Subject()
	if err != nil {
		if !message.IsUnknownCharset(err) {
			return 0, fmt.Errorf("error reading Subject: %w", err)
		}
		if subject == "" {
			// even if the subject is garbage, at least store something in the db
			subject = strings.TrimSpace(mr.Header.Get("Subject"))
		}
	}
	// TODO: Store Message-ID without "<>" in database
	messageID := mr.Header.Get("Message-ID")
	date, err := mr.Header.Date()
	if err != nil {
		log.Printf("Error reading Date in message %q: %v", messageID, err)
		// fallback on using the current time
		date = time.Now()
	}
	inReplyToList, err := mr.Header.MsgIDList("In-Reply-To")
	if err != nil {
		// do not fail miserably on malformed In-Reply-To headers
		log.Printf("Error reading In-Reply-To: %v", err)
		irp := strings.Trim(mr.Header.Get("In-Reply-To"), " \r\t\n<>,")
		if irp != "" {
			inReplyToList = []string{irp}
		}
	}
	var inReplyTo sql.NullString
	if len(inReplyToList) > 0 {
		// TODO: multiple In-Reply-To message IDs?
		inReplyTo.String = inReplyToList[0]
		inReplyTo.Valid = true
	}

	var body string

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			return 0, fmt.Errorf("error reading part of message %q: %w", messageID, err)
		}

		switch p.Header.(type) {
		case *mail.InlineHeader:
			// This could be an inline attachment, so check content type
			h := p.Header.(*mail.InlineHeader)
			ct, _, _ := h.ContentType()

			// text/plain always wins, text/* used as fallback
			if ct == "text/plain" || (strings.HasPrefix("text/", ct) && body == "") {
				b, _ := io.ReadAll(p.Body)
				body = string(b)
			}
		case *mail.AttachmentHeader:
			// Do nothing
		}
	}

	isPatch := identifyPatch(body)
	// TODO: Identify request-pull
	isRequestPull := false

	headerMap, err := json.Marshal(mr.Header.Map())
	if err != nil {
		return 0, fmt.Errorf("error marshalling message %q: %v", messageID, err)
	}

	// Take an applicative lock on the list to avoid the race condition
	// described in https://todo.sr.ht/~sircmpwn/lists.sr.ht/215
	if _, err := tx.ExecContext(ar.ctx,
		`SELECT pg_advisory_xact_lock($1)`,
		ar.listID,
	); err != nil {
		return 0, err
	}

	var exists bool
	row := tx.QueryRow(`
		SELECT EXISTS(
			SELECT FROM email WHERE list_id = $1 AND message_id = $2
		)`,
		ar.listID, messageID,
	)
	if err := row.Scan(&exists); err != nil {
		return 0, err
	}
	if exists {
		// Skip this message
		log.Printf("Skipping duplicate message %q", messageID)
		return 0, apiErr.ErrDuplicateEmail
	}

	var emailID int
	row = tx.QueryRow(`
		INSERT INTO email (
			created, updated, subject, message_id, message_date,
			raw_message, headers, body,
			list_id, parent_id, thread_id, sender_id,
			is_patch, is_request_pull,
			nreplies,
			nparticipants,
			in_reply_to,
			patchset_id,
			patch_index,
			patch_count,
			patch_version,
			patch_prefix,
			patch_subject,
			superseded_by_id
		) VALUES (
			CASE WHEN $1 THEN $2 ELSE NOW() at time zone 'utc' END,
			CASE WHEN $1 THEN $2 ELSE NOW() at time zone 'utc' END,
			$3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24
		) RETURNING id`,
		ar.isImport, date,
		subject, messageID, date,
		rawMessage.String(), string(headerMap), body,
		ar.listID, nil, nil, nil,
		isPatch, isRequestPull,
		0, 1, inReplyTo,
		nil, nil, nil, nil, nil, nil, nil,
	)
	if err := row.Scan(&emailID); err != nil {
		return 0, fmt.Errorf("error inserting message %q: %v", messageID, err)
	}

	// Set parent of this email
	var parentID int
	row = tx.QueryRow(
		`SELECT id FROM email WHERE list_id = $1 AND message_id = $2;`,
		ar.listID, "<"+inReplyTo.String+">",
	)
	if err := row.Scan(&parentID); err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
	} else {
		if _, err := tx.Exec(
			`UPDATE email SET parent_id = $1 WHERE id = $2`,
			parentID, emailID,
		); err != nil {
			return 0, err
		}
	}

	threadID, err := ar.computeThreadID(tx, emailID)
	if err != nil {
		return 0, err
	}
	if threadID != emailID {
		if _, err := tx.Exec(
			`UPDATE email SET thread_id = $1 WHERE id = $2`,
			threadID, emailID,
		); err != nil {
			return 0, err
		}
	}

	if err := ar.reparentEmails(tx, threadID, emailID, messageID); err != nil {
		return 0, err
	}
	if err := ar.updateThreadReplies(tx, threadID); err != nil {
		return 0, err
	}

	// TODO: Enumerate CC's and create SQL relationships for them
	// TODO: Some users will have many email addresses
	// TODO: Multiple From addresses?
	senders, err := mr.Header.AddressList("From")
	if err != nil {
		return 0, fmt.Errorf("error reading From: %q %w", mr.Header.Get("From"), err)
	}
	if len(senders) == 0 {
		return 0, errors.New("expected at least one From address")
	}

	// Lookup sender by email
	row = tx.QueryRow(
		`SELECT id FROM "user" WHERE email = $1`,
		senders[0].Address,
	)
	var senderID *int
	if err := row.Scan(&senderID); err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
	} else {
		if _, err := tx.Exec(
			`UPDATE email SET sender_id = $1 WHERE id = $2`,
			*senderID, emailID,
		); err != nil {
			return 0, err
		}
	}

	status := string(model.PatchsetStatusProposed)
	if ar.isImport {
		// Only allow forcing patchset status when importing from mbox
		const statusHeader = "X-Sourcehut-Patchset-Final"
		if mr.Header.Has(statusHeader) {
			s := mr.Header.Get(statusHeader)
			if model.PatchsetStatus(strings.ToUpper(s)).IsValid() {
				status = s
			}
		}
	}

	if err := ar.importPatch(tx, emailID, threadID, subject, status, body, isPatch, inReplyTo.Valid); err != nil {
		return 0, err
	}

	if !ar.isImport {
		var patchsetID *int
		row = tx.QueryRow(`
			SELECT patchset_id FROM email
			WHERE (id = $1 OR thread_id = $1)
			AND patchset_id IS NOT NULL;
		`, threadID)
		err = row.Scan(&patchsetID)
		if err != nil && err != sql.ErrNoRows {
			panic(err)
		}

		const updateHeader = "X-Sourcehut-Patchset-Update"
		if patchsetID != nil && mr.Header.Has(updateHeader) {
			err := ar.updatePatchsetStatus(
				tx,
				*patchsetID,
				mr.Header.Get(updateHeader),
				senders[0].Address,
			)
			if err != nil {
				log.Println("Failed updating patchset status:", err)
			}
		}
	}

	if _, err := tx.ExecContext(ar.ctx, `
			UPDATE list
			SET last_activity = NOW() at time zone 'utc'
			WHERE id = $1
		`, ar.listID,
	); err != nil {
		log.Printf("Failed updating list.last_activity: %s", err)
	}

	log.Printf("Archived message %q", messageID)

	return int(emailID), nil
}

// Computes the thread ID for the given email
func (ar *Archiver) computeThreadID(tx *sql.Tx, emailID int) (int, error) {
	// Keep track of seen emails to avoid reference loops
	threadID := emailID
	seen := map[int]struct{}{}
	for {
		if _, ok := seen[threadID]; ok {
			// Reference loop
			break
		}
		seen[threadID] = struct{}{}
		row := tx.QueryRow(
			`SELECT parent_id FROM email WHERE id = $1`,
			threadID,
		)
		var nextID *int
		if err := row.Scan(&nextID); err != nil {
			return 0, err
		}
		if nextID == nil {
			break
		}
		threadID = *nextID
	}
	return threadID, nil
}

// Reparent emails that arrived out-of-order
func (ar *Archiver) reparentEmails(tx *sql.Tx, threadID, emailID int, messageID string) error {
	// Message-ID header is stored with angle brackets. In-reply-to is *not*.
	// Adjust accordingly.
	children, err := tx.Query(
		`SELECT id, thread_id FROM email WHERE list_id = $1 AND in_reply_to = $2`,
		ar.listID, strings.Trim(messageID, "<>"),
	)
	if err != nil {
		return err
	}
	defer children.Close()
	var childIDs []int
	var oldThreadIDs []int
	for children.Next() {
		var childID int
		var childThreadID *int
		if err := children.Scan(&childID, &childThreadID); err != nil {
			return err
		}
		childIDs = append(childIDs, childID)
		if childThreadID == nil {
			oldThreadIDs = append(oldThreadIDs, childID)
		} else if *childThreadID != threadID {
			oldThreadIDs = append(oldThreadIDs, *childThreadID)
		}
	}
	if _, err := tx.Exec(
		`UPDATE email SET parent_id = $1, thread_id = $2 WHERE id = ANY($3)`,
		emailID, threadID, pq.Array(childIDs),
	); err != nil {
		return err
	}
	_, err = tx.Exec(
		`UPDATE email SET thread_id = $1 WHERE thread_id = ANY($2)`,
		threadID, pq.Array(oldThreadIDs),
	)
	return err
}

// Updates thread nreplies and nparticipants
func (ar *Archiver) updateThreadReplies(tx *sql.Tx, threadID int) error {
	nreplies := 0
	memberIDs := []int{threadID}
	participants := make(map[string]struct{})
	threadMembers, err := tx.Query(
		`SELECT id, (headers -> 'From')::text FROM email WHERE thread_id = $1`,
		threadID,
	)
	if err != nil {
		return err
	}
	defer threadMembers.Close()
	for threadMembers.Next() {
		var memberID int
		var fromHeader string
		if err := threadMembers.Scan(&memberID, &fromHeader); err != nil {
			return err
		}
		memberIDs = append(memberIDs, memberID)
		// TODO: multiple From addresses?
		participants[fromHeader] = struct{}{}
		nreplies++
	}
	_, err = tx.Exec(
		`UPDATE email SET nreplies = $1, nparticipants = $2 WHERE id = $3`,
		nreplies, len(participants), threadID,
	)
	return err
}
