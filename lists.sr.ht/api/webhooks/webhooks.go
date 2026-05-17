package webhooks

import (
	"context"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model"
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

func deliverListWebhook(ctx context.Context, listID int,
	event model.WebhookEvent, payload model.WebhookPayload, payloadUUID uuid.UUID) {
	q := webhooks.ForContext(ctx)
	query := sq.
		Select().
		From("gql_list_wh_sub sub").
		Join(`list ON list.id = sub.list_id`).
		LeftJoin(`access ON
			access.list_id = sub.list_id AND
			access.user_id = sub.user_id`).
		LeftJoin(`subscription lsub ON
			lsub.list_id = sub.list_id AND
			lsub.user_id = sub.user_id`).
		Where(sq.And{
			sq.Expr(`sub.list_id = ?`, listID),
			sq.Or{
				sq.Expr(`list.owner_id = sub.user_id`),
				sq.Expr(`access.permissions & ? > 0`, model.ACCESS_BROWSE),
				sq.Expr(`list.default_access & ? > 0`, model.ACCESS_BROWSE),
			},
		})
	q.Schedule(ctx, query, "list", event.String(),
		payloadUUID, payload)
}

func DeliverUserMailingListEvent(ctx context.Context,
	event model.WebhookEvent, list *model.MailingList) {
	payloadUUID := uuid.New()
	payload := model.MailingListEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		List:  list,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverUserEmailEvent(ctx context.Context,
	event model.WebhookEvent, email *model.Email) {
	payloadUUID := uuid.New()
	payload := model.EmailEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		Email: email,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverUserPatchsetEvent(ctx context.Context,
	event model.WebhookEvent, patchset *model.Patchset) {
	payloadUUID := uuid.New()
	payload := model.PatchsetEvent{
		UUID:     payloadUUID.String(),
		Event:    event,
		Date:     time.Now().UTC(),
		Patchset: patchset,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverMailingListEvent(ctx context.Context,
	event model.WebhookEvent, list *model.MailingList) {
	payloadUUID := uuid.New()
	payload := model.MailingListEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		List:  list,
	}
	deliverListWebhook(ctx, list.ID, event, &payload, payloadUUID)
}

func DeliverListEmailEvent(ctx context.Context, listID int,
	event model.WebhookEvent, email *model.Email) {
	payloadUUID := uuid.New()
	payload := model.EmailEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		Email: email,
	}
	deliverListWebhook(ctx, listID, event, &payload, payloadUUID)
}

func DeliverListPatchsetEvent(ctx context.Context, listID int,
	event model.WebhookEvent, patchset *model.Patchset) {
	payloadUUID := uuid.New()
	payload := model.PatchsetEvent{
		UUID:     payloadUUID.String(),
		Event:    event,
		Date:     time.Now().UTC(),
		Patchset: patchset,
	}
	deliverListWebhook(ctx, listID, event, &payload, payloadUUID)
}
