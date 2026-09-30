package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/model"
)

const (
	passwordResetTTL      = time.Hour
	passwordResetThrottle = 5 * time.Minute // at most one e-mail per user this often
)

const passwordResetSubject = "Obnova hesla do NanoFaktury"

const passwordResetText = `Dobrý den,

někdo (nejspíš vy) požádal o obnovu hesla k účtu {email} v NanoFaktuře.
Nové heslo si nastavíte na tomto odkazu (platí 1 hodinu):

{link}

Pokud jste o obnovu nežádali, e-mail ignorujte — vaše heslo zůstává beze změny.
`

type PasswordResetRequest struct {
	Email string `json:"email" maxLength:"254"`
}

type PasswordResetInfo struct {
	Email     string `json:"email"`
	TwoFactor bool   `json:"two_factor" doc:"The user has a second factor; it is still required after the reset"`
}

type PasswordResetConfirm struct {
	Password string `json:"password" minLength:"8" maxLength:"72"`
}

func (s *server) registerPasswordReset(public huma.API) {
	huma.Post(public, "/api/auth/password-reset", s.requestPasswordReset, status(http.StatusNoContent))
	huma.Get(public, "/api/auth/password-reset/{token}", s.getPasswordReset)
	huma.Post(public, "/api/auth/password-reset/{token}", s.confirmPasswordReset, status(http.StatusNoContent))
}

// requestPasswordReset always answers 204 so it does not reveal registered
// e-mails; mail errors are only logged for the same reason.
func (s *server) requestPasswordReset(ctx context.Context, in *struct{ Body PasswordResetRequest }) (*NoContent, error) {
	if err := s.rateLimit(s.limits.resetMail, "ip:"+clientIP(ctx)); err != nil {
		return nil, err
	}
	if err := s.rateLimit(s.limits.resetMail, "email:"+normalizeEmail(in.Body.Email)); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	var user model.User
	if err := db.Where("email = ?", normalizeEmail(in.Body.Email)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &NoContent{}, nil
		}
		return nil, dbErr(err, "user")
	}
	now := s.deps.Now()
	var recent int64
	if err := db.Model(&model.PasswordReset{}).
		Where("user_id = ? AND used_at IS NULL AND created_at > ?", user.ID, now.Add(-passwordResetThrottle)).
		Count(&recent).Error; err != nil {
		return nil, dbErr(err, "password reset")
	}
	if recent > 0 {
		return &NoContent{}, nil
	}
	plain, hash := auth.NewSecret()
	reset := model.PasswordReset{UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(passwordResetTTL), CreatedAt: now}
	if err := db.Create(&reset).Error; err != nil {
		return nil, dbErr(err, "password reset")
	}
	vars := map[string]string{"email": user.Email, "link": s.publicURL() + "/reset-password/" + plain}
	msg := mail.Message{To: []string{user.Email}, Subject: passwordResetSubject, Text: mail.Render(passwordResetText, vars)}
	if err := s.deps.Mailer.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "password reset e-mail failed", "user_id", user.ID, "err", err)
	}
	return &NoContent{}, nil
}

// validPasswordReset loads a usable reset by its link token: unknown → 404,
// used or expired → 410.
func (s *server) validPasswordReset(tx *gorm.DB, token string, lock bool) (*model.PasswordReset, error) {
	q := tx.Where("token_hash = ?", auth.HashSecret(token))
	if lock {
		q = q.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate})
	}
	var reset model.PasswordReset
	if err := q.First(&reset).Error; err != nil {
		return nil, dbErr(err, "password reset")
	}
	if reset.UsedAt != nil || !reset.ExpiresAt.After(s.deps.Now()) {
		return nil, apiError(http.StatusGone, CodeResetExpired, "password reset link has expired or was already used")
	}
	return &reset, nil
}

func (s *server) getPasswordReset(ctx context.Context, in *struct {
	Token string `path:"token" maxLength:"100"`
}) (*Out[PasswordResetInfo], error) {
	if err := s.rateLimit(s.limits.resetLink, clientIP(ctx)); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	reset, err := s.validPasswordReset(db, in.Token, false)
	if err != nil {
		return nil, err
	}
	var user model.User
	if err := db.First(&user, reset.UserID).Error; err != nil {
		return nil, dbErr(err, "password reset")
	}
	has, err := hasSecondFactor(db, &user)
	if err != nil {
		return nil, err
	}
	return &Out[PasswordResetInfo]{Body: PasswordResetInfo{Email: user.Email, TwoFactor: has}}, nil
}

func (s *server) confirmPasswordReset(ctx context.Context, in *struct {
	Token string `path:"token" maxLength:"100"`
	Body  PasswordResetConfirm
}) (*NoContent, error) {
	if err := s.rateLimit(s.limits.resetLink, clientIP(ctx)); err != nil {
		return nil, err
	}
	if err := checkPasswordBytes("password", in.Body.Password); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(in.Body.Password)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reset, err := s.validPasswordReset(tx, in.Token, true)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", reset.UserID).Update("password_hash", hash).Error; err != nil {
			return dbErr(err, "user")
		}
		// every browser is logged out and no other link works any more
		if err := auth.DeleteOtherSessions(tx, reset.UserID, ""); err != nil {
			return dbErr(err, "sessions")
		}
		if err := tx.Where("user_id = ? AND id <> ?", reset.UserID, reset.ID).Delete(&model.PasswordReset{}).Error; err != nil {
			return dbErr(err, "password reset")
		}
		if err := tx.Model(reset).Update("used_at", s.deps.Now()).Error; err != nil {
			return dbErr(err, "password reset")
		}
		return dbErrOrNil(tx.Where("user_id = ?", reset.UserID).Delete(&model.AuthChallenge{}).Error, "challenge")
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// RunAuthCleanup is the "auth-cleanup" job: removes expired sessions, login
// challenges, password reset links, expired API tokens and stale invitations.
func (s *server) RunAuthCleanup(ctx context.Context, now time.Time) error {
	db := s.db.WithContext(ctx)
	if err := auth.Cleanup(db, now); err != nil { // sessions, 2FA challenges, password reset links
		return err
	}
	if err := auth.DeleteExpired(db, now); err != nil { // expiring API tokens
		return err
	}
	return db.Where("accepted_at IS NULL AND expires_at <= ?", now.Add(-InvitationTTL)).Delete(&model.Invitation{}).Error
}
