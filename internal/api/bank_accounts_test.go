package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

func bankURL(c *client, id uint) string {
	return fmt.Sprintf("%s/%d", c.acct("/bank-accounts"), id)
}

func TestBankAccountCreate(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	// Czech number → IBAN and SWIFT derived, currency from the account, first one default
	fio := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: " Fio ", Number: "2000145399/2010"})
	if fio.Name != "Fio" || fio.Currency != "CZK" || fio.IBAN != "CZ9320100000002000145399" ||
		fio.SwiftBIC != "FIOBCZPP" || !fio.IsDefault || fio.Number != "2000145399/2010" {
		t.Fatalf("fio: %+v", fio)
	}
	// with prefix; second CZK account is not default
	kb := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "KB", Number: "19-2000145399/0100"})
	if kb.IBAN != "CZ0801000000192000145399" || kb.SwiftBIC != "KOMBCZPP" || kb.IsDefault {
		t.Fatalf("kb: %+v", kb)
	}
	// foreign IBAN only, new currency → default in EUR; IBAN normalized
	eur := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "EUR", Currency: "EUR", IBAN: "de89 3704 0044 0532 0130 00", SwiftBIC: "cobadeffxxx"})
	if eur.IBAN != "DE89370400440532013000" || eur.SwiftBIC != "COBADEFFXXX" || !eur.IsDefault || eur.Number != "" {
		t.Fatalf("eur: %+v", eur)
	}
	// explicit default moves the CZK default
	kb2 := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "KB2", Number: "2000145399/0100", IsDefault: true})
	if !kb2.IsDefault {
		t.Fatalf("kb2: %+v", kb2)
	}
	fio = doJSON[api.BankAccount](a, http.StatusOK, "GET", bankURL(a, fio.ID), nil)
	if fio.IsDefault {
		t.Fatalf("fio still default: %+v", fio)
	}
	eur = doJSON[api.BankAccount](a, http.StatusOK, "GET", bankURL(a, eur.ID), nil)
	if !eur.IsDefault {
		t.Fatalf("EUR default lost: %+v", eur)
	}

	list := doJSON[api.ListResponse[api.BankAccount]](a, http.StatusOK, "GET", a.acct("/bank-accounts"), nil)
	if list.Total != 4 || list.Items[0].ID != kb2.ID || list.Items[3].ID != eur.ID {
		t.Fatalf("list: %+v", list)
	}
	czk := doJSON[api.ListResponse[api.BankAccount]](a, http.StatusOK, "GET", a.acct("/bank-accounts?currency=CZK"), nil)
	if czk.Total != 3 {
		t.Fatalf("CZK list: %+v", czk)
	}

	// validation
	for _, tc := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"name": "X", "number": "2000145398/2010"}, "body.number"}, // mod 11
		{map[string]any{"name": "X", "number": "abc"}, "body.number"},
		{map[string]any{"name": "X"}, "body.number"}, // neither number nor iban
		{map[string]any{"name": "X", "iban": "CZ6520100000002000145390"}, "body.iban"},
		{map[string]any{"name": "X", "number": "2000145399/2010", "iban": "CZ0801000000192000145399"}, "body.iban"}, // mismatch
		{map[string]any{"name": "X", "iban": "DE89370400440532013000", "swift_bic": "BAD"}, "body.swift_bic"},
		{map[string]any{"name": "X", "iban": "DE89370400440532013000", "currency": "eur"}, "currency"},
		{map[string]any{"name": "", "iban": "DE89370400440532013000"}, "name"},
		{map[string]any{"name": "  ", "iban": "DE89370400440532013000"}, "body.name"},
	} {
		res, body := a.do("POST", a.acct("/bank-accounts"), tc.body)
		assertError(t, res, body, http.StatusUnprocessableEntity, tc.field)
	}
}

