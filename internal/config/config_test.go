package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_ALLOW_SIGNUP", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.DBDriver != "sqlite" || cfg.DBDSN != "nanofaktura.db" || !cfg.AllowSignup || cfg.SecureCookies {
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
}
