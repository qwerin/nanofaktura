// Package config načítá konfiguraci aplikace z více zdrojů (výchozí hodnoty → JSON soubor → env → flags).
package config

import (
	"encoding/json"
	"flag"
	"os"
	"strconv"
)

// AppConfig obsahuje veškerá konfigurovatelná nastavení serveru.
type AppConfig struct {
	DBDriver   string `json:"db_driver"`    // databázový driver: "sqlite" nebo "postgres"
	DBPath     string `json:"db_path"`      // cesta k SQLite souboru nebo postgres DSN
	ListenAddr string `json:"listen_addr"`  // adresa pro naslouchání, např. ":8080"
	MultiUser  bool   `json:"multi_user"`   // zapnout multi-user mód
	SessionTTL int    `json:"session_ttl"`  // platnost session v hodinách (výchozí 720 = 30 dní)
	StaticDir  string `json:"static_dir"`   // adresář se statickými soubory SPA (prázdné = neservírovat)
}

// Load načte konfiguraci v pořadí: výchozí → nanofaktura.json → env proměnné → CLI flags.
func Load() AppConfig {
	cfg := AppConfig{
		DBDriver:   "sqlite",
		DBPath:     "nanofaktura.db",
		ListenAddr: ":8080",
		MultiUser:  false,
		SessionTTL: 720,
		StaticDir:  "",
	}

	// 1. JSON soubor (volitelný)
	if data, err := os.ReadFile("nanofaktura.json"); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}

	// 2. Env proměnné
	if v := os.Getenv("NANOFAKTURA_DB_DRIVER"); v != "" {
		cfg.DBDriver = v
	}
	if v := os.Getenv("NANOFAKTURA_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("NANOFAKTURA_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("NANOFAKTURA_MULTI_USER"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.MultiUser = b
		}
	}
	if v := os.Getenv("NANOFAKTURA_SESSION_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SessionTTL = n
		}
	}
	if v := os.Getenv("NANOFAKTURA_STATIC_DIR"); v != "" {
		cfg.StaticDir = v
	}

	// 3. CLI flags (přepíší vše výše)
	flag.StringVar(&cfg.DBDriver, "db-driver", cfg.DBDriver, "databázový driver: sqlite nebo postgres")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "cesta k SQLite souboru nebo postgres DSN")
	flag.StringVar(&cfg.ListenAddr, "addr", cfg.ListenAddr, "adresa pro naslouchání (např. :8080)")
	flag.BoolVar(&cfg.MultiUser, "multi-user", cfg.MultiUser, "zapnout multi-user mód s přihlašováním")
	flag.IntVar(&cfg.SessionTTL, "session-ttl", cfg.SessionTTL, "platnost session v hodinách")
	flag.StringVar(&cfg.StaticDir, "static-dir", cfg.StaticDir, "adresář se statickými soubory SPA")
	flag.Parse()

	return cfg
}
