package model

import "time"

type User struct {
	ID           uint
	Email        string `gorm:"not null;uniqueIndex"` // always lowercase
	Name         string `gorm:"not null"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
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
	CreatedAt  time.Time
}
