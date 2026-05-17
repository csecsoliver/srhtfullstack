package model

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type OAuthGrant struct {
	ID        int       `json:"id"`
	Issued    time.Time `json:"issued"`
	Expires   time.Time `json:"expires"`
	TokenHash string    `json:"token_hash"`
	Grants    *string   `json:"grants"`

	ClientID int `json:"client"`

	alias  string
	fields *database.ModelFields
}

func (og *OAuthGrant) As(alias string) *OAuthGrant {
	og.alias = alias
	return og
}

func (o *OAuthGrant) Alias() string {
	return o.alias
}

func (o *OAuthGrant) Table() string {
	return "oauth2_grant"
}

func (o *OAuthGrant) Fields() *database.ModelFields {
	if o.fields != nil {
		return o.fields
	}
	o.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &o.ID},
			{SQL: "issued", GQL: "issued", Ptr: &o.Issued},
			{SQL: "expires", GQL: "expires", Ptr: &o.Expires},
			{SQL: "token_hash", GQL: "tokenHash", Ptr: &o.TokenHash},
			{SQL: "grants", GQL: "grants", Ptr: &o.Grants},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &o.ID},
			{SQL: "client_id", GQL: "", Ptr: &o.ClientID},
		},
	}
	return o.fields
}

func (og *OAuthGrant) Query(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder) []*OAuthGrant {

	var (
		err  error
		rows *sql.Rows
	)

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var grants []*OAuthGrant
	for rows.Next() {
		var og OAuthGrant
		if err := rows.Scan(database.Scan(ctx, &og)...); err != nil {
			panic(err)
		}
		grants = append(grants, &og)
	}

	return grants
}
