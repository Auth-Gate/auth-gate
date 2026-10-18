package client

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const keyPrefix = "ag_"

// NewAccessKey returns a fresh raw key and the hash that is stored for it.
func NewAccessKey() (raw string, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("client: crypto/rand failed: " + err.Error())
	}
	raw = keyPrefix + base64.RawURLEncoding.EncodeToString(b)
	return raw, HashKey(raw)
}

// HashKey returns hex(sha256(raw)), the lookup key of an access key item.
func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
