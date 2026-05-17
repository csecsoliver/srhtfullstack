package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/server"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	work "git.sr.ht/~sircmpwn/dowork"
	"github.com/99designs/gqlgen/graphql"
	"github.com/emersion/go-mbox"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/go-chi/chi/v5"

	"git.sr.ht/~sircmpwn/lists.sr.ht/api/account"
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph"
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/api"
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/lists"
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/loaders"
)

func main() {
	appConfig := config.LoadConfig()

	gqlConfig := api.Config{Resolvers: &graph.Resolver{}}
	gqlConfig.Directives.Private = server.Private
	gqlConfig.Directives.Anoninternal = server.AnonInternal
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

	queueSize := config.GetInt(appConfig, "lists.sr.ht::api",
		"account-del-queue-size", config.DefaultQueueSize)
	accountQueue := work.NewQueue("account", queueSize)
	queueSize = config.GetInt(appConfig, "lists.sr.ht::api",
		"spool-import-queue-size", config.DefaultQueueSize)
	listsQueue := work.NewQueue("lists", queueSize)
	webhookQueue := webhooks.NewQueue(schema, appConfig)
	legacyWebhooks := webhooks.NewLegacyQueue(appConfig)

	gsrv := server.New("lists.sr.ht", ":5106", appConfig, os.Args).
		WithDefaultMiddleware().
		WithMiddleware(
			loaders.Middleware,
			lists.Middleware(listsQueue),
			account.Middleware(accountQueue),
			webhooks.Middleware(webhookQueue),
			webhooks.LegacyMiddleware(legacyWebhooks),
		).
		WithQueues(
			listsQueue,
			accountQueue,
			webhookQueue.Queue,
			legacyWebhooks.Queue,
		).
		WithSchema(schema, scopes)

	// Bulk transfer endpoints
	gsrv.Router().Get("/query/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid mail ID\r\n"))
			return
		}
		mail, err := loaders.ForContext(r.Context()).EmailsByID.Load(id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Unknown email\r\n"))
			return
		}
		w.Header().Add("Content-Type", "message/rfc822")
		w.Write([]byte(mail.RawMessage))
	})

	gsrv.Router().Get("/query/thread/{id}.mbox", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid thread ID\r\n"))
			return
		}

		if err := database.WithTx(r.Context(), &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(r.Context(), `
				SELECT email.raw_message, email.created, patchset.status
				FROM email
				JOIN list ON list.id = email.list_id
				LEFT JOIN access ON access.user_id = $2 AND access.list_id = list.id
				LEFT JOIN patchset ON email.patchset_id = patchset.id
				WHERE email.id = $1 OR email.thread_id = $1 AND (
					list.owner_id = $2 OR
					access.permissions & $3 > 0 OR
					list.default_access & $3 > 0)
				ORDER BY email.id
			`, id, auth.ForContext(r.Context()).UserID, model.ACCESS_BROWSE)
			if err != nil {
				return err
			}
			return prepMbox(rows, w, false)
		}); err != nil {
			panic(err)
		}
	})

	gsrv.Router().Get("/query/patchset/{id}.mbox", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid patchset ID\r\n"))
			return
		}

		if err := database.WithTx(r.Context(), &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(r.Context(), `
				SELECT email.raw_message, email.created, patchset.status
				FROM email
				JOIN list ON list.id = email.list_id
				LEFT JOIN access ON access.user_id = $2 AND access.list_id = list.id
				LEFT JOIN patchset ON email.patchset_id = patchset.id
				WHERE email.patchset_id = $1 AND email.is_patch AND (
					list.owner_id = $2 OR
					access.permissions & $3 > 0 OR
					list.default_access & $3 > 0)
				ORDER BY email.patch_index, email.id
			`, id, auth.ForContext(r.Context()).UserID, model.ACCESS_BROWSE)
			if err != nil {
				return err
			}
			return prepMbox(rows, w, false)
		}); err != nil {
			panic(err)
		}
	})

	gsrv.Router().Get("/query/list/{id}.mbox", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid mailing list ID\r\n"))
			return
		}

		var since time.Time
		if val, ok := r.URL.Query()["since"]; ok {
			days, err := strconv.Atoi(val[0])
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Invalid since days\r\n"))
				return
			}
			since = time.Now().UTC().Add(-(time.Hour * 24 * time.Duration(days)))
		}

		if err := database.WithTx(r.Context(), &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(r.Context(), `
				SELECT email.raw_message, email.created, patchset.status
				FROM email
				JOIN list ON list.id = email.list_id
				LEFT JOIN access ON access.user_id = $3 AND access.list_id = list.id
				LEFT JOIN patchset ON email.patchset_id = patchset.id
				WHERE email.list_id = $1 AND email.created >= $2 AND (
					list.owner_id = $3 OR
					access.permissions & $4 > 0 OR
					list.default_access & $4 > 0)
				ORDER BY email.created
			`, id, since, auth.ForContext(r.Context()).UserID, model.ACCESS_BROWSE)
			if err != nil {
				return err
			}
			return prepMbox(rows, w, true)
		}); err != nil {
			panic(err)
		}
	})

	gsrv.Run()
}

func prepMbox(rows *sql.Rows, w http.ResponseWriter, allowEmpty bool) error {
	mbw := mbox.NewWriter(w)
	defer mbw.Close()

	var results bool
	for rows.Next() {
		results = true
		w.Header().Add("Content-Type", "application/mbox")

		var (
			rawMessage []byte
			created    time.Time
			status     sql.NullString
		)
		if err := rows.Scan(&rawMessage, &created, &status); err != nil {
			return err
		}

		reader, err := mail.CreateReader(bytes.NewBuffer(rawMessage))
		if err != nil {
			return err
		}
		from, err := reader.Header.AddressList("From")
		reader.Close()
		if err != nil {
			from = []*mail.Address{&mail.Address{Name: "unknown", Address: "unknown@example.org"}}
		}
		sink, err := mbw.CreateMessage(from[0].Address, created)
		if err != nil {
			return err
		}
		if status.Valid {
			if _, err := fmt.Fprintf(sink, "X-Sourcehut-Patchset-Final: %s\r\n", status.String); err != nil {
				return err
			}
		}
		if _, err = sink.Write([]byte(rawMessage)); err != nil {
			return err
		}
	}

	if !results {
		if allowEmpty {
			w.WriteHeader(http.StatusOK)
			w.Header().Add("Content-Type", "application/mbox")
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not found\r\n"))
		}
	}

	return nil
}
