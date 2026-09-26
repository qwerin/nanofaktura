package api_test

import (
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

func TestAccountDefaults(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	acc := doJSON[api.Account](c, http.StatusOK, "GET", c.acct(""), nil)
	if acc.Slug != "firma-a" || acc.Role != "owner" || acc.Country != "CZ" || acc.VatMode != "non_vat_payer" ||
		acc.DefaultCurrency != "CZK" || acc.DefaultDueDays != 14 || acc.DefaultPaymentMethod != "bank" ||
		acc.DefaultLanguage != "cs" || acc.DefaultVatRateBps != 2100 || acc.RoundTotal {
		t.Fatalf("defaults: %+v", acc)
	}

	var formats []model.NumberFormat
	ts.db.Order("id").Find(&formats)
	got := map[string]string{}
	for _, f := range formats {
		if !f.IsDefault {
			t.Fatalf("format not default: %+v", f)
		}
		got[f.DocumentType] = f.Format
	}
	want := map[string]string{"invoice": "{YYYY}-{NNNN}", "proforma": "Z{YYYY}-{NNNN}", "correction": "D{YYYY}-{NNNN}", "expense": "N{YYYY}-{NNNN}"}
	if len(got) != len(want) || got["invoice"] != want["invoice"] || got["proforma"] != want["proforma"] ||
		got["correction"] != want["correction"] || got["expense"] != want["expense"] {
		t.Fatalf("number formats: %v", got)
	}
}

func TestCreateAndListAccounts(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma")

	acc := doJSON[api.Account](c, http.StatusCreated, "POST", "/api/accounts", api.AccountCreate{Name: "Firma"})
	if acc.Slug != "firma-2" || acc.Role != "owner" {
		t.Fatalf("created: %+v", acc)
	}
	doJSON[api.Account](c, http.StatusCreated, "POST", "/api/accounts", api.AccountCreate{Name: "Firma"})

	list := doJSON[api.ListResponse[api.Account]](c, http.StatusOK, "GET", "/api/accounts", nil)
	if list.Total != 3 || len(list.Items) != 3 || list.Items[2].Slug != "firma-3" {
		t.Fatalf("list: %+v", list)
	}
	page := doJSON[api.ListResponse[api.Account]](c, http.StatusOK, "GET", "/api/accounts?page=2&per_page=2", nil)
	if page.Total != 3 || len(page.Items) != 1 || page.Page != 2 || page.PerPage != 2 {
		t.Fatalf("page 2: %+v", page)
	}
	res, body := c.do("GET", "/api/accounts?per_page=500", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "per_page")

	me := doJSON[api.Me](c, http.StatusOK, "GET", "/api/auth/me", nil)
	if len(me.Accounts) != 3 {
		t.Fatalf("me accounts: %+v", me.Accounts)
	}

	res, body = c.do("POST", "/api/accounts", api.AccountCreate{Name: ""})
	assertError(t, res, body, http.StatusUnprocessableEntity, "name")
}

func TestPatchAccount(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	acc := doJSON[api.Account](c, http.StatusOK, "PATCH", c.acct(""), map[string]any{
		"name": "Firma A s.r.o.", "registration_no": "12345678", "vat_mode": "vat_payer",
		"default_due_days": 30, "round_total": true, "default_vat_rate_bps": 0,
	})
	if acc.Name != "Firma A s.r.o." || acc.RegistrationNo != "12345678" || acc.VatMode != "vat_payer" ||
		acc.DefaultDueDays != 30 || !acc.RoundTotal || acc.DefaultVatRateBps != 0 || acc.Slug != "firma-a" {
		t.Fatalf("patched: %+v", acc)
	}
	// unchanged fields are kept, false/0 can be set
	acc = doJSON[api.Account](c, http.StatusOK, "PATCH", c.acct(""), map[string]any{"round_total": false})
	if acc.RoundTotal || acc.DefaultDueDays != 30 || acc.Name != "Firma A s.r.o." {
		t.Fatalf("second patch: %+v", acc)
	}

	for field, value := range map[string]any{"vat_mode": "bogus", "country": "cz", "default_currency": "Kč", "name": "   "} {
		res, body := c.do("PATCH", c.acct(""), map[string]any{field: value})
		assertError(t, res, body, http.StatusUnprocessableEntity, field)
	}
}

func TestAccountNonMemberGets404(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	for _, method := range []string{"GET", "PATCH"} {
		res, body := b.do(method, a.acct(""), map[string]any{"name": "hacked"})
		assertError(t, res, body, http.StatusNotFound, "account not found")
	}
	res, body := b.do("GET", "/api/accounts/does-not-exist", nil)
	assertError(t, res, body, http.StatusNotFound, "account not found")

	list := doJSON[api.ListResponse[api.Account]](b, http.StatusOK, "GET", "/api/accounts", nil)
	if list.Total != 1 || list.Items[0].Slug != "firma-b" {
		t.Fatalf("b's accounts: %+v", list)
	}
	res, body = ts.anon().do("GET", a.acct(""), nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

func TestMemberCannotPatchAccount(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")

	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	var user model.User
	ts.db.Where("email = ?", "b@example.cz").First(&user)
	ts.db.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: model.RoleMember})

	got := doJSON[api.Account](b, http.StatusOK, "GET", a.acct(""), nil)
	if got.Role != "member" {
		t.Fatalf("role = %q", got.Role)
	}
	res, body := b.do("PATCH", a.acct(""), map[string]any{"name": "hacked"})
	assertError(t, res, body, http.StatusForbidden, "allowed roles: owner, admin")
}
