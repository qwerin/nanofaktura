package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/maildiag"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Instance administration (SPEC §3.2): /api/admin/* for users listed in
// NANOFAKTURA_ADMIN_EMAILS whose e-mail is verified. Every mutation is
// written to the log (slog "admin action").

// Version is the application version, set at build time with
// -ldflags "-X github.com/qwerin/nanofaktura/internal/api.Version=v1.2.3";
// empty = the VCS revision from the build info (or "dev").
var Version = ""

// AppVersion returns Version or a fallback from the Go build info.
func AppVersion() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	rev, dirty := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		return "dev"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return "dev-" + rev
}

// isInstanceAdmin: listed in NANOFAKTURA_ADMIN_EMAILS AND e-mail verified.
// The verification is essential: an account owner can invite any address and
// register it, so a listed but unverified address grants nothing.
func (s *server) isInstanceAdmin(u *model.User) bool {
	return u != nil && u.EmailVerifiedAt != nil && s.cfg.IsAdminEmail(u.Email)
}

// requireInstanceAdmin guards the admin group (after auth.RequireUser):
// others get 403 not_instance_admin. Mutations are audited in the log.
func (s *server) requireInstanceAdmin(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		u := auth.UserFrom(ctx.Context())
		if !s.isInstanceAdmin(u) {
			writeError(api, ctx, apiError(http.StatusForbidden, CodeNotInstanceAdmin,
				"instance administrators only (e-mail listed in NANOFAKTURA_ADMIN_EMAILS and verified)"))
			return
		}
		if ctx.Method() != http.MethodGet {
			slog.InfoContext(ctx.Context(), "admin action", "operation", ctx.Operation().OperationID,
				"path", ctx.URL().Path, "admin_id", u.ID, "admin", u.Email, "ip", clientIP(ctx.Context()))
		}
		next(ctx)
	}
}

// writeError writes err (an apiError) from a middleware, keeping its code.
func writeError(api huma.API, ctx huma.Context, err error) {
	var se huma.StatusError
	if !errors.As(err, &se) {
		se = huma.Error500InternalServerError("internal error")
	}
	ctx.SetHeader("Content-Type", "application/problem+json")
	ctx.SetStatus(se.GetStatus())
	_ = api.Marshal(ctx.BodyWriter(), "application/problem+json", se)
}

func (s *server) registerAdmin(g huma.API) {
	huma.Get(g, "/status", s.adminStatus)
	huma.Post(g, "/email-test", s.adminEmailTest)
	huma.Get(g, "/users", s.adminListUsers)
	huma.Post(g, "/users/{id}/reset-2fa", s.adminResetTwoFactor, status(http.StatusNoContent))
	huma.Post(g, "/users/{id}/verify-email", s.adminVerifyEmail, status(http.StatusNoContent))
	huma.Post(g, "/users/{id}/send-verification", s.adminSendVerification, status(http.StatusNoContent))
}

// ---- status ----

type InstanceConfig struct {
	PublicURL        string   `json:"public_url"`
	PublicHTTPS      bool     `json:"public_https"`
	SMTPConfigured   bool     `json:"smtp_configured" doc:"NANOFAKTURA_SMTP_HOST is set (otherwise e-mails are only logged)"`
	SMTPHost         string   `json:"smtp_host"`
	SMTPPort         int      `json:"smtp_port"`
	SMTPTLS          string   `json:"smtp_tls" enum:"starttls,tls,none"`
	SMTPAuth         bool     `json:"smtp_auth" doc:"NANOFAKTURA_SMTP_USER is set (the password is never exposed)"`
	MailFrom         string   `json:"mail_from"`
	DKIMEnabled      bool     `json:"dkim_enabled"`
	DKIMDomain       string   `json:"dkim_domain,omitempty"`
	DKIMSelector     string   `json:"dkim_selector,omitempty"`
	DKIMKeyType      string   `json:"dkim_key_type,omitempty" enum:"rsa,ed25519"`
	DKIMRecord       string   `json:"dkim_record,omitempty" doc:"TXT record to publish at <selector>._domainkey.<domain> (public key)"`
	RateLimitEnabled bool     `json:"rate_limit_enabled"`
	SetupTokenSet    bool     `json:"setup_token_set"`
	AllowSignup      bool     `json:"allow_signup"`
	TrustedProxies   []string `json:"trusted_proxies" nullable:"false"`
	DataDir          string   `json:"data_dir"`
	DataDirWritable  bool     `json:"data_dir_writable"`
	SecretKeySource  string   `json:"secret_key_source" enum:"env,file" doc:"env = NANOFAKTURA_SECRET_KEY, file = DATA_DIR/secret.key"`
	AdminEmails      []string `json:"admin_emails" nullable:"false"`
	APIDocsEnabled   bool     `json:"api_docs_enabled"`
}

