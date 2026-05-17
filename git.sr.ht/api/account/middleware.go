package account

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"path"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	work "git.sr.ht/~sircmpwn/dowork"
	"git.sr.ht/~sircmpwn/git.sr.ht/api/repos"
)

type contextKey struct {
	name string
}

var ctxKey = &contextKey{"account"}

func Middleware(queue *work.Queue) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKey, queue)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// Schedules a user account deletion.
func Delete(ctx context.Context, userID int, username string) {
	queue, ok := ctx.Value(ctxKey).(*work.Queue)
	if !ok {
		panic(errors.New("no account worker for this context"))
	}

	type Artifact struct {
		Filename string
		RepoName string
	}

	conf := config.ForContext(ctx)
	repoStore, ok := conf.Get("git.sr.ht", "repos")
	if !ok || repoStore == "" {
		panic(errors.New("configuration error: [git.sr.ht]repos is unset"))
	}

	task := work.NewTask(func(ctx context.Context) error {
		log.Printf("Processing deletion of user account %d %s", userID, username)
		var artifacts []Artifact
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
				SELECT r.name, a.filename
				FROM artifacts a
				JOIN repository r ON a.repo_id = r.id
				WHERE a.user_id = $1
			`, userID)
			if err != nil {
				return err
			}

			for rows.Next() {
				var (
					filename string
					repoName string
				)
				if err := rows.Scan(&repoName, &filename); err != nil {
					return err
				}
				artifacts = append(artifacts, Artifact{
					Filename: filename,
					RepoName: repoName,
				})
			}
			return rows.Err()
		}); err != nil {
			return err
		}

		for _, art := range artifacts {
			repos.DeleteArtifactsBlocking(ctx, username,
				art.RepoName, []string{art.Filename})
		}
		userPath := path.Join(repoStore, "~"+username)
		if err := os.RemoveAll(userPath); err != nil {
			log.Printf("Failed to remove %s: %s", userPath, err.Error())
		}

		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				DELETE FROM "user" WHERE id = $1
			`, userID)
			return err
		}); err != nil {
			return err
		}

		log.Printf("Deletion of user account %d %s complete", userID, username)
		return nil
	})
	queue.Enqueue(task)
	log.Printf("Enqueued deletion of user account %d %s", userID, username)
}
