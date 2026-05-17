package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	_ "github.com/lib/pq"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/customer"
	"github.com/stripe/stripe-go/v81/paymentsource"
)

type User struct {
	ID         int
	Username   string
	CustomerID string
}

func main() {
	conf := config.LoadConfig()

	pgcs, ok := conf.Get("meta.sr.ht", "connection-string")
	if !ok {
		log.Fatalf("No connection string provided in config.ini")
	}

	db, err := sql.Open("postgres", pgcs)
	if err != nil {
		log.Fatalf("Failed to open a database connection: %v", err)
	}

	if skey, ok := conf.Get("meta.sr.ht::billing", "stripe-secret-key"); ok {
		stripe.Key = skey
	} else if !ok {
		log.Fatal("Stripe secret key missing in configuration")
	}

	users := getUsers(db)
	for n, user := range users {
		log.Printf("Processing %d/%d ~%s:%s",
			n+1, len(users), user.Username, user.CustomerID)

		tx, err := db.Begin()
		if err != nil {
			panic(err)
		}
		defer func() {
			if err := recover(); err != nil {
				tx.Rollback()
				log.Fatalf("Error updating ~%s: %v",
					user.Username, err)
			}
		}()

		nmethod := 0
		sourceIDMap := make(map[string]int)
		iter := paymentsource.List(&stripe.PaymentSourceListParams{
			Customer: stripe.String(user.CustomerID),
		})
		for iter.Next() {
			if err := iter.Err(); err != nil {
				panic(err)
			}
			source := iter.PaymentSource()
			name := fmt.Sprintf("%s ending in %s",
				source.Card.Brand, source.Card.Last4)
			expires := time.Date(
				int(source.Card.ExpYear),
				time.Month(source.Card.ExpMonth),
				0, 0, 0, 0, 0, time.UTC)

			var pmID int
			row := tx.QueryRow(`
				INSERT INTO payment_method (
					created,
					user_id,
					expires,
					name,
					processor_id
				) VALUES (
					now() at time zone 'utc',
					$1, $2, $3, $4
				)
				ON CONFLICT ON CONSTRAINT
				payment_method_processor_id_key
				DO UPDATE SET user_id = $1
				RETURNING id
			`, user.ID, expires, name, source.ID)
			if err := row.Scan(&pmID); err != nil {
				panic(err)
			}

			log.Printf("Import %s => %d", source.ID, pmID)
			sourceIDMap[source.ID] = pmID
			nmethod += 1
		}

		if nmethod == 0 {
			tx.Rollback()
			continue
		}

		c, err := customer.Get(user.CustomerID, &stripe.CustomerParams{})
		if err != nil {
			panic(err)
		}

		_, err = tx.Exec(`
			UPDATE "user"
			SET default_payment_method_id = $2
			WHERE id = $1
		`, user.ID, sourceIDMap[c.DefaultSource.ID])
		if err != nil {
			panic(err)
		}

		err = tx.Commit()
		if err != nil {
			panic(err)
		}
	}
}

func getUsers(db *sql.DB) []User {
	var users []User

	rows, err := db.Query(`
	SELECT
		id,
		username,
		payment_processor_id
	FROM "user"
	WHERE
		payment_processor_id IS NOT NULL AND
		default_payment_method_id IS NULL;
	`)
	if err != nil {
		log.Fatalf("Error fetching sources: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID     int
			username   string
			customerID string
		)
		err := rows.Scan(&userID, &username, &customerID)
		if err != nil {
			log.Fatalf("Error scanning row: %s", err.Error())
		}
		users = append(users, User{
			ID:         userID,
			Username:   username,
			CustomerID: customerID,
		})
	}
	return users
}
