package lists

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/emersion/go-message/mail"
	"github.com/lib/pq"
)

func identifyPatch(body string) bool {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	files, _, err := gitdiff.Parse(strings.NewReader(body))
	if err != nil {
		return false
	}
	return len(files) > 0
}

var (
	patchSubjectRE = regexp.MustCompile(
		`^.*\[(?:RFC )?PATCH(?: (?P<prefix>[^\]]+))?\] (?P<subject>.*)$`,
	)
	patchVersionRE = regexp.MustCompile(
		`(?:v(?P<version>[0-9]+))?(?: ?(?P<index>[0-9]+)/(?P<count>[0-9]+))?$`,
	)
)

type PatchDetails struct {
	Prefix  string
	Subject string
	Version int
	Index   int
	Count   int
}

func parsePatchSubject(subject string) (*PatchDetails, error) {
	var patch PatchDetails

	submatch := patchSubjectRE.FindStringSubmatch(subject)
	if submatch == nil {
		// TODO: figure out a better way of dealing with patches that have weird
		// subjects
		return nil, nil
	}

	patch.Prefix = submatch[patchSubjectRE.SubexpIndex("prefix")]
	patch.Subject = submatch[patchSubjectRE.SubexpIndex("subject")]
	patch.Version = 1
	patch.Index = 1
	patch.Count = 1

	submatch = patchVersionRE.FindStringSubmatch(patch.Prefix)
	if submatch != nil {
		patch.Prefix = strings.TrimSpace(
			strings.TrimSuffix(patch.Prefix, submatch[0]))

		var err error
		versionMatch := submatch[patchVersionRE.SubexpIndex("version")]
		if len(versionMatch) != 0 {
			patch.Version, err = strconv.Atoi(versionMatch)
			if err != nil {
				return nil, err
			}
		}
		indexMatch := submatch[patchVersionRE.SubexpIndex("index")]
		if len(indexMatch) != 0 {
			patch.Index, err = strconv.Atoi(indexMatch)
			if err != nil {
				return nil, err
			}
		}
		countMatch := submatch[patchVersionRE.SubexpIndex("count")]
		if len(countMatch) != 0 {
			patch.Count, err = strconv.Atoi(countMatch)
			if err != nil {
				return nil, err
			}
		}
	}

	return &patch, nil
}

type PatchVersion struct {
	id      int
	version int
	status  string
}

func (ar *Archiver) findExistingVersions(
	tx *sql.Tx, listID int, subject string, prefix string, submitter sql.NullString) []PatchVersion {
	previousVersions, err := tx.Query(`
		SELECT id, version, status FROM patchset
		WHERE
			list_id = $1 AND
			subject = $2 AND
			prefix = $3 AND
			submitter = $4
		ORDER BY version DESC`,
		listID, subject, prefix, submitter,
	)
	if err != nil {
		return nil
	}
	var versions []PatchVersion
	for previousVersions.Next() {
		var (
			id      int
			version int
			status  string
		)
		if err := previousVersions.Scan(&id, &version, &status); err != nil {
			continue
		}
		versions = append(versions, PatchVersion{id, version, status})
	}
	previousVersions.Close()
	return versions
}

