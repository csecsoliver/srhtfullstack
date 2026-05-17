package model

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type Thread struct {
	Created      time.Time `json:"created"`
	Participants int       `json:"participants"`
	Replies      int       `json:"replies"`
	Subject      string    `json:"subject"`
	Updated      time.Time `json:"updated"`

	ID            int
	MailingListID int

	SenderID   *int
	RawMessage []byte
	RawHeader  mail.Header

	alias  string
	fields *database.ModelFields
}

func (thread *Thread) As(alias string) *Thread {
	thread.alias = alias
	return thread
}

func (thread *Thread) Alias() string {
	return thread.alias
}

func (thread *Thread) Table() string {
	return "email"
}

func (thread *Thread) Fields() *database.ModelFields {
	if thread.fields != nil {
		return thread.fields
	}
	thread.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &thread.ID},
			{SQL: "created", GQL: "created", Ptr: &thread.Created},
			{SQL: "subject", GQL: "subject", Ptr: &thread.Subject},
			{SQL: "nreplies", GQL: "replies", Ptr: &thread.Replies},
			{SQL: "nparticipants", GQL: "participants", Ptr: &thread.Participants},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &thread.ID},
			{SQL: "list_id", GQL: "", Ptr: &thread.MailingListID},
			{SQL: "updated", GQL: "", Ptr: &thread.Updated},
			{SQL: "sender_id", GQL: "", Ptr: &thread.SenderID},
			{SQL: "raw_message", GQL: "", Ptr: &thread.RawMessage},
		},
	}
	return thread.fields
}

func (thread *Thread) Populate() {
	reader, err := mail.CreateReader(bytes.NewBuffer(thread.RawMessage))
	if err != nil {
		panic(fmt.Errorf("error reading email %d: %e", thread.ID, err))
	}
	thread.RawHeader = reader.Header
	reader.Close()
}

func (thread *Thread) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*Thread, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.Unix(ts, 0)
		q = q.Where(database.WithAlias(thread.alias, "updated")+"<= ?", updated)
	}
	q = q.
		OrderBy(database.WithAlias(thread.alias, "updated") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var threads []*Thread
	for rows.Next() {
		var thread Thread
		if err := rows.Scan(database.Scan(ctx, &thread)...); err != nil {
			panic(err)
		}
		thread.Populate()
		threads = append(threads, &thread)
	}

	if len(threads) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(threads[len(threads)-1].Updated.Unix(), 10),
			Search: cur.Search,
		}
		threads = threads[:cur.Count]
	} else {
		cur = nil
	}

	return threads, cur
}
