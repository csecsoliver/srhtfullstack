package model

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

const (
	ACCESS_NONE    = 0
	ACCESS_BROWSE  = 1
	ACCESS_SUBMIT  = 2
	ACCESS_COMMENT = 4
	ACCESS_EDIT    = 8
	ACCESS_TRIAGE  = 16
	ACCESS_ALL     = 1 | 2 | 4 | 8 | 16
)

type Tracker struct {
	ID          int        `json:"id"`
	RID         model.RID  `json:"rid"`
	Created     time.Time  `json:"created"`
	Updated     time.Time  `json:"updated"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Visibility  Visibility `json:"visibility"`

	OwnerID       int
	Access        int
	DefaultAccess uint
	ACLID         *int

	alias  string
	fields *database.ModelFields
}

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
			{SQL: "id", GQL: "id", Ptr: &t.ID},
			{SQL: "created", GQL: "created", Ptr: &t.Created},
			{SQL: "updated", GQL: "updated", Ptr: &t.Updated},
			{SQL: "name", GQL: "name", Ptr: &t.Name},
			{SQL: "description", GQL: "description", Ptr: &t.Description},
			{SQL: "visibility", GQL: "visibility", Ptr: &t.Visibility},
			{SQL: "default_access", GQL: "defaultACL", Ptr: &t.DefaultAccess},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &t.ID},
			{SQL: "rid", GQL: "", Ptr: &t.RID},
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
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(t.alias, "id")+"<= ?", next)
	}
	auser := auth.ForContext(ctx)
	q = q.
		OrderBy(database.WithAlias(t.alias, "id")+" DESC").
		Limit(uint64(cur.Count+1)).
		LeftJoin(`user_access ua ON ua.tracker_id = tr.id AND ua.user_id = ?`, auser.UserID).
		Column(`COALESCE(
			ua.permissions,
			CASE WHEN tr.owner_id = ?
				THEN ?
				ELSE tr.default_access
			END)`,
			auser.UserID, ACCESS_ALL).
		Column(`ua.id`)

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var trackers []*Tracker
	for rows.Next() {
		var tracker Tracker
		if err := rows.Scan(append(database.Scan(
			ctx, &tracker), &tracker.Access, &tracker.ACLID)...); err != nil {
			panic(err)
		}
		trackers = append(trackers, &tracker)
	}

	if len(trackers) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(trackers[len(trackers)-1].ID),
			Search: cur.Search,
		}
		trackers = trackers[:cur.Count]
	} else {
		cur = nil
	}

	return trackers, cur
}

func (t *Tracker) CanBrowse() bool {
	if t.Access == ACCESS_NONE {
		panic(errors.New("invariant broken: tracker access is 0"))
	}
	return t.Access&ACCESS_BROWSE == ACCESS_BROWSE
}

func (t *Tracker) CanSubmit() bool {
	if t.Access == ACCESS_NONE {
		panic(errors.New("invariant broken: tracker access is 0"))
	}
	return t.Access&ACCESS_SUBMIT == ACCESS_SUBMIT
}

func (t *Tracker) CanComment() bool {
	if t.Access == ACCESS_NONE {
		panic(errors.New("invariant broken: tracker access is 0"))
	}
	return t.Access&ACCESS_COMMENT == ACCESS_COMMENT
}

func (t *Tracker) CanEdit() bool {
	if t.Access == ACCESS_NONE {
		panic(errors.New("invariant broken: tracker access is 0"))
	}
	return t.Access&ACCESS_EDIT == ACCESS_EDIT
}

func (t *Tracker) CanTriage() bool {
	if t.Access == ACCESS_NONE {
		panic(errors.New("invariant broken: tracker access is 0"))
	}
	return t.Access&ACCESS_TRIAGE == ACCESS_TRIAGE
}
