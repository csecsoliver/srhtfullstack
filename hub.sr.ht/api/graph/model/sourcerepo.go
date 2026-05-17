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

type SourceRepo struct {
	ID          int        `json:"id"`
	RID         model.RID  `json:"remote_rid"`
	Linked      time.Time  `json:"linked"`
	Updated     time.Time  `json:"updated"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Visibility  Visibility `json:"visibility"`
	RepoType    RepoType   `json:"repo_type"`

	OwnerID int

	alias  string
	fields *database.ModelFields
}

func (r *SourceRepo) IsProjectResource() {}

func (r *SourceRepo) As(alias string) *SourceRepo {
	r.alias = alias
	return r
}

func (r *SourceRepo) Alias() string {
	return r.alias
}

func (r *SourceRepo) Table() string {
	return "source_repo"
}

func (r *SourceRepo) Fields() *database.ModelFields {
	if r.fields != nil {
		return r.fields
	}
	r.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "linked", Ptr: &r.Linked},
			{SQL: "updated", GQL: "updated", Ptr: &r.Updated},
			{SQL: "name", GQL: "name", Ptr: &r.Name},
			{SQL: "description", GQL: "description", Ptr: &r.Description},
			{SQL: "visibility", GQL: "visibility", Ptr: &r.Visibility},
			{SQL: "repo_type", GQL: "repoType", Ptr: &r.RepoType},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &r.ID},
			{SQL: "remote_rid", GQL: "rid", Ptr: &r.RID},
			{SQL: "owner_id", GQL: "", Ptr: &r.OwnerID},
		},
	}
	return r.fields
}

func (r *SourceRepo) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*SourceRepo, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.UnixMicro(ts).UTC()
		q = q.Where(database.WithAlias(r.alias, "updated")+"<= ?", updated)
	}
	q = q.
		OrderBy(database.WithAlias(r.alias, "updated") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var repos []*SourceRepo
	for rows.Next() {
		var repo SourceRepo
		if err := rows.Scan(database.Scan(ctx, &repo)...); err != nil {
			panic(err)
		}
		repos = append(repos, &repo)
	}

	if len(repos) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(repos[len(repos)-1].Updated.UnixMicro(), 10),
			Search: cur.Search,
		}
		repos = repos[:cur.Count]
	} else {
		cur = nil
	}

	return repos, cur
}
