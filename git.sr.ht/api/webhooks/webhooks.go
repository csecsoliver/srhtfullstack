package webhooks

import (
	"context"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	sq "github.com/Masterminds/squirrel"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/uuid"

	"git.sr.ht/~sircmpwn/git.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/git.sr.ht/api/loaders"
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

func DeliverRepoEvent(ctx context.Context,
	event model.WebhookEvent, repository *model.Repository) {
	payloadUUID := uuid.New()
	payload := model.RepositoryEvent{
		UUID:       payloadUUID.String(),
		Event:      event,
		Date:       time.Now().UTC(),
		Repository: repository,
	}
	deliverUserWebhook(ctx, event, &payload, payloadUUID)
}

func DeliverGitEvent(ctx context.Context, input model.GitEventInput) bool {
	payloadUUID := uuid.New()

	user := auth.ForContext(ctx)
	pusher, err := loaders.ForContext(ctx).UsersByID.Load(user.UserID)
	if err != nil {
		panic(err)
	}

	repo, err := loaders.ForContext(ctx).RepositoriesByID.Load(input.RepositoryID)
	if err != nil {
		panic(err)
	}

	grepo := repo.Repo()

	var updates []*model.UpdatedRef
	for _, ref := range input.Updates {
		var (
			gref *plumbing.Reference
			err  error
		)
		oldHash := plumbing.NewHash(ref.Old)
		newHash := plumbing.NewHash(ref.New)

		if !oldHash.IsZero() && !newHash.IsZero() {
			grepo.Lock()
			gref, err = grepo.Reference(
				plumbing.ReferenceName(ref.Ref), true)
			grepo.Unlock()
			if err != nil {
				panic(err)
			}
		}

		upd := model.NewUpdatedRef(repo, gref,
			&oldHash, &newHash)
		updates = append(updates, &upd)
	}

	payload := model.GitEvent{
		UUID:       payloadUUID.String(),
		Event:      input.Event,
		Date:       time.Now().UTC(),
		Repository: repo,
		Pusher:     pusher,
		Updates:    updates,
	}

	q := webhooks.ForContext(ctx)
	query := sq.
		Select().
		From(`gql_git_wh_sub sub`).
		Where(`sub.repo_id = ?`, repo.ID)
	q.Schedule(ctx, query, "git", input.Event.String(),
		payloadUUID, payload)

	// TODO: Synchronous delivery
	return true
}
