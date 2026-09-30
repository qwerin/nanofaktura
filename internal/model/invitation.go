package model

import "time"

// Invitation invites an e-mail address to join an account with a role.
// Pending = AcceptedAt nil and ExpiresAt in the future.
type Invitation struct {
	ID         uint
	AccountID  uint      `gorm:"not null;index"`
	Email      string    `gorm:"not null"` // lowercase
	Role       string    `gorm:"not null"`
	TokenHash  string    `gorm:"not null;uniqueIndex"` // hex sha256 of the token in the link
	TokenEnc   string    // the token encrypted with secret.Box (link shown to managers, resend)
	InvitedBy  uint      `gorm:"not null"` // user ID
	ExpiresAt  time.Time `gorm:"not null"`
	AcceptedAt *time.Time
	CreatedAt  time.Time
}
