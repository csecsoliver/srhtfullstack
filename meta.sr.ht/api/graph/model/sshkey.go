package model

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type SSHKey struct {
	ID                int        `json:"id"`
	RID               model.RID  `json:"rid"`
	Created           time.Time  `json:"created"`
	LastUsed          *time.Time `json:"lastUsed"`
	Key               string     `json:"key"`
	KeyType           string     `json:"keyType"`
	FingerprintSHA256 string     `json:"fingerprintSHA256"`
	Comment           *string    `json:"comment"`

	UserID int

	alias  string
	fields *database.ModelFields
}

func (k *SSHKey) As(alias string) *SSHKey {
	k.alias = alias
	return k
}

func (k *SSHKey) Alias() string {
	return k.alias
}

func (k *SSHKey) Table() string {
	return "sshkey"
}

func (k *SSHKey) Fields() *database.ModelFields {
	if k.fields != nil {
		return k.fields
	}
	k.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &k.ID},
			{SQL: "created", GQL: "created", Ptr: &k.Created},
			{SQL: "last_used", GQL: "lastUsed", Ptr: &k.LastUsed},
			{SQL: "key", GQL: "key", Ptr: &k.Key},
			{SQL: "key_type", GQL: "keyType", Ptr: &k.KeyType},
			{SQL: "fingerprint_sha256", GQL: "fingerprintSHA256", Ptr: &k.FingerprintSHA256},
			{SQL: "comment", GQL: "comment", Ptr: &k.Comment},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &k.ID},
			{SQL: "rid", GQL: "", Ptr: &k.RID},
			{SQL: "user_id", GQL: "", Ptr: &k.UserID},
		},
	}
	return k.fields
}

func (k *SSHKey) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*SSHKey, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(k.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(k.alias, "id")).
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var keys []*SSHKey
	for rows.Next() {
		var key SSHKey
		if err := rows.Scan(database.Scan(ctx, &key)...); err != nil {
			panic(err)
		}
		keys = append(keys, &key)
	}

	if len(keys) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(keys[len(keys)-1].ID),
			Search: cur.Search,
		}
		keys = keys[:cur.Count]
	} else {
		cur = nil
	}

	return keys, cur
}

func LookupSSHKeyByFingerprint(ctx context.Context, fingerprint string) (*SSHKey, error) {
	// Normalize fingerprint
	fingerprint = strings.ToLower(fingerprint)
	fingerprint = strings.ReplaceAll(fingerprint, ":", "")
	b, err := hex.DecodeString(fingerprint)
	if err != nil {
		return nil, err
	}
	// TODO: Consider storing the fingerprint in the database in binary
	if len(b) != 16 {
		return nil, fmt.Errorf("invalid key format; expected 16 bytes")
	}

	var normalized bytes.Buffer
	for i := range b {
		colon := ":"
		if i+1 == len(b) {
			colon = ""
		}
		normalized.WriteString(fmt.Sprintf("%02x%s", b[i], colon))
	}

	key := (&SSHKey{}).As(`key`)
	if err := database.WithTx(ctx, &sql.TxOptions{
		Isolation: 0,
		ReadOnly:  true,
	}, func(tx *sql.Tx) error {
		q := database.
			Select(ctx, key).
			From(`sshkey key`).
			Where(`key.fingerprint = ?`, normalized.String()).
			Limit(1)

		row := q.RunWith(tx).QueryRowContext(ctx)
		if err := row.Scan(database.Scan(ctx, key)...); err != nil {
			if err == sql.ErrNoRows {
				key = nil
				return nil
			}
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return key, nil
}
