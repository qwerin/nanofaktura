package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

type AuthStatus struct {
	SignupAllowed bool `json:"signup_allowed"`
	HasUsers      bool `json:"has_users"`
	// SetupTokenRequired: the first registration must present setup_token
	// (NANOFAKTURA_SETUP_TOKEN is set and there is no user yet).
	SetupTokenRequired bool `json:"setup_token_required"`
}

type User struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type MeAccount struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role" enum:"owner,admin,accountant,member"`
}

type Me struct {
	User     User        `json:"user"`
	Accounts []MeAccount `json:"accounts" nullable:"false"`
}

type RegisterRequest struct {
	Email       string `json:"email" format:"email" maxLength:"254"`
	Name        string `json:"name" minLength:"1" maxLength:"200"`
	Password    string `json:"password" minLength:"8" maxLength:"72" doc:"8–72 characters, at most 72 bytes in UTF-8"`
	AccountName string `json:"account_name,omitempty" maxLength:"200" doc:"Name of the new account; required unless invitation_token is given"`
	// InvitationToken joins the invited account instead of creating one
	// (allowed even when signup is disabled; email must match the invitation).
	InvitationToken string `json:"invitation_token,omitempty" maxLength:"100"`
	// SetupToken is required for the first registration of an empty instance
	// when NANOFAKTURA_SETUP_TOKEN is set (see AuthStatus.setup_token_required).
	SetupToken string `json:"setup_token,omitempty" maxLength:"200"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type MePatch struct {
	Name            *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	Password        *string `json:"password,omitempty" minLength:"8" maxLength:"72" doc:"New password (at most 72 bytes); also revokes the user's API tokens and other sessions"`
	CurrentPassword *string `json:"current_password,omitempty" maxLength:"200" doc:"Required when changing password"`
}

// meWithCookie is the output of register/login: Me plus the session cookie.
type meWithCookie struct {
	SetCookie string `header:"Set-Cookie"`
	Body      Me
}

func (s *server) registerAuth(public, authed huma.API) {
	huma.Get(public, "/api/auth/status", s.authStatus)
	huma.Post(public, "/api/auth/register", s.register, status(http.StatusCreated))
	huma.Post(public, "/api/auth/login", s.login)
	huma.Post(public, "/api/auth/logout", s.logout, status(http.StatusNoContent))
	huma.Get(authed, "/api/auth/me", s.getMe)
	huma.Patch(authed, "/api/auth/me", s.patchMe)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func hasUsers(db *gorm.DB) (bool, error) {
	var n int64
	err := db.Model(&model.User{}).Limit(1).Count(&n).Error
	return n > 0, err
}

func (s *server) authStatus(ctx context.Context, _ *struct{}) (*Out[AuthStatus], error) {
	has, err := hasUsers(s.db.WithContext(ctx))
	if err != nil {
		return nil, dbErr(err, "users")
	}
	return &Out[AuthStatus]{Body: AuthStatus{SignupAllowed: !has || s.cfg.AllowSignup, HasUsers: has,
		SetupTokenRequired: !has && s.cfg.SetupToken != ""}}, nil
}

// checkPasswordBytes: bcrypt uses at most 72 bytes; longer passwords (e.g.
// with diacritics) are rejected rather than silently truncated.
func checkPasswordBytes(field, password string) error {
	if len(password) > auth.MaxPasswordBytes {
		return apiError(http.StatusUnprocessableEntity, CodePasswordTooLong, "validation failed",
			&huma.ErrorDetail{Location: "body." + field, Message: "password must be at most 72 bytes (fewer characters with diacritics)"})
	}
	return nil
}

func (s *server) register(ctx context.Context, in *struct{ Body RegisterRequest }) (*meWithCookie, error) {
	if err := s.rateLimit(s.limits.register, clientIP(ctx)); err != nil {
		return nil, err
	}
	if err := checkPasswordBytes("password", in.Body.Password); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(in.Body.Password)
	if err != nil {
		return nil, err
	}
	user := model.User{Email: normalizeEmail(in.Body.Email), Name: strings.TrimSpace(in.Body.Name), PasswordHash: hash}

	accountName := strings.TrimSpace(in.Body.AccountName)
	if in.Body.InvitationToken == "" && accountName == "" {
		return nil, invalid("account_name", "account_name is required")
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inv *model.Invitation
		if in.Body.InvitationToken != "" {
			var err error
			if inv, err = s.pendingInvitation(tx, in.Body.InvitationToken); err != nil {
				return err
			}
			if inv.Email != user.Email {
				return invalid("email", "email does not match the invitation")
			}
		} else {
			has, err := hasUsers(tx)
			if err != nil {
				return dbErr(err, "users")
			}
			if has && !s.cfg.AllowSignup {
				return apiError(http.StatusForbidden, CodeSignupDisabled, "signup is disabled")
			}
			if !has && s.cfg.SetupToken != "" &&
				subtle.ConstantTimeCompare([]byte(in.Body.SetupToken), []byte(s.cfg.SetupToken)) != 1 {
				return apiError(http.StatusForbidden, CodeSetupToken, "the first registration requires the setup token (NANOFAKTURA_SETUP_TOKEN)")
			}
		}
		if err := tx.Create(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return conflict(CodeEmailTaken, "email is already registered")
			}
			return dbErr(err, "user")
		}
		if inv != nil {
			return s.joinByInvitation(tx, inv, user.ID)
		}
		_, err := newAccount(tx, accountName, user.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, &user)
}

func (s *server) startSession(ctx context.Context, user *model.User) (*meWithCookie, error) {
	cookie, err := s.auth.CreateSession(ctx, user.ID)
	if err != nil {
		return nil, dbErr(err, "session")
	}
	me, err := s.me(ctx, user)
	if err != nil {
		return nil, err
	}
	return &meWithCookie{SetCookie: cookie.String(), Body: *me}, nil
}

type logoutOutput struct {
	SetCookie string `header:"Set-Cookie"`
}

func (s *server) logout(ctx context.Context, in *struct {
	Session string `cookie:"nf_session"`
}) (*logoutOutput, error) {
	if in.Session != "" {
		if err := s.auth.DeleteSession(ctx, in.Session); err != nil {
			return nil, dbErr(err, "session")
		}
	}
	return &logoutOutput{SetCookie: s.auth.ClearCookie(ctx).String()}, nil
}

func (s *server) me(ctx context.Context, user *model.User) (*Me, error) {
	var accounts []MeAccount
	err := s.db.WithContext(ctx).Table("accounts").
		Select("accounts.slug, accounts.name, memberships.role").
		Joins("JOIN memberships ON memberships.account_id = accounts.id").
		Where("memberships.user_id = ?", user.ID).
		Order("accounts.name").
		Scan(&accounts).Error
	if err != nil {
		return nil, dbErr(err, "accounts")
	}
	if accounts == nil {
		accounts = []MeAccount{}
	}
	return &Me{User: User{ID: user.ID, Email: user.Email, Name: user.Name}, Accounts: accounts}, nil
}

func (s *server) getMe(ctx context.Context, _ *struct{}) (*Out[Me], error) {
	me, err := s.me(ctx, auth.UserFrom(ctx))
	if err != nil {
		return nil, err
	}
	return &Out[Me]{Body: *me}, nil
}

func (s *server) patchMe(ctx context.Context, in *struct {
	Session string `cookie:"nf_session"`
	Body    MePatch
}) (*Out[Me], error) {
	user := *auth.UserFrom(ctx)
	if in.Body.Name != nil {
		user.Name = strings.TrimSpace(*in.Body.Name)
	}
	if in.Body.Password != nil {
		if err := checkPasswordBytes("password", *in.Body.Password); err != nil {
			return nil, err
		}
		key := strconv.FormatUint(uint64(user.ID), 10)
		if err := s.rateBlocked(s.limits.password, key); err != nil {
			return nil, err
		}
		if in.Body.CurrentPassword == nil || !auth.CheckPassword(user.PasswordHash, *in.Body.CurrentPassword) {
			s.rateFail(s.limits.password, key)
			return nil, apiError(http.StatusUnprocessableEntity, CodeWrongPassword, "validation failed",
				&huma.ErrorDetail{Location: "body.current_password", Message: "current password is incorrect"})
		}
		hash, err := auth.HashPassword(*in.Body.Password)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = hash
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&user).Error; err != nil {
			return dbErr(err, "user")
		}
		if in.Body.Password == nil {
			return nil
		}
		// a new password logs out every other browser (the current session
		// stays) and revokes the user's API tokens
		if err := auth.DeleteOtherSessions(tx, user.ID, in.Session); err != nil {
			return dbErr(err, "sessions")
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&model.APIToken{}).Error; err != nil {
			return dbErr(err, "tokens")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	me, err := s.me(ctx, &user)
	if err != nil {
		return nil, err
	}
	return &Out[Me]{Body: *me}, nil
}
