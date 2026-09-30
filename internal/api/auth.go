package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

type AuthStatus struct {
	SignupAllowed bool `json:"signup_allowed"`
	HasUsers      bool `json:"has_users"`
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
	Password    string `json:"password" minLength:"8" maxLength:"72"`
	AccountName string `json:"account_name,omitempty" maxLength:"200" doc:"Name of the new account; required unless invitation_token is given"`
	// InvitationToken joins the invited account instead of creating one
	// (allowed even when signup is disabled; email must match the invitation).
	InvitationToken string `json:"invitation_token,omitempty" maxLength:"100"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type MePatch struct {
	Name            *string `json:"name,omitempty" minLength:"1" maxLength:"200"`
	Password        *string `json:"password,omitempty" minLength:"8" maxLength:"72"`
	CurrentPassword *string `json:"current_password,omitempty" doc:"Required when changing password"`
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
	return &Out[AuthStatus]{Body: AuthStatus{SignupAllowed: !has || s.cfg.AllowSignup, HasUsers: has}}, nil
}

func (s *server) register(ctx context.Context, in *struct{ Body RegisterRequest }) (*meWithCookie, error) {
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
	return &logoutOutput{SetCookie: s.auth.ClearCookie().String()}, nil
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
		if in.Body.CurrentPassword == nil || !auth.CheckPassword(user.PasswordHash, *in.Body.CurrentPassword) {
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
		// a new password logs out every other browser; the current session stays
		if err := auth.DeleteOtherSessions(tx, user.ID, in.Session); err != nil {
			return dbErr(err, "sessions")
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
