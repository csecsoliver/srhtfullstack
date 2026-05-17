package trackers

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	work "git.sr.ht/~sircmpwn/dowork"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/loaders"
)

type contextKey struct {
	name string
}

var ctxKey = &contextKey{"imports"}

func Middleware(queue *work.Queue) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxKey, queue)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// Schedules a tracker import.
func ImportTrackerDump(ctx context.Context, trackerID int, dump io.Reader) {
	queue, ok := ctx.Value(ctxKey).(*work.Queue)
	if !ok {
		panic("No imports worker for this context")
	}
	cfg := config.ForContext(ctx)
	ourUpstream := config.GetOrigin(cfg, "todo.sr.ht", true)
	task := work.NewTask(func(ctx context.Context) error {
		importCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		importCtx = loaders.Context(importCtx)
		err := importTrackerDump(importCtx, trackerID, dump, ourUpstream)
		return err
	})
	queue.Enqueue(task)
	log.Printf("Enqueued tracker import for tracker %d", trackerID)
}
