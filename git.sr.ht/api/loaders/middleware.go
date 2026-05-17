package loaders

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/git.sr.ht/api/graph/model"
)

var loadersCtxKey = &contextKey{"loaders"}

type contextKey struct {
	name string
}

type Loaders struct {
	UsersByID                     UsersByIDLoader
	UsersByName                   UsersByNameLoader
	RepositoriesByID              RepositoriesByIDLoader
	RepositoriesByOwnerRepoName   RepositoriesByOwnerRepoNameLoader
	RepositoriesByOwnerIDRepoName RepositoriesByOwnerIDRepoNameLoader
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

func fetchRepositoriesByID(ctx context.Context) func(ids []int) ([]*model.Repository, []error) {
	return func(ids []int) ([]*model.Repository, []error) {
		repos := make([]*model.Repository, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			auser := auth.ForContext(ctx)
			query := database.
				Select(ctx, (&model.Repository{}).As(`repo`)).
				Distinct().
				From(`repository repo`).
				LeftJoin(`access ON repo.id = access.repo_id`).
				Where(sq.And{
					sq.Expr(`repo.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`? IN (access.user_id, repo.owner_id)`, auser.UserID),
						sq.Expr(`repo.visibility != 'PRIVATE'`),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			reposById := map[int]*model.Repository{}
			for rows.Next() {
				repo := model.Repository{}
				if err := rows.Scan(database.Scan(ctx, &repo)...); err != nil {
					panic(err)
				}
				reposById[repo.ID] = &repo
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, id := range ids {
				repos[i] = reposById[id]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return repos, nil
	}
}

type OwnerRepoName struct {
	Owner    string
	RepoName string
}

func (or OwnerRepoName) Value() (driver.Value, error) {
	return fmt.Sprintf("(%q,%q)", or.Owner, or.RepoName), nil
}

func fetchRepositoriesByOwnerRepoName(ctx context.Context) func([]OwnerRepoName) ([]*model.Repository, []error) {
	return func(ownerRepoNames []OwnerRepoName) ([]*model.Repository, []error) {
		repos := make([]*model.Repository, len(ownerRepoNames))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx).
				Prefix(`WITH owner_repo_names AS (
					SELECT owner, repo_name
					FROM unnest(?::owner_repo_name[]))`, pq.GenericArray{A: ownerRepoNames}).
				Columns(database.Columns(ctx, (&model.Repository{}).As(`repo`))...).
				Columns(`o.owner`).
				Distinct().
				From(`owner_repo_names o`).
				Join(`"user" u on o.owner = u.username`).
				Join(`repository repo ON o.repo_name = repo.name
					AND u.id = repo.owner_id`).
				LeftJoin(`access ON repo.id = access.repo_id`).
				Where(sq.Or{
					sq.Expr(`? IN (access.user_id, repo.owner_id)`,
						auth.ForContext(ctx).UserID),
					sq.Expr(`repo.visibility != 'PRIVATE'`),
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			reposByOwnerRepoName := map[OwnerRepoName]*model.Repository{}
			for rows.Next() {
				var ownerName string
				repo := model.Repository{}
				if err := rows.Scan(append(
					database.Scan(ctx, &repo), &ownerName)...); err != nil {
					panic(err)
				}
				reposByOwnerRepoName[OwnerRepoName{ownerName, repo.Name}] = &repo
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, or := range ownerRepoNames {
				repos[i] = reposByOwnerRepoName[or]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return repos, nil
	}
}

type OwnerIDRepoName struct {
	OwnerID  int
	RepoName string
}

func (or OwnerIDRepoName) Value() (driver.Value, error) {
	return fmt.Sprintf("(%d,%q)", or.OwnerID, or.RepoName), nil
}

func fetchRepositoriesByOwnerIDRepoName(ctx context.Context) func([]OwnerIDRepoName) ([]*model.Repository, []error) {
	return func(ownerIDRepoNames []OwnerIDRepoName) ([]*model.Repository, []error) {
		repos := make([]*model.Repository, len(ownerIDRepoNames))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx).
				Prefix(`WITH owner_id_repo_names AS (
					SELECT owner_id, repo_name
					FROM unnest(?::owner_id_repo_name[]))`, pq.GenericArray{A: ownerIDRepoNames}).
				Columns(database.Columns(ctx, (&model.Repository{}).As(`repo`))...).
				Columns(`o.owner_id`).
				Distinct().
				From(`owner_id_repo_names o`).
				Join(`repository repo ON o.repo_name = repo.name
					AND o.owner_id = repo.owner_id`).
				LeftJoin(`access ON repo.id = access.repo_id`).
				Where(sq.Or{
					sq.Expr(`? IN (access.user_id, repo.owner_id)`,
						auth.ForContext(ctx).UserID),
					sq.Expr(`repo.visibility != 'PRIVATE'`),
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				panic(err)
			}
			defer rows.Close()

			reposByOwnerIDRepoName := map[OwnerIDRepoName]*model.Repository{}
			for rows.Next() {
				var ownerID int
				repo := model.Repository{}
				if err := rows.Scan(append(
					database.Scan(ctx, &repo), &ownerID)...); err != nil {
					panic(err)
				}
				reposByOwnerIDRepoName[OwnerIDRepoName{ownerID, repo.Name}] = &repo
			}
			if err = rows.Err(); err != nil {
				panic(err)
			}

			for i, or := range ownerIDRepoNames {
				repos[i] = reposByOwnerIDRepoName[or]
			}
			return nil
		}); err != nil {
			panic(err)
		}
		return repos, nil
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
			RepositoriesByID: RepositoriesByIDLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchRepositoriesByID(r.Context()),
			},
			RepositoriesByOwnerRepoName: RepositoriesByOwnerRepoNameLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchRepositoriesByOwnerRepoName(r.Context()),
			},
			RepositoriesByOwnerIDRepoName: RepositoriesByOwnerIDRepoNameLoader{
				maxBatch: 100,
				wait:     1 * time.Millisecond,
				fetch:    fetchRepositoriesByOwnerIDRepoName(r.Context()),
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
