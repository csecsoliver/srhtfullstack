package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"git.sr.ht/~sircmpwn/core-go/client"
	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/crypto"
	_ "github.com/lib/pq"
)

type Invoice struct {
	ID       int
	UserID   int
	Username string
}

func main() {
	conf := config.LoadConfig()
	crypto.InitCrypto(conf)
	ctx := config.Context(context.Background(), conf, "meta.sr.ht")

	pgcs, ok := conf.Get("meta.sr.ht", "connection-string")
	if !ok {
		log.Fatalf("No connection string provided in config.ini")
	}

	db, err := sql.Open("postgres", pgcs)
	if err != nil {
		log.Fatalf("Failed to open a database connection: %v", err)
	}

	invoices := getInvoices(db)

	for i, inv := range invoices {
		fmt.Printf("Uploading %d/%d: %d", i+1, len(invoices), inv.ID)

		query := client.GraphQLQuery{
			Query: `
				mutation UploadInvoice($id: Int!) {
					uploadInvoice(id: $id)
				}
			`,
			Variables: map[string]interface{}{
				"id": inv.ID,
			},
		}
		resp := struct {
			UploadInvoice string `json:"uploadInvoice"`
		}{}
		err := client.Do(ctx, inv.Username, "meta.sr.ht", query, &resp)
		if err != nil {
			fmt.Printf(": %s\n", err.Error())
		} else {
			fmt.Printf(": %s\n", resp.UploadInvoice)
		}
	}
}

func getInvoices(db *sql.DB) []Invoice {
	var invoices []Invoice

	rows, err := db.Query(`
		SELECT
			invoice.id, invoice.user_id, "user".username
		FROM invoice
		JOIN "user" on "user".id = invoice.user_id
		WHERE invoice.uuid IS NULL
		ORDER BY invoice.id;
	`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id       int
			userID   int
			username string
		)
		err = rows.Scan(&id, &userID, &username)
		if err != nil {
			panic(err)
		}
		invoices = append(invoices, Invoice{
			ID:       id,
			UserID:   userID,
			Username: username,
		})
	}

	return invoices
}
