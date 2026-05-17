package model

import (
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type JobGroup struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`
	Note    *string   `json:"note"`

	OwnerID int

	alias  string
	fields *database.ModelFields
}

func (j *JobGroup) As(alias string) *JobGroup {
	j.alias = alias
	return j
}

func (j *JobGroup) Alias() string {
	return j.alias
}

func (j *JobGroup) Table() string {
	return `"job_group"`
}

func (j *JobGroup) Fields() *database.ModelFields {
	if j.fields != nil {
		return j.fields
	}
	j.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &j.Created},
			{SQL: "note", GQL: "note", Ptr: &j.Note},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &j.ID},
			{SQL: "owner_id", GQL: "", Ptr: &j.OwnerID},
		},
	}
	return j.fields
}
