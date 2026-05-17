package loaders

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"text/template"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/client"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
)

var loadersCtxKey = &contextKey{"loaders"}

type contextKey struct {
	name string
}

type Loaders struct {
	EntitiesByParticipantID EntitiesByParticipantIDLoader

	UsersByID           UsersByIDLoader
	UsersByName         UsersByNameLoader
	TrackersByID        TrackersByIDLoader
	TrackersByName      TrackersByNameLoader
	TrackersByOwnerName TrackersByOwnerNameLoader
	TicketsByID         TicketsByIDLoader
	TicketsByTrackerID  TicketsByTrackerIDLoader
	LabelsByID          LabelsByIDLoader

	// Upserts
	ParticipantsByUserID ParticipantsByUserIDLoader
	// Upserts & fetches from meta.sr.ht
	ParticipantsByUsername ParticipantsByUsernameLoader

	CommentsByIDUnsafe    CommentsByIDLoader
	SubsByTicketIDUnsafe  SubsByTicketIDLoader
	SubsByTrackerIDUnsafe SubsByTrackerIDLoader
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

			usersByID := map[int]*model.User{}
			for rows.Next() {
				user := model.User{}
				if err := rows.Scan(database.Scan(ctx, &user)...); err != nil {
					return err
				}
				usersByID[user.ID] = &user
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				users[i] = usersByID[id]
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

func fetchTrackersByID(ctx context.Context) func(ids []int) ([]*model.Tracker, []error) {
	return func(ids []int) ([]*model.Tracker, []error) {
		trackers := make([]*model.Tracker, len(ids))
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
				Select(ctx, (&model.Tracker{}).As(`tr`)).
				From(`"tracker" tr`).
				LeftJoin(`user_access ua ON ua.tracker_id = tr.id AND ua.user_id = ?`, auser.UserID).
				Column(`COALESCE(
					ua.permissions,
					CASE WHEN tr.owner_id = ?
						THEN ?
						ELSE tr.default_access
					END)`,
					auser.UserID, model.ACCESS_ALL).
				Column(`ua.id`).
				Column(`tr.default_access`).
				Where(sq.And{
					sq.Expr(`tr.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`tr.owner_id = ?`, auser.UserID),
						sq.Expr(`tr.visibility != 'PRIVATE'`),
						sq.Expr(`ua.permissions > 0`),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			trackersByID := map[int]*model.Tracker{}
			for rows.Next() {
				tracker := model.Tracker{}
				if err := rows.Scan(append(
					database.Scan(ctx, &tracker),
					&tracker.Access, &tracker.ACLID,
					&tracker.DefaultAccess)...); err != nil {
					return err
				}
				trackersByID[tracker.ID] = &tracker
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				trackers[i] = trackersByID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return trackers, nil
	}
}

func fetchTrackersByName(ctx context.Context) func(names []string) ([]*model.Tracker, []error) {
	return func(names []string) ([]*model.Tracker, []error) {
		trackers := make([]*model.Tracker, len(names))
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
				Select(ctx, (&model.Tracker{}).As(`t`)).
				From(`"tracker" t`).
				Where(sq.And{
					sq.Expr(`t.name = ANY(?)`, pq.Array(names)),
					sq.Expr(`t.owner_id = ?`, auser.UserID),
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			trackersByName := map[string]*model.Tracker{}
			for rows.Next() {
				tracker := model.Tracker{}
				if err := rows.Scan(database.Scan(ctx, &tracker)...); err != nil {
					return err
				}
				tracker.Access = model.ACCESS_ALL
				trackersByName[tracker.Name] = &tracker
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, name := range names {
				trackers[i] = trackersByName[name]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return trackers, nil
	}
}

func fetchTrackersByOwnerName(ctx context.Context) func(tuples [][2]string) ([]*model.Tracker, []error) {
	return func(tuples [][2]string) ([]*model.Tracker, []error) {
		trackers := make([]*model.Tracker, len(tuples))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err        error
				rows       *sql.Rows
				ownerNames = make([]string, len(tuples))
			)
			for i, tuple := range tuples {
				ownerNames[i] = tuple[0] + "/" + tuple[1]
			}
			auser := auth.ForContext(ctx)
			query := database.
				Select(ctx).
				Prefix(`WITH user_tracker AS (
					SELECT
						substring(un for position('/' in un)-1) AS owner,
						substring(un from position('/' in un)+1) AS tracker
					FROM unnest(?::text[]) un)`, pq.Array(ownerNames)).
				Columns(database.Columns(ctx, (&model.Tracker{}).As(`tr`))...).
				Columns(`u.username`).
				Distinct().
				From(`user_tracker ut`).
				Join(`"user" u on ut.owner = u.username`).
				Join(`"tracker" tr ON ut.tracker = tr.name
					AND u.id = tr.owner_id`).
				LeftJoin(`user_access ua ON ua.tracker_id = tr.id AND ua.user_id = ?`, auser.UserID).
				Column(`COALESCE(
					ua.permissions,
					CASE WHEN tr.owner_id = ?
						THEN ?
						ELSE tr.default_access
					END)`,
					auser.UserID, model.ACCESS_ALL).
				Where(sq.Or{
					sq.Expr(`tr.owner_id = ?`, auser.UserID),
					sq.Expr(`tr.visibility != 'PRIVATE'`),
					sq.Expr(`ua.permissions > 0`),
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			trackersByOwnerName := map[[2]string]*model.Tracker{}
			for rows.Next() {
				var ownerName string
				tracker := model.Tracker{}
				if err := rows.Scan(append(database.Scan(ctx, &tracker),
					&ownerName, &tracker.Access)...); err != nil {
					return err
				}
				trackersByOwnerName[[2]string{ownerName, tracker.Name}] = &tracker
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, tuple := range tuples {
				trackers[i] = trackersByOwnerName[tuple]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return trackers, nil
	}
}

func fetchTicketsByID(ctx context.Context) func(ids []int) ([]*model.Ticket, []error) {
	return func(ids []int) ([]*model.Ticket, []error) {
		tickets := make([]*model.Ticket, len(ids))

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
				Select(ctx, (&model.Ticket{}).As(`ti`)).
				From(`"ticket" ti`).
				Join(`"tracker" tr ON tr.id = ti.tracker_id`).
				LeftJoin(`user_access ua ON ua.tracker_id = tr.id AND ua.user_id = ?`, auser.UserID).
				Where(sq.And{
					sq.Expr(`ti.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`tr.owner_id = ?`, auser.UserID),
						sq.Expr(`tr.visibility != 'PRIVATE'`),
						sq.Expr(`ua.permissions > 0`),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			ticketsByID := map[int]*model.Ticket{}
			for rows.Next() {
				ticket := model.Ticket{}
				if err := rows.Scan(database.Scan(ctx, &ticket)...); err != nil {
					return err
				}
				ticketsByID[ticket.PKID] = &ticket
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				tickets[i] = ticketsByID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return tickets, nil
	}
}

func fetchTicketsByTrackerID(ctx context.Context) func(ids [][2]int) ([]*model.Ticket, []error) {
	return func(ids [][2]int) ([]*model.Ticket, []error) {
		tickets := make([]*model.Ticket, len(ids))
		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			var (
				err        error
				rows       *sql.Rows
				trackerIDs = make([]int, len(ids))
				scopedIDs  = make([]int, len(ids))
			)
			for i, items := range ids {
				trackerIDs[i] = items[0]
				scopedIDs[i] = items[1]
			}

			tx.ExecContext(ctx, `
				CREATE TEMP TABLE lut
				ON COMMIT DROP
				AS (SELECT
					unnest($1::int[]) AS tracker_id,
					unnest($2::int[]) AS scoped_id);
			`, pq.Array(trackerIDs), pq.Array(scopedIDs))

			auser := auth.ForContext(ctx)
			query := database.
				Select(ctx, (&model.Ticket{}).As(`tk`)).
				Columns(`tk.tracker_id`, `tk.scoped_id`).
				From(`"ticket" tk`).
				Join(`"tracker" tr ON tr.id = tk.tracker_id`).
				LeftJoin(`user_access ua ON ua.tracker_id = tr.id AND ua.user_id = ?`, auser.UserID).
				Where(sq.And{
					sq.Expr(`(tk.tracker_id, tk.scoped_id) IN (SELECT * FROM lut)`),
					sq.Or{
						sq.Expr(`tr.owner_id = ?`, auser.UserID),
						sq.Expr(`tr.visibility != 'PRIVATE'`),
						sq.Expr(`ua.permissions > 0`),
					},
				})

			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			ticketsByTrackerID := make(map[[2]int]*model.Ticket)
			for rows.Next() {
				var (
					ticket    model.Ticket
					trackerID int
					scopedID  int
				)
				if err := rows.Scan(append(database.Scan(ctx, &ticket),
					&trackerID, &scopedID)...); err != nil {
					return err
				}
				ticketsByTrackerID[[2]int{trackerID, scopedID}] = &ticket
			}

			for i, items := range ids {
				tickets[i] = ticketsByTrackerID[[2]int{items[0], items[1]}]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return tickets, nil
	}
}

// This function presumes that the user is authorized to read this comment, no
// ACL tests are attempted.
func fetchCommentsByIDUnsafe(ctx context.Context) func(ids []int) ([]*model.Comment, []error) {
	return func(ids []int) ([]*model.Comment, []error) {
		comments := make([]*model.Comment, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			if rows, err = tx.QueryContext(ctx, `
				SELECT id, text, authenticity, superceeded_by_id
				FROM ticket_comment
				WHERE id = ANY($1)
			`, pq.Array(ids)); err != nil {
				return err
			}
			defer rows.Close()

			commentsByID := map[int]*model.Comment{}
			for rows.Next() {
				comment := model.Comment{}
				if err := rows.Scan(&comment.Database.ID,
					&comment.Database.Text,
					&comment.Database.Authenticity,
					&comment.Database.SuperceededByID); err != nil {
					return err
				}
				commentsByID[comment.Database.ID] = &comment
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				comments[i] = commentsByID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return comments, nil
	}
}

func fetchEntitiesByParticipantID(ctx context.Context) func(ids []int) ([]model.Entity, []error) {
	return func(ids []int) ([]model.Entity, []error) {
		entities := make([]model.Entity, len(ids))
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			if rows, err = tx.QueryContext(ctx, `
					SELECT
						participant.id,
						participant_type,

						-- User fields:
						COALESCE("user".id, 0),
						COALESCE("user".created, now() at time zone 'utc'),
						COALESCE("user".updated, now() at time zone 'utc'),
						COALESCE("user".username, ''),
						COALESCE("user".email, ''),
						"user".url, "user".location, "user".bio,

						-- Email fields:
						COALESCE(participant.email, ''),
						participant.email_name,

						-- External user fields:
						COALESCE(participant.external_id, ''),
						participant.external_url
					FROM participant
					LEFT JOIN "user" on participant.user_id = "user".id
					WHERE participant.id = ANY($1)
			`, pq.Array(ids)); err != nil {
				return err
			}
			defer rows.Close()

			entitiesByID := map[int]model.Entity{}
			for rows.Next() {
				var (
					pid    int
					ptype  string
					entity model.Entity
					email  model.EmailAddress
					ext    model.ExternalUser
					user   model.User
				)

				if err := rows.Scan(&pid, &ptype, &user.ID, &user.Created,
					&user.Updated, &user.Username, &user.Email, &user.URL,
					&user.Location, &user.Bio, &email.Mailbox, &email.Name,
					&ext.ExternalID, &ext.ExternalURL); err != nil {

					if err == sql.ErrNoRows {
						return nil
					} else {
						return err
					}
				}

				switch ptype {
				case "user":
					entity = &user
				case "email":
					entity = &email
				case "external":
					entity = &ext
				default:
					panic(fmt.Errorf("database invariant broken; invalid participant type for ID %d", pid))
				}

				entitiesByID[pid] = entity
			}

			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				entities[i] = entitiesByID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return entities, nil
	}
}

func fetchLabelsByID(ctx context.Context) func(ids []int) ([]*model.Label, []error) {
	return func(ids []int) ([]*model.Label, []error) {
		labels := make([]*model.Label, len(ids))

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
				Select(ctx, (&model.Label{}).As(`l`)).
				From(`"label" l`).
				Join(`"tracker" tr ON tr.id = l.tracker_id`).
				LeftJoin(`user_access ua ON ua.tracker_id = tr.id AND ua.user_id = ?`, auser.UserID).
				Where(sq.And{
					sq.Expr(`l.id = ANY(?)`, pq.Array(ids)),
					sq.Or{
						sq.Expr(`tr.owner_id = ?`, auser.UserID),
						sq.Expr(`tr.visibility != 'PRIVATE'`),
						sq.Expr(`ua.permissions > 0`),
					},
				})
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			labelsByID := map[int]*model.Label{}
			for rows.Next() {
				label := model.Label{}
				if err := rows.Scan(database.Scan(ctx, &label)...); err != nil {
					return err
				}
				labelsByID[label.ID] = &label
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				labels[i] = labelsByID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return labels, nil
	}
}

func fetchParticipantsByUserID(ctx context.Context) func(ids []int) ([]*model.Participant, []error) {
	return func(ids []int) ([]*model.Participant, []error) {
		parts := make([]*model.Participant, len(ids))

		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			// XXX: This is optimized for working with many user IDs at once,
			// for a low number of IDs it might be faster to do it differently
			_, err := tx.ExecContext(ctx, `
				CREATE TEMP TABLE participant_users (user_id int)
				ON COMMIT DROP;
			`)
			if err != nil {
				return err
			}
			stmt, err := tx.Prepare(pq.CopyIn("participant_users", "user_id"))
			if err != nil {
				return err
			}

			for _, id := range ids {
				_, err := stmt.Exec(id)
				if err != nil {
					return err
				}
			}

			_, err = stmt.Exec()
			if err != nil {
				return err
			}

			rows, err := tx.QueryContext(ctx, `
				INSERT INTO participant (
					created, participant_type, user_id
				) SELECT
					NOW() at time zone 'utc',
					'user',
					user_id
				FROM participant_users
				ON CONFLICT ON CONSTRAINT participant_user_id_key
				DO UPDATE SET created = participant.created
				RETURNING id, user_id
			`)
			if err != nil {
				return err
			}
			defer rows.Close()

			partsByUserID := make(map[int]*model.Participant)
			for rows.Next() {
				var (
					userID int
					part   model.Participant
				)
				if err := rows.Scan(&part.ID, &userID); err != nil {
					return err
				}
				partsByUserID[userID] = &part
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				parts[i] = partsByUserID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return parts, nil
	}
}

// TODO: All of these user-fetching-from-meta bits could go in core-go
var (
	fetchUserTemplate = template.Must(template.New("FetchUser").
		Funcs(map[string]any{
			"escape": escapeUsername,
		}).Parse(`
			query FetchUsers {
				{{range .}}
				{{. | escape}}: userByName(username: "{{.}}") {
					...userDetails
				}
				{{end}}
			}

			fragment userDetails on User {
				id, created, updated
				username, email
				url, location, bio
				userType, suspensionNotice
			}
		`))
)

func escapeUsername(name string) string {
	h := sha256.New()
	h.Write([]byte(name))
	return fmt.Sprintf("_%x", h.Sum(nil))
}

type UserInfo struct {
	Id               int       `json:"id"`
	Created          time.Time `json:"created"`
	Updated          time.Time `json:"updated"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	Url              *string   `json:"url"`
	Location         *string   `json:"location"`
	Bio              *string   `json:"bio"`
	UserType         string    `json:"userType"`
	SuspensionNotice *string   `json:"suspensionNotice"`
}

func fetchParticipantsByUsername(ctx context.Context) func(names []string) ([]*model.Participant, []error) {
	return func(names []string) ([]*model.Participant, []error) {
		parts := make([]*model.Participant, len(names))

		if err := database.WithTx(ctx, nil, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				CREATE TEMP TABLE participant_users (
					username varchar NOT NULL
				) ON COMMIT DROP;
			`)
			if err != nil {
				return err
			}
			stmt, err := tx.Prepare(pq.CopyIn("participant_users", "username"))
			if err != nil {
				return err
			}

			for _, username := range names {
				_, err := stmt.Exec(username)
				if err != nil {
					return err
				}
			}

			_, err = stmt.Exec()
			if err != nil {
				return err
			}

			// Find a list of usernames we need to retrieve from meta.sr.ht
			rows, err := tx.QueryContext(ctx, `
				SELECT pu.username
				FROM participant_users pu
				LEFT JOIN "user" ON "user".username = pu.username
				WHERE "user".id IS NULL;
			`)
			if err != nil {
				return err
			}

			var toFetch []string
			for rows.Next() {
				var username string
				if err := rows.Scan(&username); err != nil {
					return err
				}
				toFetch = append(toFetch, username)
			}

			if len(toFetch) != 0 {
				stmt, err = tx.Prepare(pq.CopyIn(`user`,
					"id", "created", "updated", "username", "email",
					"user_type", "url", "location", "bio",
					"suspension_notice"))
				if err != nil {
					return err
				}

				var (
					resp map[string]*UserInfo
					gql  bytes.Buffer
				)
				err = fetchUserTemplate.Execute(&gql, toFetch)
				if err != nil {
					panic(err)
				}
				query := client.GraphQLQuery{Query: gql.String()}
				err = client.Do(ctx, auth.ForContext(ctx).Username,
					"meta.sr.ht", query, &resp)
				if err != nil {
					return err
				}

				for _, user := range toFetch {
					details := resp[escapeUsername(user)]
					if details == nil {
						continue
					}
					_, err = stmt.Exec(details.Id, details.Created, details.Updated,
						details.Username, details.Email,
						details.UserType,
						details.Url, details.Location, details.Bio,
						details.SuspensionNotice)
					if err != nil {
						return err
					}
				}

				_, err = stmt.Exec()
				if err != nil {
					return err
				}
			}

			rows, err = tx.QueryContext(ctx, `
				INSERT INTO participant (
					created, participant_type, user_id
				) SELECT
					NOW() at time zone 'utc',
					'user',
					"user".id
				FROM participant_users pu
				JOIN "user" ON "user".username = pu.username
				ON CONFLICT ON CONSTRAINT participant_user_id_key
				DO UPDATE SET created = participant.created
				RETURNING id, user_id
			`)
			if err != nil {
				return err
			}
			defer rows.Close()

			partsByUserID := make(map[int]*model.Participant)
			for rows.Next() {
				var (
					userID int
					part   model.Participant
				)
				if err := rows.Scan(&part.ID, &userID); err != nil {
					return err
				}
				partsByUserID[userID] = &part
			}
			if err = rows.Err(); err != nil {
				return err
			}

			rows, err = tx.QueryContext(ctx, `
				SELECT "user".id, "user".username
				FROM participant_users pu
				JOIN "user" ON "user".username = pu.username
			`)
			if err != nil {
				return err
			}
			defer rows.Close()

			userIDsByUsername := make(map[string]int)
			for rows.Next() {
				var (
					id       int
					username string
				)
				if err := rows.Scan(&id, &username); err != nil {
					return err
				}
				userIDsByUsername[username] = id
			}

			for i, name := range names {
				parts[i] = partsByUserID[userIDsByUsername[name]]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return parts, nil
	}
}

func fetchSubsByTicketIDUnsafe(ctx context.Context) func(ids []int) ([]*model.TicketSubscription, []error) {
	return func(ids []int) ([]*model.TicketSubscription, []error) {
		subs := make([]*model.TicketSubscription, len(ids))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.SubscriptionInfo{}).As(`sub`)).
				Column(`sub.ticket_id`).
				From(`ticket_subscription sub`).
				Join(`participant p ON p.id = sub.participant_id`).
				Where(`p.user_id = ? AND sub.ticket_id = ANY(?)`,
					auth.ForContext(ctx).UserID, pq.Array(ids))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			subsByTicketID := map[int]*model.TicketSubscription{}
			for rows.Next() {
				var ticketID int
				si := model.SubscriptionInfo{}
				if err := rows.Scan(append(database.Scan(
					ctx, &si), &ticketID)...); err != nil {
					return err
				}
				subsByTicketID[ticketID] = &model.TicketSubscription{
					ID:       si.ID,
					Created:  si.Created,
					TicketID: ticketID,
				}
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				subs[i] = subsByTicketID[id]
			}

			return nil
		}); err != nil {
			panic(err)
		}

		return subs, nil
	}
}

func fetchSubsByTrackerIDUnsafe(ctx context.Context) func(ids []int) ([]*model.TrackerSubscription, []error) {
	return func(ids []int) ([]*model.TrackerSubscription, []error) {
		subs := make([]*model.TrackerSubscription, len(ids))

		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			var (
				err  error
				rows *sql.Rows
			)
			query := database.
				Select(ctx, (&model.SubscriptionInfo{}).As(`sub`)).
				Column(`sub.tracker_id`).
				From(`ticket_subscription sub`).
				Join(`participant p ON p.id = sub.participant_id`).
				Where(`p.user_id = ? AND sub.tracker_id = ANY(?)`,
					auth.ForContext(ctx).UserID, pq.Array(ids))
			if rows, err = query.RunWith(tx).QueryContext(ctx); err != nil {
				return err
			}
			defer rows.Close()

			subsByTrackerID := map[int]*model.TrackerSubscription{}
			for rows.Next() {
				var trackerID int
				si := model.SubscriptionInfo{}
				if err := rows.Scan(append(database.Scan(
					ctx, &si), &trackerID)...); err != nil {
					return err
				}
				subsByTrackerID[trackerID] = &model.TrackerSubscription{
					ID:        si.ID,
					Created:   si.Created,
					TrackerID: trackerID,
				}
			}
			if err = rows.Err(); err != nil {
				return err
			}

			for i, id := range ids {
				subs[i] = subsByTrackerID[id]
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
		ctx := Context(r.Context())
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, loadersCtxKey, &Loaders{
		UsersByID: UsersByIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchUsersByID(ctx),
		},
		UsersByName: UsersByNameLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchUsersByName(ctx),
		},
		TrackersByID: TrackersByIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchTrackersByID(ctx),
		},
		TrackersByName: TrackersByNameLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchTrackersByName(ctx),
		},
		TrackersByOwnerName: TrackersByOwnerNameLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchTrackersByOwnerName(ctx),
		},
		TicketsByID: TicketsByIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchTicketsByID(ctx),
		},
		TicketsByTrackerID: TicketsByTrackerIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchTicketsByTrackerID(ctx),
		},
		CommentsByIDUnsafe: CommentsByIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchCommentsByIDUnsafe(ctx),
		},
		EntitiesByParticipantID: EntitiesByParticipantIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchEntitiesByParticipantID(ctx),
		},
		LabelsByID: LabelsByIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchLabelsByID(ctx),
		},
		ParticipantsByUserID: ParticipantsByUserIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchParticipantsByUserID(ctx),
		},
		ParticipantsByUsername: ParticipantsByUsernameLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchParticipantsByUsername(ctx),
		},
		SubsByTicketIDUnsafe: SubsByTicketIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchSubsByTicketIDUnsafe(ctx),
		},
		SubsByTrackerIDUnsafe: SubsByTrackerIDLoader{
			maxBatch: 100,
			wait:     1 * time.Millisecond,
			fetch:    fetchSubsByTrackerIDUnsafe(ctx),
		},
	})
}

func ForContext(ctx context.Context) *Loaders {
	raw, ok := ctx.Value(loadersCtxKey).(*Loaders)
	if !ok {
		panic(errors.New("invalid data loaders context"))
	}
	return raw
}
