package model

import (
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type User struct {
	ID       int       `json:"id"`
	Created  time.Time `json:"created"`
	Username string    `json:"username"`

	alias  string
	fields *database.ModelFields
}

func (User) IsEntity() {}

func (u *User) CanonicalName() string {
	return "~" + u.Username
}

func (u *User) As(alias string) *User {
	u.alias = alias
	return u
}

func (u *User) Alias() string {
	return u.alias
}

func (u *User) Table() string {
	return "user"
}

func (u *User) Fields() *database.ModelFields {
	if u.fields != nil {
		return u.fields
	}
	u.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &u.Created},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &u.ID},
			{SQL: "username", GQL: "", Ptr: &u.Username},
		},
	}
	return u.fields
}
