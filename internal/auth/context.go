package auth

import (
	"context"

	"github.com/qwerin/nanofaktura/internal/model"
)

type ctxKey int

const (
	userKey ctxKey = iota
	accountKey
	roleKey
)

// UserFrom returns the authenticated user. It is non-nil in every handler
// registered behind RequireUser (and therefore RequireAccount).
func UserFrom(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

// AccountFrom returns the account resolved from the {slug} path parameter.
// It is non-nil in every handler registered behind RequireAccount.
func AccountFrom(ctx context.Context) *model.Account {
	a, _ := ctx.Value(accountKey).(*model.Account)
	return a
}

// WithAccount returns ctx acting in acc with role — for background jobs that
// reuse account-scoped code outside an HTTP request (no user is set).
func WithAccount(ctx context.Context, acc *model.Account, role string) context.Context {
	return context.WithValue(context.WithValue(ctx, accountKey, acc), roleKey, role)
}

// RoleFrom returns the current user's role in AccountFrom(ctx).
func RoleFrom(ctx context.Context) string {
	r, _ := ctx.Value(roleKey).(string)
	return r
}
