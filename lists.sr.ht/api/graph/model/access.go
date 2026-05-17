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

type GeneralACL struct {
	Browse   bool `json:"browse"`
	Reply    bool `json:"reply"`
	Post     bool `json:"post"`
	Moderate bool `json:"moderate"`
}

func (GeneralACL) IsACL() {}

type MailingListACL struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`

	UserID        *int
	Email         *string
	MailingListID int
	RawAccess     uint

	alias  string
	fields *database.ModelFields
}

func (MailingListACL) IsACL() {}

func (acl *MailingListACL) As(alias string) *MailingListACL {
	acl.alias = alias
	return acl
}

func (acl *MailingListACL) Alias() string {
	return acl.alias
}

func (acl *MailingListACL) Table() string {
	return "access"
}

func (acl *MailingListACL) Browse() bool {
	return acl.RawAccess&ACCESS_BROWSE != 0
}

func (acl *MailingListACL) Reply() bool {
	return acl.RawAccess&ACCESS_REPLY != 0
}

func (acl *MailingListACL) Post() bool {
	return acl.RawAccess&ACCESS_POST != 0
}

func (acl *MailingListACL) Moderate() bool {
	return acl.RawAccess&ACCESS_MODERATE != 0
}

func (acl *MailingListACL) Fields() *database.ModelFields {
	if acl.fields != nil {
		return acl.fields
	}
	acl.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &acl.Created},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &acl.ID},
			{SQL: "permissions", GQL: "", Ptr: &acl.RawAccess},
			{SQL: "list_id", GQL: "", Ptr: &acl.MailingListID},
			{SQL: "user_id", GQL: "", Ptr: &acl.UserID},
			{SQL: "email", GQL: "", Ptr: &acl.Email},
		},
	}
	return acl.fields
}

func UserACL(ctx context.Context, tx *sql.Tx, listID int, email string) (*GeneralACL, error) {
	var access uint
	row := tx.QueryRowContext(ctx,
		`SELECT COALESCE( (
			SELECT 0xF
			FROM list l JOIN "user" u ON l.owner_id = u.id
			WHERE l.id = $1 AND u.email = $2
		), (
			SELECT a.permissions
			FROM access a LEFT OUTER JOIN "user" u ON a.user_id = u.id
			WHERE a.list_id = $1 AND (u.email = $2 OR a.email = $2)
			LIMIT 1
		), (
			SELECT default_access
			FROM list
			WHERE list.id = $1
		) );`,
		listID, email,
	)
	if err := row.Scan(&access); err != nil {
		return nil, err
	}
	return &GeneralACL{
		Browse:   access&ACCESS_BROWSE == ACCESS_BROWSE,
		Reply:    access&ACCESS_REPLY == ACCESS_REPLY,
		Post:     access&ACCESS_POST == ACCESS_POST,
		Moderate: access&ACCESS_MODERATE == ACCESS_MODERATE,
	}, nil
}

func (acl *MailingListACL) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*MailingListACL, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		created := time.Unix(ts, 0)
		q = q.Where(database.WithAlias(acl.alias, "created")+"<= ?", created)
	}
	q = q.
		OrderBy(database.WithAlias(acl.alias, `created`) + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var (
		acls   []*MailingListACL
		latest time.Time
	)
	for rows.Next() {
		var acl MailingListACL
		if err := rows.Scan(database.Scan(ctx, &acl)...); err != nil {
			panic(err)
		}
		latest = acl.Created
		acls = append(acls, &acl)
	}

	if len(acls) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(latest.Unix(), 10),
			Search: cur.Search,
		}
		acls = acls[:cur.Count]
	} else {
		cur = nil
	}

	return acls, cur
}
