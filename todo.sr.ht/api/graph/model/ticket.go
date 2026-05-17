package model

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/model"
)

type Ticket struct {
	ID           int              `json:"id"` // tracker-scoped ID
	Created      time.Time        `json:"created"`
	Updated      time.Time        `json:"updated"`
	Subject      string           `json:"subject"`
	Body         *string          `json:"body"`
	Authenticity Authenticity     `json:"authenticity"`
	Status       TicketStatus     `json:"status"`
	Resolution   TicketResolution `json:"resolution"`

	PKID        int // global ID
	TrackerID   int
	TrackerName string
	OwnerName   string
	SubmitterID int

	alias  string
	fields *database.ModelFields
}

func (t *Ticket) As(alias string) *Ticket {
	t.alias = alias
	return t
}

func (t *Ticket) Alias() string {
	return t.alias
}

func (t *Ticket) Table() string {
	return `"tracker"`
}

func (t *Ticket) Ref() string {
	return fmt.Sprintf("~%s/%s#%d", t.OwnerName, t.TrackerName, t.ID)
}

func (t *Ticket) EmailRef(domain string) string {
	return fmt.Sprintf("~%s/%s/%d@%s", t.OwnerName, t.TrackerName, t.ID, domain)
}

func (t *Ticket) Fields() *database.ModelFields {
	if t.fields != nil {
		return t.fields
	}
	t.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "created", GQL: "created", Ptr: &t.Created},
			{SQL: "updated", GQL: "updated", Ptr: &t.Updated},
			{SQL: "title", GQL: "subject", Ptr: &t.Subject},
			{SQL: "description", GQL: "body", Ptr: &t.Body},
			{SQL: "authenticity", GQL: "authenticity", Ptr: &t.Authenticity},
			{SQL: "status", GQL: "status", Ptr: &t.Status},
			{SQL: "resolution", GQL: "resolution", Ptr: &t.Resolution},
			{SQL: "tracker.name", GQL: "ref", Ptr: &t.TrackerName},
			{SQL: `"user".username`, GQL: "ref", Ptr: &t.OwnerName},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &t.PKID},
			{SQL: "scoped_id", GQL: "", Ptr: &t.ID},
			{SQL: "submitter_id", GQL: "", Ptr: &t.SubmitterID},
			{SQL: "tracker_id", GQL: "", Ptr: &t.TrackerID},
		},
	}
	return t.fields
}

func (t *Ticket) Select(q sq.SelectBuilder) sq.SelectBuilder {
	return q.LeftJoin(fmt.Sprintf(`tracker on %s = tracker.id`,
		database.WithAlias(t.alias, "tracker_id"))).
		LeftJoin(`"user" on tracker.owner_id = "user".id`)
}

func (t *Ticket) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*Ticket, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(t.alias, "scoped_id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(t.alias, "scoped_id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var tickets []*Ticket
	for rows.Next() {
		var ticket Ticket
		if err := rows.Scan(database.Scan(ctx, &ticket)...); err != nil {
			panic(err)
		}
		tickets = append(tickets, &ticket)
	}

	if len(tickets) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(tickets[len(tickets)-1].ID),
			Search: cur.Search,
		}
		tickets = tickets[:cur.Count]
	} else {
		cur = nil
	}

	return tickets, cur
}
