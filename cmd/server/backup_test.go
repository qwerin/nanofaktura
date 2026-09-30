package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

func TestBackupCLIRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "nf.db")
	t.Setenv("NANOFAKTURA_DB_DRIVER", "sqlite")
	t.Setenv("NANOFAKTURA_DB_DSN", dsn)
	t.Setenv("NANOFAKTURA_DATA_DIR", filepath.Join(dir, "data"))

	gdb, err := db.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	user := model.User{Email: "admin@example.cz", Name: "Admin", PasswordHash: "x"}
	acc := model.Account{Slug: "firma", Name: "Firma"}
	gdb.Create(&user)
	gdb.Create(&acc)
	gdb.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: model.RoleOwner})
	subj := model.Subject{AccountID: acc.ID, Name: "ACME", Type: model.SubjectCustomer}
	gdb.Create(&subj)
	gdb.Create(&model.Attachment{AccountID: acc.ID, OwnerType: model.OwnerSubject, OwnerID: subj.ID, Filename: "a.pdf",
		ContentType: "application/pdf", Size: 5, StorageKey: "1/abc"})
	st := storage.NewLocal(filepath.Join(dir, "data", "attachments"))
	if err := st.Put(context.Background(), "1/abc", strings.NewReader("%PDF-")); err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := gdb.DB(); err == nil {
		_ = sqlDB.Close()
	}

	zipPath := filepath.Join(dir, "b.zip")
	var out bytes.Buffer
	if err := runBackup(context.Background(), []string{"export", "--account", "firma", "--out", zipPath}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "exported account firma") {
		t.Fatalf("export output: %s", out.String())
	}
	if err := runBackup(context.Background(), []string{"export", "--account", "firma", "--out", zipPath}, io.Discard, io.Discard); err == nil {
		t.Fatal("export must not overwrite an existing file")
	}
	out.Reset()
	if err := runBackup(context.Background(), []string{"import", zipPath, "--owner", "ADMIN@example.cz", "--name", "Firma kopie"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "imported as account firma-kopie") {
		t.Fatalf("import output: %s", out.String())
	}

	gdb, err = db.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	var copyAcc model.Account
	if err := gdb.Where("slug = ?", "firma-kopie").First(&copyAcc).Error; err != nil {
		t.Fatal(err)
	}
	var n int64
	gdb.Model(&model.Subject{}).Where("account_id = ? AND name = ?", copyAcc.ID, "ACME").Count(&n)
	var att model.Attachment
	gdb.Where("account_id = ?", copyAcc.ID).First(&att)
	rc, err := st.Get(context.Background(), att.StorageKey)
	if n != 1 || err != nil {
		t.Fatalf("imported subject %d, attachment %v", n, err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "%PDF-" {
		t.Fatalf("attachment content %q", b)
	}
	var m model.Membership
	if err := gdb.Where("account_id = ? AND user_id = ? AND role = ?", copyAcc.ID, user.ID, model.RoleOwner).First(&m).Error; err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{}, {"nope"}, {"export"}, {"import", zipPath}, {"import", "--owner", "nobody@example.cz", zipPath}} {
		if err := runBackup(context.Background(), args, io.Discard, io.Discard); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
	_ = os.Remove(zipPath)
}
