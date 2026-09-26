package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_ALLOW_SIGNUP", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.DBDriver != "sqlite" || cfg.DBDSN != "nanofaktura.db" || !cfg.AllowSignup || cfg.SecureCookies ||
		cfg.SMTPPort != 587 || cfg.SMTPTLS != "starttls" || cfg.DataDir != "./data" || cfg.PublicURL != "http://localhost:8080" {
		t.Fatalf("unexpected config: %+v", cfg)
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
