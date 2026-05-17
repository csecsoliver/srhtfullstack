package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/database"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/ssh"
)

func main() {
	log.Println("Starting...")

	conf := config.LoadConfig()

	pgcs, ok := conf.Get("meta.sr.ht", "connection-string")
	if !ok {
		log.Fatalf("No connection string provided in config.ini")
	}

	db, err := sql.Open("postgres", pgcs)
	if err != nil {
		log.Fatalf("Failed to open a database connection: %v", err)
	}

	q := `SELECT id, user_id, raw FROM "sshkey" WHERE key is NULL`
	rows, err := db.Query(q)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		// Just a tiny bit of throttling, we're not in a rush...
		time.Sleep(10 * time.Millisecond)

		var (
			id      int64
			user_id int64
			raw_    string
		)
		if err := rows.Scan(&id, &user_id, &raw_); err != nil {
			log.Fatal(err)
		}
		log.Printf("[%d] processing key of user %d\n", id, user_id)

		raw := strings.TrimSpace(raw_)

		key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))

		// There seem to be two prevalent forms of malformed keys that got
		// accepted at some point. Try to handle them if the key cannot be
		// parsed as is.
		if err != nil {
			if strings.HasPrefix(raw, "----") {
				log.Printf("[%d] looks like a PEM key, trying to decode", id)
				// Why, openssh, why?
				raw = strings.ReplaceAll(raw, "---- BEGIN", "-----BEGIN")
				raw = strings.ReplaceAll(raw, "---- END", "-----END")
				raw = strings.ReplaceAll(raw, "KEY ----", "KEY-----")

				block, _ := pem.Decode([]byte(raw))
				if block != nil {
					key, err = ssh.ParsePublicKey(block.Bytes)
					comment = block.Headers["Comment"]
				}
			} else if strings.Contains(raw, "\n") {
				log.Printf("[%d] has newlines, trying to fix", id)
				raw = strings.ReplaceAll(raw, "\n", " ")
				raw = strings.ReplaceAll(raw, "\r", " ")
				key, comment, _, _, err = ssh.ParseAuthorizedKey([]byte(raw))
			}
		}

		// We tried, but this key is invalid (at least to x/crypto/ssh). Bite
		// the bullet and remove it, but add the raw entry to a user note. In
		// the unlikely event that this should be required it could be manually
		// restored.
		if err != nil {
			log.Printf("[%d] failed to parse: %v", id, err)
			log.Printf("[%d] raw entry:\n%s", id, raw_)

			ctx := database.Context(context.Background(), db)
			err = database.WithTx(ctx, nil, func(tx *sql.Tx) error {
				note := fmt.Sprintf("The following invalid SSH key was removed:\n\n%s", raw_)
				q := `INSERT INTO "user_notes" (created, user_id, note)
					VALUES (NOW(), $1, $2)`
				_, err = tx.Exec(q, user_id, note)
				if err != nil {
					return err
				}
				q = `DELETE FROM "sshkey" WHERE ID = $1`
				_, err = tx.Exec(q, id)
				return err
			})
			if err != nil {
				log.Fatalf("[%d] failed to add note and delete key: %v", id, err)
			}

			continue
		}

		// Ok, so we have something that looks like a key...
		fingerprintSHA256 := ssh.FingerprintSHA256(key)
		keyb64 := base64.StdEncoding.EncodeToString(key.Marshal())

		q := `UPDATE "sshkey" SET 
			key = $2, key_type = $3, fingerprint_sha256 = $4, comment = $5
			WHERE id = $1`
		_, err = db.Exec(q, id, keyb64, key.Type(), fingerprintSHA256, comment)
		if err != nil {
			log.Fatalf("[%d] error updating key details: %v", id, err)
		}
	}

	log.Println("Done.")
}
