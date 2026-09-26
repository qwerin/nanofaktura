package numbering_test

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/numbering"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		format string
		ok     bool
	}{
		{"{YYYY}-{NNNN}", true},
		{"Z{YYYY}-{NNNN}", true},
		{"{YY}{MM}{NNN}", true},
		{"{N}", true},
		{"FA-{NNNNNN}", true},
		{"{NN}/{YYYY}/{NN}", true},
		{"", false},
		{"{YYYY}", false},       // no sequence
		{"2026-0001", false},    // no sequence
		{"{NNNNNNN}", false},    // too wide
		{"{YYYY}-{NNNN", false}, // unclosed
		{"{YYYY}}-{NNNN}", false},
		{"}{N}", false},
		{"{DD}-{N}", false},
		{"{}{N}", false},
		{"{nnnn}", false},
		{"{N}" + string(make([]byte, 60)), false}, // too long
	}
	for _, tt := range tests {
		err := numbering.Validate(tt.format)
		if (err == nil) != tt.ok {
			t.Errorf("Validate(%q) = %v, want ok=%v", tt.format, err, tt.ok)
		}
		if err != nil && !errors.Is(err, numbering.ErrInvalidFormat) {
			t.Errorf("Validate(%q) error %v does not wrap ErrInvalidFormat", tt.format, err)
		}
	}
}

func TestPeriodAndRender(t *testing.T) {
	d := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		format, period, want string
		n                    int64
	}{
		{"{YYYY}-{NNNN}", "2026", "2026-0001", 1},
		{"Z{YYYY}-{NNNN}", "2026", "Z2026-0042", 42},
		{"{YY}{MM}{NNN}", "2026-03", "2603007", 7},
		{"{MM}/{N}", "2026-03", "03/5", 5},
		{"FA{NNNNNN}", "", "FA000123", 123},
		{"{NN}", "", "12345", 12345}, // wider than padding is not truncated
		{"{N}-{YYYY}-{NN}", "2026", "3-2026-03", 3},
	}
	for _, tt := range tests {
		if got := numbering.Period(tt.format, d); got != tt.period {
			t.Errorf("Period(%q) = %q, want %q", tt.format, got, tt.period)
		}
		got, err := numbering.Render(tt.format, d, tt.n)
		if err != nil || got != tt.want {
			t.Errorf("Render(%q, %d) = %q, %v; want %q", tt.format, tt.n, got, err, tt.want)
		}
	}
	if _, err := numbering.Render("{X}", d, 1); !errors.Is(err, numbering.ErrInvalidFormat) {
		t.Errorf("Render invalid: %v", err)
	}
	if p := numbering.Period("bad", d); p != "" {
		t.Errorf("Period invalid = %q", p)
	}
}

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := gdb.DB(); err == nil {
			_ = s.Close()
		}
	})
	return gdb
}

func TestNextAndPreview(t *testing.T) {
	gdb := openDB(t)
	acc := model.Account{Slug: "a", Name: "A"}
	gdb.Create(&acc)
	other := model.Account{Slug: "b", Name: "B"}
	gdb.Create(&other)
	inv := model.NumberFormat{AccountID: acc.ID, DocumentType: model.DocInvoice, Format: "{YYYY}-{NNNN}", IsDefault: true}
	gdb.Create(&inv)
	gdb.Create(&model.NumberFormat{AccountID: acc.ID, DocumentType: model.DocInvoice, Format: "X{N}", IsDefault: false})
	gdb.Create(&model.NumberFormat{AccountID: acc.ID, DocumentType: model.DocProforma, Format: "Z{YY}{MM}-{NN}", IsDefault: true})
	gdb.Create(&model.NumberFormat{AccountID: other.ID, DocumentType: model.DocInvoice, Format: "B{NNN}", IsDefault: true})

	next := func(accountID uint, docType, date string) string {
		t.Helper()
		var n string
		err := gdb.Transaction(func(tx *gorm.DB) (err error) {
			n, err = numbering.Next(tx, accountID, docType, date)
			return err
		})
		if err != nil {
			t.Fatalf("Next(%s, %s): %v", docType, date, err)
		}
		return n
	}

	preview, err := numbering.Preview(gdb, &inv, "2026-01-05")
	if err != nil || preview != "2026-0001" {
		t.Fatalf("preview before any number: %q, %v", preview, err)
	}
	for i, want := range []string{"2026-0001", "2026-0002", "2026-0003"} {
		if got := next(acc.ID, model.DocInvoice, "2026-0"+string(rune('1'+i))+"-10"); got != want {
			t.Fatalf("invoice #%d = %q, want %q", i, got, want)
		}
	}
	if got := next(acc.ID, model.DocInvoice, "2027-01-01"); got != "2027-0001" {
		t.Fatalf("new year = %q", got)
	}
	if got := next(acc.ID, model.DocInvoice, "2026-12-31"); got != "2026-0004" {
		t.Fatalf("back in 2026 = %q", got)
	}
	preview, _ = numbering.Preview(gdb, &inv, "2026-06-01")
	if preview != "2026-0005" {
		t.Fatalf("preview = %q", preview)
	}
	if again, _ := numbering.Preview(gdb, &inv, "2026-06-01"); again != preview {
		t.Fatalf("preview moved the counter: %q", again)
	}

	// monthly series
	if got := next(acc.ID, model.DocProforma, "2026-03-01"); got != "Z2603-01" {
		t.Fatalf("proforma = %q", got)
	}
	if got := next(acc.ID, model.DocProforma, "2026-04-01"); got != "Z2604-01" {
		t.Fatalf("proforma april = %q", got)
	}
	// other account has its own counter
	if got := next(other.ID, model.DocInvoice, "2026-03-01"); got != "B001" {
		t.Fatalf("other account = %q", got)
	}

	// rollback does not move the counter
	_ = gdb.Transaction(func(tx *gorm.DB) error {
		if _, err := numbering.Next(tx, acc.ID, model.DocInvoice, "2026-06-01"); err != nil {
			t.Fatal(err)
		}
		return errors.New("rollback")
	})
	if got := next(acc.ID, model.DocInvoice, "2026-06-01"); got != "2026-0005" {
		t.Fatalf("after rollback = %q", got)
	}

	// errors
	err = gdb.Transaction(func(tx *gorm.DB) error {
		_, err := numbering.Next(tx, acc.ID, model.DocCorrection, "2026-01-01")
		return err
	})
	if !errors.Is(err, numbering.ErrNoFormat) {
		t.Fatalf("missing format: %v", err)
	}
	err = gdb.Transaction(func(tx *gorm.DB) error {
		_, err := numbering.Next(tx, acc.ID, model.DocInvoice, "1.1.2026")
		return err
	})
	if !errors.Is(err, numbering.ErrInvalidDate) {
		t.Fatalf("bad date: %v", err)
	}
	if _, err := numbering.Preview(gdb, &inv, "2026-13-01"); !errors.Is(err, numbering.ErrInvalidDate) {
		t.Fatalf("preview bad date: %v", err)
	}
}