func TestBankAccountPatchAndDelete(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	fio := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	kb := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "KB", Number: "19-2000145399/0100"})

	// changing the number re-derives IBAN and SWIFT
	got := doJSON[api.BankAccount](a, http.StatusOK, "PATCH", bankURL(a, fio.ID),
		map[string]any{"name": "Hlavní", "number": "2000145399/0100"})
	if got.Name != "Hlavní" || got.IBAN != "CZ2201000000002000145399" || got.SwiftBIC != "KOMBCZPP" || !got.IsDefault {
		t.Fatalf("patched: %+v", got)
	}
	// make kb default → fio loses it
	got = doJSON[api.BankAccount](a, http.StatusOK, "PATCH", bankURL(a, kb.ID), map[string]any{"is_default": true})
	if !got.IsDefault {
		t.Fatalf("kb: %+v", got)
	}
	if f := doJSON[api.BankAccount](a, http.StatusOK, "GET", bankURL(a, fio.ID), nil); f.IsDefault {
		t.Fatalf("fio still default: %+v", f)
	}
	// switch to a foreign account: clear number, set IBAN
	got = doJSON[api.BankAccount](a, http.StatusOK, "PATCH", bankURL(a, fio.ID),
		map[string]any{"number": "", "iban": "DE89370400440532013000", "swift_bic": "COBADEFFXXX", "currency": "EUR"})
	if got.Number != "" || got.IBAN != "DE89370400440532013000" || got.Currency != "EUR" {
		t.Fatalf("foreign: %+v", got)
	}

	res, body := a.do("PATCH", bankURL(a, kb.ID), map[string]any{"number": "124/0100"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "body.number")
	res, body = a.do("PATCH", bankURL(a, kb.ID), map[string]any{"iban": "DE89370400440532013000"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "body.iban")
	res, body = a.do("PATCH", bankURL(a, 9999), map[string]any{"name": "x"})
	assertError(t, res, body, http.StatusNotFound, "bank account not found")

	// deleting the default promotes the oldest remaining account of the currency
	kb3 := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "KB3", Number: "2000145399/2010"})
	kb4 := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "KB4", Number: "2000145399/0800"})
	a.mustDo(http.StatusNoContent, "DELETE", bankURL(a, kb.ID), nil)
	if g := doJSON[api.BankAccount](a, http.StatusOK, "GET", bankURL(a, kb3.ID), nil); !g.IsDefault {
		t.Fatalf("kb3 not promoted: %+v", g)
	}
	if g := doJSON[api.BankAccount](a, http.StatusOK, "GET", bankURL(a, kb4.ID), nil); g.IsDefault {
		t.Fatalf("kb4 promoted: %+v", g)
	}
	res, body = a.do("GET", bankURL(a, kb.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "bank account not found")
	res, body = a.do("DELETE", bankURL(a, kb.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "bank account not found")
}

func TestBankAccountsTenantIsolationAndRoles(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	ba := doJSON[api.BankAccount](a, http.StatusCreated, "POST", a.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio", Number: "2000145399/2010"})
	// b's own first CZK account must be default even though a has one
	bb := doJSON[api.BankAccount](b, http.StatusCreated, "POST", b.acct("/bank-accounts"),
		api.BankAccountCreate{Name: "Fio B", Number: "2000145399/2010", IsDefault: true})
	if !bb.IsDefault {
		t.Fatalf("b default: %+v", bb)
	}
	if g := doJSON[api.BankAccount](a, http.StatusOK, "GET", bankURL(a, ba.ID), nil); !g.IsDefault {
		t.Fatalf("a's default touched by b: %+v", g)
	}

	for _, r := range []struct {
		method string
		body   any
	}{{"GET", nil}, {"PATCH", map[string]any{"name": "hack"}}, {"DELETE", nil}} {
		res, body := b.do(r.method, bankURL(b, ba.ID), r.body)
		assertError(t, res, body, http.StatusNotFound, "bank account not found")
	}
	list := doJSON[api.ListResponse[api.BankAccount]](b, http.StatusOK, "GET", b.acct("/bank-accounts"), nil)
	if list.Total != 1 || list.Items[0].ID != bb.ID {
		t.Fatalf("b list: %+v", list)
	}

	// member: read-only
	var acc model.Account
	ts.db.Where("slug = ?", a.slug).First(&acc)
	var user model.User
	ts.db.Where("email = ?", "b@example.cz").First(&user)
	ts.db.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: model.RoleMember})
	b.mustDo(http.StatusOK, "GET", bankURL(a, ba.ID), nil)
	res, body := b.do("POST", a.acct("/bank-accounts"), api.BankAccountCreate{Name: "x", Number: "2000145399/2010"})
	assertError(t, res, body, http.StatusForbidden, "allowed roles: owner, admin")
	res, body = b.do("PATCH", bankURL(a, ba.ID), map[string]any{"name": "x"})
	assertError(t, res, body, http.StatusForbidden, "allowed roles: owner, admin")
	res, body = b.do("DELETE", bankURL(a, ba.ID), nil)
	assertError(t, res, body, http.StatusForbidden, "allowed roles: owner, admin")
}
