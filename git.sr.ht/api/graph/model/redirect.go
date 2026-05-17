package model

import (
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type Redirect struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`

	OwnerID      int
	RepositoryID int

	alias  string
	fields *database.ModelFields
}

func (r *Redirect) As(alias string) *Redirect {
	r.alias = alias
	return r
}

func (r *Redirect) Alias() string {
	return r.alias
}

func (r *Redirect) Table() string {
	return "redirect"
}

func (r *Redirect) Fields() *database.ModelFields {
	if r.fields != nil {
		return r.fields
	}
	r.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &r.ID},
			{SQL: "created", GQL: "created", Ptr: &r.Created},
			{SQL: "name", GQL: "name", Ptr: &r.Name},
			{SQL: "name", GQL: "name", Ptr: &r.Name},
			{SQL: "path", GQL: "originalPath", Ptr: &r.Path},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &r.ID},
			{SQL: "owner_id", GQL: "", Ptr: &r.OwnerID},
			{SQL: "new_repo_id", GQL: "", Ptr: &r.RepositoryID},
		},
	}
	return r.fields
}
