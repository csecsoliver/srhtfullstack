package webhooks

import (
	"context"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
)

func deliverUserWebhook(ctx context.Context, event model.WebhookEvent,
	payload model.WebhookPayload, payloadUUID uuid.UUID) {
	q := webhooks.ForContext(ctx)
	userID := auth.ForContext(ctx).UserID
	query := sq.
		Select().
		From("gql_user_wh_sub sub").
		Where("sub.user_id = ?", userID)
	q.Schedule(ctx, query, "user", event.String(),
		payloadUUID, payload)
}

func deliverTrackerWebhook(ctx context.Context, trackerID int,
	event model.WebhookEvent, payload model.WebhookPayload, payloadUUID uuid.UUID) {
	q := webhooks.ForContext(ctx)
	query := sq.
		Select().
		From(`gql_tracker_wh_sub sub`).
		Join(`tracker tr ON tr.id = sub.tracker_id`).
		LeftJoin(`user_access ua ON ua.tracker_id = sub.tracker_id AND ua.user_id = sub.user_id`).
		Where(sq.And{
			sq.Expr(`sub.tracker_id = ?`, trackerID),
			sq.Or{
				sq.Expr(`tr.owner_id = sub.user_id`),
				sq.Expr(`tr.visibility != 'PRIVATE'`),
				sq.Expr(`ua.permissions > 0`),
			},
		})
	q.Schedule(ctx, query, "tracker", event.String(),
		payloadUUID, payload)
}

func deliverTicketWebhook(ctx context.Context, ticketID int,
	event model.WebhookEvent, payload model.WebhookPayload, payloadUUID uuid.UUID) {
	q := webhooks.ForContext(ctx)
	query := sq.
		Select().
		From("gql_ticket_wh_sub sub").
		Join(`tracker tr ON tr.id = sub.tracker_id`).
		LeftJoin(`user_access ua ON ua.tracker_id = sub.tracker_id AND ua.user_id = sub.user_id`).
		Where(sq.And{
			sq.Expr(`sub.ticket_id = ?`, ticketID),
			sq.Or{
				sq.Expr(`tr.owner_id = sub.user_id`),
				sq.Expr(`tr.visibility != 'PRIVATE'`),
				sq.Expr(`ua.permissions > 0`),
			},
		})
	q.Schedule(ctx, query, "ticket", event.String(),
		payloadUUID, payload)
}

func DeliverUserTrackerEvent(ctx context.Context,
	event model.WebhookEvent, tracker *model.Tracker) {
	payloadUUID := uuid.New()
	payload := model.TrackerEvent{
		UUID:    payloadUUID.String(),
		Event:   event,
		Date:    time.Now().UTC(),
		Tracker: tracker,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverUserTicketEvent(ctx context.Context,
	event model.WebhookEvent, ticket *model.Ticket) {
	payloadUUID := uuid.New()
	payload := model.TicketEvent{
		UUID:   payloadUUID.String(),
		Event:  event,
		Date:   time.Now().UTC(),
		Ticket: ticket,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverTrackerEvent(ctx context.Context,
	event model.WebhookEvent, tracker *model.Tracker) {
	payloadUUID := uuid.New()
	payload := model.TrackerEvent{
		UUID:    payloadUUID.String(),
		Event:   event,
		Date:    time.Now().UTC(),
		Tracker: tracker,
	}
	deliverTrackerWebhook(ctx, tracker.ID, event, &payload, payloadUUID)
}

func DeliverTrackerLabelEvent(ctx context.Context,
	event model.WebhookEvent, trackerID int, label *model.Label) {
	payloadUUID := uuid.New()
	payload := model.LabelEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		Label: label,
	}
	deliverTrackerWebhook(ctx, trackerID, event, &payload, payloadUUID)
}

func DeliverTrackerTicketEvent(ctx context.Context,
	event model.WebhookEvent, trackerID int, ticket *model.Ticket) {
	payloadUUID := uuid.New()
	payload := model.TicketEvent{
		UUID:   payloadUUID.String(),
		Event:  event,
		Date:   time.Now().UTC(),
		Ticket: ticket,
	}
	deliverTrackerWebhook(ctx, trackerID, event, &payload, payloadUUID)
}

func DeliverTrackerTicketDeletedEvent(ctx context.Context, trackerID int, ticket *model.Ticket) {
	event := model.WebhookEventTicketDeleted
	payloadUUID := uuid.New()
	payload := model.TicketDeletedEvent{
		UUID:      payloadUUID.String(),
		Event:     event,
		Date:      time.Now().UTC(),
		TrackerID: ticket.TrackerID,
		TicketID:  ticket.ID,
	}
	deliverTrackerWebhook(ctx, trackerID, event, &payload, payloadUUID)
}

func DeliverTrackerEventCreated(ctx context.Context, trackerID int, newEvent *model.Event) {
	event := model.WebhookEventEventCreated
	payloadUUID := uuid.New()
	payload := model.EventCreated{
		UUID:     payloadUUID.String(),
		Event:    event,
		Date:     time.Now().UTC(),
		NewEvent: newEvent,
	}
	deliverTrackerWebhook(ctx, trackerID, event, &payload, payloadUUID)
}

func DeliverTicketEvent(ctx context.Context,
	event model.WebhookEvent, ticketID int, ticket *model.Ticket) {
	payloadUUID := uuid.New()
	payload := model.TicketEvent{
		UUID:   payloadUUID.String(),
		Event:  event,
		Date:   time.Now().UTC(),
		Ticket: ticket,
	}
	deliverTicketWebhook(ctx, ticketID, event, &payload, payloadUUID)
}

func DeliverTicketDeletedEvent(ctx context.Context, ticketID int, ticket *model.Ticket) {
	event := model.WebhookEventTicketDeleted
	payloadUUID := uuid.New()
	payload := model.TicketDeletedEvent{
		UUID:      payloadUUID.String(),
		Event:     event,
		Date:      time.Now().UTC(),
		TrackerID: ticket.TrackerID,
		TicketID:  ticket.ID,
	}
	deliverTicketWebhook(ctx, ticketID, event, &payload, payloadUUID)
}

func DeliverTicketEventCreated(ctx context.Context, ticketID int, newEvent *model.Event) {
	event := model.WebhookEventEventCreated
	payloadUUID := uuid.New()
	payload := model.EventCreated{
		UUID:     payloadUUID.String(),
		Event:    event,
		Date:     time.Now().UTC(),
		NewEvent: newEvent,
	}
	deliverTicketWebhook(ctx, ticketID, event, &payload, payloadUUID)
}
