package db

import (
	"errors"
	"testing"

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
