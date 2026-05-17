package webhooks

import (
	"context"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/builds.sr.ht/api/graph/model"
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

func DeliverUserJobEvent(ctx context.Context,
	event model.WebhookEvent, job *model.Job) {
	payloadUUID := uuid.New()
	payload := model.JobEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		Job:   job,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}
