package main

import (
	"context"
	"fmt"
	"io"
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

	"git.sr.ht/~sircmpwn/builds.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/builds.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/builds.sr.ht/api/graph/api"
	"git.sr.ht/~sircmpwn/builds.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/builds.sr.ht/api/loaders"
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
	gqlConfig.Directives.Worker = func(ctx context.Context, obj any,
		next graphql.Resolver) (any, error) {
		return nil, fmt.Errorf("access denied")
	}
	schema := api.NewExecutableSchema(gqlConfig)

	scopes := make([]string, len(model.AllAccessScope))
	for i, s := range model.AllAccessScope {
		scopes[i] = s.String()
	}

	queueSize := config.GetInt(appConfig, "builds.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)
	accountQueue := work.NewQueue("account", queueSize)
	webhookQueue := webhooks.NewQueue(schema, appConfig)

	srv := server.New("builds.sr.ht", ":5102", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			loaders.Middleware,
			account.Middleware(accountQueue),
			webhooks.Middleware(webhookQueue),
		).
		WithSchema(schema, scopes).
		WithQueues(
			accountQueue,
			webhookQueue.Queue,
		)

	srv.Router().Head("/query/log/{job_id}/log", proxyLog)
	srv.Router().Head("/query/log/{job_id}/{task_name}/log", proxyLog)
	srv.Router().Get("/query/log/{job_id}/log", proxyLog)
	srv.Router().Get("/query/log/{job_id}/{task_name}/log", proxyLog)
	srv.Run()
}

func proxyLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jobId, err := strconv.Atoi(chi.URLParam(r, "job_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid job ID\r\n"))
		return
	}
	job, err := loaders.ForContext(ctx).JobsByID.Load(jobId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Unknown build job\r\n"))
		return
	}
	if job.Runner == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("This build job has not been started yet\r\n"))
		return
	}

	var url string
	taskName := chi.URLParam(r, "task_name")
	if taskName == "" {
		url = fmt.Sprintf("http://%s/logs/%d/log", *job.Runner, job.ID)
	} else {
		url = fmt.Sprintf("http://%s/logs/%d/%s/log",
			*job.Runner, job.ID, taskName)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, url, nil)
	if err != nil {
		log.Printf("Error fetching logs: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error\r\n"))
		return
	}

	rrange := r.Header.Get("Range")
	if rrange != "" {
		req.Header.Add("Range", rrange)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Failed to retrieve build log\r\n"))
		return
	}
	defer resp.Body.Close()
	for key, val := range resp.Header {
		for _, val := range val {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error forwarding log: %s", err.Error())
	}
}
