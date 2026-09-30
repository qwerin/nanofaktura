package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/maildiag/smtptest"
	"github.com/qwerin/nanofaktura/internal/model"
)

func TestMailTest(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{})
	t.Setenv("NANOFAKTURA_DB_DRIVER", "")
	t.Setenv("NANOFAKTURA_SMTP_HOST", "127.0.0.1")
	t.Setenv("NANOFAKTURA_SMTP_PORT", strconv.Itoa(srv.Addr.Port))
	t.Setenv("NANOFAKTURA_SMTP_TLS", "none")
	t.Setenv("NANOFAKTURA_MAIL_FROM", "faktury@example.cz")
	dns := &smtptest.Resolver{}

	if err := runMail(context.Background(), []string{"test"}, &bytes.Buffer{}); err == nil {
		t.Fatal("--to must be required")
	}
	if err := mailTest(context.Background(), []string{"--to", "a@example.com", "--dkim-selector", "a b"}, &bytes.Buffer{}, dns); err == nil {
		t.Fatal("invalid selector accepted")
	}
	var out bytes.Buffer
	// the sender domain has no records in the fake DNS → DNS errors → exit status 1, but the message is sent
	err := mailTest(context.Background(), []string{"--to", "a@example.com"}, &out, dns)
	if err == nil || !strings.Contains(out.String(), "Zpráva odeslána") || !strings.Contains(out.String(), "[ OK ] Příjemce (RCPT TO)") ||
		len(srv.Messages()) != 1 || srv.Messages()[0].To[0] != "a@example.com" {
		t.Fatalf("%v\n%s", err, out.String())
	}
}

func TestUserVerifyEmail(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "nf.db")
	t.Setenv("NANOFAKTURA_DB_DRIVER", "sqlite")
	t.Setenv("NANOFAKTURA_DB_DSN", dsn)
	gdb, err := db.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "admin@example.cz", Name: "Admin", PasswordHash: "x"}
	gdb.Create(&user)
	if sqlDB, err := gdb.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if err := runUser(context.Background(), []string{"verify-email", "--email", "nobody@example.cz"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown user must fail")
	}
	var out bytes.Buffer
	if err := runUser(context.Background(), []string{"verify-email", "--email", "Admin@example.cz"}, &out); err != nil ||
		!strings.Contains(out.String(), "admin@example.cz is verified") {
		t.Fatalf("%v %s", err, out.String())
	}
	out.Reset()
	if err := runUser(context.Background(), []string{"verify-email", "--email", "admin@example.cz"}, &out); err != nil ||
		!strings.Contains(out.String(), "already verified") {
		t.Fatalf("%v %s", err, out.String())
	}
	gdb, _ = db.Open("sqlite", dsn)
	var u model.User
	gdb.First(&u, user.ID)
	if u.EmailVerifiedAt == nil {
		t.Fatal("not verified")
	}
}
