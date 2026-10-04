package db

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/model"
)

func TestOpenMigrate(t *testing.T) {
	d, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}

	var fk int
	d.Raw("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}

	u := model.User{Email: "a@b.cz", Name: "A", PasswordHash: "x"}
	if err := d.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	dup := model.User{Email: "a@b.cz", Name: "B", PasswordHash: "x"}
	if err := d.Create(&dup).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate email: got %v, want ErrDuplicatedKey", err)
	}

	inv := model.Invoice{AccountID: 1, DocumentType: model.DocInvoice, Number: "1", Status: model.StatusOpen,
		SubjectID: 1, IssuedOn: "2026-01-01", Currency: "CZK", ExchangeRate: "1", Language: "cs",
		PaymentMethod: "bank", Tags: []string{"a", "b"}, Lines: []model.InvoiceLine{{Name: "x", QuantityMilli: 1000}}}
	if err := d.Create(&inv).Error; err != nil {
		t.Fatal(err)
	}
	var got model.Invoice
	if err := d.Preload("Lines").First(&got, inv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 2 || got.Tags[1] != "b" || len(got.Lines) != 1 {
		t.Fatalf("roundtrip failed: %+v", got)
	}
	if err := d.Delete(&got).Error; err != nil {
		t.Fatal(err)
	}
	var lines int64
	d.Model(&model.InvoiceLine{}).Count(&lines)
	if lines != 0 {
		t.Fatalf("lines not cascaded: %d", lines)
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	if _, err := Open("mysql", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoggerQuietAndWithoutValues(t *testing.T) {
	var buf bytes.Buffer
	lg, err := newLogger(options{logLevel: "info", slow: time.Second, out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	d, err := gorm.Open(sqlite.Open(sqliteDSN(":memory:")), &gorm.Config{Logger: lg})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	var u model.User
	err = d.Where("email = ?", "tajny@example.cz").First(&u).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "tajny@example.cz") {
		t.Errorf("query values must not be logged:\n%s", out)
	}
	if strings.Contains(out, "record not found") {
		t.Errorf("record not found must not be logged:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("log must be without colors")
	}

	buf.Reset()
	quiet, _ := newLogger(options{logLevel: "error", slow: time.Second, out: &buf})
	d = d.Session(&gorm.Session{Logger: quiet})
	d.Where("email = ?", "x").First(&u)
	if buf.Len() != 0 {
		t.Errorf("default level error must stay quiet for normal queries: %s", buf.String())
	}
	if _, err := newLogger(options{logLevel: "loud"}); err == nil {
		t.Error("unknown level must fail")
	}
}

// TestMigrateInvoiceNumberIndex: the unique number index of older databases is
// replaced by a partial one, so drafts (number "") do not collide.
func TestMigrateInvoiceNumberIndex(t *testing.T) {
	d, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	// the schema before drafts
	for _, q := range []string{
		"DROP INDEX idx_invoices_number",
		"CREATE UNIQUE INDEX idx_invoices_account_type_number ON invoices (account_id, document_type, number)",
	} {
		if err := d.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	if d.Migrator().HasIndex(&model.Invoice{}, "idx_invoices_account_type_number") {
		t.Fatal("old index kept")
	}
	doc := func(number, token string) error {
		return d.Create(&model.Invoice{AccountID: 1, DocumentType: model.DocInvoice, Number: number, Status: model.StatusDraft,
			SubjectID: 1, IssuedOn: "2026-01-01", PublicToken: token}).Error
	}
	if err := doc("", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := doc("", "t2"); err != nil {
		t.Fatalf("second draft: %v", err)
	}
	if err := doc("2026-0001", "t3"); err != nil {
		t.Fatal(err)
	}
	if err := doc("2026-0001", "t4"); !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate number: got %v, want ErrDuplicatedKey", err)
	}
}
