package account

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"

	"git.sr.ht/~sircmpwn/core-go/client"
	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/objects"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
func Delete(ctx context.Context, userID int, username string, reserve bool) {
	queue, ok := ctx.Value(ctxKey).(*work.Queue)
	if !ok {
		panic("No account worker for this context")
	}

	var services []string
	conf := config.ForContext(ctx)
	for key := range conf {
		if !strings.HasSuffix(key, ".sr.ht") || key == "meta.sr.ht" {
			continue
		}
		services = append(services, key)
	}

	avatarsEnabled := true
	bucket, _ := conf.Get("meta.sr.ht", "avatar-bucket")
	prefix, _ := conf.Get("meta.sr.ht", "avatar-prefix")
	sc, err := objects.NewClient(conf)
	if err != nil || bucket == "" || prefix == "" {
		avatarsEnabled = false
	}

	var wg sync.WaitGroup
	task := work.NewTask(func(ctx context.Context) error {
		log.Printf("Processing deletion of user account %d %s", userID, username)
		wg.Wait()

		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			if reserve {
				_, err := tx.ExecContext(ctx, `
					INSERT INTO reserved_usernames (
						username
					) VALUES ($1);
				`, username)
				if err != nil {
					return err
				}
			}

			var avatar *string
			row := tx.QueryRowContext(ctx, `
				DELETE FROM "user" WHERE id = $1
				RETURNING avatar
			`, userID)

			if err := row.Scan(&avatar); err != nil {
				return err
			}

			if avatarsEnabled && avatar != nil {
				filepath := path.Join(prefix, *avatar)
				_, err := sc.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(filepath),
				})
				if err != nil {
					log.Printf("Failed to delete %s: %s",
						filepath, err.Error())
				}
			}

			return nil
		}); err != nil {
			return err
		}

		log.Printf("Deletion of user account %d %s complete", userID, username)
		return nil
	})

	wg.Add(len(services))
	for _, svc := range services {
		svc := svc
		task := work.NewTask(func(ctx context.Context) error {
			log.Printf("Deleting user account %s on service %s",
				username, svc)
			query := client.GraphQLQuery{
				Query: `mutation {
					deleteUser
				}`,
			}
			return client.Do(ctx, username, svc, query, struct{}{})
		}).After(func(ctx context.Context, task *work.Task) {
			wg.Done()
		}).Retries(3)
		queue.Enqueue(task)
	}

	go func() {
		wg.Wait()
		queue.Enqueue(task)
	}()
	log.Printf("Enqueued deletion of user account %d %s", userID, username)
}
