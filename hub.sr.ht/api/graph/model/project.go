package model

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type Project struct {
	ID                int        `json:"id"`
	RID               model.RID  `json:"rid"`
	Created           time.Time  `json:"created"`
	Updated           time.Time  `json:"updated"`
	Name              string     `json:"name"`
	Description       *string    `json:"description"`
	Visibility        Visibility `json:"visibility"`
	Tags              []string   `json:"tags"`
	Website           *string    `json:"website"`
	ChecklistComplete bool       `json:"checklist_complete"`

	OwnerID int

	alias  string
	fields *database.ModelFields
}

func (p *Project) As(alias string) *Project {
	p.alias = alias
	return p
}

func (p *Project) Alias() string {
	return p.alias
}

func (p *Project) Table() string {
	return "project"
}

func (p *Project) Fields() *database.ModelFields {
	if p.fields != nil {
		return p.fields
	}
	p.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &p.Created},
			{SQL: "updated", GQL: "updated", Ptr: &p.Updated},
			{SQL: "description", GQL: "description", Ptr: &p.Description},
			{SQL: "visibility", GQL: "visibility", Ptr: &p.Visibility},
			{SQL: "tags", GQL: "tags", Ptr: pq.Array(&p.Tags)},
			{SQL: "website", GQL: "website", Ptr: &p.Website},
			{SQL: "checklist_complete", GQL: "checklistComplete", Ptr: &p.ChecklistComplete},
			{SQL: "rid", GQL: "rid", Ptr: &p.RID},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &p.ID},
			{SQL: "owner_id", GQL: "", Ptr: &p.OwnerID},
			{SQL: "name", GQL: "", Ptr: &p.Name},
		},
	}
	return p.fields
}

func (p *Project) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*Project, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.UnixMicro(ts).UTC()
		q = q.Where(database.WithAlias(p.alias, "updated")+"<= ?", updated)
	}
	q = q.
		OrderBy(database.WithAlias(p.alias, "updated") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(database.Scan(ctx, &project)...); err != nil {
			panic(err)
		}
		projects = append(projects, &project)
	}

	if len(projects) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(projects[len(projects)-1].Updated.UnixMicro(), 10),
			Search: cur.Search,
		}
		projects = projects[:cur.Count]
	} else {
		cur = nil
	}

	return projects, cur
}
