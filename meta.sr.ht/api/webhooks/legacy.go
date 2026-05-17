package webhooks

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/database"
	"git.sr.ht/~sircmpwn/core-go/webhooks"
	sq "github.com/Masterminds/squirrel"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

func DeliverLegacyProfileUpdate(ctx context.Context, user *model.User) {
	// This legacy garbage is due to be removed at our earliest convenience.
	q := webhooks.LegacyForContext(ctx)

	type WebhookPayload struct {
		CanonicalName string  `json:"canonical_name"`
		Name          string  `json:"name"`
		Email         string  `json:"email"`
		URL           *string `json:"url"`
		Location      *string `json:"location"`
		Bio           *string `json:"bio"`
		UsePGPKey     *string `json:"use_pgp_key"`

		UserType         string  `json:"user_type,omitempty"`
		SuspensionNotice *string `json:"suspension_notice,omitempty"`
	}

	// XXX: Technically we could do the PGP key lookup in a single SQL query if
	// we had a more sophisticated database system (particularly if we had lazy
	// loading queries)
	var fingerprint *[]byte
	if err := database.WithTx(ctx, &sql.TxOptions{
		Isolation: 1,
		ReadOnly:  true,
	}, func(tx *sql.Tx) error {
		return sq.
			Select("p.fingerprint").
			From(`"user" u`).
			LeftJoin(`pgpkey p ON p.id = u.pgp_key_id`).
			Where("u.id = ?", user.ID).
			PlaceholderFormat(sq.Dollar).
			RunWith(tx).
			ScanContext(ctx, &fingerprint)
	}); err != nil {
		panic(err)
	}

	var fprint *string
	if fingerprint != nil {
		encoded := strings.ToUpper(hex.EncodeToString(*fingerprint))
		fprint = &encoded
	}

	payload := WebhookPayload{
		CanonicalName:    user.CanonicalName(),
		Name:             user.Username,
		Email:            user.Email,
		URL:              user.URL,
		Location:         user.Location,
		Bio:              user.Bio,
		UsePGPKey:        fprint,
		UserType:         user.UserType.String(),
		SuspensionNotice: user.SuspensionNotice,
	}
	encoded, err := json.Marshal(&payload)
	if err != nil {
		panic(err) // Programmer error
	}

	query := sq.
		Select().
		From("user_webhook_subscription sub").
		Where("sub.user_id = ?", user.ID)
	q.Schedule(ctx, query, "user", "profile:update", encoded)
}

func DeliverLegacyPGPKeyAdded(ctx context.Context, key *model.PGPKey) {
	q := webhooks.LegacyForContext(ctx)

	type WebhookPayload struct {
		ID          int       `json:"id"`
		Key         string    `json:"key"`
		Fingerprint string    `json:"fingerprint"`
		Email       string    `json:"email"`
		Authorized  time.Time `json:"authorized"`

		Owner struct {
			CanonicalName string `json:"canonical_name"`
			Name          string `json:"name"`
		} `json:"owner"`
	}

	fingerprint := strings.ToUpper(hex.EncodeToString(key.RawFingerprint))
	payload := WebhookPayload{
		ID:          key.ID,
		Key:         key.Key,
		Fingerprint: fingerprint,
		Authorized:  key.Created,
	}

	// TODO: User groups
	user := auth.ForContext(ctx)
	if user.UserID != key.UserID {
		// At the time of writing, the only consumers of this function are in a
		// context where the authenticated user is the owner of this PGP key. We
		// can skip the database round-trip if we just grab their auth context.
		panic(errors.New("TODO: look up user details for this key"))
	}
	payload.Owner.CanonicalName = "~" + user.Username
	payload.Owner.Name = user.Username

	encoded, err := json.Marshal(&payload)
	if err != nil {
		panic(err) // Programmer error
	}

	query := sq.
		Select().
		From("user_webhook_subscription sub").
		Where("sub.user_id = ?", key.UserID)
	q.Schedule(ctx, query, "user", "pgp-key:add", encoded)
}

func DeliverLegacyPGPKeyRemoved(ctx context.Context, key *model.PGPKey) {
	q := webhooks.LegacyForContext(ctx)

	type WebhookPayload struct {
		ID int `json:"id"`
	}
	payload := WebhookPayload{key.ID}

	encoded, err := json.Marshal(&payload)
	if err != nil {
		panic(err) // Programmer error
	}

	query := sq.
		Select().
		From("user_webhook_subscription sub").
		Where("sub.user_id = ?", key.UserID)
	q.Schedule(ctx, query, "user", "pgp-key:remove", encoded)
}
