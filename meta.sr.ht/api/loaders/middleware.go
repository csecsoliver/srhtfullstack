package loaders

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

var loadersCtxKey = &contextKey{"loaders"}

type contextKey struct {
	name string
}

type Loaders struct {
	UsersByID    UsersByIDLoader
	UsersByName  UsersByNameLoader
	UsersByEmail UsersByEmailLoader

	OAuthClientsByID   OAuthClientsByIDLoader
	OAuthClientsByUUID OAuthClientsByUUIDLoader

	SubscriptionsByID     SubscriptionsByIDLoader
	SubscriptionsByUserID SubscriptionsByUserIDLoader
	SubscriptionsByIntent SubscriptionsByIntentLoader
}

func fetchUsersByID(ctx context.Context) func(ids []int) ([]*model.User, []error) {
	return func(ids []int) ([]*model.User, []error) {
		users := make([]*model.User, len(ids))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.User{}).As(`u`)).
				From(`"user" u`).
				Where(sq.Expr(`u.id = ANY(?)`, pq.Array(ids)))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			usersById := map[int]*model.User{}
			for rows.Next() {
				var user model.User
				if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
					return err
				}
				usersById[user.ID] = &user
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				users[i] = usersById[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}

		return users, nil
	}
}

func fetchUsersByName(ctx context.Context) func(names []string) ([]*model.User, []error) {
	return func(names []string) ([]*model.User, []error) {
		users := make([]*model.User, len(names))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.User{}).As(`u`)).
				From(`"user" u`).
				Where(sq.Expr(`u.username = ANY(?)`, pq.Array(names)))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			usersByName := map[string]*model.User{}
			for rows.Next() {
				user := model.User{}
				if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
					return err
				}
				usersByName[user.Username] = &user
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, name := range names {
				users[i] = usersByName[name]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return users, nil
	}
}

func fetchUsersByEmail(ctx context.Context) func(emails []string) ([]*model.User, []error) {
	return func(emails []string) ([]*model.User, []error) {
		users := make([]*model.User, len(emails))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.User{}).As(`u`)).
				From(`"user" u`).
				Where(sq.Expr(`u.email = ANY(?)`, pq.Array(emails)))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			usersByEmail := map[string]*model.User{}
			for rows.Next() {
				user := model.User{}
				if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
					return err
				}
				usersByEmail[user.Email] = &user
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, email := range emails {
				users[i] = usersByEmail[email]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return users, nil
	}
}

func fetchOAuthClientsByID(ctx context.Context) func(ids []int) ([]*model.OAuthClient, []error) {
	return func(ids []int) ([]*model.OAuthClient, []error) {
		clients := make([]*model.OAuthClient, len(ids))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.OAuthClient{}).As(`c`)).
				From(`oauth2_client c`).
				Where(sq.Expr(`c.id = ANY(?)`, pq.Array(ids))).
				Where(`c.revoked = false`)
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			clientsById := map[int]*model.OAuthClient{}
			for rows.Next() {
				var client model.OAuthClient
				if err := rows.Scan(database.Scan(ctx, &client)...); err != nil {
					return err
				}
				clientsById[client.ID] = &client
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				clients[i] = clientsById[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}

		return clients, nil
	}
}

func fetchOAuthClientsByUUID(ctx context.Context) func(uuids []string) ([]*model.OAuthClient, []error) {
	return func(uuids []string) ([]*model.OAuthClient, []error) {
		clients := make([]*model.OAuthClient, len(uuids))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.OAuthClient{}).As(`c`)).
				From(`oauth2_client c`).
				Where(sq.Expr(`c.client_uuid = ANY(?)`, pq.Array(uuids))).
				Where(`c.revoked = false`)
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			clientsByUUID := map[string]*model.OAuthClient{}
			for rows.Next() {
				var client model.OAuthClient
				if err := rows.Scan(database.Scan(ctx, &client)...); err != nil {
					return err
				}
				clientsByUUID[client.UUID] = &client
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, uuid := range uuids {
				clients[i] = clientsByUUID[uuid]
			}
			return nil
		}); err != nil {
			return nil, []error{err}
		}

		return clients, nil
	}
}

