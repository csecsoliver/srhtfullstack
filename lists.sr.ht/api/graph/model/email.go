package model

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/lib/pq"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type Patch struct {
	Index    *int    `json:"index,omitempty"`
	Count    *int    `json:"count,omitempty"`
	Version  *int    `json:"version,omitempty"`
	Prefix   *string `json:"prefix,omitempty"`
	Subject  *string `json:"subject,omitempty"`
	trailers []string
}

func (p *Patch) Trailers() []*Trailer {
	trailers := make([]*Trailer, len(p.trailers))

	for i, s := range p.trailers {
		t := ParseTrailer(s)
		if t == nil {
			panic(fmt.Errorf("invalid trailer '%s' in database", s))
		}
		trailers[i] = t
	}

	return trailers
}

type Email struct {
	ID        int       `json:"id"`
	Received  time.Time `json:"received"`
	Body      string    `json:"body"`
	Subject   string    `json:"subject"`
	MessageID string    `json:"messageID"`
	InReplyTo *string   `json:"inReplyTo"`
	Patch     Patch     `json:"patch"`

	MailingListID int
	PatchsetID    *int
	ThreadID      *int
	ParentID      *int
	SenderID      *int

	RawMessage []byte
	RawHeader  mail.Header

	alias  string
	fields *database.ModelFields
}

func (email *Email) As(alias string) *Email {
	email.alias = alias
	return email
}

func (email *Email) Alias() string {
	return email.alias
}

func (email *Email) Table() string {
	return "email"
}

func (email *Email) Fields() *database.ModelFields {
	if email.fields != nil {
		return email.fields
	}
	email.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &email.ID},
			{SQL: "body", GQL: "body", Ptr: &email.Body},
			{SQL: "subject", GQL: "subject", Ptr: &email.Subject},
			{SQL: "message_id", GQL: "messageID", Ptr: &email.MessageID},
			{SQL: "patch_index", GQL: "patch", Ptr: &email.Patch.Index},
			{SQL: "patch_count", GQL: "patch", Ptr: &email.Patch.Count},
			{SQL: "patch_version", GQL: "patch", Ptr: &email.Patch.Version},
			{SQL: "patch_prefix", GQL: "patch", Ptr: &email.Patch.Prefix},
			{SQL: "patch_subject", GQL: "patch", Ptr: &email.Patch.Subject},
			{SQL: "patch_trailers", GQL: "", Ptr: pq.Array(&email.Patch.trailers)},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &email.ID},
			{SQL: "list_id", GQL: "", Ptr: &email.MailingListID},
			{SQL: "patchset_id", GQL: "", Ptr: &email.PatchsetID},
			{SQL: "thread_id", GQL: "", Ptr: &email.ThreadID},
			{SQL: "parent_id", GQL: "", Ptr: &email.ParentID},
			{SQL: "sender_id", GQL: "", Ptr: &email.SenderID},
			{SQL: "created", GQL: "", Ptr: &email.Received},
			{SQL: "raw_message", GQL: "", Ptr: &email.RawMessage},
		},
	}
	return email.fields
}

func (email *Email) Populate() {
	reader, err := mail.CreateReader(bytes.NewBuffer(email.RawMessage))
	if err != nil {
		return
	}
	email.RawHeader = reader.Header
	if msgId, err := email.RawHeader.MessageID(); err != nil {
		log.Printf("Invalid message ID for email %d: %e", email.ID, err)
	} else {
		email.MessageID = msgId
	}
	if ids, _ := email.RawHeader.MsgIDList("In-Reply-To"); ids != nil {
		if len(ids) != 1 {
			log.Printf("Multiple In-Reply-To IDs for email %d", email.ID)
		}
		indir := ids[0]
		email.InReplyTo = &indir
	}
	reader.Close()
}

func (email *Email) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*Email, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.Unix(ts, 0)
		q = q.Where(database.WithAlias(email.alias, "created")+"<= ?", updated)
	}
	q = q.
		Column(database.WithAlias(email.alias, "raw_message")).
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var emails []*Email
	for rows.Next() {
		var (
			email Email
			data  string
		)
		if err := rows.Scan(append(
			database.Scan(ctx, &email),
			&data)...); err != nil {
			panic(err)
		}
		email.Populate()
		emails = append(emails, &email)
	}

	if len(emails) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(emails[len(emails)-1].Received.Unix(), 10),
			Search: cur.Search,
		}
		emails = emails[:cur.Count]
	} else {
		cur = nil
	}

	return emails, cur
}
