package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
)

// Out is the generic huma output: the body is the JSON response as-is.
type Out[T any] struct {
	Body T
}

// NoContent is the output of operations answering 204.
type NoContent struct{}

// status sets the operation's success status, e.g. huma.Post(g, "/x", h, status(201)).
func status(code int) func(*huma.Operation) {
	return func(o *huma.Operation) { o.DefaultStatus = code }
}

// scoped returns a query limited to the current account (from the {slug} path).
func (s *server) scoped(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Scopes(inAccount(ctx))
}

// inAccount is a GORM scope filtering by the current account; use it inside
// transactions: tx.Scopes(inAccount(ctx)).First(&inv, id).
func inAccount(ctx context.Context) func(*gorm.DB) *gorm.DB {
	id := auth.AccountFrom(ctx).ID
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(clause.Eq{Column: clause.Column{Table: clause.CurrentTable, Name: "account_id"}, Value: id})
	}
}

// apply copies *src into *dst when src is non-nil (PATCH semantics).
func apply[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

// dbErr maps a GORM error to an HTTP error: record not found → 404,
// unique violation → 409, anything else → 500. what names the resource.
func dbErr(err error, what string) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return notFound(what)
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return huma.Error409Conflict(what + " already exists")
	default:
		return huma.Error500InternalServerError("database error", err)
	}
}

// notFound is the 404 for missing and foreign records alike.
func notFound(what string) error {
	return huma.Error404NotFound(what + " not found")
}

// conflict is the 409 for actions not allowed in the current state.
func conflict(msg string) error {
	return huma.Error409Conflict(msg)
}

// invalid is a 422 pointing at one body field, e.g. invalid("iban", "invalid checksum").
func invalid(field, msg string) error {
	return huma.NewError(http.StatusUnprocessableEntity, "validation failed",
		&huma.ErrorDetail{Location: "body." + field, Message: msg})
}
