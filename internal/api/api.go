// Package api registers the HTTP API (huma v2 on chi).
//
// # Route groups and authentication
//
// New builds three huma groups; every operation is registered into exactly one:
//
//   - public  — no authentication (health, auth status, register, login, logout).
//   - authed  — auth.RequireUser middleware: 401 unless the request carries a valid
//     nf_session cookie or `Authorization: Bearer nf_…` token; auth.UserFrom(ctx)
//     is then non-nil.
//   - account — authed + prefix "/api/accounts/{slug}" + auth.RequireAccount: the
//     slug is resolved to an account the user is a member of (non-member → 404);
//     auth.AccountFrom(ctx) and auth.RoleFrom(ctx) are then set. The {slug} path
//     parameter is added to the OpenAPI operation automatically, handler inputs
//     do not declare it.
//
// Membership is thus enforced centrally by the group, never per handler.
//
// # Adding a resource
//
// Create <resource>.go with a `func (s *server) register<Resource>(g huma.API)`
// method that calls huma.Get/Post/Patch/Delete on g with paths relative to the
// group (e.g. "/subjects/{id}"), and call it from New. Query domain data only
// through s.scoped(ctx) or tx.Scopes(inAccount(ctx)) so every query is filtered
// by account_id. DTOs live next to the handlers (see accounts.go for the
// naming: Resource, ResourceCreate, ResourcePatch, toResource).
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/ares"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/numbering"
)

// Deps holds swappable dependencies (tests replace them).
type Deps struct {
	Now  func() time.Time // defaults to time.Now
	ARES ARES             // defaults to ares.New(cfg.AresURL)

	// NextNumber assigns the next document number of docType for a document
	// issued on issuedOn ("YYYY-MM-DD") inside the creating transaction tx.
	// Defaults to numbering.Next.
	NextNumber func(tx *gorm.DB, accountID uint, docType, issuedOn string) (string, error)
}

// ARES looks up subjects in the Czech business register (see internal/ares
// for the error contract: ares.ErrInvalidICO, ares.ErrNotFound, other = unavailable).
type ARES interface {
	Lookup(ctx context.Context, ico string) (*ares.Result, error)
}

type server struct {
	db   *gorm.DB
	cfg  config.Config
	deps Deps
	auth *auth.Service
}

// New builds the API router. db may be nil when only the OpenAPI document is needed.
func New(db *gorm.DB, cfg config.Config, deps Deps) (http.Handler, huma.API) {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.ARES == nil {
		deps.ARES = ares.New(cfg.AresURL)
	}
	if deps.NextNumber == nil {
		deps.NextNumber = numbering.Next
	}
	s := &server{db: db, cfg: cfg, deps: deps, auth: auth.NewService(db, deps.Now, cfg.SecureCookies)}

	router := chi.NewRouter()
	hc := huma.DefaultConfig("NanoFaktura API", "1.0.0")
	hc.OpenAPIPath = "/api/openapi"
	hc.DocsPath = "/api/docs"
	hc.SchemasPath = "/api/schemas"
	hc.CreateHooks = nil // no "$schema" field in response bodies
	hc.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"session": {Type: "apiKey", In: "cookie", Name: auth.SessionCookie},
		"bearer":  {Type: "http", Scheme: "bearer"},
	}
	api := humachi.New(router, hc)

	public := huma.NewGroup(api)

	authed := huma.NewGroup(api)
	authed.UseMiddleware(s.auth.RequireUser(api))
	authed.UseSimpleModifier(func(o *huma.Operation) {
		o.Security = []map[string][]string{{"session": {}}, {"bearer": {}}}
	})

	account := huma.NewGroup(authed, "/api/accounts/{slug}")
	account.UseMiddleware(s.auth.RequireAccount(api))
	account.UseSimpleModifier(addSlugParam)

	registerHealth(public)
	s.registerAuth(public, authed)
	s.registerTokens(authed)
	s.registerAccounts(authed, account)
	s.registerAres(authed)
	s.registerBankAccounts(account)
	s.registerNumberFormats(account)
	s.registerSubjects(account)
	s.registerInvoices(account)
	s.registerInvoiceActions(account)
	s.registerPayments(account)
	s.registerDashboard(account)

	return router, api
}

// addSlugParam documents the {slug} path parameter of account-scoped operations.
func addSlugParam(o *huma.Operation) {
	for _, p := range o.Parameters {
		if p.In == "path" && p.Name == "slug" {
			return
		}
	}
	o.Parameters = append([]*huma.Param{{
		Name: "slug", In: "path", Required: true, Schema: &huma.Schema{Type: "string"},
		Description: "Account slug",
	}}, o.Parameters...)
}
