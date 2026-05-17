package model

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type OAuthClient struct {
	ID          int     `json:"id"`
	UUID        string  `json:"uuid"`
	RedirectURL string  `json:"redirectUrl"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	URL         *string `json:"url"`

	OwnerID          int
	clientSecretHash string

	alias  string
	fields *database.ModelFields
}

func (oc *OAuthClient) As(alias string) *OAuthClient {
	oc.alias = alias
	return oc
}

func (o *OAuthClient) Alias() string {
	return o.alias
}

func (o *OAuthClient) Table() string {
	return "oauth2_client"
}

func (o *OAuthClient) Fields() *database.ModelFields {
	if o.fields != nil {
		return o.fields
	}
	o.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &o.ID},
			{SQL: "client_uuid", GQL: "uuid", Ptr: &o.UUID},
			{SQL: "client_name", GQL: "name", Ptr: &o.Name},
			{SQL: "client_description", GQL: "description", Ptr: &o.Description},
			{SQL: "client_url", GQL: "url", Ptr: &o.URL},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &o.ID},
			{SQL: "owner_id", GQL: "", Ptr: &o.OwnerID},
			{SQL: "client_secret_hash", GQL: "", Ptr: &o.clientSecretHash},
			{SQL: "redirect_url", GQL: "", Ptr: &o.RedirectURL},
		},
	}
	return o.fields
}

func (oc *OAuthClient) VerifyClientSecret(clientSecret string) bool {
	wantHash, err := hex.DecodeString(oc.clientSecretHash)
	if err != nil {
		panic(err)
	}

	b, err := base64.StdEncoding.DecodeString(clientSecret)
	if err != nil {
		return false
	}
	gotHash := sha512.Sum512(b)

	return subtle.ConstantTimeCompare(wantHash, gotHash[:]) == 1
}

func (oc *OAuthClient) Query(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder) []*OAuthClient {

	var (
		err  error
		rows *sql.Rows
	)

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var clients []*OAuthClient
	for rows.Next() {
		var oc OAuthClient
		if err := rows.Scan(database.Scan(ctx, &oc)...); err != nil {
			panic(err)
		}
		clients = append(clients, &oc)
	}

	return clients
}
