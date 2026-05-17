package model

import (
	"context"
	"database/sql"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type PGPKey struct {
	ID      int       `json:"id"`
	RID     model.RID `json:"rid"`
	Created time.Time `json:"created"`
	Key     string    `json:"key"`

	UserID         int
	RawFingerprint []byte

	alias  string
	fields *database.ModelFields
}

func (k *PGPKey) Fingerprint() string {
	return strings.ToUpper(hex.EncodeToString(k.RawFingerprint))
}

func (k *PGPKey) As(alias string) *PGPKey {
	k.alias = alias
	return k
}

func (k *PGPKey) Alias() string {
	return k.alias
}

func (k *PGPKey) Table() string {
	return "pgpkey"
}

func (k *PGPKey) Fields() *database.ModelFields {
	if k.fields != nil {
		return k.fields
	}
	k.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &k.ID},
			{SQL: "created", GQL: "created", Ptr: &k.Created},
			{SQL: "key", GQL: "key", Ptr: &k.Key},
			{SQL: "fingerprint", GQL: "fingerprint", Ptr: &k.RawFingerprint},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &k.ID},
			{SQL: "rid", GQL: "", Ptr: &k.RID},
			{SQL: "user_id", GQL: "", Ptr: &k.UserID},
		},
	}
	return k.fields
}

func (k *PGPKey) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*PGPKey, *model.Cursor) {
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

	var keys []*PGPKey
	for rows.Next() {
		var key PGPKey
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
