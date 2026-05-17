package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/server"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/99designs/gqlgen/graphql"
	"github.com/go-chi/chi/v5"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/billing"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/api"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/loaders"
)

func main() {
	appConfig := config.LoadConfig()

	gqlConfig := api.Config{Resolvers: &graph.Resolver{}}
	gqlConfig.Directives.Admin = server.Admin
	gqlConfig.Directives.Anoninternal = server.AnonInternal
	gqlConfig.Directives.Internal = server.Internal
	gqlConfig.Directives.Private = server.Private
	gqlConfig.Directives.Access = func(ctx context.Context, obj interface{},
		next graphql.Resolver, scope model.AccessScope,
		kind model.AccessKind) (interface{}, error) {
		return server.Access(ctx, obj, next, scope.String(), kind.String())
	}
	schema := api.NewExecutableSchema(gqlConfig)

	scopes := make([]string, len(model.AllAccessScope))
	for i, s := range model.AllAccessScope {
		scopes[i] = s.String()
	}

	queueSize := config.GetInt(appConfig, "meta.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)

	accountQueue := work.NewQueue("account", queueSize)
	webhookQueue := webhooks.NewQueue(schema, appConfig)
	legacyWebhooks := webhooks.NewLegacyQueue(appConfig)

	srv := server.New("meta.sr.ht", ":5100", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			loaders.Middleware,
			account.Middleware(accountQueue),
			webhooks.Middleware(webhookQueue),
			webhooks.LegacyMiddleware(legacyWebhooks),
		)

	if billing.IsEnabled(appConfig) {
		billing.Init(appConfig)
		srv = srv.WithMiddleware(billing.Middleware(appConfig))
		srv.Router().Post("/query/invoice/{id}", GetInvoiceByID)
	}

	srv = srv.
		WithSchema(schema, scopes).
		WithQueues(
			accountQueue,
			webhookQueue.Queue,
			legacyWebhooks.Queue,
		)

	srv.Run()
}

func GetInvoiceByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		log.Printf("Invalid invoice ID: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid invoice ID\r\n"))
		return
	}

	user := auth.ForContext(r.Context())

	var invoice model.Invoice
	if err := database.WithTx(r.Context(), &sql.TxOptions{
		Isolation: 0,
		ReadOnly:  true,
	}, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(r.Context(), `
			SELECT
				id,
				uuid,
				invoice_no,
				user_id
			FROM invoice
			WHERE id = $1
		`, id)

		return row.Scan(
			&invoice.ID,
			&invoice.UUID,
			&invoice.InternalInvoiceNo,
			&invoice.UserID,
		)
	}); err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not found"))
			return
		}
		panic(err)
	}

	if invoice.UserID != user.UserID &&
		user.UserType != auth.USER_TYPE_ADMIN {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))
		return
	}

	rd, err := billing.DownloadInvoice(r.Context(), &invoice)
	if err != nil {
		log.Printf("Failed to download invoice: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
		return
	}
	defer rd.Close()

	w.Header().Add("Content-Type", "application/pdf")
	w.Header().Add("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"invoice_%03d-%004d.pdf\"",
			invoice.UserID, invoice.InternalInvoiceNo))
	_, err = io.Copy(w, rd)
	if err != nil {
		log.Printf("Failed to write response: %s", err.Error())
	}
}
