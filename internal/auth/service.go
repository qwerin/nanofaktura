// Package auth implements password hashing, cookie sessions, API tokens and the
// huma middlewares that authenticate requests and resolve the current account.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/model"
)

const (
	// SessionCookie is the name of the session cookie.
	SessionCookie = "nf_session"
	// SessionTTL is the sliding session lifetime.
	SessionTTL = 30 * 24 * time.Hour
	// sessionRefresh: the expiry is pushed forward at most this often.
	sessionRefresh = 24 * time.Hour
	// APITokenPrefix starts every API token.
	APITokenPrefix = "nf_"
)

// ErrInvalidCredentials means an unknown, expired or revoked session/token.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service manages sessions and API tokens.
type Service struct {
	db     *gorm.DB
	now    func() time.Time
	secure func(context.Context) bool
}

// NewService: secure decides per request whether the session cookie gets the
// Secure attribute (nil = never).
func NewService(db *gorm.DB, now func() time.Time, secure func(context.Context) bool) *Service {
	if secure == nil {
		secure = func(context.Context) bool { return false }
	}
	return &Service{db: db, now: now, secure: secure}
}

// CreateSession stores a new session for userID and returns the cookie to set.
func (s *Service) CreateSession(ctx context.Context, userID uint) (*http.Cookie, error) {
	plain, hash := newToken("")
	sess := model.Session{UserID: userID, TokenHash: hash, ExpiresAt: s.now().Add(SessionTTL)}
	if err := s.db.WithContext(ctx).Create(&sess).Error; err != nil {
		return nil, err
	}
	return s.cookie(ctx, plain, sess.ExpiresAt), nil
}

// DeleteSession removes the session identified by the cookie value (no-op if unknown).
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Where("token_hash = ?", hashToken(token)).Delete(&model.Session{}).Error
}

// DeleteOtherSessions removes every session of userID except the one
// identified by keepToken (the cookie value; empty = remove all) using db
// (may be a transaction).
func DeleteOtherSessions(db *gorm.DB, userID uint, keepToken string) error {
	q := db.Where("user_id = ?", userID)
	if keepToken != "" {
		q = q.Where("token_hash <> ?", hashToken(keepToken))
	}
	return q.Delete(&model.Session{}).Error
}

// ClearCookie returns a cookie that removes the session cookie in the browser.
func (s *Service) ClearCookie(ctx context.Context) *http.Cookie {
	c := s.cookie(ctx, "", time.Time{})
	c.MaxAge = -1
	return c
}

func (s *Service) cookie(ctx context.Context, value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookie, Value: value, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.secure(ctx), SameSite: http.SameSiteLaxMode,
	}
}

// userBySession resolves a session cookie value. When the session is older
// than a day its expiry slides forward and the refreshed cookie is returned.
func (s *Service) userBySession(ctx context.Context, token string) (*model.User, *http.Cookie, error) {
	db := s.db.WithContext(ctx)
	now := s.now()
	var sess model.Session
	err := db.Where("token_hash = ? AND expires_at > ?", hashToken(token), now).First(&sess).Error
	if err != nil {
		return nil, nil, notFoundAs(err, ErrInvalidCredentials)
	}
	var u model.User
	if err := db.First(&u, sess.UserID).Error; err != nil {
		return nil, nil, notFoundAs(err, ErrInvalidCredentials)
	}
	var refreshed *http.Cookie
	if sess.ExpiresAt.Sub(now) < SessionTTL-sessionRefresh {
		sess.ExpiresAt = now.Add(SessionTTL)
		if err := db.Model(&sess).Update("expires_at", sess.ExpiresAt).Error; err != nil {
			return nil, nil, err
		}
		refreshed = s.cookie(ctx, token, sess.ExpiresAt)
	}
	return &u, refreshed, nil
}

// CreateAPIToken stores a new API token (expiresAt nil = no expiry). The
// plaintext is returned only here.
func (s *Service) CreateAPIToken(ctx context.Context, userID uint, name string, expiresAt *time.Time) (string, *model.APIToken, error) {
	plain, hash := newToken(APITokenPrefix)
	tok := model.APIToken{UserID: userID, Name: name, TokenHash: hash, Prefix: plain[:8], ExpiresAt: expiresAt}
	if err := s.db.WithContext(ctx).Create(&tok).Error; err != nil {
		return "", nil, err
	}
	return plain, &tok, nil
}

func (s *Service) userByAPIToken(ctx context.Context, plain string) (*model.User, error) {
	if !strings.HasPrefix(plain, APITokenPrefix) {
		return nil, ErrInvalidCredentials
	}
	db := s.db.WithContext(ctx)
	now := s.now()
	var tok model.APIToken
	if err := db.Where("token_hash = ? AND (expires_at IS NULL OR expires_at > ?)", hashToken(plain), now).First(&tok).Error; err != nil {
		return nil, notFoundAs(err, ErrInvalidCredentials)
	}
	var u model.User
	if err := db.First(&u, tok.UserID).Error; err != nil {
		return nil, notFoundAs(err, ErrInvalidCredentials)
	}
	// last use is recorded with minute precision (no write on every request)
	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) >= time.Minute {
		if err := db.Model(&tok).Update("last_used_at", now).Error; err != nil {
			return nil, err
		}
	}
	return &u, nil
}

// DeleteExpired removes expired sessions and API tokens (cleanup job).
func DeleteExpired(db *gorm.DB, now time.Time) error {
	if err := db.Where("expires_at <= ?", now).Delete(&model.Session{}).Error; err != nil {
		return err
	}
	return db.Where("expires_at IS NOT NULL AND expires_at <= ?", now).Delete(&model.APIToken{}).Error
}

func notFoundAs(err, target error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return target
	}
	return err
}

// ResetSecondFactor turns off every second factor of userID (TOTP, security
// keys, recovery codes, pending logins) using db (may be a transaction).
// Used by the "nanofaktura user reset-2fa" command for a locked-out user.
func ResetSecondFactor(db *gorm.DB, userID uint) error {
	err := db.Model(&model.User{}).Where("id = ?", userID).
		Updates(map[string]any{"totp_secret_enc": "", "totp_pending_enc": "", "totp_last_step": 0}).Error
	if err != nil {
		return err
	}
	for _, m := range []any{&model.WebAuthnCredential{}, &model.RecoveryCode{}, &model.AuthChallenge{}} {
		if err := db.Where("user_id = ?", userID).Delete(m).Error; err != nil {
			return err
		}
	}
	return nil
}

// Cleanup deletes expired sessions, challenges, password reset and e-mail verification links.
func Cleanup(db *gorm.DB, now time.Time) error {
	if err := db.Where("expires_at <= ?", now).Delete(&model.Session{}).Error; err != nil {
		return err
	}
	if err := db.Where("expires_at <= ?", now).Delete(&model.AuthChallenge{}).Error; err != nil {
		return err
	}
	// used links are kept a day so a second click still says "already used"
	if err := db.Where("expires_at <= ?", now.Add(-24*time.Hour)).Delete(&model.PasswordReset{}).Error; err != nil {
		return err
	}
	return db.Where("expires_at <= ?", now.Add(-24*time.Hour)).Delete(&model.EmailVerification{}).Error
}
