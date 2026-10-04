package model

import "time"

// OAuthClient is an OAuth client registered dynamically (RFC 7591), e.g. an
// AI assistant connecting to the MCP endpoint (SPEC §7.17).
type OAuthClient struct {
	ID           uint
	ClientID     string   `gorm:"not null;uniqueIndex"`
	SecretHash   string   // hex sha256 of the client secret; "" = public client (PKCE only)
	Name         string   `gorm:"not null"`
	RedirectURIs []string `gorm:"serializer:json"`
	CreatedAt    time.Time
}

// OAuthCode is a one-time authorization code waiting to be exchanged for tokens.
type OAuthCode struct {
	ID            uint
	CodeHash      string `gorm:"not null;uniqueIndex"`
	ClientID      string `gorm:"not null;index"`
	UserID        uint   `gorm:"not null;index"`
	RedirectURI   string `gorm:"not null"`
	CodeChallenge string `gorm:"not null"` // PKCE S256
	Resource      string
	ExpiresAt     time.Time `gorm:"not null;index"`
	CreatedAt     time.Time
}

// OAuthGrant is a user's consent to a client ("connected application"). It
// holds the rotating refresh token; its access tokens are APITokens with
// OAuthGrantID set. Deleting the grant disconnects the application.
type OAuthGrant struct {
	ID          uint
	UserID      uint   `gorm:"not null;index"`
	ClientID    string `gorm:"not null;index"`
	ClientName  string `gorm:"not null"`
	RefreshHash string `gorm:"not null;uniqueIndex"`
	Resource    string
	ExpiresAt   time.Time `gorm:"not null;index"` // of the refresh token (sliding)
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (OAuthClient) TableName() string { return "oauth_clients" }
func (OAuthCode) TableName() string   { return "oauth_codes" }
func (OAuthGrant) TableName() string  { return "oauth_grants" }
