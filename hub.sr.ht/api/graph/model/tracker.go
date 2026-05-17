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

type Tracker struct {
	ID          int        `json:"id"`
	RID         model.RID  `json:"remote_rid"`
	Linked      time.Time  `json:"linked"`
	Updated     time.Time  `json:"updated"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Visibility  Visibility `json:"visibility"`

	OwnerID int

	alias  string
	fields *database.ModelFields
}

func (t *Tracker) IsProjectResource() {}

func (t *Tracker) As(alias string) *Tracker {
	t.alias = alias
	return t
}

func (t *Tracker) Alias() string {
	return t.alias
}

func (t *Tracker) Table() string {
	return `"tracker"`
}

func (t *Tracker) Fields() *database.ModelFields {
	if t.fields != nil {
		return t.fields
	}
	t.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "linked", Ptr: &t.Linked},
			{SQL: "updated", GQL: "updated", Ptr: &t.Updated},
			{SQL: "name", GQL: "name", Ptr: &t.Name},
			{SQL: "description", GQL: "description", Ptr: &t.Description},
			{SQL: "visibility", GQL: "visibility", Ptr: &t.Visibility},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &t.ID},
			{SQL: "remote_rid", GQL: "rid", Ptr: &t.RID},
			{SQL: "owner_id", GQL: "", Ptr: &t.OwnerID},
		},
	}
	return t.fields
}

func (t *Tracker) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*Tracker, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.UnixMicro(ts).UTC()
		q = q.Where(database.WithAlias(t.alias, "updated")+"<= ?", updated)
	}
	q = q.
		OrderBy(database.WithAlias(t.alias, "updated") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var trackers []*Tracker
	for rows.Next() {
		var tracker Tracker
		if err := rows.Scan(database.Scan(ctx, &tracker)...); err != nil {
			panic(err)
		}
		trackers = append(trackers, &tracker)
	}

	if len(trackers) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(trackers[len(trackers)-1].Updated.UnixMicro(), 10),
			Search: cur.Search,
		}
		trackers = trackers[:cur.Count]
	} else {
		cur = nil
	}

	return trackers, cur
}
