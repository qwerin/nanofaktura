package model

import "time"

type User struct {
	ID           uint
	Email        string `gorm:"not null;uniqueIndex"` // always lowercase
	Name         string `gorm:"not null"`
	PasswordHash string `gorm:"not null"`
	// TOTP second factor (SPEC §3.1): the base32 secret encrypted with
	// secret.Box; empty = TOTP off. TOTPPendingEnc holds a secret being set up
	// (not yet confirmed by a code), TOTPLastStep the last accepted time step
	// (a code is never accepted twice).
	TOTPSecretEnc  string
	TOTPPendingEnc string
	TOTPLastStep   int64
	// EmailVerifiedAt: the user proved they receive mail at Email (verification
	// link or password reset, SPEC §3.2); nil = unverified. Instance admin
	// rights require it.
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// EmailVerification is an e-mail verification link (SPEC §3.2).
type EmailVerification struct {
	ID        uint
	UserID    uint      `gorm:"not null;index"`
	TokenHash string    `gorm:"not null;uniqueIndex"` // hex sha256 of the token in the link
	Email     string    `gorm:"not null"`             // the address the link was sent to
	ExpiresAt time.Time `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

// PasswordReset is a "forgotten password" link (SPEC §3.1).
type PasswordReset struct {
	ID        uint
	UserID    uint      `gorm:"not null;index"`
	TokenHash string    `gorm:"not null;uniqueIndex"` // hex sha256 of the token in the link
	ExpiresAt time.Time `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

// RecoveryCode is a one-time backup code for the second factor.
type RecoveryCode struct {
	ID        uint
	UserID    uint   `gorm:"not null;index"`
	CodeHash  string `gorm:"not null"` // hex sha256 of the normalized code
	UsedAt    *time.Time
	CreatedAt time.Time
}

// WebAuthnCredential is a registered security key / passkey (second factor).
type WebAuthnCredential struct {
	ID           uint
	UserID       uint   `gorm:"not null;index"`
	Name         string `gorm:"not null"`
	CredentialID string `gorm:"not null;uniqueIndex"` // base64url of the credential ID
	Data         string `gorm:"not null"`             // JSON of webauthn.Credential (public key, sign count, flags)
	LastUsedAt   *time.Time
	CreatedAt    time.Time
}

// AuthChallenge purposes.
const (
	ChallengeLogin            = "login"             // password verified, second factor pending
	ChallengeWebAuthnRegister = "webauthn_register" // security key registration in progress
)

// AuthChallenge is a short-lived multi-step authentication state identified
// by a random token held by the client.
type AuthChallenge struct {
	ID              uint
	UserID          uint      `gorm:"not null;index"`
	Purpose         string    `gorm:"not null"`
	TokenHash       string    `gorm:"not null;uniqueIndex"`
	WebAuthnSession string    // JSON of webauthn.SessionData of the running ceremony
	Attempts        int       `gorm:"not null;default:0"`
	ExpiresAt       time.Time `gorm:"not null"`
	CreatedAt       time.Time
}

type Membership struct {
	ID        uint
	UserID    uint   `gorm:"not null;uniqueIndex:idx_memberships_user_account"`
	AccountID uint   `gorm:"not null;uniqueIndex:idx_memberships_user_account;index"`
	Role      string `gorm:"not null"`
	CreatedAt time.Time
}

type Session struct {
	ID        uint
	UserID    uint   `gorm:"not null;index"`
	TokenHash string `gorm:"not null;uniqueIndex"` // hex sha256 of the cookie value
	ExpiresAt time.Time
	CreatedAt time.Time
}

type APIToken struct {
	ID         uint
	UserID     uint   `gorm:"not null;index"`
	Name       string `gorm:"not null"`
	TokenHash  string `gorm:"not null;uniqueIndex"` // hex sha256 of the plaintext token
	Prefix     string `gorm:"not null"`             // first 8 chars, for display
	LastUsedAt *time.Time
	ExpiresAt  *time.Time `gorm:"index"` // nil = never expires
	CreatedAt  time.Time
}
