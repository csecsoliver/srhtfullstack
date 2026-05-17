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

type Paste struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`

	PKID       int
	UserID     int
	Visibility Visibility

	alias  string
	fields *database.ModelFields
}

func (paste *Paste) As(alias string) *Paste {
	paste.alias = alias
	return paste
}

func (paste *Paste) Alias() string {
	return paste.alias
}

func (paste *Paste) Table() string {
	return "paste"
}

func (paste *Paste) Fields() *database.ModelFields {
	if paste.fields != nil {
		return paste.fields
	}
	paste.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &paste.Created},
			{SQL: "visibility", GQL: "visibility", Ptr: &paste.Visibility},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &paste.PKID},
			{SQL: "sha", GQL: "", Ptr: &paste.ID},
			{SQL: "user_id", GQL: "", Ptr: &paste.UserID},
		},
	}
	return paste.fields
}

func (paste *Paste) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*Paste, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.Atoi(cur.Next)
		q = q.Where(database.WithAlias(paste.alias, "id")+"<= ?", next)
	}
	q = q.
		Column(database.WithAlias(paste.alias, "id")).
		OrderBy(database.WithAlias(paste.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var (
		pastes []*Paste
		id     int
	)
	for rows.Next() {
		var paste Paste
		if err := rows.Scan(append(
			database.Scan(ctx, &paste),
			&id)...); err != nil {
			panic(err)
		}
		pastes = append(pastes, &paste)
	}

	if len(pastes) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(id),
			Search: cur.Search,
		}
		pastes = pastes[:cur.Count]
	} else {
		cur = nil
	}

	return pastes, cur
}
