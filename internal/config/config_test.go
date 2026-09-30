package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_ALLOW_SIGNUP", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.DBDriver != "sqlite" || cfg.DBDSN != "nanofaktura.db" || !cfg.AllowSignup || cfg.SecureCookies != nil ||
		cfg.SMTPPort != 587 || cfg.SMTPTLS != "starttls" || cfg.DataDir != "./data" || cfg.PublicURL != "http://localhost:8080" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadSecurity(t *testing.T) {
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_SECURE_COOKIES", "false")
	t.Setenv("NANOFAKTURA_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.1")
	t.Setenv("NANOFAKTURA_SETUP_TOKEN", " abc ")
	t.Setenv("NANOFAKTURA_PUBLIC_URL", "https://f.example.cz")
	cfg, err := Load()
	if err != nil || cfg.SecureCookies == nil || *cfg.SecureCookies || len(cfg.TrustedProxies) != 2 ||
		cfg.SetupToken != "abc" || !cfg.PublicHTTPS() || cfg.DisableRateLimit {
		t.Fatalf("%+v %v", cfg, err)
	}
	t.Setenv("NANOFAKTURA_SECURE_COOKIES", "auto")
	if cfg, err := Load(); err != nil || cfg.SecureCookies != nil {
		t.Fatalf("auto: %+v %v", cfg, err)
	}
	t.Setenv("NANOFAKTURA_TRUSTED_PROXIES", "proxy.local")
	if _, err := Load(); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Setenv("NANOFAKTURA_DB_DRIVER", "mysql")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unsupported driver")
	}
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_SECURE_COOKIES", "maybe")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid bool")
	}
	t.Setenv("NANOFAKTURA_SECURE_COOKIES", "")
	t.Setenv("NANOFAKTURA_SMTP_TLS", "ssl")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid SMTP TLS mode")
	}
	t.Setenv("NANOFAKTURA_SMTP_TLS", "tls")
	t.Setenv("NANOFAKTURA_PUBLIC_URL", "https://f.example.cz/")
	cfg, err := Load()
	if err != nil || cfg.SMTPPort != 465 || cfg.PublicURL != "https://f.example.cz" {
		t.Fatalf("tls defaults: %+v %v", cfg, err)
	}
}

func TestLoadAdminsAndDKIM(t *testing.T) {
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_ADMIN_EMAILS", " Admin@Example.cz ,, druhy@example.cz,admin@example.cz")
	cfg, err := Load()
	if err != nil || len(cfg.AdminEmails) != 2 || !cfg.IsAdminEmail("ADMIN@example.cz ") || cfg.IsAdminEmail("x@example.cz") || cfg.DKIM() != nil {
		t.Fatalf("%+v %v", cfg.AdminEmails, err)
	}

	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	file := filepath.Join(t.TempDir(), "dkim.pem")
	if err := os.WriteFile(file, []byte(pemText), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NANOFAKTURA_DKIM_DOMAIN", "Example.cz")
	if _, err := Load(); err == nil {
		t.Fatal("partial DKIM config accepted")
	}
	t.Setenv("NANOFAKTURA_DKIM_SELECTOR", "nf")
	t.Setenv("NANOFAKTURA_DKIM_PRIVATE_KEY_FILE", file)
	cfg, err = Load()
	if err != nil || cfg.DKIM() == nil || cfg.DKIM().Domain != "example.cz" || cfg.SMTP().DKIM == nil {
		t.Fatalf("file key: %v", err)
	}
	t.Setenv("NANOFAKTURA_DKIM_PRIVATE_KEY", pemText)
	if _, err := Load(); err == nil {
		t.Fatal("both key and key file accepted")
	}
	t.Setenv("NANOFAKTURA_DKIM_PRIVATE_KEY_FILE", "")
	if cfg, err := Load(); err != nil || cfg.DKIMKey == nil {
		t.Fatalf("env key: %v", err)
	}
	t.Setenv("NANOFAKTURA_DKIM_PRIVATE_KEY", "-----BEGIN PRIVATE KEY-----\nSECRETSTUFF\n-----END PRIVATE KEY-----")
	if _, err := Load(); err == nil || strings.Contains(err.Error(), "SECRETSTUFF") {
		t.Fatalf("invalid key: %v", err)
	}
}
