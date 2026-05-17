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

type Artifact struct {
	ID       int       `json:"id"`
	Created  time.Time `json:"created"`
	Filename string    `json:"filename"`
	Checksum string    `json:"checksum"`
	Size     int       `json:"size"`

	Commit string
	RepoID int

	alias  string
	fields *database.ModelFields
}

func (a *Artifact) As(alias string) *Artifact {
	a.alias = alias
	return a
}

func (a *Artifact) Alias() string {
	return a.alias
}

func (a *Artifact) Table() string {
	return "artifacts"
}

func (a *Artifact) Fields() *database.ModelFields {
	if a.fields != nil {
		return a.fields
	}
	a.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &a.Created},
			{SQL: "size", GQL: "size", Ptr: &a.Size},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &a.ID},
			{SQL: "checksum", GQL: "", Ptr: &a.Checksum},
			{SQL: "commit", GQL: "", Ptr: &a.Commit},
			{SQL: "filename", GQL: "", Ptr: &a.Filename},
			{SQL: "repo_id", GQL: "", Ptr: &a.RepoID},
		},
	}
	return a.fields
}

func (a *Artifact) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*Artifact, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.Atoi(cur.Next)
		q = q.Where(database.WithAlias(a.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(a.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var artifacts []*Artifact
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(database.Scan(ctx, &a)...); err != nil {
			panic(err)
		}
		artifacts = append(artifacts, &a)
	}

	if len(artifacts) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(artifacts[len(artifacts)-1].ID),
			Search: cur.Search,
		}
		artifacts = artifacts[:cur.Count]
	} else {
		cur = nil
	}

	return artifacts, cur
}
