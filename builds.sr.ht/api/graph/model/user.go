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
	return `"user"`
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

func (u *User) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*User, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(u.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(u.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var user User
		if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
			panic(err)
		}
		users = append(users, &user)
	}

	if len(users) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(users[len(users)-1].ID),
			Search: cur.Search,
		}
		users = users[:cur.Count]
	} else {
		cur = nil
	}

	return users, cur
}
