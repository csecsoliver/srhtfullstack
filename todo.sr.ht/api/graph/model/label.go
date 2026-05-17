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

type Label struct {
	ID              int       `json:"id"`
	Created         time.Time `json:"created"`
	Name            string    `json:"name"`
	BackgroundColor string    `json:"backgroundColor"`
	ForegroundColor string    `json:"foregroundColor"`

	TrackerID int

	alias  string
	fields *database.ModelFields
}

func (l *Label) As(alias string) *Label {
	l.alias = alias
	return l
}

func (l *Label) Alias() string {
	return l.alias
}

func (l *Label) Table() string {
	return `"label"`
}

func (l *Label) Fields() *database.ModelFields {
	if l.fields != nil {
		return l.fields
	}
	l.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &l.ID},
			{SQL: "created", GQL: "created", Ptr: &l.Created},
			{SQL: "name", GQL: "name", Ptr: &l.Name},
			{SQL: "color", GQL: "backgroundColor", Ptr: &l.BackgroundColor},
			{SQL: "text_color", GQL: "foregroundColor", Ptr: &l.ForegroundColor},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &l.ID},
			{SQL: "tracker_id", GQL: "", Ptr: &l.TrackerID},
		},
	}
	return l.fields
}

func (l *Label) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*Label, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(l.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(l.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var labels []*Label
	for rows.Next() {
		var label Label
		if err := rows.Scan(database.Scan(ctx, &label)...); err != nil {
			panic(err)
		}
		labels = append(labels, &label)
	}

	if len(labels) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(labels[len(labels)-1].ID),
			Search: cur.Search,
		}
		labels = labels[:cur.Count]
	} else {
		cur = nil
	}

	return labels, cur
}
