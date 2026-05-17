package graph

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/errors"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/emails"
	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

var usernameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]+$`)

var (
	ErrInvalidEmailToken = errors.New("ERR_INVALID_EMAIL_TOKEN", "Invalid email token")
	ErrNotPastDue        = errors.New("ERR_NOT_PAST_DUE", "Account is not past due")
	ErrNoSubscription    = errors.New("ERR_NO_SUBSCRIPTION", "Account does not have an active subscription")
)

type Resolver struct{}

type AuthorizationPayload struct {
	Grants     string
	ClientUUID string
	UserID     int
}

// Asserts that the given user is the user that authenticated for this request.
func requireAuthAs(ctx context.Context, user *model.User) error {
	if auth.ForContext(ctx).UserID != user.ID {
		return fmt.Errorf("access denied")
	}
	return nil
}

// Populates the auth context for anonymous resolvers when the user's identity
// is established through a side channel (such as private knowledge of a token
// emailed to them)
func populateAuthCtx(ctx context.Context, user *model.User) {
	authCtx := auth.ForContext(ctx)
	if authCtx.AuthMethod != auth.AUTH_ANON_INTERNAL {
		panic(fmt.Errorf("called populateAuthCtx in already authenticated context"))
	}

	authCtx.UserID = user.ID
	authCtx.Username = user.Username
	authCtx.Email = user.Email

	pgpKey, err := emails.PGPKeyForUser(ctx, user)
	if err != nil {
		panic(err) // Invariant
	}

	authCtx.PGPKey = pgpKey
}

// Send a security-related notice to the authorized user.
func sendSecurityNotification(ctx context.Context, subject, details string) {
	user := auth.ForContext(ctx)
	emails.SendSecurityNotificationTo(ctx, user.Username, user.Email,
		subject, details, user.PGPKey)
}

// Records an event in the authorized user's audit log.
func recordAuditLog(ctx context.Context, eventType, details string) {
	database.WithTx(ctx, nil, func(tx *sql.Tx) error {
		user := auth.ForContext(ctx)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO audit_log_entry (
				created, user_id, ip_address, event_type, details
			) VALUES (
				NOW() at time zone 'utc',
				$1, $2, $3, $4
			);
		`, user.UserID, user.IPAddress, eventType, details)
		if err != nil {
			panic(err)
		}

		log.Printf("Audit log: %s: %s", eventType, details)
		return nil
	})
}
