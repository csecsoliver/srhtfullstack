package model

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type MailingListSubscription struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`

	UserID *int
	ListID int
	Email  *string

	alias  string
	fields *database.ModelFields
}

func (sub MailingListSubscription) IsActivitySubscription() {
}

func (sub *MailingListSubscription) As(alias string) *MailingListSubscription {
	sub.alias = alias
	return sub
}

func (sub *MailingListSubscription) Alias() string {
	return sub.alias
}

func (sub *MailingListSubscription) Table() string {
	return "subscription"
}

func (sub *MailingListSubscription) Fields() *database.ModelFields {
	if sub.fields != nil {
		return sub.fields
	}
	sub.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			// Always fetch everything
			{SQL: "id", GQL: "", Ptr: &sub.ID},
			{SQL: "created", GQL: "", Ptr: &sub.Created},
			{SQL: "user_id", GQL: "", Ptr: &sub.UserID},
			{SQL: "list_id", GQL: "", Ptr: &sub.ListID},
			{SQL: "email", GQL: "", Ptr: &sub.Email},
		},
	}
	return sub.fields
}

func (sub *MailingListSubscription) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]ActivitySubscription, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		created := time.Unix(ts, 0)
		q = q.Where(database.WithAlias(sub.alias, "created")+"<= ?", created)
	}
	q = q.Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var (
		subs        []ActivitySubscription
		lastCreated time.Time
	)
	for rows.Next() {
		var sub MailingListSubscription
		if err := rows.Scan(database.Scan(ctx, &sub)...); err != nil {
			panic(err)
		}
		subs = append(subs, &sub)
		lastCreated = sub.Created
	}

	if len(subs) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(lastCreated.Unix(), 10),
			Search: cur.Search,
		}
		subs = subs[:cur.Count]
	} else {
		cur = nil
	}

	return subs, cur
}
