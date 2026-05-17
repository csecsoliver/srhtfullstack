package trackers

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/crypto"
	"git.sr.ht/~sircmpwn/core-go/database"
	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/loaders"
)

func ExportDump(ctx context.Context, trackerID int, w io.Writer) error {
	tracker, err := loaders.ForContext(ctx).TrackersByID.Load(trackerID)
	if err != nil {
		return err
	}

	if tracker == nil {
		return errors.New("access denied")
	}

	owner, err := loaders.ForContext(ctx).UsersByID.Load(tracker.OwnerID)
	if err != nil {
		return err
	}

	trackerDump := TrackerDump{
		ID: tracker.ID,
		Owner: User{
			ID:            owner.ID,
			CanonicalName: owner.CanonicalName(),
			Name:          owner.Username,
		},
		Created:     tracker.Created,
		Updated:     tracker.Updated,
		Name:        tracker.Name,
		Description: convertNullString(tracker.Description),
	}

	if err := database.WithTx(ctx, &sql.TxOptions{
		ReadOnly: true,
	}, func(tx *sql.Tx) error {
		trackerDump.Labels, err = exportLabels(ctx, tx, tracker.ID)
		if err != nil {
			return err
		}

		var pkids []int
		trackerDump.Tickets, pkids, err = exportTickets(ctx, tx, tracker)
		if err != nil {
			return err
		}

		for i := range trackerDump.Tickets {
			ticket := &trackerDump.Tickets[i]

			ticket.Labels, err = exportTicketLabels(ctx, tx, pkids[i])
			if err != nil {
				return err
			}

			ticket.Assignees, err = exportTicketAssignees(ctx, tx, pkids[i])
			if err != nil {
				return err
			}

			ticket.Events, err = exportTicketEvents(ctx, tx, pkids[i])
			if err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	signDump(&trackerDump)

	gw := gzip.NewWriter(w)
	if err := json.NewEncoder(gw).Encode(&trackerDump); err != nil {
		return err
	}
	return gw.Close()
}

func signDump(tracker *TrackerDump) {
	for i := range tracker.Tickets {
		ticket := &tracker.Tickets[i]

		if ticket.Submitter.Type == "user" {
			signTicket(ticket, tracker.ID)
		}

		for j := range ticket.Events {
			event := &ticket.Events[j]
			if event.Participant != nil && event.Participant.Type == "user" && event.Comment != nil {
				signCommentEvent(event, tracker.ID, ticket.ID)
			}
		}
	}
}

func signTicket(ticket *Ticket, trackerID int) {
	sigdata := TicketSignatureData{
		TrackerID:   trackerID,
		TicketID:    ticket.ID,
		Subject:     ticket.Subject,
		Body:        ticket.Body,
		SubmitterID: ticket.Submitter.UserID,
		Upstream:    ticket.Upstream,
	}
	payload, err := json.Marshal(sigdata)
	if err != nil {
		panic(err)
	}
	ticket.Nonce, ticket.Signature = crypto.SignWebhook(payload)
}

func signCommentEvent(event *Event, trackerID, ticketID int) {
	sigdata := CommentSignatureData{
		TrackerID: trackerID,
		TicketID:  ticketID,
		Comment:   event.Comment.Text,
		AuthorID:  event.Comment.Author.UserID,
		Upstream:  event.Upstream,
	}
	payload, err := json.Marshal(sigdata)
	if err != nil {
		panic(err)
	}
	event.Nonce, event.Signature = crypto.SignWebhook(payload)
}

func exportLabels(ctx context.Context, tx *sql.Tx, trackerID int) ([]Label, error) {
	label := (&model.Label{}).As(`l`)
	query := database.
		Select(ctx, label).
		From(`label l`).
		Where(`l.tracker_id = ?`, trackerID)
	rows, err := query.RunWith(tx).QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var l []Label
	for rows.Next() {
		var label model.Label
		if err := rows.Scan(database.Scan(ctx, &label)...); err != nil {
			return nil, err
		}
		l = append(l, Label{
			ID:              label.ID,
			Created:         label.Created,
			Name:            label.Name,
			BackgroundColor: label.BackgroundColor,
			ForegroundColor: label.ForegroundColor,
		})
	}

	return l, nil
}

func exportTickets(ctx context.Context, tx *sql.Tx, tracker *model.Tracker) ([]Ticket, []int, error) {
	ticket := (&model.Ticket{}).As(`tk`)
	var query sq.SelectBuilder
	if tracker.CanBrowse() {
		query = database.
			Select(ctx, ticket).
			From(`ticket tk`).
			Where(`tk.tracker_id = ?`, tracker.ID)
	} else {
		user := auth.ForContext(ctx)
		query = database.
			Select(ctx, ticket).
			From(`ticket tk`).
			Join(`participant p ON p.user_id = ?`, user.UserID).
			Where(sq.And{
				sq.Expr(`tk.tracker_id = ?`, tracker.ID),
				sq.Expr(`tk.submitter_id = p.id`),
			})
	}

	rows, err := query.RunWith(tx).QueryContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	origin := config.GetOrigin(config.ForContext(ctx), "todo.sr.ht", true)

	var (
		l     []Ticket
		pkids []int
	)
	for rows.Next() {
		var ticket model.Ticket
		if err := rows.Scan(database.Scan(ctx, &ticket)...); err != nil {
			return nil, nil, err
		}

		submitter, err := loaders.ForContext(ctx).EntitiesByParticipantID.Load(ticket.SubmitterID)
		if err != nil {
			return nil, nil, err
		}

		l = append(l, Ticket{
			ID:         ticket.ID,
			Created:    ticket.Created,
			Updated:    ticket.Updated,
			Submitter:  *exportParticipant(submitter),
			Ref:        ticket.Ref(),
			Subject:    ticket.Subject,
			Body:       convertNullString(ticket.Body),
			Status:     strings.ToLower(ticket.Status.String()),
			Resolution: strings.ToLower(ticket.Resolution.String()),
			Upstream:   origin,
		})
		pkids = append(pkids, ticket.PKID)
	}

	return l, pkids, nil
}

func exportTicketEvents(ctx context.Context, tx *sql.Tx, ticketID int) ([]Event, error) {
	event := (&model.Event{}).As(`ev`)
	query := database.
		Select(ctx, event).
		From(`event ev`).
		Where(`ev.ticket_id = ?`, ticketID)

	rows, err := query.RunWith(tx).QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	origin := config.GetOrigin(config.ForContext(ctx), "todo.sr.ht", true)

	var l []Event
	for rows.Next() {
		var ev model.Event
		if err := rows.Scan(database.Scan(ctx, &ev)...); err != nil {
			return nil, err
		}

		evDump := Event{
			ID:       ev.ID,
			Created:  ev.Created,
			Upstream: origin,
		}

		if ev.EventType&model.EVENT_CREATED != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeCreated))
		}
		if ev.EventType&model.EVENT_COMMENT != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeComment))
		}
		if ev.EventType&model.EVENT_STATUS_CHANGE != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeStatusChange))
			evDump.OldStatus = strings.ToLower(ev.OldStatus.String())
			evDump.NewStatus = strings.ToLower(ev.NewStatus.String())
			evDump.OldResolution = strings.ToLower(ev.OldResolution.String())
			evDump.NewResolution = strings.ToLower(ev.NewResolution.String())
		}
		if ev.EventType&model.EVENT_LABEL_ADDED != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeLabelAdded))
		}
		if ev.EventType&model.EVENT_LABEL_REMOVED != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeLabelRemoved))
		}
		if ev.EventType&model.EVENT_ASSIGNED_USER != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeAssignedUser))
		}
		if ev.EventType&model.EVENT_UNASSIGNED_USER != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeUnassignedUser))
		}
		if ev.EventType&model.EVENT_USER_MENTIONED != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeUserMentioned))
		}
		if ev.EventType&model.EVENT_TICKET_MENTIONED != 0 {
			evDump.EventType = append(evDump.EventType, string(model.EventTypeTicketMentioned))
		}

		if ev.ParticipantID != nil {
			entity, err := loaders.ForContext(ctx).EntitiesByParticipantID.Load(*ev.ParticipantID)
			if err != nil {
				return nil, err
			}
			evDump.Participant = exportParticipant(entity)
		}
		if ev.EventType&model.EVENT_COMMENT != 0 {
			comment, err := loaders.ForContext(ctx).CommentsByIDUnsafe.Load(*ev.CommentID)
			if err != nil {
				return nil, err
			}
			author, err := loaders.ForContext(ctx).EntitiesByParticipantID.Load(*ev.ParticipantID)
			if err != nil {
				return nil, err
			}
			evDump.Comment = &Comment{
				ID:      comment.Database.ID,
				Created: ev.Created,
				Author:  *exportParticipant(author),
				Text:    comment.Database.Text,
			}
		}
		if ev.EventType&(model.EVENT_LABEL_ADDED|model.EVENT_LABEL_REMOVED) != 0 {
			name, err := exportLabelName(ctx, *ev.LabelID)
			if err != nil {
				return nil, err
			}
			evDump.Label = &name
		}
		if ev.EventType&(model.EVENT_ASSIGNED_USER|model.EVENT_UNASSIGNED_USER) != 0 {
			byUser, err := loaders.ForContext(ctx).EntitiesByParticipantID.Load(*ev.ByParticipantID)
			if err != nil {
				return nil, err
			}
			evDump.ByUser = exportParticipant(byUser)
		}
		if ev.EventType&model.EVENT_TICKET_MENTIONED != 0 {
			var (
				fromTicket             *model.Ticket
				fromTicketTracker      *model.Tracker
				fromTicketTrackerOwner *model.User
				err                    error
			)
			// TODO write a custom loader, maybe?
			fromTicket, err = loaders.ForContext(ctx).TicketsByID.Load(*ev.FromTicketID)
			if err != nil {
				return nil, err
			}
			if fromTicket != nil {
				fromTicketTracker, err = loaders.ForContext(ctx).TrackersByID.Load(fromTicket.TrackerID)
				if err != nil {
					return nil, err
				}
			}
			if fromTicketTracker != nil {
				fromTicketTrackerOwner, err = loaders.ForContext(ctx).UsersByID.Load(fromTicketTracker.OwnerID)
				if err != nil {
					return nil, err
				}
			}
			if fromTicketTrackerOwner != nil {
				evDump.FromTicket = exportTicketRef(fromTicket, fromTicketTracker, fromTicketTrackerOwner)
			}
		}

		l = append(l, evDump)
	}

	return l, nil
}

