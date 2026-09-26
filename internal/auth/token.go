package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// newToken returns prefix + 32 random bytes (base64url) and its storage hash.
func newToken(prefix string) (plain, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails (crypto/rand panics on failure)
	plain = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, hashToken(plain)
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
