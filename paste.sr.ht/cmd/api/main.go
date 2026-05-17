package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/server"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/99designs/gqlgen/graphql"
	"github.com/go-chi/chi/v5"

	"git.sr.ht/~sircmpwn/paste.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/paste.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/paste.sr.ht/api/graph/api"
	"git.sr.ht/~sircmpwn/paste.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/paste.sr.ht/api/loaders"
)

func main() {
	appConfig := config.LoadConfig()

	gqlConfig := api.Config{Resolvers: &graph.Resolver{}}
	gqlConfig.Directives.Private = server.Private
	gqlConfig.Directives.Internal = server.Internal
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

	queueSize := config.GetInt(appConfig, "paste.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)
	accountQueue := work.NewQueue("account", queueSize)
	webhookQueue := webhooks.NewQueue(schema, appConfig)

	gsrv := server.New("paste.sr.ht", ":5111", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			loaders.Middleware,
			account.Middleware(accountQueue),
			webhooks.Middleware(webhookQueue),
		).
		WithSchema(schema, scopes).
		WithQueues(accountQueue, webhookQueue.Queue)

	// Bulk transfer endpoints
	gsrv.Router().Get("/query/blob/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := database.WithTx(r.Context(), &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var data []byte
			row := tx.QueryRowContext(r.Context(), `
				SELECT contents
				FROM blob
				WHERE sha = $1;
			`, id)
			if err := row.Scan(&data); err != nil {
				return err
			}
			_, err := w.Write(data)
			return err
		}); err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Blob not found\r\n"))
				return
			} else {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Blob not found\r\n"))
				return
			}
		}
	})

	gsrv.Run()
}
