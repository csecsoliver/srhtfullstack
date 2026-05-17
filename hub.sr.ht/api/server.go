package main

import (
	"context"
	"os"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/server"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/99designs/gqlgen/graphql"

	"git.sr.ht/~sircmpwn/hub.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/hub.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/hub.sr.ht/api/graph/api"
	"git.sr.ht/~sircmpwn/hub.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/hub.sr.ht/api/loaders"
)

func main() {
	appConfig := config.LoadConfig()

	gqlConfig := api.Config{Resolvers: &graph.Resolver{}}
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

	queueSize := config.GetInt(appConfig, "hub.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)
	accountQueue := work.NewQueue("account", queueSize)

	gsrv := server.New("hub.sr.ht", ":5114", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			loaders.Middleware,
			account.Middleware(accountQueue),
		).
		WithSchema(schema, scopes).
		WithQueues(accountQueue)

	gsrv.Run()
}