func exportLabelName(ctx context.Context, labelID int) (string, error) {
	var name string
	err := database.WithTx(ctx, &sql.TxOptions{
		ReadOnly: true,
	}, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT name FROM label WHERE id = $1`, labelID).Scan(&name)
	})
	return name, err
}

func exportTicketAssignees(ctx context.Context, tx *sql.Tx, ticketID int) ([]User, error) {
	user := (&model.User{}).As(`u`)
	query := database.
		Select(ctx, user).
		From(`ticket_assignee ta`).
		Join(`"user" u ON ta.assignee_id = u.id`).
		Where(`ta.ticket_id = ?`, ticketID)
	rows, err := query.RunWith(tx).QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var l []User
	for rows.Next() {
		var user model.User
		if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
			return nil, err
		}
		l = append(l, User{
			ID:            user.ID,
			CanonicalName: user.CanonicalName(),
			Name:          user.Username,
		})
	}

	return l, nil
}

func exportTicketLabels(ctx context.Context, tx *sql.Tx, ticketID int) ([]string, error) {
	rows, err := tx.Query(`SELECT l.name
		FROM label l
		JOIN ticket_label tl ON tl.label_id = l.id
		WHERE tl.ticket_id = $1`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var l []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		l = append(l, name)
	}

	return l, nil
}

func exportParticipant(e model.Entity) *Participant {
	switch e := e.(type) {
	case *model.User:
		return &Participant{
			Type:          "user",
			UserID:        e.ID,
			CanonicalName: e.CanonicalName(),
			Name:          e.Username,
		}
	case *model.EmailAddress:
		return &Participant{
			Type:    "email",
			Address: e.Mailbox,
			Name:    convertNullString(e.Name),
		}
	case *model.ExternalUser:
		return &Participant{
			Type:        "external",
			ExternalID:  e.ExternalID,
			ExternalURL: convertNullString(e.ExternalURL),
		}
	}
	panic(fmt.Errorf("unknown entity type %T", e))
}

func exportTicketRef(ticket *model.Ticket, tracker *model.Tracker, owner *model.User) *TicketRef {
	return &TicketRef{
		ID:  ticket.ID,
		Ref: ticket.Ref(),
		Tracker: TrackerRef{
			Owner: User{
				ID:            owner.ID,
				CanonicalName: owner.CanonicalName(),
				Name:          owner.Username,
			},
			Created: tracker.Created,
			Updated: tracker.Updated,
			Name:    tracker.Name,
		},
	}
}

func convertNullString(ns *string) string {
	if ns == nil {
		return ""
	}
	return *ns
}
