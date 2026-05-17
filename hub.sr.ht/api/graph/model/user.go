package model

import (
	"git.sr.ht/~sircmpwn/core-go/database"
)

type User struct {
	ID       int       `json:"id"`
	Username string    `json:"username"`

	alias  string
	fields *database.ModelFields
}

func (User) IsEntity() {}

func (u User) CanonicalName() string {
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
			{SQL: "id", GQL: "id", Ptr: &u.ID},
			{SQL: "username", GQL: "username", Ptr: &u.Username},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &u.ID},
		},
	}
	return u.fields
}
