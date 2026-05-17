package model

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type Patchset struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Subject string    `json:"subject"`
	Prefix  *string   `json:"prefix"`
	Version int       `json:"version"`

	MailingListID  int
	CoverLetterID  *int
	SupersededByID *int
	SupersedesID   *int

	RawStatus string

	alias  string
	fields *database.ModelFields
}

func (patch *Patchset) Status() PatchsetStatus {
	switch patch.RawStatus {
	case "unknown":
		return PatchsetStatusUnknown
	case "proposed":
		return PatchsetStatusProposed
	case "needs_revision":
		return PatchsetStatusNeedsRevision
	case "superseded":
		return PatchsetStatusSuperseded
	case "approved":
		return PatchsetStatusApproved
	case "rejected":
		return PatchsetStatusRejected
	case "applied":
		return PatchsetStatusApplied
	default:
		panic(fmt.Errorf("Patchset %d has unknown status '%s'",
			patch.ID, patch.RawStatus))
	}
}

func (patch *Patchset) As(alias string) *Patchset {
	patch.alias = alias
	return patch
}

func (patch *Patchset) Alias() string {
	return patch.alias
}

func (patch *Patchset) Table() string {
	return "email"
}

func (patch *Patchset) Fields() *database.ModelFields {
	if patch.fields != nil {
		return patch.fields
	}
	patch.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &patch.ID},
			{SQL: "updated", GQL: "updated", Ptr: &patch.Updated},
			{SQL: "subject", GQL: "subject", Ptr: &patch.Subject},
			{SQL: "prefix", GQL: "prefix", Ptr: &patch.Prefix},
			{SQL: "version", GQL: "version", Ptr: &patch.Version},
			{SQL: "status", GQL: "status", Ptr: &patch.RawStatus},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &patch.ID},
			{SQL: "list_id", GQL: "", Ptr: &patch.MailingListID},
			{SQL: "cover_letter_id", GQL: "", Ptr: &patch.CoverLetterID},
			{SQL: "superseded_by_id", GQL: "", Ptr: &patch.SupersededByID},
			{SQL: "supersedes_id", GQL: "", Ptr: &patch.SupersedesID},
			{SQL: "created", GQL: "", Ptr: &patch.Created},
		},
	}
	return patch.fields
}

func (patch *Patchset) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*Patchset, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.Unix(ts, 0)
		q = q.Where(database.WithAlias(patch.alias, "created")+"<= ?", updated)
	}
	q = q.
		Limit(uint64(cur.Count + 1)).
		OrderBy(database.WithAlias(patch.alias, "created") + "DESC")

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var patches []*Patchset
	for rows.Next() {
		var patch Patchset
		if err := rows.Scan(database.Scan(ctx, &patch)...); err != nil {
			panic(err)
		}
		patches = append(patches, &patch)
	}

	if len(patches) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(patches[len(patches)-1].Created.Unix(), 10),
			Search: cur.Search,
		}
		patches = patches[:cur.Count]
	} else {
		cur = nil
	}

	return patches, cur
}
