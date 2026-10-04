package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// memStorage is an in-memory storage.Storage; failPut makes Put fail.
type memStorage struct {
	files   map[string][]byte
	failPut bool
	failGet bool
}

func (m *memStorage) Put(_ context.Context, key string, r io.Reader) error {
	if m.failPut {
		return errors.New("disk full")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.files[key] = b
	return nil
}

func (m *memStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if m.failGet {
		return nil, errors.New("io error")
	}
	b, ok := m.files[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *memStorage) Delete(_ context.Context, key string) error {
	delete(m.files, key)
	return nil
}

type fx struct {
	db    *gorm.DB
	st    *memStorage
	acc   model.Account
	owner model.User
	other model.User
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newFx(t *testing.T) *fx {
	t.Helper()
	gdb, err := db.Open("sqlite", ":memory:")
	must(t, err)
	must(t, db.Migrate(gdb))
	f := &fx{db: gdb, st: &memStorage{files: map[string][]byte{}}}
	f.owner = model.User{Email: "owner@example.cz", Name: "Owner", PasswordHash: "x"}
	f.other = model.User{Email: "other@example.cz", Name: "Other", PasswordHash: "x"}
	must(t, gdb.Create(&f.owner).Error)
	must(t, gdb.Create(&f.other).Error)
	f.acc = model.Account{Slug: "firma", Name: "Firma s.r.o.", RegistrationNo: "12345678",
		AccountMailSettings: model.AccountMailSettings{RemindersEnabled: true}}
	must(t, gdb.Create(&f.acc).Error)
	must(t, gdb.Create(&model.Membership{UserID: f.owner.ID, AccountID: f.acc.ID, Role: model.RoleOwner}).Error)

	subj := model.Subject{AccountID: f.acc.ID, Name: "ACME"}
	must(t, gdb.Create(&subj).Error)
	inv := model.Invoice{AccountID: f.acc.ID, DocumentType: model.DocInvoice, Number: "2026-0001", Status: model.StatusOpen,
		SubjectID: new(subj.ID), IssuedOn: "2026-01-01", Currency: "CZK", ExchangeRate: "1", Language: "cs", PaymentMethod: "bank",
		PublicToken: "tok-original", Total: 12100,
		Lines: []model.InvoiceLine{{Name: "Práce", QuantityMilli: 1000, UnitPrice: 10000, VatRateBps: 2100, Position: 1}}}
	must(t, gdb.Create(&inv).Error)
	must(t, gdb.Create(&model.Payment{AccountID: f.acc.ID, InvoiceID: inv.ID, PaidOn: "2026-01-05", Amount: 5000}).Error)
	exp := model.Expense{AccountID: f.acc.ID, Number: "N2026-0001", Status: model.StatusOpen, SubjectID: &subj.ID, IssuedOn: "2026-01-02",
		Currency: "CZK", ExchangeRate: "1", Lines: []model.ExpenseLine{{Name: "Papír", QuantityMilli: 2000, UnitPrice: 100, Position: 1}}}
	must(t, gdb.Create(&exp).Error)
	must(t, gdb.Create(&model.Event{AccountID: f.acc.ID, Name: "invoice.created", SubjectType: "invoice", SubjectID: inv.ID,
		Text: "Vystaveno", CreatedAt: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}).Error)

	for i, a := range []model.Attachment{
		{AccountID: f.acc.ID, OwnerType: "invoice", OwnerID: inv.ID, Filename: "smlouva.pdf", ContentType: "application/pdf", Size: 4, StorageKey: "1/present"},
		{AccountID: f.acc.ID, OwnerType: "expense", OwnerID: exp.ID, Filename: "uctenka.jpg", ContentType: "image/jpeg", Size: 3, StorageKey: "1/missing"},
	} {
		must(t, gdb.Create(&a).Error)
		if i == 0 {
			f.st.files[a.StorageKey] = []byte("%PDF")
		}
	}
	// another account's data must never leak into the backup
	foreign := model.Account{Slug: "cizi", Name: "Cizí"}
	must(t, gdb.Create(&foreign).Error)
	must(t, gdb.Create(&model.Subject{AccountID: foreign.ID, Name: "Tajný klient"}).Error)
	return f
}

func (f *fx) export(t *testing.T, accID uint) ([]byte, *Manifest) {
	t.Helper()
	var buf bytes.Buffer
	man, err := Export(context.Background(), f.db, f.st, accID, &buf)
	must(t, err)
	return buf.Bytes(), man
}

func TestExportImportExport(t *testing.T) {
	f := newFx(t)
	data, man := f.export(t, f.acc.ID)
	if bytes.Contains(data, []byte("Tajný klient")) {
		t.Fatal("foreign account data in the backup")
	}
	if man.Format != Format || man.Version != Version || man.Account.Slug != "firma" || man.AppVersion == "" {
		t.Fatalf("manifest %+v", man)
	}
	for k, want := range map[string]int{"subjects": 1, "invoices": 1, "expenses": 1, "events": 1, "attachments": 2} {
		if man.Counts[k] != want {
			t.Errorf("count %s = %d, want %d (%v)", k, man.Counts[k], want, man.Counts)
		}
	}
	must(t, RecordExported(f.db, &f.acc, &f.owner.ID, time.Now(), man))

	now := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	acc, warns, err := Import(context.Background(), f.db, f.st, bytes.NewReader(data), int64(len(data)),
		ImportOptions{OwnerUserID: f.other.ID, Now: now})
	must(t, err)
	if acc.ID == f.acc.ID || acc.Slug == "firma" || acc.Name != "Firma s.r.o." || acc.RemindersEnabled {
		t.Fatalf("imported account %+v", acc)
	}
	codes := map[string]int{}
	for _, w := range warns {
		codes[w.Code] = w.Count
	}
	if codes[WarnAttachmentsMissing] != 1 || codes[WarnPublicLinksRegenerated] != 1 {
		t.Fatalf("warnings %+v", warns)
	}
	var m model.Membership
	must(t, f.db.Where("account_id = ?", acc.ID).First(&m).Error)
	if m.UserID != f.other.ID || m.Role != model.RoleOwner {
		t.Fatalf("membership %+v", m)
	}
	var inv model.Invoice
	must(t, f.db.Preload("Lines").Where("account_id = ?", acc.ID).First(&inv).Error)
	if inv.PublicToken == "tok-original" || inv.PublicToken == "" || len(inv.Lines) != 1 || inv.Lines[0].Name != "Práce" || inv.Number != "2026-0001" {
		t.Fatalf("invoice %+v", inv)
	}
	var att []model.Attachment
	f.db.Where("account_id = ?", acc.ID).Find(&att)
	if len(att) != 1 || att[0].OwnerID != inv.ID || string(f.st.files[att[0].StorageKey]) != "%PDF" {
		t.Fatalf("attachments %+v", att)
	}
	var imported int64
	f.db.Model(&model.Event{}).Where("account_id = ? AND name = ?", acc.ID, "account.imported").Count(&imported)
	if imported != 1 {
		t.Fatalf("account.imported events: %d", imported)
	}

	// exporting the copy gives the same entities (the missing attachment is gone for good)
	_, man2 := f.export(t, acc.ID)
	for k, n := range man.Counts {
		want := n
		switch k {
		case "attachments":
			want = 1
		case "events":
			want = n + 1 // + account.imported
		case "number_formats":
			want = len(defaultFormats) // added for document types without a series
		}
		if man2.Counts[k] != want {
			t.Errorf("re-export count %s = %d, want %d", k, man2.Counts[k], want)
		}
	}

	// custom name → slug from the name
	acc3, _, err := Import(context.Background(), f.db, f.st, bytes.NewReader(data), int64(len(data)),
		ImportOptions{OwnerUserID: f.other.ID, Name: "Kopie"})
	must(t, err)
	if acc3.Name != "Kopie" || acc3.Slug != "kopie" {
		t.Fatalf("named import %+v", acc3)
	}
	if !strings.HasPrefix(Filename("firma", now), "nanofaktura-firma-2026-02-01") {
		t.Fatal(Filename("firma", now))
	}
}

func TestImportFailuresRollBack(t *testing.T) {
	f := newFx(t)
	data, _ := f.export(t, f.acc.ID)
	var before int64
	f.db.Model(&model.Account{}).Count(&before)

	// storage failure while writing attachment contents → nothing is created
	f.st.failPut = true
	_, _, err := Import(context.Background(), f.db, f.st, bytes.NewReader(data), int64(len(data)), ImportOptions{OwnerUserID: f.other.ID})
	if err == nil || IsImportError(err) {
		t.Fatalf("storage failure: %v (import error %v)", err, IsImportError(err))
	}
	f.st.failPut = false
	var after int64
	f.db.Model(&model.Account{}).Count(&after)
	if after != before {
		t.Fatalf("accounts %d → %d after failed import", before, after)
	}

	// garbage and too large archives are validation errors
	_, _, err = Import(context.Background(), f.db, f.st, bytes.NewReader([]byte("not a zip")), 9, ImportOptions{OwnerUserID: f.other.ID})
	if !errors.Is(err, ErrCorrupt) || !IsImportError(err) {
		t.Fatalf("garbage: %v", err)
	}
	_, _, err = Import(context.Background(), f.db, f.st, bytes.NewReader(data), int64(len(data)), ImportOptions{OwnerUserID: f.other.ID, MaxBytes: 100})
	if !errors.Is(err, ErrTooLarge) || !IsImportError(err) {
		t.Fatalf("too large: %v", err)
	}
	if IsImportError(errors.New("x")) {
		t.Fatal("plain error is not an import error")
	}
}

func TestExportErrors(t *testing.T) {
	f := newFx(t)
	var buf bytes.Buffer
	if _, err := Export(context.Background(), f.db, f.st, 9999, &buf); err == nil {
		t.Fatal("unknown account exported")
	}
	f.st.failGet = true
	if _, err := Export(context.Background(), f.db, f.st, f.acc.ID, &buf); err == nil || !strings.Contains(err.Error(), "attachment") {
		t.Fatalf("storage error: %v", err)
	}
}

func TestAppVersion(t *testing.T) {
	if v := AppVersion(); v == "" {
		t.Fatal("empty app version")
	}
}
