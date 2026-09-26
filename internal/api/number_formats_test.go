package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/numbering"
)

// formatsByType returns the account's number formats of one document type.
func formatsByType(t *testing.T, c *client, docType string) []api.NumberFormat {
	t.Helper()
	return doJSON[api.ListResponse[api.NumberFormat]](c, http.StatusOK, "GET", c.acct("/number-formats?document_type="+docType), nil).Items
}

func TestNumberFormatsListAndGet(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	list := doJSON[api.ListResponse[api.NumberFormat]](a, http.StatusOK, "GET", a.acct("/number-formats"), nil)
	if list.Total != 4 || len(list.Items) != 4 {
		t.Fatalf("list: %+v", list)
	}
	inv := formatsByType(t, a, "invoice")
	if len(inv) != 1 || inv[0].Format != "{YYYY}-{NNNN}" || !inv[0].IsDefault {
		t.Fatalf("invoice formats: %+v", inv)
	}
	exp := formatsByType(t, a, "expense")
	if len(exp) != 1 || exp[0].Format != "N{YYYY}-{NNNN}" {
		t.Fatalf("expense formats: %+v", exp)
	}
	got := doJSON[api.NumberFormat](a, http.StatusOK, "GET", fmt.Sprintf("%s/%d", a.acct("/number-formats"), inv[0].ID), nil)
	if got != inv[0] {
		t.Fatalf("get: %+v", got)
	}
	res, body := a.do("GET", a.acct("/number-formats/9999"), nil)
	assertError(t, res, body, http.StatusNotFound, "number format not found")
	res, body = a.do("GET", a.acct("/number-formats?document_type=bogus"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "document_type")
}

func TestNumberFormatCreateAndDefault(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	old := formatsByType(t, a, "invoice")[0]

	// non-default format of a type that already has a default
	nf := doJSON[api.NumberFormat](a, http.StatusCreated, "POST", a.acct("/number-formats"),
		api.NumberFormatCreate{DocumentType: "invoice", Format: " FV{YY}{MM}{NNN} "})
	if nf.IsDefault || nf.Format != "FV{YY}{MM}{NNN}" || nf.DocumentType != "invoice" {
		t.Fatalf("created: %+v", nf)
	}
	// new default unsets the old one
	def := doJSON[api.NumberFormat](a, http.StatusCreated, "POST", a.acct("/number-formats"),
		api.NumberFormatCreate{DocumentType: "invoice", Format: "{YYYY}{NNNNN}", IsDefault: true})
	if !def.IsDefault {
		t.Fatalf("default: %+v", def)
	}
	defaults := 0
	for _, f := range formatsByType(t, a, "invoice") {
		if f.IsDefault {
			defaults++
			if f.ID != def.ID {
				t.Fatalf("wrong default: %+v", f)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults = %d", defaults)
	}

	// PATCH is_default=true moves the default back
	got := doJSON[api.NumberFormat](a, http.StatusOK, "PATCH", fmt.Sprintf("%s/%d", a.acct("/number-formats"), old.ID),
		map[string]any{"is_default": true})
	if !got.IsDefault {
		t.Fatalf("patched: %+v", got)
	}
	def = doJSON[api.NumberFormat](a, http.StatusOK, "GET", fmt.Sprintf("%s/%d", a.acct("/number-formats"), def.ID), nil)
	if def.IsDefault {
		t.Fatalf("old default kept: %+v", def)
	}

	// the first format of a type becomes default even when not asked: simulate an empty type
	ts.db.Where("document_type = ?", model.DocExpense).Delete(&model.NumberFormat{})
	exp := doJSON[api.NumberFormat](a, http.StatusCreated, "POST", a.acct("/number-formats"),
		api.NumberFormatCreate{DocumentType: "expense", Format: "E{N}"})
	if !exp.IsDefault {
		t.Fatalf("first expense format not default: %+v", exp)
	}

	// validation and conflicts
	for _, f := range []string{"{YYYY}", "{YYYY}-{NNNN", "{X}{N}", "{NNNNNNN}"} {
		res, body := a.do("POST", a.acct("/number-formats"), api.NumberFormatCreate{DocumentType: "invoice", Format: f})
		assertError(t, res, body, http.StatusUnprocessableEntity, "body.format")
	}
	res, body := a.do("POST", a.acct("/number-formats"), api.NumberFormatCreate{DocumentType: "offer", Format: "{N}"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "document_type")
	res, body = a.do("POST", a.acct("/number-formats"), api.NumberFormatCreate{DocumentType: "invoice", Format: "{YYYY}-{NNNN}"})
	assertError(t, res, body, http.StatusConflict, "already exists")
	// the same format for another document type is fine
	doJSON[api.NumberFormat](a, http.StatusCreated, "POST", a.acct("/number-formats"),
		api.NumberFormatCreate{DocumentType: "proforma", Format: "{YYYY}-{NNNN}"})
}

func TestNumberFormatPatchAndDelete(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	def := formatsByType(t, a, "invoice")[0]
	defURL := fmt.Sprintf("%s/%d", a.acct("/number-formats"), def.ID)

	got := doJSON[api.NumberFormat](a, http.StatusOK, "PATCH", defURL, map[string]any{"format": "F{YYYY}/{NNN}"})
	if got.Format != "F{YYYY}/{NNN}" || !got.IsDefault || got.DocumentType != "invoice" {
		t.Fatalf("patched: %+v", got)
	}
	res, body := a.do("PATCH", defURL, map[string]any{"format": "no-sequence"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "body.format")
	res, body = a.do("PATCH", defURL, map[string]any{"is_default": false})
	assertError(t, res, body, http.StatusConflict, "default")

	// deleting the only / default format of a type is refused
	res, body = a.do("DELETE", defURL, nil)
	assertError(t, res, body, http.StatusConflict, "default")
	var only model.NumberFormat
	ts.db.Where("document_type = ?", model.DocProforma).First(&only)
	ts.db.Model(&only).Update("is_default", false) // corrupt state: the last format is not default
	res, body = a.do("DELETE", fmt.Sprintf("%s/%d", a.acct("/number-formats"), only.ID), nil)
	assertError(t, res, body, http.StatusConflict, "last number format")

	// a non-default one can be deleted, including its counters
	other := doJSON[api.NumberFormat](a, http.StatusCreated, "POST", a.acct("/number-formats"),
		api.NumberFormatCreate{DocumentType: "invoice", Format: "X{N}"})
	ts.db.Create(&model.NumberCounter{NumberFormatID: other.ID, Period: "", LastNumber: 5})
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("%s/%d", a.acct("/number-formats"), other.ID), nil)
	var n int64
	ts.db.Model(&model.NumberCounter{}).Where("number_format_id = ?", other.ID).Count(&n)
	if n != 0 {
		t.Fatalf("counters left: %d", n)
	}
	res, body = a.do("DELETE", fmt.Sprintf("%s/%d", a.acct("/number-formats"), other.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "number format not found")
}

func TestNumberFormatPreview(t *testing.T) {
	ts := newTestServer(t) // clock: 2026-03-15
	a := ts.signup("a@example.cz", "Firma A")
	def := formatsByType(t, a, "invoice")[0]
	url := fmt.Sprintf("%s/%d/preview", a.acct("/number-formats"), def.ID)

	p := doJSON[api.NumberPreview](a, http.StatusOK, "GET", url, nil)
	if p.Number != "2026-0001" {
		t.Fatalf("preview today: %+v", p)
	}
	p = doJSON[api.NumberPreview](a, http.StatusOK, "GET", url+"?date=2025-12-31", nil)
	if p.Number != "2025-0001" {
		t.Fatalf("preview 2025: %+v", p)
	}

	// issue two numbers through numbering.Next (as invoice creation does)
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	for range 2 {
		if err := ts.db.Transaction(func(tx *gorm.DB) error {
			_, err := numbering.Next(tx, acc.ID, model.DocInvoice, "2026-02-01")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 { // preview does not move the counter
		p = doJSON[api.NumberPreview](a, http.StatusOK, "GET", url+"?date=2026-06-30", nil)
		if p.Number != "2026-0003" {
			t.Fatalf("preview after 2: %+v", p)
		}
	}

	res, body := a.do("GET", url+"?date=15.3.2026", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "date")
	res, body = a.do("GET", a.acct("/number-formats/9999/preview"), nil)
	assertError(t, res, body, http.StatusNotFound, "number format not found")
}

func TestNumberFormatsTenantIsolationAndRoles(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	aFormat := formatsByType(t, a, "invoice")[0]
	foreign := fmt.Sprintf("%s/%d", b.acct("/number-formats"), aFormat.ID)

	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"GET", foreign, nil}, {"PATCH", foreign, map[string]any{"format": "HACK{N}"}},
		{"DELETE", foreign, nil}, {"GET", foreign + "/preview", nil},
	} {
		res, body := b.do(r.method, r.path, r.body)
		assertError(t, res, body, http.StatusNotFound, "number format not found")
	}
	if list := formatsByType(t, b, "invoice"); len(list) != 1 || list[0].ID == aFormat.ID {
		t.Fatalf("b sees: %+v", list)
	}
	res, body := b.do("GET", a.acct("/number-formats"), nil)
	assertError(t, res, body, http.StatusNotFound, "account not found")

	// a member (not owner) can read but not change number formats
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	var user model.User
	ts.db.Where("email = ?", "b@example.cz").First(&user)
	ts.db.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: model.RoleMember})
	own := fmt.Sprintf("%s/%d", a.acct("/number-formats"), aFormat.ID)
	b.mustDo(http.StatusOK, "GET", own, nil)
	b.mustDo(http.StatusOK, "GET", own+"/preview", nil)
	res, body = b.do("PATCH", own, map[string]any{"format": "HACK{N}"})
	assertError(t, res, body, http.StatusForbidden, "owner")
	res, body = b.do("POST", a.acct("/number-formats"), api.NumberFormatCreate{DocumentType: "invoice", Format: "M{N}"})
	assertError(t, res, body, http.StatusForbidden, "owner")
	res, body = b.do("DELETE", own, nil)
	assertError(t, res, body, http.StatusForbidden, "owner")
}
