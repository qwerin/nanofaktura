package api

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/model"
)

// newWebAuthn configures the relying party from NANOFAKTURA_PUBLIC_URL (RP ID =
// its hostname) plus NANOFAKTURA_WEBAUTHN_ORIGINS (SPEC §3.1).
func newWebAuthn(publicURL string, extraOrigins []string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("webauthn: invalid NANOFAKTURA_PUBLIC_URL %q", publicURL)
	}
	timeout := webauthn.TimeoutConfig{Enforce: true, Timeout: challengeTTL, TimeoutUVD: challengeTTL}
	return webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "NanoFaktura",
		RPOrigins:     append([]string{u.Scheme + "://" + u.Host}, extraOrigins...),
		Timeouts:      webauthn.TimeoutsConfig{Login: timeout, Registration: timeout},
	})
}

// waUser adapts a user and their stored credentials to webauthn.User.
type waUser struct {
	user  *model.User
	rows  []model.WebAuthnCredential
	creds []webauthn.Credential // parallel to rows
}

// webAuthnUserID is the stable user handle (8 bytes of the user ID; keys are
// used as a second factor only, so the handle never identifies the user).
func webAuthnUserID(id uint) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(id))
}

func (u *waUser) WebAuthnID() []byte                         { return webAuthnUserID(u.user.ID) }
func (u *waUser) WebAuthnName() string                       { return u.user.Email }
func (u *waUser) WebAuthnDisplayName() string                { return u.user.Name }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// loadWAUser loads the security keys of user using db (may be a transaction).
func loadWAUser(db *gorm.DB, user *model.User) (*waUser, error) {
	u := &waUser{user: user}
	if err := db.Where("user_id = ?", user.ID).Order("id").Find(&u.rows).Error; err != nil {
		return nil, dbErr(err, "security keys")
	}
	for _, r := range u.rows {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(r.Data), &c); err != nil {
			return nil, fmt.Errorf("security key %d: %w", r.ID, err)
		}
		u.creds = append(u.creds, c)
	}
	return u, nil
}

// row returns the stored row of credential c (matched by ID).
func (u *waUser) row(c *webauthn.Credential) *model.WebAuthnCredential {
	id := credentialKey(c.ID)
	for i := range u.rows {
		if u.rows[i].CredentialID == id {
			return &u.rows[i]
		}
	}
	return nil
}

func credentialKey(id []byte) string { return base64.RawURLEncoding.EncodeToString(id) }

func encodeCredential(c *webauthn.Credential) (string, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

func encodeSession(sd *webauthn.SessionData) (string, error) {
	b, err := json.Marshal(sd)
	return string(b), err
}

func decodeSession(s string) (webauthn.SessionData, error) {
	var sd webauthn.SessionData
	err := json.Unmarshal([]byte(s), &sd)
	return sd, err
}

// WebAuthnKey is a registered security key in API outputs.
type WebAuthnKey struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func toWebAuthnKey(m *model.WebAuthnCredential) WebAuthnKey {
	return WebAuthnKey{ID: m.ID, Name: m.Name, CreatedAt: m.CreatedAt, LastUsedAt: m.LastUsedAt}
}
