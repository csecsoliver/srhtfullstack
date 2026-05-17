package account

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"git.sr.ht/~sircmpwn/core-go/database"
	work "git.sr.ht/~sircmpwn/dowork"
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
		panic("No account worker for this context")
	}

	task := work.NewTask(func(ctx context.Context) error {
		log.Printf("Processing deletion of user account %d %s", userID, username)

		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				UPDATE participant
				SET
					participant_type = 'email',
					user_id = null,
					email = u.email,
					email_name = '~' || u.username
				FROM "user" u
				WHERE u.id = $1 AND participant.user_id = $1
			`, userID)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
				DELETE FROM "user" WHERE id = $1;
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