type InstanceStatus struct {
	Version       string         `json:"version"`
	GoVersion     string         `json:"go_version"`
	DBDriver      string         `json:"db_driver" enum:"sqlite,postgres"`
	StartedAt     time.Time      `json:"started_at"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	Users         int64          `json:"users"`
	Accounts      int64          `json:"accounts"`
	Config        InstanceConfig `json:"config"`
	Warnings      []string       `json:"warnings" nullable:"false" doc:"Czech descriptions of risky settings"`
}

func (s *server) dataDir() string {
	if s.cfg.DataDir == "" {
		return "./data"
	}
	return s.cfg.DataDir
}

// dirWritable creates and removes a temporary file in dir.
func dirWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".nf-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name) == nil
}

func (s *server) instanceConfig() InstanceConfig {
	cfg := s.cfg
	c := InstanceConfig{
		PublicURL: s.publicURL(), PublicHTTPS: cfg.PublicHTTPS(),
		SMTPConfigured: cfg.SMTPHost != "", SMTPHost: cfg.SMTPHost, SMTPPort: cfg.SMTPPort,
		SMTPTLS: defaultString(cfg.SMTPTLS, mail.TLSStartTLS), SMTPAuth: cfg.SMTPUser != "", MailFrom: cfg.MailFrom,
		RateLimitEnabled: !cfg.DisableRateLimit, SetupTokenSet: cfg.SetupToken != "", AllowSignup: cfg.AllowSignup,
		TrustedProxies: []string{}, DataDir: s.dataDir(), DataDirWritable: dirWritable(s.dataDir()),
		SecretKeySource: "file", AdminEmails: append([]string{}, cfg.AdminEmails...), APIDocsEnabled: !cfg.DisableAPIDocs,
	}
	if cfg.SecretKey != "" {
		c.SecretKeySource = "env"
	}
	for _, p := range cfg.TrustedProxies {
		c.TrustedProxies = append(c.TrustedProxies, p.String())
	}
	if d := cfg.DKIM(); d != nil {
		c.DKIMEnabled, c.DKIMDomain, c.DKIMSelector = true, d.Domain, d.Selector
		c.DKIMKeyType, c.DKIMRecord = mail.DKIMKeyType(d.Key), mail.DKIMRecord(d.Key)
	}
	return c
}

// InstanceWarnings lists risky settings in Czech (admin status page).
func InstanceWarnings(cfg config.Config, hasUsers bool) []string {
	w := []string{}
	if cfg.SMTPHost == "" {
		w = append(w, "SMTP není nastavené (NANOFAKTURA_SMTP_HOST): e-maily včetně pozvánek a odkazů pro obnovu hesla se jen vypisují do logu.")
	}
	if !cfg.PublicHTTPS() {
		w = append(w, "NANOFAKTURA_PUBLIC_URL není https: v pořádku lokálně, v produkci provozujte aplikaci přes HTTPS.")
	}
	if !hasUsers && cfg.SetupToken == "" {
		w = append(w, "Zatím není registrovaný žádný uživatel a NANOFAKTURA_SETUP_TOKEN není nastavený: první se může zaregistrovat kdokoli.")
	}
	if cfg.DisableRateLimit {
		w = append(w, "Omezení počtu pokusů je vypnuté (NANOFAKTURA_DISABLE_RATE_LIMIT): přihlašování není chráněné proti hádání hesel.")
	}
	if cfg.SMTPHost != "" && cfg.SMTPTLS == mail.TLSNone && !isLocalHost(cfg.SMTPHost) {
		w = append(w, "Spojení se SMTP serverem není šifrované (NANOFAKTURA_SMTP_TLS=none) a server není lokální.")
	}
	if cfg.SMTPHost != "" && cfg.DKIM() == nil {
		w = append(w, "E-maily nepodepisuje aplikace (DKIM). Pokud je nepodepisuje ani váš SMTP poskytovatel, mohou končit ve spamu — ověřte to testem e-mailu.")
	}
	return w
}

func isLocalHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || strings.HasSuffix(h, ".local")
}

func defaultString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (s *server) adminStatus(ctx context.Context, _ *struct{}) (*Out[InstanceStatus], error) {
	db := s.db.WithContext(ctx)
	st := InstanceStatus{Version: AppVersion(), GoVersion: runtime.Version(), DBDriver: defaultString(s.cfg.DBDriver, "sqlite"),
		StartedAt: s.started, UptimeSeconds: int64(time.Since(s.started).Seconds()), Config: s.instanceConfig()}
	if err := db.Model(&model.User{}).Count(&st.Users).Error; err != nil {
		return nil, dbErr(err, "users")
	}
	if err := db.Model(&model.Account{}).Count(&st.Accounts).Error; err != nil {
		return nil, dbErr(err, "accounts")
	}
	st.Warnings = InstanceWarnings(s.cfg, st.Users > 0)
	return &Out[InstanceStatus]{Body: st}, nil
}

// ---- e-mail test ----

type EmailTestRequest struct {
	To           string `json:"to,omitempty" format:"email" maxLength:"254" doc:"Recipient; default = the admin's own e-mail"`
	DKIMSelector string `json:"dkim_selector,omitempty" maxLength:"63" doc:"DKIM selector to check when the application does not sign (e.g. the SMTP provider's)"`
}

// EmailTestOptions builds the diagnostics options for a test e-mail to "to"
// (shared by the API and the CLI "nanofaktura mail test").
func EmailTestOptions(cfg config.Config, to, selector string, resolver maildiag.Resolver, now time.Time) maildiag.Options {
	smtp := cfg.SMTP()
	if smtp.TLS == "" {
		smtp.TLS = mail.TLSStartTLS
	}
	publicURL := cfg.PublicURL
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}
	return maildiag.Options{SMTP: smtp, To: to, Message: maildiag.TestMessage(publicURL, smtp, to, now),
		DKIMSelector: selector, Resolver: resolver, Now: func() time.Time { return now }}
}

func (s *server) adminEmailTest(ctx context.Context, in *struct{ Body EmailTestRequest }) (*Out[maildiag.DiagReport], error) {
	user := auth.UserFrom(ctx)
	to := normalizeEmail(in.Body.To)
	if to == "" {
		to = user.Email
	}
	sel := strings.TrimSpace(in.Body.DKIMSelector)
	if sel != "" && !maildiag.ValidSelector(sel) {
		return nil, invalid("dkim_selector", "invalid DKIM selector (letters, digits, '.', '-', '_')")
	}
	if err := s.rateLimit(s.limits.emailTest, strconv.FormatUint(uint64(user.ID), 10)); err != nil {
		return nil, err
	}
	o := EmailTestOptions(s.cfg, to, sel, s.deps.Resolver, s.deps.Now())
	if s.deps.SMTPTLSConfig != nil {
		o.SMTP.TLSConfig = s.deps.SMTPTLSConfig
	}
	r := maildiag.Run(ctx, o)
	slog.InfoContext(ctx, "admin e-mail test", "admin_id", user.ID, "to", to, "status", r.Status, "sent", r.Sent, "queue_id", r.QueueID)
	return &Out[maildiag.DiagReport]{Body: *r}, nil
}

// ---- users ----

type AdminUser struct {
	ID              uint       `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	CreatedAt       time.Time  `json:"created_at"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	TwoFactor       bool       `json:"two_factor"`
	Accounts        int64      `json:"accounts" doc:"Number of accounts the user is a member of"`
	InstanceAdmin   bool       `json:"instance_admin"`
	AdminListed     bool       `json:"admin_listed" doc:"Listed in NANOFAKTURA_ADMIN_EMAILS (admin once verified)"`
}

func (s *server) adminListUsers(ctx context.Context, in *struct {
	PageParams
	Query string `query:"query" maxLength:"200"`
}) (*Out[ListResponse[AdminUser]], error) {
	db := s.db.WithContext(ctx)
	q := db.Model(&model.User{}).Order("email")
	if t := strings.ToLower(strings.TrimSpace(in.Query)); t != "" {
		q = q.Where("LOWER(email) LIKE ? OR LOWER(name) LIKE ?", "%"+t+"%", "%"+t+"%")
	}
	out, err := paginate(q, in.PageParams, func(u *model.User) AdminUser {
		return AdminUser{ID: u.ID, Email: u.Email, Name: u.Name, CreatedAt: u.CreatedAt, EmailVerifiedAt: u.EmailVerifiedAt,
			InstanceAdmin: s.isInstanceAdmin(u), AdminListed: s.cfg.IsAdminEmail(u.Email)}
	})
	if err != nil {
		return nil, err
	}
	for i := range out.Body.Items {
		it := &out.Body.Items[i]
		var u model.User
		if err := db.First(&u, it.ID).Error; err != nil {
			return nil, dbErr(err, "user")
		}
		if it.TwoFactor, err = hasSecondFactor(db, &u); err != nil {
			return nil, err
		}
		if err := db.Model(&model.Membership{}).Where("user_id = ?", u.ID).Count(&it.Accounts).Error; err != nil {
			return nil, dbErr(err, "memberships")
		}
	}
	return out, nil
}

type adminUserIn struct {
	ID uint `path:"id"`
}

func (s *server) adminUser(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, dbErr(err, "user")
	}
	return &u, nil
}

func (s *server) adminResetTwoFactor(ctx context.Context, in *adminUserIn) (*NoContent, error) {
	u, err := s.adminUser(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return auth.ResetSecondFactor(tx, u.ID) }); err != nil {
		return nil, dbErr(err, "user")
	}
	slog.InfoContext(ctx, "admin reset two-factor authentication", "admin_id", auth.UserFrom(ctx).ID, "user_id", u.ID, "user", u.Email)
	return &NoContent{}, nil
}

func (s *server) adminVerifyEmail(ctx context.Context, in *adminUserIn) (*NoContent, error) {
	u, err := s.adminUser(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := markEmailVerified(s.db.WithContext(ctx), u, s.deps.Now()); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "admin marked e-mail verified", "admin_id", auth.UserFrom(ctx).ID, "user_id", u.ID, "user", u.Email)
	return &NoContent{}, nil
}

func (s *server) adminSendVerification(ctx context.Context, in *adminUserIn) (*NoContent, error) {
	u, err := s.adminUser(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if u.EmailVerifiedAt != nil {
		return nil, conflict(CodeEmailVerified, "the e-mail address is already verified")
	}
	if err := s.sendEmailVerification(ctx, u); err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}
