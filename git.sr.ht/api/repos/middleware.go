package repos

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"path"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/objects"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-git/go-git/v5"
)

type contextKey struct {
	name string
}

var ctxKey = &contextKey{"repos"}

func Middleware(queue *work.Queue) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKey, queue)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

type CloneStatus string

const (
	CloneNone       CloneStatus = "NONE"
	CloneInProgress CloneStatus = "IN_PROGRESS"
	CloneComplete   CloneStatus = "COMPLETE"
	CloneError      CloneStatus = "ERROR"
)

// Schedules a clone.
func Clone(ctx context.Context, repoID int, repo *git.Repository, cloneURL string) {
	queue, ok := ctx.Value(ctxKey).(*work.Queue)
	if !ok {
		panic("No repos worker for this context")
	}
	task := work.NewTask(func(ctx context.Context) error {
		log.Printf("Processing clone of %s", cloneURL)
		cloneCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		err := repo.Clone(cloneCtx, &git.CloneOptions{
			URL:               cloneURL,
			RecurseSubmodules: git.NoRecurseSubmodules,
		})
		cloneStatus := CloneComplete
		var cloneError sql.NullString
		if err != nil {
			cloneStatus = CloneError
			cloneError.String = err.Error()
			cloneError.Valid = true
		}
		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			_, err := tx.Exec(`
				UPDATE repository
				SET clone_status = $2, clone_error = $3
				WHERE id = $1;`, repoID, cloneStatus, cloneError)
			return err
		}); err != nil {
			panic(err)
		}
		log.Printf("Clone %s complete", cloneURL)
		return nil
	})
	queue.Enqueue(task)
	log.Printf("Enqueued clone of %s", cloneURL)
}

// Schedules deletion of artifacts.
func DeleteArtifacts(ctx context.Context, username, repoName string, filenames []string) {
	queue, ok := ctx.Value(ctxKey).(*work.Queue)
	if !ok {
		panic("No repos worker for this context")
	}
	task := work.NewTask(func(ctx context.Context) error {
		return DeleteArtifactsBlocking(ctx, username, repoName, filenames)
	})
	queue.Enqueue(task)
	log.Printf("Enqueued deletion of %d artifacts", len(filenames))
}

func DeleteArtifactsBlocking(
	ctx context.Context,
	username,
	repoName string,
	filenames []string,
) error {
	conf := config.ForContext(ctx)

	sc, err := objects.NewClient(conf)
	if err != nil {
		return err
	}

	bucket, _ := conf.Get("git.sr.ht", "s3-bucket")
	prefix, _ := conf.Get("git.sr.ht", "s3-prefix")

	for _, filename := range filenames {
		s3key := path.Join(prefix, "artifacts", "~"+username, repoName, filename)
		_, err := sc.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(s3key),
		})
		if err != nil {
			return err
		}
	}
	return nil
}