func fetchSubscriptionsByID(ctx context.Context) func(ids []int) ([]*model.BillingSubscription, []error) {
	return func(ids []int) ([]*model.BillingSubscription, []error) {
		subs := make([]*model.BillingSubscription, len(ids))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
				SELECT
					id,
					user_id,
					product_id,
					created,
					updated,
					status,
					currency,
					interval,
					autorenew,
					payment_intent,
					payment_outcome,
					payment_error
				FROM subscription
				WHERE id = ANY($1)
			`, pq.Array(ids))
			if err != nil {
				return err
			}

			defer rows.Close()

			subsById := map[int]*model.BillingSubscription{}
			for rows.Next() {
				var sub model.BillingSubscription
				if err := rows.Scan(
					&sub.ID,
					&sub.UserID,
					&sub.ProductID,
					&sub.Created,
					&sub.Updated,
					&sub.Status,
					&sub.Currency,
					&sub.Interval,
					&sub.Autorenew,
					&sub.IntentID,
					&sub.Payment.Status,
					&sub.Payment.Error,
				); err != nil {
					return err
				}
				subsById[sub.ID] = &sub
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				subs[i] = subsById[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}

		return subs, nil
	}
}

func fetchSubscriptionsByUserID(ctx context.Context) func(userIDs []int) ([]*model.BillingSubscription, []error) {
	return func(userIDs []int) ([]*model.BillingSubscription, []error) {
		subs := make([]*model.BillingSubscription, len(userIDs))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
				SELECT
					id,
					user_id,
					product_id,
					created,
					updated,
					status,
					currency,
					interval,
					autorenew,
					payment_intent,
					payment_outcome,
					payment_error
				FROM subscription
				WHERE
					user_id = ANY($1) AND
					status IN ('ACTIVE', 'SETTLEMENT')
			`, pq.Array(userIDs))
			if err != nil {
				return err
			}

			defer rows.Close()

			subsByUserID := map[int]*model.BillingSubscription{}
			for rows.Next() {
				var sub model.BillingSubscription
				if err := rows.Scan(
					&sub.ID,
					&sub.UserID,
					&sub.ProductID,
					&sub.Created,
					&sub.Updated,
					&sub.Status,
					&sub.Currency,
					&sub.Interval,
					&sub.Autorenew,
					&sub.IntentID,
					&sub.Payment.Status,
					&sub.Payment.Error,
				); err != nil {
					return err
				}
				subsByUserID[sub.UserID] = &sub
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range userIDs {
				subs[i] = subsByUserID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}

		return subs, nil
	}
}

func fetchSubscriptionsByIntent(ctx context.Context) func(intents []string) ([]*model.BillingSubscription, []error) {
	return func(intents []string) ([]*model.BillingSubscription, []error) {
		subs := make([]*model.BillingSubscription, len(intents))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
				SELECT
					id,
					user_id,
					product_id,
					created,
					updated,
					status,
					currency,
					interval,
					autorenew,
					payment_intent,
					payment_outcome,
					payment_error
				FROM subscription
				WHERE payment_intent = ANY($1)
			`, pq.Array(intents))
			if err != nil {
				return err
			}

			defer rows.Close()

			subsByIntent := map[string]*model.BillingSubscription{}
			for rows.Next() {
				var sub model.BillingSubscription
				if err := rows.Scan(
					&sub.ID,
					&sub.UserID,
					&sub.ProductID,
					&sub.Created,
					&sub.Updated,
					&sub.Status,
					&sub.Currency,
					&sub.Interval,
					&sub.Autorenew,
					&sub.IntentID,
					&sub.Payment.Status,
					&sub.Payment.Error,
				); err != nil {
					return err
				}
				subsByIntent[*sub.IntentID] = &sub
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range intents {
				subs[i] = subsByIntent[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}

		return subs, nil
	}
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), loadersCtxKey, &Loaders{
			UsersByID: UsersByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchUsersByID(r.Context()),
			},
			UsersByName: UsersByNameLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchUsersByName(r.Context()),
			},
			UsersByEmail: UsersByEmailLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchUsersByEmail(r.Context()),
			},
			OAuthClientsByID: OAuthClientsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchOAuthClientsByID(r.Context()),
			},
			OAuthClientsByUUID: OAuthClientsByUUIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchOAuthClientsByUUID(r.Context()),
			},
			SubscriptionsByID: SubscriptionsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchSubscriptionsByID(r.Context()),
			},
			SubscriptionsByUserID: SubscriptionsByUserIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchSubscriptionsByUserID(r.Context()),
			},
			SubscriptionsByIntent: SubscriptionsByIntentLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchSubscriptionsByIntent(r.Context()),
			},
		})
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func ForContext(ctx context.Context) *Loaders {
	raw, ok := ctx.Value(loadersCtxKey).(*Loaders)
	if !ok {
		panic(errors.New("invalid data loaders context"))
	}
	return raw
}