func (ar *Archiver) importPatch(tx *sql.Tx, emailID, threadID int, subject, status, body string, isPatch, isReply bool) error {
	patch, err := parsePatchSubject(subject)
	if err != nil {
		return fmt.Errorf("error parsing patch subject: %v", err)
	} else if patch == nil {
		return nil
	}

	// check status validity
	if !model.PatchsetStatus(strings.ToUpper(status)).IsValid() {
		return fmt.Errorf("invalid status %q", status)
	}
	status = strings.ToLower(status)

	// Consider cover letters (index = 0) as valid patches. This makes it much
	// easier to look up the cover letter later on by querying patch_index = 0.
	// This also handles the case where the cover letter is received last.
	// However, make sure to ignore replies to the cover letter.
	if !isPatch && (patch.Index != 0 || isReply) {
		return nil
	}

	// Parse git trailers
	trailers := make([]*model.Trailer, 0)
	if isPatch {
		rd := bytes.NewBufferString(body)
		scanner := bufio.NewScanner(rd)
		for scanner.Scan() {
			if scanner.Text() == "---" {
				break
			}
			t := model.ParseTrailer(scanner.Text())
			if t == nil {
				// Reset the list
				trailers = make([]*model.Trailer, 0)
				continue
			}
			trailers = append(trailers, t)
		}
	}

	trailerStrings := make([]string, len(trailers))
	for i, t := range trailers {
		trailerStrings[i] = t.String()
	}

	if _, err := tx.Exec(
		`UPDATE email SET
			patch_index = $1, patch_count = $2, patch_version = $3,
			patch_prefix = $4, patch_subject = $5,
			patch_trailers = $6
		WHERE id = $7`,
		patch.Index, patch.Count, patch.Version, patch.Prefix, patch.Subject,
		pq.Array(trailerStrings), emailID,
	); err != nil {
		return err
	}

	log.Printf("Received patch %d/%d: %q", patch.Index, patch.Count, patch.Subject)

	// Check if the patchset is complete
	complete := true
	for i := 1; i <= patch.Count; i++ {
		var exists bool
		row := tx.QueryRow(
			`SELECT EXISTS (
				SELECT FROM email
				WHERE (id = $1 OR thread_id = $1) AND patch_index = $2
			)`,
			threadID, i,
		)
		if err := row.Scan(&exists); err != nil {
			return err
		}
		if !exists {
			complete = false
			break
		}
	}
	if !complete {
		return nil
	}
	log.Println("Complete patchset received")

	// Look for existing patchset
	var existing bool
	row := tx.QueryRow(
		`SELECT EXISTS (
			SELECT FROM email
			WHERE (id = $1 OR thread_id = $1) AND patchset_id IS NOT NULL
		)`,
		threadID,
	)
	if err := row.Scan(&existing); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else if existing {
		// TODO: is this a new revision? complicated
		// TODO: if patch.Index == 0, the cover letter arrived last and
		// the patchset title should be updated
		return nil
	}

	// Look for a cover letter
	var coverLetterID *int
	var patchsetSubject string
	var patchsetPrefix string
	var patchsetVersion int
	row = tx.QueryRow(
		`SELECT
			id, patch_subject, patch_prefix, patch_version
		FROM email WHERE (id = $1 OR thread_id = $1) AND patch_index = 0`,
		threadID,
	)
	if err := row.Scan(&coverLetterID,
		&patchsetSubject, &patchsetPrefix, &patchsetVersion); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	}

	if coverLetterID == nil && patch.Index == 1 {
		// TODO: handle the case where patch subjects/prefixes/versions/senders
		// don't match?
		patchsetSubject = patch.Subject
		patchsetPrefix = patch.Prefix
		patchsetVersion = patch.Version
	} else if coverLetterID == nil {
		// If no cover letter, use the subject of the first patch of
		// the series as patchset title
		row = tx.QueryRow(
			`SELECT
				patch_subject, patch_prefix, patch_version
			FROM email WHERE (id = $1 OR thread_id = $1) AND patch_index = 1`,
			threadID,
		)
		if err := row.Scan(
			&patchsetSubject, &patchsetPrefix, &patchsetVersion); err != nil {
			// XXX: can this even happen?
			return fmt.Errorf("incomplete patchset, database inconsistent: %w", err)
		}
	}

	// Get the info need to reply to the last message in the patchset.
	// This info is exposed via the lists.sr.ht REST API and is used by
	// hub.sr.ht to automatically reply to patches.
	var messageID string
	var submitter sql.NullString
	var replyTo sql.NullString
	row = tx.QueryRow(
		`SELECT
			message_id, headers -> 'From' -> 0, headers -> 'Reply-To' -> 0
		FROM email WHERE (id = $1 OR thread_id = $1) AND patch_index = $2`,
		threadID, patch.Count,
	)
	if err := row.Scan(&messageID, &submitter, &replyTo); err != nil {
		return err
	}

	var patchsetID int
	var created, updated time.Time
	row = tx.QueryRow(`
		INSERT INTO patchset (
			created, updated,
			subject, prefix, version, list_id, cover_letter_id,
			message_id, submitter, reply_to, status
		) VALUES (
			NOW() at time zone 'utc',
			NOW() at time zone 'utc',
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, created, updated`,
		patchsetSubject, patchsetPrefix, patchsetVersion, ar.listID, coverLetterID,
		messageID, submitter, replyTo, status,
	)
	if err := row.Scan(&patchsetID, &created, &updated); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE email SET patchset_id = $1 WHERE id = $2 OR thread_id = $2`,
		patchsetID, threadID,
	); err != nil {
		return err
	}

	// Supersede the most recent previous version from the submitter if
	// it's still active.
	prevVersions := ar.findExistingVersions(tx, ar.listID,
		patchsetSubject, patchsetPrefix, submitter)
	if len(prevVersions) <= 1 {
		// Nothing to supersede.
	} else if prevVersions[0].id != patchsetID {
		// The patch does not have the highest version number; there
		// has been a user "sequencing error" at some point. Bail out.
	} else {
		// If this patchset supersedes anything, it's the second element.
		candidate := prevVersions[1]
		if candidate.version < patchsetVersion {
			switch candidate.status {
			case "proposed", "needs_revision":
				tx.Exec(`
					UPDATE patchset
					SET status = 'superseded', superseded_by_id = $1
					WHERE id = $2`,
					patchsetID, candidate.id)
				tx.Exec(`
					UPDATE patchset
					SET supersedes_id = $1
					WHERE id = $2`,
					candidate.id, patchsetID)
			}
		}
	}

	return nil
}

// Returns whether the update comes from the patch submitter and the target
// status is allowed to patch submitters.
func (ar *Archiver) allowedUpdateBySubmitter(tx *sql.Tx, patchsetID int, status, sender string) bool {
	switch status {
	case "rejected", "superseded", "needs_revision":
		// Only transitions allowed to patch submitters; keep going.
	default:
		return false
	}

	var (
		currentStatus string
		submitter     *string
		err           error
	)
	row := tx.QueryRow(
		`SELECT status, submitter FROM patchset WHERE id = $1`,
		patchsetID,
	)
	if err = row.Scan(&currentStatus, &submitter); err != nil || submitter == nil {
		return false
	}

	switch currentStatus {
	case "applied", "approved", "rejected":
		// Patches in final state cannot be updated.
		return false
	}

	// The submitter is something like "Joe Barr \u003cjoe@bar.tld\u003e" in the
	// database, and needs unquoting.
	var (
		submitterAddr     *mail.Address
		unquotedSubmitter string
	)
	unquotedSubmitter, err = strconv.Unquote(*submitter)
	if err != nil {
		return false
	}
	submitterAddr, err = mail.ParseAddress(unquotedSubmitter)
	if err != nil {
		return false
	}
	return submitterAddr.Address == sender
}

func (ar *Archiver) updatePatchsetStatus(tx *sql.Tx, patchsetID int, status, sender string) error {
	// check new status validity
	if !model.PatchsetStatus(strings.ToUpper(status)).IsValid() {
		return fmt.Errorf("invalid status %q", status)
	}
	status = strings.ToLower(status)

	// check sender has permissions to update patchset status
	access, err := model.UserACL(ar.ctx, tx, ar.listID, sender)
	if err != nil {
		return fmt.Errorf("UserACL: %w", err)
	} else if !access.Moderate && !ar.allowedUpdateBySubmitter(tx, patchsetID, status, sender) {
		return fmt.Errorf("sender does not have moderate permission")
	}

	// update status
	res, err := tx.ExecContext(ar.ctx, `
		UPDATE patchset SET status = $1 WHERE id = $2;
	`, status, patchsetID)
	if n, e := res.RowsAffected(); n == 0 || e != nil {
		panic(fmt.Errorf("patchsetID not found"))
	}
	return err
}
