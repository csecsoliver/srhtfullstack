package model

import (
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type User struct {
	ID       int       `json:"id"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
	URL      *string   `json:"url"`
	Location *string   `json:"location"`
	Bio      *string   `json:"bio"`

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
			{SQL: "id", GQL: "id", Ptr: &u.ID},
			{SQL: "created", GQL: "created", Ptr: &u.Created},
			{SQL: "updated", GQL: "updated", Ptr: &u.Updated},
			{SQL: "username", GQL: "username", Ptr: &u.Username},
			{SQL: "email", GQL: "email", Ptr: &u.Email},
			{SQL: "url", GQL: "url", Ptr: &u.URL},
			{SQL: "location", GQL: "location", Ptr: &u.Location},
			{SQL: "bio", GQL: "bio", Ptr: &u.Bio},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &u.ID},
			{SQL: "username", GQL: "", Ptr: &u.Username},
		},
	}
	return u.fields
}
