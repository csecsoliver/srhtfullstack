package main

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/config"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	_ "github.com/lib/pq"
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

	q := `SELECT id, key FROM "pgpkey" WHERE expiration is NULL`
	rows, err := db.Query(q)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		// Just a tiny bit of throttling, we're not in a rush...
		time.Sleep(10 * time.Millisecond)

		var (
			id  int64
			key string
		)
		if err := rows.Scan(&id, &key); err != nil {
			log.Fatal(err)
		}
		keys, err := openpgp.ReadArmoredKeyRing(strings.NewReader(key))
		if err != nil {
			log.Printf("Key ID %d: failed to read key material\n", id)
			continue
		}
		entity := keys[0]

		var sig *packet.Signature
		var pkey *packet.PublicKey

		ekey, found := entity.EncryptionKey(time.Now())
		if !found {
			// The key may have already expired, try to find a date manually
			if entity.PrimaryKey == nil {
				log.Printf("Key ID %d: no encryption key, no primary key\n", id)
				continue
			}
			ident := entity.PrimaryIdentity()
			if ident == nil {
				log.Printf("Key ID %d: no encryption key, no identity found\n", id)
				continue
			}
			if ident.SelfSignature == nil {
				log.Printf("Key ID %d: no encryption key, identity without self-signature\n", id)
				continue
			}
			pkey = entity.PrimaryKey
			sig = ident.SelfSignature
		} else {
			// We can rely on sig being non-nil and sane if entity.EncryptionKey() did not complain
			pkey = ekey.PublicKey
			sig = ekey.SelfSignature
		}
		var expiration *time.Time
		if sig.KeyLifetimeSecs != nil && *sig.KeyLifetimeSecs != 0 {
			e := pkey.CreationTime.Add(time.Duration(*sig.KeyLifetimeSecs) * time.Second)
			expiration = &e
		}
		if expiration != nil {
			log.Printf("Found date: %s", expiration)
			q := `UPDATE "pgpkey" SET expiration = $1 WHERE id = $2`
			_, err := db.Exec(q, expiration, id)
			if err != nil {
				log.Fatal(err)
			}
		} else if !found {
			log.Printf("Key ID %d: no encryption key, but no expiration date found\n", id)
		}
	}
	log.Println("Done.")
}
