package main

import (
	"os"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/server"
	work "git.sr.ht/~sircmpwn/dowork"

	"git.sr.ht/~sircmpwn/man.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/man.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/man.sr.ht/api/graph/api"
)

func main() {
	appConfig := config.LoadConfig()

	gqlConfig := api.Config{Resolvers: &graph.Resolver{}}
	gqlConfig.Directives.Internal = server.Internal
	schema := api.NewExecutableSchema(gqlConfig)

	// TODO: Populate this after scopes have been added to the schema
	scopes := make([]string, 0)

	queueSize := config.GetInt(appConfig, "man.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)
	accountQueue := work.NewQueue("account", queueSize)

	gsrv := server.New("man.sr.ht", ":5104", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			account.Middleware(accountQueue),
		).
		WithSchema(schema, scopes).
		WithQueues(accountQueue)

	gsrv.Run()
}
