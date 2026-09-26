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
	secure bool
}

func NewService(db *gorm.DB, now func() time.Time, secureCookies bool) *Service {
	return &Service{db: db, now: now, secure: secureCookies}
}

// CreateSession stores a new session for userID and returns the cookie to set.
func (s *Service) CreateSession(ctx context.Context, userID uint) (*http.Cookie, error) {
	plain, hash := newToken("")
	sess := model.Session{UserID: userID, TokenHash: hash, ExpiresAt: s.now().Add(SessionTTL)}
	if err := s.db.WithContext(ctx).Create(&sess).Error; err != nil {
		return nil, err
	}
	return s.cookie(plain, sess.ExpiresAt), nil
}

// DeleteSession removes the session identified by the cookie value (no-op if unknown).
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Where("token_hash = ?", hashToken(token)).Delete(&model.Session{}).Error
}

// ClearCookie returns a cookie that removes the session cookie in the browser.
func (s *Service) ClearCookie() *http.Cookie {
	c := s.cookie("", time.Time{})
	c.MaxAge = -1
	return c
}

func (s *Service) cookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookie, Value: value, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
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
		refreshed = s.cookie(token, sess.ExpiresAt)
	}
	return &u, refreshed, nil
}

// CreateAPIToken stores a new API token. The plaintext is returned only here.
func (s *Service) CreateAPIToken(ctx context.Context, userID uint, name string) (string, *model.APIToken, error) {
	plain, hash := newToken(APITokenPrefix)
	tok := model.APIToken{UserID: userID, Name: name, TokenHash: hash, Prefix: plain[:8]}
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
	var tok model.APIToken
	if err := db.Where("token_hash = ?", hashToken(plain)).First(&tok).Error; err != nil {
		return nil, notFoundAs(err, ErrInvalidCredentials)
	}
	var u model.User
	if err := db.First(&u, tok.UserID).Error; err != nil {
		return nil, notFoundAs(err, ErrInvalidCredentials)
	}
	if err := db.Model(&tok).Update("last_used_at", s.now()).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func notFoundAs(err, target error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return target
	}
	return err
}
