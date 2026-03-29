package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const bcryptCost = 12

// HashPassword zahashuje heslo bcryptem.
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(b), err
}

// CheckPassword ověří heslo proti bcrypt hashi.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// HashToken vrací SHA-256 hex-string surového tokenu — ukládáme jen hash, ne token samotný.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// NewSession vytvoří novou session pro daného uživatele a vrátí surový token (pro cookie).
func NewSession(db *gorm.DB, userID uint, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	s := Session{
		TokenHash: HashToken(raw),
		UserID:    userID,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := db.Create(&s).Error; err != nil {
		return "", err
	}
	return raw, nil
}

// ValidateSession ověří token a vrátí přihlášeného uživatele. Vrátí chybu pokud je token neplatný nebo expirovaný.
func ValidateSession(db *gorm.DB, rawToken string) (*User, error) {
	var s Session
	err := db.Preload("User").
		Where("token_hash = ? AND expires_at > ?", HashToken(rawToken), time.Now()).
		First(&s).Error
	if err != nil {
		return nil, err
	}
	if !s.User.IsActive {
		return nil, gorm.ErrRecordNotFound
	}
	return &s.User, nil
}

// AnyUsers vrací true pokud existuje alespoň jeden uživatel v databázi.
func AnyUsers(db *gorm.DB) (bool, error) {
	var count int64
	err := db.Model(&User{}).Count(&count).Error
	return count > 0, err
}

// CleanExpiredSessions smaže fyzicky expirované session záznamy.
func CleanExpiredSessions(db *gorm.DB) {
	db.Where("expires_at < ?", time.Now()).Delete(&Session{})
}
