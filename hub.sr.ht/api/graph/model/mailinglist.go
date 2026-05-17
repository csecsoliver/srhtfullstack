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

type MailingList struct {
	ID          int        `json:"id"`
	RID         model.RID  `json:"remote_rid"`
	Linked      time.Time  `json:"linked"`
	Updated     time.Time  `json:"updated"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	Visibility  Visibility `json:"visibility"`

	OwnerID int

	alias  string
	fields *database.ModelFields
}

func (list *MailingList) IsProjectResource() {}

func (list *MailingList) As(alias string) *MailingList {
	list.alias = alias
	return list
}

func (list *MailingList) Alias() string {
	return list.alias
}

func (list *MailingList) Table() string {
	return "mailing_list"
}

func (list *MailingList) Fields() *database.ModelFields {
	if list.fields != nil {
		return list.fields
	}
	list.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "linked", Ptr: &list.Linked},
			{SQL: "updated", GQL: "updated", Ptr: &list.Updated},
			{SQL: "name", GQL: "name", Ptr: &list.Name},
			{SQL: "description", GQL: "description", Ptr: &list.Description},
			{SQL: "visibility", GQL: "visibility", Ptr: &list.Visibility},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &list.ID},
			{SQL: "remote_rid", GQL: "rid", Ptr: &list.RID},
			{SQL: "owner_id", GQL: "", Ptr: &list.OwnerID},
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
	q = q.
		OrderBy(database.WithAlias(list.alias, "updated") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var lists []*MailingList
	for rows.Next() {
		var list MailingList
		if err := rows.Scan(database.Scan(ctx, &list)...); err != nil {
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
