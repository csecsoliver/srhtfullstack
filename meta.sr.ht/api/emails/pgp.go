package emails

import (
	"context"
	"database/sql"

	"git.sr.ht/~sircmpwn/core-go/database"

	"git.sr.ht/~sircmpwn/meta.sr.ht/api/graph/model"
)

// Retrieves the PGP key for outgoing email encryption for a user account (or
// nil if the user hasn't configured one).
func PGPKeyForUser(ctx context.Context, user *model.User) (*string, error) {
	var key *string
	if user.PGPKeyID != nil {
		if err := database.WithTx(ctx, &sql.TxOptions{
			Isolation: 0,
			ReadOnly:  true,
		}, func(tx *sql.Tx) error {
			row := tx.QueryRowContext(ctx, `
				SELECT key
				FROM "pgpkey" WHERE id = $1;
				`, *user.PGPKeyID)
			return row.Scan(&key)
		}); err != nil {
			return nil, err
		}
	}
	return key, nil
}
