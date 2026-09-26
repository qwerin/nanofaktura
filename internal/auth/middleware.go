package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/nanofaktura/internal/model"
)

// Middleware is a huma router middleware.
type Middleware = func(ctx huma.Context, next func(huma.Context))

// RequireUser authenticates the request via `Authorization: Bearer nf_…` or the
// nf_session cookie (bearer wins when both are present) and stores the user in
// the context (see UserFrom). Unauthenticated requests get 401.
func (s *Service) RequireUser(api huma.API) Middleware {
	return func(ctx huma.Context, next func(huma.Context)) {
		u, err := s.authenticate(ctx)
		if err != nil {
			if errors.Is(err, ErrInvalidCredentials) {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "authentication required")
			} else {
				_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "authentication failed", err)
			}
			return
		}
		next(huma.WithValue(ctx, userKey, u))
	}
}

func (s *Service) authenticate(ctx huma.Context) (*model.User, error) {
	if h := ctx.Header("Authorization"); h != "" {
		token, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			return nil, ErrInvalidCredentials
		}
		return s.userByAPIToken(ctx.Context(), token)
	}
	c, err := huma.ReadCookie(ctx, SessionCookie)
	if err != nil || c.Value == "" {
		return nil, ErrInvalidCredentials
	}
	u, refreshed, err := s.userBySession(ctx.Context(), c.Value)
	if err != nil {
		return nil, err
	}
	if refreshed != nil {
		ctx.AppendHeader("Set-Cookie", refreshed.String())
	}
	return u, nil
}

// RequireAccount resolves the {slug} path parameter to an account the current
// user is a member of and stores it with the role in the context (see
// AccountFrom, RoleFrom). Must run after RequireUser. Non-members get 404 so
// the existence of foreign accounts is not revealed. Members whose role is not
// allowed by the operation (see Allow) get 403.
func (s *Service) RequireAccount(api huma.API) Middleware {
	return func(ctx huma.Context, next func(huma.Context)) {
		acc, role, err := s.membership(ctx.Context(), UserFrom(ctx.Context()), ctx.Param("slug"))
		if err != nil {
			if errors.Is(err, ErrInvalidCredentials) {
				_ = huma.WriteErr(api, ctx, http.StatusNotFound, "account not found")
			} else {
				_ = huma.WriteErr(api, ctx, http.StatusInternalServerError, "account lookup failed", err)
			}
			return
		}
		if op := ctx.Operation(); !roleAllowed(op, role) {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, forbiddenMsg(role, AllowedRoles(op)))
			return
		}
		ctx = huma.WithValue(ctx, accountKey, acc)
		next(huma.WithValue(ctx, roleKey, role))
	}
}

func (s *Service) membership(ctx context.Context, u *model.User, slug string) (*model.Account, string, error) {
	if u == nil {
		return nil, "", ErrInvalidCredentials
	}
	db := s.db.WithContext(ctx)
	var acc model.Account
	if err := db.Where("slug = ?", slug).First(&acc).Error; err != nil {
		return nil, "", notFoundAs(err, ErrInvalidCredentials)
	}
	var m model.Membership
	if err := db.Where("user_id = ? AND account_id = ?", u.ID, acc.ID).First(&m).Error; err != nil {
		return nil, "", notFoundAs(err, ErrInvalidCredentials)
	}
	return &acc, m.Role, nil
}
