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

type Event struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`

	EventType     int
	ParticipantID *int
	TicketID      int

	ByParticipantID *int
	CommentID       *int
	LabelID         *int
	FromTicketID    *int

	OldStatus     *TicketStatus
	OldResolution *TicketResolution
	NewStatus     *TicketStatus
	NewResolution *TicketResolution

	alias  string
	fields *database.ModelFields
}

type Created struct {
	EventType     EventType `json:"eventType"`
	TicketID      int
	ParticipantID int
}

func (Created) IsEventDetail() {}

type Comment struct {
	EventType     EventType `json:"eventType"`
	TicketID      int
	ParticipantID int

	Database struct {
		ID              int
		Text            string
		Authenticity    Authenticity
		SuperceededByID *int
	}
}

func (Comment) IsEventDetail() {}

type StatusChange struct {
	EventType     EventType `json:"eventType"`
	TicketID      int
	ParticipantID int

	OldStatus     TicketStatus     `json:"oldStatus"`
	NewStatus     TicketStatus     `json:"newStatus"`
	OldResolution TicketResolution `json:"oldResolution"`
	NewResolution TicketResolution `json:"newResolution"`
}

func (StatusChange) IsEventDetail() {}

type LabelUpdate struct {
	EventType     EventType `json:"eventType"`
	TicketID      int
	ParticipantID int
	LabelID       int
}

func (LabelUpdate) IsEventDetail() {}

type Assignment struct {
	EventType  EventType `json:"eventType"`
	TicketID   int
	AssignerID int
	AssigneeID int
}

func (Assignment) IsEventDetail() {}

type UserMention struct {
	EventType     EventType `json:"eventType"`
	TicketID      int
	ParticipantID int
	MentionedID   int
}

func (UserMention) IsEventDetail() {}

type TicketMention struct {
	EventType     EventType `json:"eventType"`
	TicketID      int
	ParticipantID int
	MentionedID   int
}

func (TicketMention) IsEventDetail() {}

const (
	EVENT_CREATED          = 1
	EVENT_COMMENT          = 2
	EVENT_STATUS_CHANGE    = 4
	EVENT_LABEL_ADDED      = 8
	EVENT_LABEL_REMOVED    = 16
	EVENT_ASSIGNED_USER    = 32
	EVENT_UNASSIGNED_USER  = 64
	EVENT_USER_MENTIONED   = 128
	EVENT_TICKET_MENTIONED = 256
)

func (ev *Event) Changes() []EventDetail {
	var changes []EventDetail

	if ev.EventType&EVENT_CREATED != 0 {
		changes = append(changes, Created{
			EventType:     EventTypeCreated,
			TicketID:      ev.TicketID,
			ParticipantID: *ev.ParticipantID,
		})
	}

	if ev.EventType&EVENT_COMMENT != 0 {
		comment := Comment{
			EventType:     EventTypeComment,
			TicketID:      ev.TicketID,
			ParticipantID: *ev.ParticipantID,
		}
		comment.Database.ID = *ev.CommentID
		changes = append(changes, comment)
	}

	if ev.EventType&EVENT_STATUS_CHANGE != 0 {
		changes = append(changes, StatusChange{
			EventType:     EventTypeStatusChange,
			TicketID:      ev.TicketID,
			ParticipantID: *ev.ParticipantID,

			OldStatus:     *ev.OldStatus,
			NewStatus:     *ev.NewStatus,
			OldResolution: *ev.OldResolution,
			NewResolution: *ev.NewResolution,
		})
	}

	if ev.EventType&EVENT_LABEL_ADDED != 0 {
		changes = append(changes, LabelUpdate{
			EventType:     EventTypeLabelAdded,
			TicketID:      ev.TicketID,
			ParticipantID: *ev.ParticipantID,
			LabelID:       *ev.LabelID,
		})
	}

	if ev.EventType&EVENT_LABEL_REMOVED != 0 {
		changes = append(changes, LabelUpdate{
			EventType:     EventTypeLabelRemoved,
			TicketID:      ev.TicketID,
			ParticipantID: *ev.ParticipantID,
			LabelID:       *ev.LabelID,
		})
	}

	if ev.EventType&EVENT_ASSIGNED_USER != 0 {
		changes = append(changes, Assignment{
			EventType:  EventTypeAssignedUser,
			TicketID:   ev.TicketID,
			AssigneeID: *ev.ParticipantID,
			AssignerID: *ev.ByParticipantID,
		})
	}

	if ev.EventType&EVENT_UNASSIGNED_USER != 0 {
		changes = append(changes, Assignment{
			EventType:  EventTypeUnassignedUser,
			TicketID:   ev.TicketID,
			AssigneeID: *ev.ParticipantID,
			AssignerID: *ev.ByParticipantID,
		})
	}

	if ev.EventType&EVENT_USER_MENTIONED != 0 {
		changes = append(changes, UserMention{
			EventType:     EventTypeUserMentioned,
			TicketID:      ev.TicketID,
			ParticipantID: *ev.ByParticipantID,
			MentionedID:   *ev.ParticipantID,
		})
	}

	if ev.EventType&EVENT_TICKET_MENTIONED != 0 {
		changes = append(changes, TicketMention{
			EventType:     EventTypeTicketMentioned,
			TicketID:      *ev.FromTicketID,
			ParticipantID: *ev.ByParticipantID,
			MentionedID:   ev.TicketID,
		})
	}

	return changes
}

func (ev *Event) As(alias string) *Event {
	ev.alias = alias
	return ev
}

func (ev *Event) Alias() string {
	return ev.alias
}

func (ev *Event) Table() string {
	return "event"
}

func (ev *Event) Fields() *database.ModelFields {
	if ev.fields != nil {
		return ev.fields
	}
	ev.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "id", GQL: "id", Ptr: &ev.ID},
			{SQL: "created", GQL: "created", Ptr: &ev.Created},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &ev.ID},
			{SQL: "created", GQL: "", Ptr: &ev.Created},
			{SQL: "event_type", GQL: "", Ptr: &ev.EventType},
			{SQL: "participant_id", GQL: "", Ptr: &ev.ParticipantID},
			{SQL: "ticket_id", GQL: "", Ptr: &ev.TicketID},
			{SQL: "by_participant_id", GQL: "", Ptr: &ev.ByParticipantID},
			{SQL: "comment_id", GQL: "", Ptr: &ev.CommentID},
			{SQL: "label_id", GQL: "", Ptr: &ev.LabelID},
			{SQL: "from_ticket_id", GQL: "", Ptr: &ev.FromTicketID},
			{SQL: "old_status", GQL: "", Ptr: &ev.OldStatus},
			{SQL: "old_resolution", GQL: "", Ptr: &ev.OldResolution},
			{SQL: "new_status", GQL: "", Ptr: &ev.NewStatus},
			{SQL: "new_resolution", GQL: "", Ptr: &ev.NewResolution},
		},
	}
	return ev.fields
}

func (ev *Event) QueryWithCursor(ctx context.Context, runner sq.BaseRunner,
	q sq.SelectBuilder, cur *model.Cursor) ([]*Event, *model.Cursor) {
	var (
		err  error
		rows *sql.Rows
	)

	if cur.Next != "" {
		next, _ := strconv.ParseInt(cur.Next, 10, 64)
		q = q.Where(database.WithAlias(ev.alias, "id")+"<= ?", next)
	}
	q = q.
		OrderBy(database.WithAlias(ev.alias, "id") + " DESC").
		Limit(uint64(cur.Count + 1))

	if rows, err = q.RunWith(runner).QueryContext(ctx); err != nil {
		panic(err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(database.Scan(ctx, &event)...); err != nil {
			panic(err)
		}
		events = append(events, &event)
	}

	if len(events) > cur.Count {
		cur = &model.Cursor{
			Count:  cur.Count,
			Next:   strconv.Itoa(events[len(events)-1].ID),
			Search: cur.Search,
		}
		events = events[:cur.Count]
	} else {
		cur = nil
	}

	return events, cur
}
