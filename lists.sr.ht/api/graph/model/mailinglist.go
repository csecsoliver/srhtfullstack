package model

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

const (
	ACCESS_NONE     = 0
	ACCESS_BROWSE   = 1
	ACCESS_REPLY    = 2
	ACCESS_POST     = 4
	ACCESS_MODERATE = 8
	ACCESS_ALL      = 1 | 2 | 4 | 8
)

type MailingList struct {
	ID           int        `json:"id"`
	RID          model.RID  `json:"rid"`
	Created      time.Time  `json:"created"`
	Updated      time.Time  `json:"updated"`
	Name         string     `json:"name"`
	Description  *string    `json:"description"`
	Visibility   Visibility `json:"visibility"`
	Importing    bool       `json:"importing"`
	LastActivity *time.Time `json:"lastActivity"`

	OwnerID       int
	RawPermitMime string
	RawRejectMime string

	Access         int
	DefaultAccess  uint
	AccessID       *int
	SubscriptionID *int

	alias  string
	fields *database.ModelFields
}

func (list *MailingList) PermitMime() []string {
	if len(list.RawPermitMime) == 0 {
		return nil
	}
	return strings.Split(list.RawPermitMime, ",")
}

func (list *MailingList) RejectMime() []string {
	if len(list.RawRejectMime) == 0 {
		return nil
	}
	return strings.Split(list.RawRejectMime, ",")
}

func (list *MailingList) DefaultACL() *GeneralACL {
	return &GeneralACL{
		Browse:   list.DefaultAccess&ACCESS_BROWSE > 0,
		Reply:    list.DefaultAccess&ACCESS_REPLY > 0,
		Post:     list.DefaultAccess&ACCESS_POST > 0,
		Moderate: list.DefaultAccess&ACCESS_MODERATE > 0,
	}
}

func (list *MailingList) As(alias string) *MailingList {
	list.alias = alias
	return list
}

func (list *MailingList) Alias() string {
	return list.alias
}

func (list *MailingList) Table() string {
	return "list"
}

func (list *MailingList) Fields() *database.ModelFields {
	if list.fields != nil {
		return list.fields
	}
	list.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &list.ID},
			{SQL: "created", GQL: "created", Ptr: &list.Created},
			{SQL: "name", GQL: "name", Ptr: &list.Name},
			{SQL: "description", GQL: "description", Ptr: &list.Description},
			{SQL: "import_in_progress", GQL: "importing", Ptr: &list.Importing},
			{SQL: "permit_mimetypes", GQL: "permitMime", Ptr: &list.RawPermitMime},
			{SQL: "reject_mimetypes", GQL: "rejectMime", Ptr: &list.RawRejectMime},
			{SQL: "visibility", GQL: "visibility", Ptr: &list.Visibility},
			{SQL: "default_access", GQL: "defaultACL", Ptr: &list.DefaultAccess},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &list.ID},
			{SQL: "rid", GQL: "", Ptr: &list.RID},
			{SQL: "owner_id", GQL: "", Ptr: &list.OwnerID},
			{SQL: "updated", GQL: "", Ptr: &list.Updated},
			{SQL: "last_activity", GQL: "", Ptr: &list.LastActivity},
		},
	}
	return list.fields
}

func (list *MailingList) QueryWithCursor(ctx context.Context,
	runner sq.BaseRunner, q sq.SelectBuilder,
	cur *model.Cursor) ([]*MailingList, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		ts, _ := strconv.ParseInt(cur.Next, 10, 64)
		updated := time.UnixMicro(ts).UTC()
		q = q.Where(database.WithAlias(list.alias, "updated")+"<= ?", updated)
	}
	user := auth.ForContext(ctx)
	q = q.
		LeftJoin(fmt.Sprintf(`access ON
			access.list_id = %s.id AND
			access.user_id = ?`, list.alias), user.UserID).
		LeftJoin(fmt.Sprintf(`subscription sub ON
			sub.list_id = %s.id AND
			sub.user_id = ?`, list.alias), user.UserID).
		Column(fmt.Sprintf(`COALESCE(
			access.permissions,
			CASE WHEN %s.owner_id = ?
				THEN ?
				ELSE %s.default_access
			END)`, list.alias, list.alias),
			user.UserID, ACCESS_ALL).
		Column(`access.id`).
		Column(`sub.id`).
		OrderBy(database.WithAlias(list.alias, `updated`) + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var lists []*MailingList
	for rows.Next() {
		var list MailingList
		if err := rows.Scan(append(database.Scan(ctx, &list),
			&list.Access,
			&list.AccessID,
			&list.SubscriptionID)...); err != nil {
			panic(err)
		}
		lists = append(lists, &list)
	}

	if len(lists) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.FormatInt(lists[len(lists)-1].Updated.Unix(), 10),
			Search: cur.Search,
		}
		lists = lists[:cur.Count]
	} else {
		cur = nil
	}

	return lists, cur
}
