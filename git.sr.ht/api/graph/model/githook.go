package model

import (
	"context"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

const MAX_COMMITS = 50

// This event is used for pre-receive and post-receive git hooks.
//
// If a pre-receive event is configured to be delievered synchronously, the git
// push operation is blocked until the webhook destination server returns a
// response. If the response code is not 200 OK, the push operation is cancelled.
// This allows the webhook user to accept or reject pushes according to
// user-specific policy, for instance enforcing an OWNERS file or denying certain
// users the right to push to certain branches. If a push is rejected, the HTTP
// response content is printed on the pushing user's console.
//
// post-receive events are never processed synchronously and the HTTP response has
// no outcome on the push operation.
type GitEvent struct {
	UUID       string        `json:"uuid"`
	Event      WebhookEvent  `json:"event"`
	Date       time.Time     `json:"date"`
	Repository *Repository   `json:"repository"`
	Pusher     Entity        `json:"pusher"`
	Updates    []*UpdatedRef `json:"updates"`
}

func (GitEvent) IsWebhookPayload() {}

type UpdatedRef struct {
	repo    *Repository
	ref     *plumbing.Reference
	oldHash *plumbing.Hash
	newHash *plumbing.Hash
}

func NewUpdatedRef(
	repo *Repository,
	ref *plumbing.Reference,
	oldHash, newHash *plumbing.Hash) UpdatedRef {
	return UpdatedRef{repo, ref, oldHash, newHash}
}

func (ref *UpdatedRef) Ref() (*Reference, error) {
	if ref.ref == nil {
		return nil, nil
	}
	return &Reference{ref.repo, ref.ref}, nil
}

func (ref *UpdatedRef) Old() (Object, error) {
	if ref.oldHash == nil || ref.oldHash.IsZero() {
		return nil, nil
	}
	return LookupObject(ref.repo.Repo(), *ref.oldHash)
}

func (ref *UpdatedRef) New() (Object, error) {
	if ref.newHash == nil || ref.newHash.IsZero() {
		return nil, nil
	}
	return LookupObject(ref.repo.Repo(), *ref.newHash)
}

func (ref *UpdatedRef) Log(ctx context.Context) (*CommitCursor, error) {
	repo := ref.repo.Repo()
	repo.Lock()
	defer repo.Unlock()

	opts := &git.LogOptions{
		Order: git.LogOrderCommitterTime,
		From:  *ref.newHash,
	}

	log, err := repo.Log(opts)
	if err != nil {
		return nil, err
	}

	// Collect up to 50 commits
	var commits []*Commit
	log.ForEach(func(c *object.Commit) error {
		if c.ID().String() == ref.oldHash.String() ||
			len(commits) == MAX_COMMITS {
			return storer.ErrStop
		}
		commits = append(commits, CommitFromObject(repo, c))
		return nil
	})

	return &CommitCursor{
		Results: commits,
	}, nil
}

func (ref *UpdatedRef) Diff(ctx context.Context) (*string, error) {
	repo := ref.repo.Repo()
	repo.Lock()
	defer repo.Unlock()

	oldObject, err := repo.Object(plumbing.CommitObject, *ref.oldHash)
	if err != nil {
		return nil, err
	}

	newObject, err := repo.Object(plumbing.CommitObject, *ref.newHash)
	if err != nil {
		return nil, err
	}

	oldCommit := oldObject.(*object.Commit)
	newCommit := newObject.(*object.Commit)

	newctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	patch, err := oldCommit.PatchContext(newctx, newCommit)
	cancel()
	if err == context.DeadlineExceeded {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	text := patch.String()
	return &text, nil
}
