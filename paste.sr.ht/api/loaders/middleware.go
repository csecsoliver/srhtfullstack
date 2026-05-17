package loaders

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/paste.sr.ht/api/graph/model"
)

var loadersCtxKey = &contextKey{"loaders"}

type contextKey struct {
	name string
}

type Loaders struct {
	BlobsByID   BlobsByIDLoader
	PastesBySHA PastesBySHALoader
	UsersByID   UsersByIDLoader
	UsersByName UsersByNameLoader
}

func fetchBlobsByID(ctx context.Context) func(ids []int) ([]*model.Blob, []error) {
	return func(ids []int) ([]*model.Blob, []error) {
		blobs := make([]*model.Blob, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, `
				SELECT id, sha FROM blob WHERE id = ANY($1);
			`, pq.Array(ids))
			if err != nil {
				return err
			}
			defer rows.Close()

			blobsByID := map[int]*model.Blob{}
			for rows.Next() {
				var blob model.Blob
				if err := rows.Scan(&blob.ID, &blob.SHA); err != nil {
					panic(err)
				}
				blobsByID[blob.ID] = &blob
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				blobs[i] = blobsByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return blobs, nil
	}
}

func fetchPastesBySHA(ctx context.Context) func(shas []string) ([]*model.Paste, []error) {
	return func(shas []string) ([]*model.Paste, []error) {
		pastes := make([]*model.Paste, len(shas))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			user := auth.ForContext(ctx)
			query := database.
				Select(ctx, (&model.Paste{}).As(`paste`)).
				From(`paste`).
				Where(sq.And{
					sq.Expr(`paste.sha = ANY(?)`, pq.Array(shas)),
					sq.Or{
						sq.Expr(`paste.user_id = ?`, user.UserID),
						sq.Expr(`paste.visibility != 'PRIVATE'`),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			pastesBySHA := map[string]*model.Paste{}
			for rows.Next() {
				var paste model.Paste
				if err := rows.Scan(database.Scan(ctx, &paste)...); err != nil {
					panic(err)
				}
				pastesBySHA[paste.ID] = &paste
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, sha := range shas {
				pastes[i] = pastesBySHA[sha]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return pastes, nil
	}
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
				panic(err)
			}
			defer rows.Close()

			usersById := map[int]*model.User{}
			for rows.Next() {
				var user model.User
				if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
					panic(err)
				}
				usersById[user.ID] = &user
			}
			if err = rows.Err(); err != nil {
				panic(err)
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
				panic(err)
			}
			defer rows.Close()

			usersByName := map[string]*model.User{}
			for rows.Next() {
				user := model.User{}
				if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
					panic(err)
				}
				usersByName[user.Username] = &user
			}
			if err = rows.Err(); err != nil {
				panic(err)
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

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), loadersCtxKey, &Loaders{
			BlobsByID: BlobsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchBlobsByID(r.Context()),
			},
			PastesBySHA: PastesBySHALoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchPastesBySHA(r.Context()),
			},
			UsersByName: UsersByNameLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchUsersByName(r.Context()),
			},
			UsersByID: UsersByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchUsersByID(r.Context()),
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
