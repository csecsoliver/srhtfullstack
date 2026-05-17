package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/server"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/99designs/gqlgen/graphql"
	"github.com/go-chi/chi/v5"

	"git.sr.ht/~sircmpwn/todo.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/api"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/loaders"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/trackers"
)

func main() {
	appConfig := config.LoadConfig()

	gqlConfig := api.Config{Resolvers: &graph.Resolver{}}
	gqlConfig.Directives.Internal = server.Internal
	gqlConfig.Directives.Private = server.Private
	gqlConfig.Directives.Access = func(ctx context.Context, obj any,
		next graphql.Resolver, scope model.AccessScope,
		kind model.AccessKind) (any, error) {
		return server.Access(ctx, obj, next, scope.String(), kind.String())
	}
	schema := api.NewExecutableSchema(gqlConfig)

	scopes := make([]string, len(model.AllAccessScope))
	for i, s := range model.AllAccessScope {
		scopes[i] = s.String()
	}

	queueSize := config.GetInt(appConfig, "todo.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)
	accountQueue := work.NewQueue("account", queueSize)

	queueSize = config.GetInt(appConfig, "todo.sr.ht::api",
		"tracker-import-queue-size", config.DefaultQueueSize)
	trackersQueue := work.NewQueue("trackers", queueSize)

	webhookQueue := webhooks.NewQueue(schema, appConfig)

	gsrv := server.New("todo.sr.ht", ":5103", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			loaders.Middleware,
			account.Middleware(accountQueue),
			trackers.Middleware(trackersQueue),
			webhooks.Middleware(webhookQueue),
		).
		WithSchema(schema, scopes).
		WithQueues(
			accountQueue,
			trackersQueue,
			webhookQueue.Queue,
		)

	gsrv.Router().Get("/query/tracker/{id}.json.gz", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid tracker ID\r\n"))
			return
		}

		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", `attachment; filename="tracker.json.gz"`)
		if err := trackers.ExportDump(r.Context(), id, w); err != nil {
			log.Printf("Tracker export failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	gsrv.Run()
}
