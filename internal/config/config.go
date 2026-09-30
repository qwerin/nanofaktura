// Package config loads application configuration from environment variables (SPEC §5).
package config

import (
	"crypto"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/qwerin/nanofaktura/internal/httpsec"
	"github.com/qwerin/nanofaktura/internal/mail"
)

// Config holds all runtime settings.
type Config struct {
	ListenAddr  string // NANOFAKTURA_LISTEN_ADDR
	DBDriver    string // NANOFAKTURA_DB_DRIVER: sqlite | postgres
	DBDSN       string // NANOFAKTURA_DB_DSN
	StaticDir   string // NANOFAKTURA_STATIC_DIR: SPA build directory, empty = API only
	AllowSignup bool   // NANOFAKTURA_ALLOW_SIGNUP: allow registration when users already exist
	// SecureCookies (NANOFAKTURA_SECURE_COOKIES): nil = auto (Secure when
	// PublicURL is https or the request arrived over HTTPS); true/false force it.
	SecureCookies *bool
	AresURL       string // NANOFAKTURA_ARES_URL: ARES base URL override (tests)
	PublicURL     string // NANOFAKTURA_PUBLIC_URL: external base URL for links in e-mails (no trailing slash)
	DataDir       string // NANOFAKTURA_DATA_DIR: attachments etc. (default ./data)

	SMTPHost     string // NANOFAKTURA_SMTP_HOST: empty = e-mails are only logged to stdout
	SMTPPort     int    // NANOFAKTURA_SMTP_PORT (default 587, or 465 with TLS=tls)
	SMTPUser     string // NANOFAKTURA_SMTP_USER: empty = no AUTH
	SMTPPassword string // NANOFAKTURA_SMTP_PASSWORD
	SMTPTLS      string // NANOFAKTURA_SMTP_TLS: starttls (default) | tls (implicit) | none
	MailFrom     string // NANOFAKTURA_MAIL_FROM: sender address, e.g. "NanoFaktura <faktury@example.cz>"
	CNBURL       string // NANOFAKTURA_CNB_URL: ČNB daily rates URL override (tests)
	ViesURL      string // NANOFAKTURA_VIES_URL: VIES REST API base URL override (tests)
	VatRegURL    string // NANOFAKTURA_VATREG_URL: VAT payer registry SOAP endpoint override (tests)
	FioURL       string // NANOFAKTURA_FIO_URL: Fio banka API base URL override (tests)
	SecretKey    string // NANOFAKTURA_SECRET_KEY: 32-byte key (base64/hex) encrypting stored secrets; empty = DataDir/secret.key

	// ImportMaxMB (NANOFAKTURA_IMPORT_MAX_MB, default 512) limits the total
	// uncompressed size of an imported account backup (and the upload).
	ImportMaxMB int

	// DBLog (NANOFAKTURA_DB_LOG, default "error"): SQL logging — silent|error|warn|info.
	// warn adds slow queries (≥ DBSlowMS), info every query; values are never logged.
	DBLog string
	// DBSlowMS (NANOFAKTURA_DB_SLOW_MS, default 1000): slow-query threshold for DBLog=warn.
	DBSlowMS int

	// WebhooksAllowPrivate (NANOFAKTURA_WEBHOOKS_ALLOW_PRIVATE) lets webhooks call private,
	// loopback and link-local addresses (SSRF protection off; LAN setups, tests).
	WebhooksAllowPrivate bool

	// WebAuthnOrigins (NANOFAKTURA_WEBAUTHN_ORIGINS, comma-separated) are origins
	// accepted for security keys besides the PublicURL origin (dev: the Vite server).
	WebAuthnOrigins []string

	// TrustedProxies (NANOFAKTURA_TRUSTED_PROXIES): comma separated IPs/CIDRs of
	// reverse proxies whose X-Forwarded-For / X-Forwarded-Proto are believed
	// (client IP for rate limits, HTTPS detection). Default none.
	TrustedProxies []netip.Prefix

	// SetupToken (NANOFAKTURA_SETUP_TOKEN): when set, the first registration of
	// an empty instance must present it (protects a fresh deployment).
	SetupToken string

	// DisableRateLimit (NANOFAKTURA_DISABLE_RATE_LIMIT) turns the in-process
	// rate limits off (load tests, trusted single-user setups).
	DisableRateLimit bool

	// DisableAPIDocs (NANOFAKTURA_DISABLE_API_DOCS) hides /api/docs,
	// /api/openapi.json and /api/schemas (public by default).
	DisableAPIDocs bool

	// AdminEmails (NANOFAKTURA_ADMIN_EMAILS, comma-separated, lowercased):
	// instance administrators. A user is an admin only with a listed AND
	// verified e-mail (SPEC §3.2).
	AdminEmails []string

	// DKIM signing of outgoing mail (all three or none): NANOFAKTURA_DKIM_DOMAIN,
	// NANOFAKTURA_DKIM_SELECTOR and NANOFAKTURA_DKIM_PRIVATE_KEY (PEM) or
	// NANOFAKTURA_DKIM_PRIVATE_KEY_FILE. DKIMKey is the parsed key (never logged).
	DKIMDomain   string
	DKIMSelector string
	DKIMKey      crypto.Signer
}

