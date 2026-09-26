package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

func subjectURL(c *client, id uint) string {
	return fmt.Sprintf("%s/%d", c.acct("/subjects"), id)
}

func strPtr(s string) *string { return &s }

func TestSubjectCreateAndGet(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	minimal := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), api.SubjectCreate{Name: " ACME "})
	if minimal.Name != "ACME" || minimal.Type != "customer" || minimal.Country != "CZ" || minimal.CustomID != nil || minimal.DueDays != nil {
		t.Fatalf("defaults: %+v", minimal)
	}
	due := 30
	full := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), api.SubjectCreate{
		Type: "both", CustomID: strPtr(" K-001 "), Name: "Dodavatel s.r.o.", FullName: "Jan Novák",
		RegistrationNo: "0006947", VatNo: "cz 00006947", Street: "Letenská 15", City: "Praha", Zip: "11800",
		Country: "sk", Email: "info@example.cz", IBAN: "cz65 0800 0000 1920 0014 5399", SwiftBIC: "gibaczpx",
		DueDays: &due, Note: "VIP",
	})
	if full.Type != "both" || full.CustomID == nil || *full.CustomID != "K-001" || full.RegistrationNo != "00006947" ||
		full.VatNo != "CZ00006947" || full.Country != "SK" || full.IBAN != "CZ6508000000192000145399" ||
		full.SwiftBIC != "GIBACZPX" || full.DueDays == nil || *full.DueDays != 30 || full.Note != "VIP" {
		t.Fatalf("full: %+v", full)
	}
	got := doJSON[api.Subject](a, http.StatusOK, "GET", subjectURL(a, full.ID), nil)
	if got.Name != full.Name || *got.CustomID != "K-001" {
		t.Fatalf("get: %+v", got)
	}
	res, body := a.do("GET", subjectURL(a, 9999), nil)
	assertError(t, res, body, http.StatusNotFound, "subject not found")

	// empty custom_id is stored as NULL, so several subjects may have none
	e1 := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), api.SubjectCreate{Name: "E1", CustomID: strPtr("")})
	doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), api.SubjectCreate{Name: "E2", CustomID: strPtr(" ")})
	if e1.CustomID != nil {
		t.Fatalf("empty custom_id: %+v", e1)
	}
	res, body = a.do("POST", a.acct("/subjects"), api.SubjectCreate{Name: "Dup", CustomID: strPtr("K-001")})
	assertError(t, res, body, http.StatusConflict, "custom_id")

	for _, tc := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"name": ""}, "name"},
		{map[string]any{"name": "   "}, "body.name"},
		{map[string]any{"name": "X", "registration_no": "12345678"}, "body.registration_no"}, // checksum
		{map[string]any{"name": "X", "registration_no": "123"}, "body.registration_no"},
		{map[string]any{"name": "X", "country": "C1"}, "body.country"},
		{map[string]any{"name": "X", "country": "CZE"}, "country"},
		{map[string]any{"name": "X", "iban": "CZ0000000000000000000000"}, "body.iban"},
		{map[string]any{"name": "X", "swift_bic": "12"}, "body.swift_bic"},
		{map[string]any{"name": "X", "type": "partner"}, "type"},
		{map[string]any{"name": "X", "due_days": -1}, "due_days"},
	} {
		res, body := a.do("POST", a.acct("/subjects"), tc.body)
		assertError(t, res, body, http.StatusUnprocessableEntity, tc.field)
	}
}

func TestSubjectList(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	for _, s := range []api.SubjectCreate{
		{Name: "Zeta a.s.", Type: "supplier", Email: "zeta@example.cz"},
		{Name: "alfa s.r.o.", RegistrationNo: "27074358"},
		{Name: "Beta", Type: "both", Email: "Office@Beta.cz"},
		{Name: "100% jistota", Type: "customer"},
	} {
		doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"), s)
	}
	names := func(q string) []string {
		t.Helper()
		l := doJSON[api.ListResponse[api.Subject]](a, http.StatusOK, "GET", a.acct("/subjects"+q), nil)
		out := make([]string, len(l.Items))
		for i, s := range l.Items {
			out[i] = s.Name
		}
		return out
	}
	check := func(q string, want ...string) {
		t.Helper()
		got := names(q)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: got %q, want %q", q, got, want)
		}
	}
	check("", "100% jistota", "alfa s.r.o.", "Beta", "Zeta a.s.") // case-insensitive order
	check("?query=ALFA", "alfa s.r.o.")
	check("?query=2707", "alfa s.r.o.") // IČO
	check("?query=office@beta", "Beta") // email, case-insensitive
	check("?query=%25", "100% jistota") // LIKE wildcard is escaped
	check("?type=customer", "100% jistota", "alfa s.r.o.", "Beta")
	check("?type=supplier", "Beta", "Zeta a.s.")
	check("?type=both", "Beta")
	check("?type=supplier&query=zeta", "Zeta a.s.")
	check("?query=nothing")
	check("?per_page=2&page=2", "Beta", "Zeta a.s.")

	res, body := a.do("GET", a.acct("/subjects?type=partner"), nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "type")
}

