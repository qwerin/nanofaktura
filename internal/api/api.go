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
// Roles likewise: account operations declare the allowed roles at
// registration (auth.ForEditors, auth.ForManagers, auth.Allow(...)) and
// RequireAccount answers 403 to other roles; undeclared = every member.
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
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/ares"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/bankimport"
	"github.com/qwerin/nanofaktura/internal/cnb"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/numbering"
	"github.com/qwerin/nanofaktura/internal/secret"
	"github.com/qwerin/nanofaktura/internal/storage"
	"github.com/qwerin/nanofaktura/internal/vatreg"
	"github.com/qwerin/nanofaktura/internal/vies"
)

// Deps holds swappable dependencies (tests replace them).
type Deps struct {
	Now  func() time.Time // defaults to time.Now
	ARES ARES             // defaults to ares.New(cfg.AresURL)

	// NextNumber assigns the next document number of docType for a document
	// issued on issuedOn ("YYYY-MM-DD") inside the creating transaction tx.
	// Defaults to numbering.Next.
	NextNumber func(tx *gorm.DB, accountID uint, docType, issuedOn string) (string, error)

	// Mailer sends e-mails. Defaults to SMTP when cfg.SMTPHost is set,
	// otherwise to a mail.LogMailer printing to stdout. Tests: mailtest.New().
	Mailer mail.Mailer
	// Storage keeps attachment content. Defaults to storage.NewLocal(cfg.DataDir + "/attachments").
	Storage     storage.Storage
	CNB         ExchangeRates  // defaults to cnb.NewService(db, cnb.New(cfg.CNBURL), Now)
	VIES        vies.Checker   // defaults to vies.New(cfg.ViesURL) cached 24 h
	VatRegistry vatreg.Checker // defaults to vatreg.New(cfg.VatRegURL) cached 24 h

	// Fio is the Fio banka API client. Defaults to bankimport.NewFioClient(cfg.FioURL)
	// (enforces Fio's 1 request / 30 s per token locally).
	Fio bankimport.Fio
	// Secrets encrypts stored secrets (Fio tokens). cmd/server passes a box
	// keyed by NANOFAKTURA_SECRET_KEY or DATA_DIR/secret.key (secret.LoadOrCreateKey);
	// the default is a random per-process key (tests, OpenAPI generation).
	Secrets *secret.Box
}

// ExchangeRates returns ČNB rates (see cnb.Service.Rate for the error
// contract: cnb.ErrInvalidCurrency, cnb.ErrInvalidDate, cnb.ErrUnknownCurrency,
// other = unavailable). rate is CZK per 1 unit, rateDate the ČNB list date.
type ExchangeRates interface {
	Rate(ctx context.Context, currency, date string) (rate, rateDate string, err error)
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
	s := newServer(db, cfg, deps)

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
	s.registerExchangeRates(authed)
	s.registerVies(authed)
	s.registerVatRegistry(authed)
	s.registerBankAccounts(account)
	s.registerNumberFormats(account)
	s.registerSubjects(account)
	s.registerInvoices(account)
	s.registerInvoiceActions(account)
	s.registerPayments(account)
	s.registerInvoicePDF(account)
	s.registerInvoiceISDOC(account)
	s.registerPublicInvoices(public, account)
	s.registerExports(account)
	s.registerReports(account)
	s.registerDashboard(account)
	s.registerMembers(public, authed, account)
	s.registerAttachments(account)
	s.registerPriceItems(account)
	s.registerStockMoves(account)
	s.registerExpenses(account)
	s.registerTemplates(account)
	s.registerRecurring(account)
	s.registerEmails(account)
	s.registerBankTransactions(account)
	s.registerSubjectVatStatus(account)

	return router, api
}

// newServer applies the dependency defaults (shared by New and Jobs).
func newServer(db *gorm.DB, cfg config.Config, deps Deps) *server {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.ARES == nil {
		deps.ARES = ares.New(cfg.AresURL)
	}
	if deps.NextNumber == nil {
		deps.NextNumber = numbering.Next
	}
	if deps.Mailer == nil {
		deps.Mailer = defaultMailer(cfg)
	}
	if deps.Storage == nil {
		dir := cfg.DataDir
		if dir == "" {
			dir = "./data"
		}
		deps.Storage = storage.NewLocal(filepath.Join(dir, "attachments"))
	}
	if deps.CNB == nil {
		deps.CNB = cnb.NewService(db, cnb.New(cfg.CNBURL), deps.Now)
	}
	if deps.VIES == nil {
		deps.VIES = vies.NewCached(vies.New(cfg.ViesURL), 24*time.Hour, deps.Now)
	}
	if deps.VatRegistry == nil {
		deps.VatRegistry = vatreg.NewCached(vatreg.New(cfg.VatRegURL), 24*time.Hour, deps.Now)
	}
	if deps.Fio == nil {
		deps.Fio = bankimport.NewFioClient(cfg.FioURL)
	}
	if deps.Secrets == nil {
		deps.Secrets = secret.NewRandom()
	}
	return &server{db: db, cfg: cfg, deps: deps, auth: auth.NewService(db, deps.Now, cfg.SecureCookies)}
}

func defaultMailer(cfg config.Config) mail.Mailer {
	if cfg.SMTPHost == "" {
		return mail.NewLogMailer(os.Stdout, cfg.MailFrom)
	}
	return mail.NewSMTP(mail.SMTPConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUser, Password: cfg.SMTPPassword,
		TLS: cfg.SMTPTLS, From: cfg.MailFrom,
	})
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
