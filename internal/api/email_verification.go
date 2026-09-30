package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/model"
)

// E-mail verification (SPEC §3.2): proves the user receives mail at their
// address. Instance admin rights require a verified e-mail, so an account
// owner who invites (and registers) an admin's address gains nothing.

const emailVerificationTTL = 24 * time.Hour

const emailVerificationSubject = "Ověření e-mailu v NanoFaktuře"

const emailVerificationText = `Dobrý den,

pro dokončení ověření adresy {email} v NanoFaktuře klikněte na tento odkaz (platí 24 hodin):

{link}

Pokud jste o ověření nežádali, e-mail ignorujte.
`

type EmailVerificationInfo struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

// EmailVerificationConfirm is the body of POST /api/auth/verify-email (the
// token travels in the body, not the URL, so it does not end up in access logs).
type EmailVerificationConfirm struct {
	Token string `json:"token" minLength:"1" maxLength:"100"`
}

func (s *server) registerEmailVerification(public, authed huma.API) {
	huma.Post(authed, "/api/auth/me/verify-email", s.requestEmailVerification, status(http.StatusNoContent))
	huma.Post(public, "/api/auth/verify-email", s.confirmEmailVerification)
}

// requestEmailVerification sends the current user a verification link.
func (s *server) requestEmailVerification(ctx context.Context, _ *struct{}) (*NoContent, error) {
	user := auth.UserFrom(ctx)
	if user.EmailVerifiedAt != nil {
		return nil, conflict(CodeEmailVerified, "the e-mail address is already verified")
	}
	if err := s.sendEmailVerification(ctx, user); err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// sendEmailVerification creates a link for user (older unused links stop
// working) and e-mails it; limited to limits.verifyMail per user.
func (s *server) sendEmailVerification(ctx context.Context, user *model.User) error {
	if err := s.rateLimit(s.limits.verifyMail, strconv.FormatUint(uint64(user.ID), 10)); err != nil {
		return err
	}
	now := s.deps.Now()
	plain, hash := auth.NewSecret()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND used_at IS NULL", user.ID).Delete(&model.EmailVerification{}).Error; err != nil {
			return dbErr(err, "email verification")
		}
		v := model.EmailVerification{UserID: user.ID, TokenHash: hash, Email: user.Email, ExpiresAt: now.Add(emailVerificationTTL), CreatedAt: now}
		return dbErrOrNil(tx.Create(&v).Error, "email verification")
	})
	if err != nil {
		return err
	}
	vars := map[string]string{"email": user.Email, "link": s.publicURL() + "/verify-email/" + plain}
	msg := mail.Message{To: []string{user.Email}, Subject: emailVerificationSubject, Text: mail.Render(emailVerificationText, vars)}
	if err := s.deps.Mailer.Send(ctx, msg); err != nil {
		return huma.Error502BadGateway("failed to send the verification e-mail", err)
	}
	return nil
}

// emailVerificationByToken loads (and locks) a verification by its link token:
// unknown → 404, expired → 410; used links are returned (the caller answers 410).
func (s *server) emailVerificationByToken(tx *gorm.DB, token string) (*model.EmailVerification, error) {
	q := tx.Where("token_hash = ?", auth.HashSecret(token)).Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate})
	var v model.EmailVerification
	if err := q.First(&v).Error; err != nil {
		return nil, dbErr(err, "email verification")
	}
	if v.UsedAt == nil && !v.ExpiresAt.After(s.deps.Now()) {
		return nil, apiError(http.StatusGone, CodeVerificationExpired, "the verification link has expired; request a new one")
	}
	return &v, nil
}

// confirmEmailVerification marks the e-mail verified. A used link answers
// 410 (it cannot be replayed, e.g. after the address changed).
func (s *server) confirmEmailVerification(ctx context.Context, in *struct {
	Body EmailVerificationConfirm
}) (*Out[EmailVerificationInfo], error) {
	if err := s.rateLimit(s.limits.verifyLink, clientIP(ctx)); err != nil {
		return nil, err
	}
	var user model.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		v, err := s.emailVerificationByToken(tx, in.Body.Token)
		if err != nil {
			return err
		}
		if v.UsedAt != nil {
			return apiError(http.StatusGone, CodeVerificationExpired, "the verification link was already used")
		}
		now := s.deps.Now()
		if err := tx.Model(v).Update("used_at", now).Error; err != nil {
			return dbErr(err, "email verification")
		}
		if err := tx.First(&user, v.UserID).Error; err != nil {
			return dbErr(err, "user")
		}
		if !strings.EqualFold(user.Email, v.Email) { // the address changed since the link was sent
			return apiError(http.StatusGone, CodeVerificationExpired, "the verification link is no longer valid")
		}
		return markEmailVerified(tx, &user, now)
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "e-mail verified", "user_id", user.ID)
	return &Out[EmailVerificationInfo]{Body: EmailVerificationInfo{Email: user.Email, Verified: true}}, nil
}

// markEmailVerified sets users.email_verified_at (kept when already set).
func markEmailVerified(tx *gorm.DB, user *model.User, now time.Time) error {
	if user.EmailVerifiedAt != nil {
		return nil
	}
	user.EmailVerifiedAt = &now
	return dbErrOrNil(tx.Model(&model.User{}).Where("id = ? AND email_verified_at IS NULL", user.ID).
		Update("email_verified_at", now).Error, "user")
}
