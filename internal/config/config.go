// Package config loads application configuration from environment variables (SPEC §5).
package config

import (
	"fmt"
	"os"
	"strconv"
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
}

// Load reads the configuration from the environment and applies defaults.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr: env("NANOFAKTURA_LISTEN_ADDR", ":8080"),
		DBDriver:   env("NANOFAKTURA_DB_DRIVER", "sqlite"),
		DBDSN:      env("NANOFAKTURA_DB_DSN", "nanofaktura.db"),
		StaticDir:  os.Getenv("NANOFAKTURA_STATIC_DIR"),
		AresURL:    os.Getenv("NANOFAKTURA_ARES_URL"),
	}
	if cfg.DBDriver != "sqlite" && cfg.DBDriver != "postgres" {
		return Config{}, fmt.Errorf("NANOFAKTURA_DB_DRIVER: unsupported driver %q (sqlite|postgres)", cfg.DBDriver)
	}
	var err error
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
