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
	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model"
)

var loadersCtxKey = &contextKey{"loaders"}

type contextKey struct {
	name string
}

type Loaders struct {
	ACLsByID                ACLsByIDLoader
	EmailsByID              EmailsByIDLoader
	EmailsByIDUnsafe        EmailsByIDLoader
	MailingListsByID        MailingListsByIDLoader
	MailingListsByOwnerName MailingListsByOwnerNameLoader
	PatchsetsByID           PatchsetsByIDLoader
	PatchsetsByIDUnsafe     PatchsetsByIDLoader
	SubscriptionsByIDUnsafe SubscriptionsByIDLoader
	ThreadsByIDUnsafe       ThreadsByIDLoader
	UsersByID               UsersByIDLoader
	UsersByName             UsersByNameLoader
}

func fetchACLsByID(ctx context.Context) func(ids []int) ([]*model.MailingListACL, []error) {
	return func(ids []int) ([]*model.MailingListACL, []error) {
		acls := make([]*model.MailingListACL, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.MailingListACL{}).As(`acl`)).
				From(`"access" acl`).
				Where(sq.Expr(`acl.id = ANY(?)`, pq.Array(ids)))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			aclsById := map[int]*model.MailingListACL{}
			for rows.Next() {
				var acl model.MailingListACL
				if err := rows.Scan(database.Scan(ctx, &acl)...); err != nil {
					panic(err)
				}
				aclsById[acl.ID] = &acl
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				acls[i] = aclsById[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return acls, nil
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

func fetchMailingListsByID(ctx context.Context) func(ids []int) ([]*model.MailingList, []error) {
	return func(ids []int) ([]*model.MailingList, []error) {
		lists := make([]*model.MailingList, len(ids))
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
				Select(ctx, (&model.MailingList{}).As(`list`)).
				From(`list`).
				LeftJoin(`access ON
					access.list_id = list.id AND
					access.user_id = ?`, user.UserID).
				LeftJoin(`subscription sub ON
					sub.list_id = list.id AND
					sub.user_id = ?`, user.UserID).
				Column(`COALESCE(
					access.permissions,
					CASE WHEN list.owner_id = ?
						THEN ?
						ELSE list.default_access
					END)`,
					user.UserID, model.ACCESS_ALL).
				Column(`access.id`).
				Column(`sub.id`).
				Where(sq.And{
					sq.Expr(`list.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`list.owner_id = ?`, user.UserID),
						sq.Expr(`list.visibility != 'PRIVATE'`),
						sq.Expr(`access.permissions > 0`),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			listsByID := map[int]*model.MailingList{}
			for rows.Next() {
				list := model.MailingList{}
				if err := rows.Scan(append(
					database.Scan(ctx, &list),
					&list.Access,
					&list.AccessID,
					&list.SubscriptionID,
				)...); err != nil {
					panic(err)
				}
				listsByID[list.ID] = &list
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				lists[i] = listsByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return lists, nil
	}
}

func fetchMailingListsByOwnerName(ctx context.Context) func(names [][2]string) ([]*model.MailingList, []error) {
	return func(names [][2]string) ([]*model.MailingList, []error) {
		lists := make([]*model.MailingList, len(names))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err    error
				rows   *sql.Rows
				_names = make([]string, len(names))
			)
			for i, name := range names {
				// This is a hack, but it works around limitations with
				// PostgreSQL and is guaranteed to work because / is invalid in
				// both usernames and list names
				_names[i] = name[0] + "/" + name[1]
			}

			user := auth.ForContext(ctx)
			query := database.
				Select(ctx).
				Prefix(`WITH user_list AS (
					SELECT
						substring(un for position('/' in un)-1) AS owner,
						substring(un from position('/' in un)+1) AS name
					FROM unnest(?::text[]) un)`, pq.Array(_names)).
				Columns(database.Columns(ctx, (&model.MailingList{}).As(`list`))...).
				Columns(`u.username`).
				Distinct().
				From(`user_list ul`).
				Join(`"user" u on ul.owner = u.username`).
				Join(`list ON ul.name = list.name AND u.id = list.owner_id`).
				LeftJoin(`access ON
					access.list_id = list.id AND
					access.user_id = ?`, user.UserID).
				LeftJoin(`subscription sub ON
					sub.list_id = list.id AND
					sub.user_id = ?`, user.UserID).
				Column(`COALESCE(
					access.permissions,
					CASE WHEN list.owner_id = ?
						THEN ?
						ELSE list.default_access
					END)`,
					user.UserID, model.ACCESS_ALL).
				Column(`access.id`).
				Column(`sub.id`).
				Where(sq.Or{
					sq.Expr(`list.owner_id = ?`, user.UserID),
					sq.Expr(`list.visibility != 'PRIVATE'`),
					sq.Expr(`access.permissions > 0`),
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			listsByOwnerName := map[[2]string]*model.MailingList{}
			for rows.Next() {
				var (
					ownerName string
					list      model.MailingList
				)
				if err := rows.Scan(append(
					database.Scan(ctx, &list),
					&ownerName,
					&list.Access,
					&list.AccessID,
					&list.SubscriptionID)...); err != nil {
					panic(err)
				}
				listsByOwnerName[[2]string{ownerName, list.Name}] = &list
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, name := range names {
				lists[i] = listsByOwnerName[name]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return lists, nil
	}
}

func fetchEmailsByID(ctx context.Context) func(ids []int) ([]*model.Email, []error) {
	return func(ids []int) ([]*model.Email, []error) {
		emails := make([]*model.Email, len(ids))
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
				Select(ctx, (&model.Email{}).As(`email`)).
				From(`email`).
				LeftJoin(`list ON email.list_id = list.id`).
				LeftJoin(`access ON
					access.list_id = list.id AND
					access.user_id = ?`, user.UserID).
				LeftJoin(`subscription sub ON
					sub.list_id = list.id AND
					sub.user_id = ?`, user.UserID).
				Where(sq.And{
					sq.Expr(`email.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`list.owner_id = ?`, user.UserID),
						sq.Expr(`access.permissions & ? > 0`, model.ACCESS_BROWSE),
						sq.Expr(`list.default_access & ? > 0`, model.ACCESS_BROWSE),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			emailsByID := map[int]*model.Email{}
			for rows.Next() {
				var email model.Email
				if err := rows.Scan(database.Scan(ctx, &email)...); err != nil {
					panic(err)
				}
				email.Populate()
				emailsByID[email.ID] = &email
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				emails[i] = emailsByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return emails, nil
	}
}

func fetchEmailsByIDUnsafe(ctx context.Context) func(ids []int) ([]*model.Email, []error) {
	return func(ids []int) ([]*model.Email, []error) {
		emails := make([]*model.Email, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.Email{}).As(`email`)).
				From(`email`).
				Where(`email.id = ANY(?)`, pq.Array(ids))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			emailsByID := map[int]*model.Email{}
			for rows.Next() {
				var email model.Email
				if err := rows.Scan(database.Scan(ctx, &email)...); err != nil {
					panic(err)
				}
				email.Populate()
				emailsByID[email.ID] = &email
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				emails[i] = emailsByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return emails, nil
	}
}

func fetchThreadsByIDUnsafe(ctx context.Context) func(ids []int) ([]*model.Thread, []error) {
	return func(ids []int) ([]*model.Thread, []error) {
		threads := make([]*model.Thread, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.Thread{}).As(`thread`)).
				From(`email thread`).
				Where(`thread.id = ANY(?) AND thread.thread_id IS NULL`, pq.Array(ids))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			threadsByID := map[int]*model.Thread{}
			for rows.Next() {
				var thread model.Thread
				if err := rows.Scan(database.Scan(ctx, &thread)...); err != nil {
					panic(err)
				}
				thread.Populate()
				threadsByID[thread.ID] = &thread
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				threads[i] = threadsByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return threads, nil
	}
}

func fetchPatchsetsByID(ctx context.Context) func(ids []int) ([]*model.Patchset, []error) {
	return func(ids []int) ([]*model.Patchset, []error) {
		patches := make([]*model.Patchset, len(ids))
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
				Select(ctx, (&model.Patchset{}).As(`patch`)).
				From(`patchset patch`).
				LeftJoin(`list ON patch.list_id = list.id`).
				LeftJoin(`access ON
					access.list_id = list.id AND
					access.user_id = ?`, user.UserID).
				LeftJoin(`subscription sub ON
					sub.list_id = list.id AND
					sub.user_id = ?`, user.UserID).
				Where(sq.And{
					sq.Expr(`patch.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`list.owner_id = ?`, user.UserID),
						sq.Expr(`access.permissions & ? > 0`, model.ACCESS_BROWSE),
						sq.Expr(`list.default_access & ? > 0`, model.ACCESS_BROWSE),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			patchesByID := map[int]*model.Patchset{}
			for rows.Next() {
				var patch model.Patchset
				if err := rows.Scan(database.Scan(ctx, &patch)...); err != nil {
					panic(err)
				}
				patchesByID[patch.ID] = &patch
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				patches[i] = patchesByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return patches, nil
	}
}

func fetchPatchsetsByIDUnsafe(ctx context.Context) func(ids []int) ([]*model.Patchset, []error) {
	return func(ids []int) ([]*model.Patchset, []error) {
		patches := make([]*model.Patchset, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.Patchset{}).As(`patch`)).
				From(`patchset patch`).
				Where(sq.Expr(`patch.id = ANY(?)`, pq.Array(ids)))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			patchesByID := map[int]*model.Patchset{}
			for rows.Next() {
				var patch model.Patchset
				if err := rows.Scan(database.Scan(ctx, &patch)...); err != nil {
					panic(err)
				}
				patchesByID[patch.ID] = &patch
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				patches[i] = patchesByID[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return patches, nil
	}
}

func fetchSubscriptionsByIDUnsafe(ctx context.Context) func(ids []int) ([]model.ActivitySubscription, []error) {
	return func(ids []int) ([]model.ActivitySubscription, []error) {
		subs := make([]model.ActivitySubscription, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.MailingListSubscription{}).As(`sub`)).
				From(`subscription sub`).
				Where(sq.And{
					sq.Expr(`sub.id = ANY(?)`, pq.Array(ids)),
					sq.Expr(`sub.user_id = ?`, auth.ForContext(ctx).UserID),
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			subsByID := make(map[int]model.ActivitySubscription)
			for rows.Next() {
				var sub model.MailingListSubscription
				if err := rows.Scan(database.Scan(ctx, &sub)...); err != nil {
					panic(err)
				}
				subsByID[sub.ID] = &sub
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				subs[i] = subsByID[id]
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
			ACLsByID: ACLsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchACLsByID(r.Context()),
			},
			EmailsByID: EmailsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchEmailsByID(r.Context()),
			},
			EmailsByIDUnsafe: EmailsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchEmailsByIDUnsafe(r.Context()),
			},
			MailingListsByID: MailingListsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchMailingListsByID(r.Context()),
			},
			MailingListsByOwnerName: MailingListsByOwnerNameLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchMailingListsByOwnerName(r.Context()),
			},
			PatchsetsByID: PatchsetsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchPatchsetsByID(r.Context()),
			},
			PatchsetsByIDUnsafe: PatchsetsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchPatchsetsByIDUnsafe(r.Context()),
			},
			SubscriptionsByIDUnsafe: SubscriptionsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchSubscriptionsByIDUnsafe(r.Context()),
			},
			ThreadsByIDUnsafe: ThreadsByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchThreadsByIDUnsafe(r.Context()),
			},
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
