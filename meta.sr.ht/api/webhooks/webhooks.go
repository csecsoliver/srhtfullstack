package webhooks

import (
	"context"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

func deliverProfileWebhook(ctx context.Context, event model.WebhookEvent,
	payload model.WebhookPayload, payloadUUID uuid.UUID) {
	q := webhooks.ForContext(ctx)
	userID := auth.ForContext(ctx).UserID
	query := sq.
		Select().
		From("gql_profile_wh_sub sub").
		Where("sub.user_id = ?", userID)
	q.Schedule(ctx, query, "profile", event.String(),
		payloadUUID, payload)
}

func DeliverProfileUpdate(ctx context.Context, user *model.User) {
	payloadUUID := uuid.New()
	payload := model.ProfileUpdateEvent{
		UUID:    payloadUUID.String(),
		Event:   model.WebhookEventProfileUpdate,
		Date:    time.Now().UTC(),
		Profile: user,
	}
	event := model.WebhookEventProfileUpdate
	deliverProfileWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverPGPKeyEvent(ctx context.Context,
	event model.WebhookEvent, key *model.PGPKey) {
	payloadUUID := uuid.New()
	payload := model.PGPKeyEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		Key:   key,
	}
	deliverProfileWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverSSHKeyEvent(ctx context.Context,
	event model.WebhookEvent, key *model.SSHKey) {
	payloadUUID := uuid.New()
	payload := model.SSHKeyEvent{
		UUID:  payloadUUID.String(),
		Event: event,
		Date:  time.Now().UTC(),
		Key:   key,
	}
	deliverProfileWebhook(ctx, event, &payload, payloadUUID)
}
