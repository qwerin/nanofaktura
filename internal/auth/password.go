package auth

import "golang.org/x/crypto/bcrypt"

// MinPasswordLength is the minimum accepted password length.
const MinPasswordLength = 8

// MaxPasswordBytes is bcrypt's input limit; longer passwords are rejected.
const MaxPasswordBytes = 72

// dummyHash is compared against when a user does not exist, so login timing
// does not reveal which emails are registered.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("nanofaktura-dummy"), bcrypt.DefaultCost)

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword reports whether password matches hash. An empty hash is
// treated as "no such user" and still costs one bcrypt comparison.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
