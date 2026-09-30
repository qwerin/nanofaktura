package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

func TestUserReset2FA(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "nf.db")
	t.Setenv("NANOFAKTURA_DB_DRIVER", "sqlite")
	t.Setenv("NANOFAKTURA_DB_DSN", dsn)

	gdb, err := db.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "admin@example.cz", Name: "Admin", PasswordHash: "x", TOTPSecretEnc: "enc", TOTPLastStep: 5}
	gdb.Create(&user)
	gdb.Create(&model.WebAuthnCredential{UserID: user.ID, Name: "YubiKey", CredentialID: "abc", Data: "{}"})
	gdb.Create(&model.RecoveryCode{UserID: user.ID, CodeHash: "h"})
	if sqlDB, err := gdb.DB(); err == nil {
		_ = sqlDB.Close()
	}

	if err := runUser(context.Background(), []string{"reset-2fa"}, &bytes.Buffer{}); err == nil {
		t.Fatal("--email must be required")
	}
	if err := runUser(context.Background(), []string{"reset-2fa", "--email", "nobody@example.cz"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown user must fail")
	}
	var out bytes.Buffer
	if err := runUser(context.Background(), []string{"reset-2fa", "--email", "ADMIN@example.cz"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "admin@example.cz is off") {
		t.Fatalf("output: %s", out.String())
	}

	gdb, err = db.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var u model.User
	gdb.First(&u, user.ID)
	var keys, codes int64
	gdb.Model(&model.WebAuthnCredential{}).Count(&keys)
	gdb.Model(&model.RecoveryCode{}).Count(&codes)
	if u.TOTPSecretEnc != "" || u.TOTPLastStep != 0 || keys != 0 || codes != 0 {
		t.Fatalf("2FA not reset: %+v keys=%d codes=%d", u, keys, codes)
	}
}