// IsAdminEmail reports whether email is listed in NANOFAKTURA_ADMIN_EMAILS
// (verification is checked by the caller).
func (c Config) IsAdminEmail(email string) bool {
	return slices.Contains(c.AdminEmails, strings.ToLower(strings.TrimSpace(email)))
}

// DKIM returns the signing configuration, nil when DKIM is off.
func (c Config) DKIM() *mail.DKIMConfig {
	if c.DKIMKey == nil {
		return nil
	}
	return &mail.DKIMConfig{Domain: c.DKIMDomain, Selector: c.DKIMSelector, Key: c.DKIMKey}
}

// SMTP returns the mailer configuration (host empty = no SMTP).
func (c Config) SMTP() mail.SMTPConfig {
	return mail.SMTPConfig{
		Host: c.SMTPHost, Port: c.SMTPPort, Username: c.SMTPUser, Password: c.SMTPPassword,
		TLS: c.SMTPTLS, From: c.MailFrom, DKIM: c.DKIM(),
	}
}

// PublicHTTPS reports whether PublicURL is an https URL.
func (c Config) PublicHTTPS() bool {
	return strings.HasPrefix(strings.ToLower(c.PublicURL), "https://")
}

// Load reads the configuration from the environment and applies defaults.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr: env("NANOFAKTURA_LISTEN_ADDR", ":8080"),
		DBDriver:   env("NANOFAKTURA_DB_DRIVER", "sqlite"),
		DBDSN:      env("NANOFAKTURA_DB_DSN", "nanofaktura.db"),
		StaticDir:  os.Getenv("NANOFAKTURA_STATIC_DIR"),
		AresURL:    os.Getenv("NANOFAKTURA_ARES_URL"),
		PublicURL:  strings.TrimRight(env("NANOFAKTURA_PUBLIC_URL", "http://localhost:8080"), "/"),
		DataDir:    env("NANOFAKTURA_DATA_DIR", "./data"),

		SMTPHost:     os.Getenv("NANOFAKTURA_SMTP_HOST"),
		SMTPUser:     os.Getenv("NANOFAKTURA_SMTP_USER"),
		SMTPPassword: os.Getenv("NANOFAKTURA_SMTP_PASSWORD"),
		SMTPTLS:      env("NANOFAKTURA_SMTP_TLS", "starttls"),
		MailFrom:     env("NANOFAKTURA_MAIL_FROM", "NanoFaktura <nanofaktura@localhost>"),
		CNBURL:       os.Getenv("NANOFAKTURA_CNB_URL"),
		ViesURL:      os.Getenv("NANOFAKTURA_VIES_URL"),
		VatRegURL:    os.Getenv("NANOFAKTURA_VATREG_URL"),
		FioURL:       os.Getenv("NANOFAKTURA_FIO_URL"),
		SecretKey:    os.Getenv("NANOFAKTURA_SECRET_KEY"),
	}
	for _, o := range strings.Split(os.Getenv("NANOFAKTURA_WEBAUTHN_ORIGINS"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			cfg.WebAuthnOrigins = append(cfg.WebAuthnOrigins, o)
		}
	}
	if cfg.DBDriver != "sqlite" && cfg.DBDriver != "postgres" {
		return Config{}, fmt.Errorf("NANOFAKTURA_DB_DRIVER: unsupported driver %q (sqlite|postgres)", cfg.DBDriver)
	}
	switch cfg.SMTPTLS {
	case "starttls", "tls", "none":
	default:
		return Config{}, fmt.Errorf("NANOFAKTURA_SMTP_TLS: unsupported mode %q (starttls|tls|none)", cfg.SMTPTLS)
	}
	defPort := "587"
	if cfg.SMTPTLS == "tls" {
		defPort = "465"
	}
	port, err := strconv.Atoi(env("NANOFAKTURA_SMTP_PORT", defPort))
	if err != nil || port <= 0 || port > 65535 {
		return Config{}, fmt.Errorf("NANOFAKTURA_SMTP_PORT: invalid port")
	}
	cfg.SMTPPort = port
	if cfg.ImportMaxMB, err = strconv.Atoi(env("NANOFAKTURA_IMPORT_MAX_MB", "512")); err != nil || cfg.ImportMaxMB <= 0 {
		return Config{}, fmt.Errorf("NANOFAKTURA_IMPORT_MAX_MB: invalid size")
	}
	cfg.DBLog = strings.ToLower(env("NANOFAKTURA_DB_LOG", "error"))
	if !slices.Contains([]string{"silent", "error", "warn", "info"}, cfg.DBLog) {
		return Config{}, fmt.Errorf("NANOFAKTURA_DB_LOG: use silent, error, warn or info")
	}
	if cfg.DBSlowMS, err = strconv.Atoi(env("NANOFAKTURA_DB_SLOW_MS", "1000")); err != nil || cfg.DBSlowMS <= 0 {
		return Config{}, fmt.Errorf("NANOFAKTURA_DB_SLOW_MS: invalid milliseconds")
	}
	if cfg.AllowSignup, err = envBool("NANOFAKTURA_ALLOW_SIGNUP"); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("NANOFAKTURA_SECURE_COOKIES"); v != "" && v != "auto" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("NANOFAKTURA_SECURE_COOKIES: invalid value %q (auto|true|false)", v)
		}
		cfg.SecureCookies = &b
	}
	if cfg.TrustedProxies, err = httpsec.ParseProxies(os.Getenv("NANOFAKTURA_TRUSTED_PROXIES")); err != nil {
		return Config{}, fmt.Errorf("NANOFAKTURA_TRUSTED_PROXIES: %w", err)
	}
	cfg.SetupToken = strings.TrimSpace(os.Getenv("NANOFAKTURA_SETUP_TOKEN"))
	if cfg.DisableRateLimit, err = envBool("NANOFAKTURA_DISABLE_RATE_LIMIT"); err != nil {
		return Config{}, err
	}
	if cfg.DisableAPIDocs, err = envBool("NANOFAKTURA_DISABLE_API_DOCS"); err != nil {
		return Config{}, err
	}
	if cfg.WebhooksAllowPrivate, err = envBool("NANOFAKTURA_WEBHOOKS_ALLOW_PRIVATE"); err != nil {
		return Config{}, err
	}
	for _, e := range strings.Split(os.Getenv("NANOFAKTURA_ADMIN_EMAILS"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" && !slices.Contains(cfg.AdminEmails, e) {
			cfg.AdminEmails = append(cfg.AdminEmails, e)
		}
	}
	if err := loadDKIM(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadDKIM reads the optional DKIM settings; errors never contain key material.
func loadDKIM(cfg *Config) error {
	cfg.DKIMDomain = strings.ToLower(strings.TrimSpace(os.Getenv("NANOFAKTURA_DKIM_DOMAIN")))
	cfg.DKIMSelector = strings.TrimSpace(os.Getenv("NANOFAKTURA_DKIM_SELECTOR"))
	pemText := os.Getenv("NANOFAKTURA_DKIM_PRIVATE_KEY")
	if file := os.Getenv("NANOFAKTURA_DKIM_PRIVATE_KEY_FILE"); file != "" {
		if pemText != "" {
			return fmt.Errorf("NANOFAKTURA_DKIM_PRIVATE_KEY and NANOFAKTURA_DKIM_PRIVATE_KEY_FILE: set only one")
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("NANOFAKTURA_DKIM_PRIVATE_KEY_FILE: %w", err)
		}
		pemText = string(b)
	}
	if cfg.DKIMDomain == "" && cfg.DKIMSelector == "" && pemText == "" {
		return nil
	}
	if cfg.DKIMDomain == "" || cfg.DKIMSelector == "" || pemText == "" {
		return fmt.Errorf("DKIM: set NANOFAKTURA_DKIM_DOMAIN, NANOFAKTURA_DKIM_SELECTOR and NANOFAKTURA_DKIM_PRIVATE_KEY(_FILE) together")
	}
	key, err := mail.ParseDKIMKey(pemText)
	if err != nil {
		return fmt.Errorf("NANOFAKTURA_DKIM_PRIVATE_KEY: %w", err)
	}
	cfg.DKIMKey = key
	return nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q", key, v)
	}
	return b, nil
}
