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

const (
	SECRET_PGPKEY = "pgp_key"
	SECRET_SSHKEY = "ssh_key"
	SECRET_FILE   = "plaintext_file"
)

type Secret interface {
	IsSecret()
}

type RawSecret struct {
	ID         int
	Created    time.Time
	UUID       string
	SecretType string
	Secret     []byte
	Name       *string
	Path       *string
	Mode       *int

	FromUserID *int

	alias  string
	fields *database.ModelFields
}

func (s *RawSecret) As(alias string) *RawSecret {
	s.alias = alias
	return s
}

func (s *RawSecret) Alias() string {
	return s.alias
}

func (s *RawSecret) Table() string {
	return `"secret"`
}

type PGPKey struct {
	ID         int       `json:"id"`
	Created    time.Time `json:"created"`
	UUID       string    `json:"uuid"`
	Name       *string   `json:"name"`
	PrivateKey []byte    `json:"privateKey"`

	FromUserID *int
}

func (PGPKey) IsSecret() {}

type SSHKey struct {
	ID         int       `json:"id"`
	Created    time.Time `json:"created"`
	UUID       string    `json:"uuid"`
	Name       *string   `json:"name"`
	PrivateKey []byte    `json:"privateKey"`

	FromUserID *int
}

func (SSHKey) IsSecret() {}

type SecretFile struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`
	UUID    string    `json:"uuid"`
	Name    *string   `json:"name"`
	Path    string    `json:"path"`
	Mode    int       `json:"mode"`
	Data    []byte    `json:"data"`

	FromUserID *int
}

func (SecretFile) IsSecret() {}

func (s *RawSecret) ToSecret() Secret {
	switch s.SecretType {
	case SECRET_PGPKEY:
		return &PGPKey{
			ID:         s.ID,
			Created:    s.Created,
			UUID:       s.UUID,
			Name:       s.Name,
			PrivateKey: s.Secret,
			FromUserID: s.FromUserID,
		}
	case SECRET_SSHKEY:
		return &SSHKey{
			ID:         s.ID,
			Created:    s.Created,
			UUID:       s.UUID,
			Name:       s.Name,
			PrivateKey: s.Secret,
			FromUserID: s.FromUserID,
		}
	case SECRET_FILE:
		return &SecretFile{
			ID:         s.ID,
			Created:    s.Created,
			UUID:       s.UUID,
			Name:       s.Name,
			Path:       *s.Path,
			Mode:       *s.Mode,
			Data:       s.Secret,
			FromUserID: s.FromUserID,
		}
	default:
		panic("Database invariant broken: unknown secret type")
	}
}

func (s *RawSecret) Fields() *database.ModelFields {
	if s.fields != nil {
		return s.fields
	}
	s.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &s.Created},
			{SQL: "uuid", GQL: "uuid", Ptr: &s.UUID},
			{SQL: "name", GQL: "name", Ptr: &s.Name},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &s.ID},
			{SQL: "from_user_id", GQL: "", Ptr: &s.FromUserID},
			{SQL: "secret_type", GQL: "", Ptr: &s.SecretType},
			{SQL: "secret", GQL: "", Ptr: &s.Secret},
			{SQL: "path", GQL: "", Ptr: &s.Path},
			{SQL: "mode", GQL: "", Ptr: &s.Mode},
		},
	}
	return s.fields
}

func (s *RawSecret) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]Secret, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(s.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(s.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var (
		secrets []Secret
		lastId  int
	)
	for rows.Next() {
		var secret RawSecret
		if err := rows.Scan(database.Scan(ctx, &secret)...); err != nil {
			panic(err)
		}
		lastId = secret.ID
		secrets = append(secrets, secret.ToSecret())
	}

	if len(secrets) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(lastId),
			Search: cur.Search,
		}
		secrets = secrets[:cur.Count]
	} else {
		cur = nil
	}

	return secrets, cur
}
