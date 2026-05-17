package model

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type SubscriptionInfo struct {
	ID        int
	Created   time.Time
	TicketID  *int
	TrackerID *int

	alias  string
	fields *database.ModelFields
}

type ActivitySubscription interface {
	IsSubscription()
	DBID() int
}

type TicketSubscription struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`

	TicketID int
}

func (TicketSubscription) IsSubscription() {}

func (ts TicketSubscription) DBID() int {
	return ts.ID
}

type TrackerSubscription struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`

	TrackerID int
}

func (TrackerSubscription) IsSubscription() {}

func (ts TrackerSubscription) DBID() int {
	return ts.ID
}

func (si *SubscriptionInfo) As(alias string) *SubscriptionInfo {
	si.alias = alias
	return si
}

func (si *SubscriptionInfo) Alias() string {
	return si.alias
}

func (si *SubscriptionInfo) Table() string {
	return `"ticket_subscription"`
}

func (si *SubscriptionInfo) Fields() *database.ModelFields {
	if si.fields != nil {
		return si.fields
	}
	si.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &si.ID},
			{SQL: "created", GQL: "created", Ptr: &si.Created},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &si.ID},
			{SQL: "tracker_id", GQL: "", Ptr: &si.TrackerID},
			{SQL: "ticket_id", GQL: "", Ptr: &si.TicketID},
		},
	}
	return si.fields
}

func (si *SubscriptionInfo) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]ActivitySubscription, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(si.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(si.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var subs []ActivitySubscription
	for rows.Next() {
		var si SubscriptionInfo
		if err := rows.Scan(database.Scan(ctx, &si)...); err != nil {
			panic(err)
		}
		if si.TicketID != nil {
			subs = append(subs, &TicketSubscription{
				ID:       si.ID,
				Created:  si.Created,
				TicketID: *si.TicketID,
			})
		} else if si.TrackerID != nil {
			subs = append(subs, &TrackerSubscription{
				ID:        si.ID,
				Created:   si.Created,
				TrackerID: *si.TrackerID,
			})
		} else {
			panic(errors.New("database invariant broken: subscription with null tracker & ticket"))
		}
	}

	if len(subs) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(subs[len(subs)-1].DBID()),
			Search: cur.Search,
		}
		subs = subs[:cur.Count]
	} else {
		cur = nil
	}

	return subs, cur
}
