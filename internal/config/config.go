// Package config loads application configuration from environment variables (SPEC §5).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime settings.
type Config struct {
	ListenAddr    string // NANOFAKTURA_LISTEN_ADDR
	DBDriver      string // NANOFAKTURA_DB_DRIVER: sqlite | postgres
	DBDSN         string // NANOFAKTURA_DB_DSN
	StaticDir     string // NANOFAKTURA_STATIC_DIR: SPA build directory, empty = API only
	AllowSignup   bool   // NANOFAKTURA_ALLOW_SIGNUP: allow registration when users already exist
	SecureCookies bool   // NANOFAKTURA_SECURE_COOKIES
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
	if cfg.AllowSignup, err = envBool("NANOFAKTURA_ALLOW_SIGNUP"); err != nil {
		return Config{}, err
	}
	if cfg.SecureCookies, err = envBool("NANOFAKTURA_SECURE_COOKIES"); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
