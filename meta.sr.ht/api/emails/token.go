package emails

import (
	"crypto/rand"
	"encoding/base64"
)

// Generates a short, unique token
func GenToken() string {
	var seed [18]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(seed[:])
}