func TestSubjectPatchAndDelete(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	s := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"),
		api.SubjectCreate{Name: "ACME", CustomID: strPtr("A1"), City: "Brno"})
	other := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"),
		api.SubjectCreate{Name: "Other", CustomID: strPtr("B1")})

	got := doJSON[api.Subject](a, http.StatusOK, "PATCH", subjectURL(a, s.ID), map[string]any{
		"name": "ACME s.r.o.", "type": "supplier", "registration_no": "27074358", "due_days": 0, "country": "de",
	})
	if got.Name != "ACME s.r.o." || got.Type != "supplier" || got.RegistrationNo != "27074358" ||
		got.DueDays == nil || *got.DueDays != 0 || got.Country != "DE" || got.City != "Brno" || *got.CustomID != "A1" {
		t.Fatalf("patched: %+v", got)
	}
	res, body := a.do("PATCH", subjectURL(a, s.ID), map[string]any{"custom_id": "B1"})
	assertError(t, res, body, http.StatusConflict, "custom_id")
	got = doJSON[api.Subject](a, http.StatusOK, "PATCH", subjectURL(a, s.ID), map[string]any{"custom_id": ""})
	if got.CustomID != nil {
		t.Fatalf("custom_id not cleared: %+v", got)
	}
	res, body = a.do("PATCH", subjectURL(a, s.ID), map[string]any{"registration_no": "27074359"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "body.registration_no")
	res, body = a.do("PATCH", subjectURL(a, s.ID), map[string]any{"name": " "})
	assertError(t, res, body, http.StatusUnprocessableEntity, "body.name")
	res, body = a.do("PATCH", subjectURL(a, 9999), map[string]any{"name": "x"})
	assertError(t, res, body, http.StatusNotFound, "subject not found")

	// a subject with invoices cannot be deleted
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	if err := ts.db.Create(&model.Invoice{AccountID: acc.ID, DocumentType: model.DocInvoice, Number: "2026-0001",
		Status: model.StatusOpen, SubjectID: other.ID, IssuedOn: "2026-03-15", Currency: "CZK",
		ExchangeRate: "1", Language: "cs", PaymentMethod: "bank"}).Error; err != nil {
		t.Fatal(err)
	}
	res, body = a.do("DELETE", subjectURL(a, other.ID), nil)
	assertError(t, res, body, http.StatusConflict, "invoices")

	a.mustDo(http.StatusNoContent, "DELETE", subjectURL(a, s.ID), nil)
	res, body = a.do("GET", subjectURL(a, s.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "subject not found")
	res, body = a.do("DELETE", subjectURL(a, s.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "subject not found")
}

func TestSubjectsTenantIsolation(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	sa := doJSON[api.Subject](a, http.StatusCreated, "POST", a.acct("/subjects"),
		api.SubjectCreate{Name: "Shared name", CustomID: strPtr("X1")})
	// custom_id is unique per account only
	sb := doJSON[api.Subject](b, http.StatusCreated, "POST", b.acct("/subjects"),
		api.SubjectCreate{Name: "Shared name", CustomID: strPtr("X1")})

	for _, r := range []struct {
		method string
		body   any
	}{{"GET", nil}, {"PATCH", map[string]any{"name": "hack"}}, {"DELETE", nil}} {
		res, body := b.do(r.method, subjectURL(b, sa.ID), r.body)
		assertError(t, res, body, http.StatusNotFound, "subject not found")
	}
	list := doJSON[api.ListResponse[api.Subject]](b, http.StatusOK, "GET", b.acct("/subjects?query=shared"), nil)
	if list.Total != 1 || list.Items[0].ID != sb.ID {
		t.Fatalf("b list: %+v", list)
	}
	res, body := b.do("GET", a.acct("/subjects"), nil)
	assertError(t, res, body, http.StatusNotFound, "account not found")

	// a member of the account can manage subjects
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	var user model.User
	ts.db.Where("email = ?", "b@example.cz").First(&user)
	ts.db.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: model.RoleMember})
	doJSON[api.Subject](b, http.StatusOK, "PATCH", subjectURL(a, sa.ID), map[string]any{"note": "member edit"})
}
