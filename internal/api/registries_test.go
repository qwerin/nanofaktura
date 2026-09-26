package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/cnb"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/vatreg"
	"github.com/qwerin/nanofaktura/internal/vies"
)

type fakeRates struct{ lastDate string }

func (f *fakeRates) Rate(_ context.Context, currency, date string) (string, string, error) {
	f.lastDate = date
	switch {
	case date == "bad":
		return "", "", cnb.ErrInvalidDate
	case currency == "EUR" || currency == "eur":
		return "24.350", "2026-03-13", nil
	case currency == "XAU":
		return "", "", cnb.ErrUnknownCurrency
	case currency == "USD":
		return "", "", errors.New("connection refused")
	default:
		return "", "", cnb.ErrInvalidCurrency
	}
}

type fakeVIES struct{}

func (fakeVIES) Check(_ context.Context, vatNo string) (*vies.Result, error) {
	switch vatNo {
	case "DE811907980":
		return &vies.Result{CountryCode: "DE", VatNumber: "811907980", Valid: true}, nil
	case "CZ12345678":
		return &vies.Result{CountryCode: "CZ", VatNumber: "12345678"}, nil
	case "EL094014201":
		return nil, fmt.Errorf("%w: MS_UNAVAILABLE", vies.ErrUnavailable)
	}
	return nil, vies.ErrInvalidInput
}

type fakeVatReg struct{}

func (fakeVatReg) Check(_ context.Context, dic string) (*vatreg.Result, error) {
	f := false
	switch dic {
	case "CZ699001234":
		return &vatreg.Result{VatNo: "CZ699001234", Registered: true, Reliable: &f, UnreliableSince: "2024-03-01",
			Name: "JAN NOVÁK", Street: "Nádražní 12", City: "Beroun", Zip: "26601",
			Accounts: []vatreg.Account{{Number: "19-2000145399/0800", IBAN: "CZ6508000000192000145399", PublishedOn: "2019-05-02"}}}, nil
	case "12345678":
		return &vatreg.Result{VatNo: "CZ12345678"}, nil
	case "11111111":
		return nil, vatreg.ErrUnavailable
	}
	return nil, vatreg.ErrInvalidDIC
}

func withRegistries(ts *testServer, d api.Deps) {
	d.Now = func() time.Time { return ts.now }
	ts.handler, _ = api.New(ts.db, config.Config{AllowSignup: true}, d)
}

func TestExchangeRates(t *testing.T) {
	ts := newTestServer(t)
	rates := &fakeRates{}
	withRegistries(ts, api.Deps{CNB: rates})
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.ExchangeRate](c, http.StatusOK, "GET", "/api/exchange-rates?currency=eur&date=2026-03-14", nil)
	if got != (api.ExchangeRate{Currency: "EUR", Date: "2026-03-14", RateDate: "2026-03-13", Rate: "24.350"}) {
		t.Fatalf("got %+v", got)
	}
	got = doJSON[api.ExchangeRate](c, http.StatusOK, "GET", "/api/exchange-rates?currency=EUR", nil)
	if got.Date != "2026-03-15" || rates.lastDate != "2026-03-15" { // ts.now, Prague calendar
		t.Fatalf("default date: %+v", got)
	}

	res, body := c.do("GET", "/api/exchange-rates?currency=EUR&date=bad", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "query.date")
	res, body = c.do("GET", "/api/exchange-rates?currency=EURO", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "query.currency")
	res, body = c.do("GET", "/api/exchange-rates", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "currency")
	res, body = c.do("GET", "/api/exchange-rates?currency=XAU", nil)
	assertError(t, res, body, http.StatusNotFound, "exchange rate not found")
	res, body = c.do("GET", "/api/exchange-rates?currency=USD", nil)
	assertError(t, res, body, http.StatusBadGateway, "unavailable")
	res, body = ts.anon().do("GET", "/api/exchange-rates?currency=EUR", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

// TestExchangeRatesRealClient runs the default ČNB client + DB cache against a
// fake ČNB server (NANOFAKTURA_CNB_URL) serving a recorded list.
func TestExchangeRatesRealClient(t *testing.T) {
	list, err := os.ReadFile("../cnb/testdata/denni_kurz_2026-09-25.txt")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("date") != "26.09.2026" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(list)
	}))
	defer srv.Close()
	ts := newTestServer(t, func(c *config.Config) { c.CNBURL = srv.URL })
	ts.now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.ExchangeRate](c, http.StatusOK, "GET", "/api/exchange-rates?currency=JPY&date=2026-09-26", nil)
	if got != (api.ExchangeRate{Currency: "JPY", Date: "2026-09-26", RateDate: "2026-09-25", Rate: "0.13546"}) {
		t.Fatalf("got %+v", got)
	}
	got = doJSON[api.ExchangeRate](c, http.StatusOK, "GET", "/api/exchange-rates?currency=EUR&date=2026-09-26", nil)
	if got.Rate != "24.350" || calls != 1 {
		t.Fatalf("got %+v, calls %d", got, calls)
	}
	res, body := c.do("GET", "/api/exchange-rates?currency=BGN&date=2026-09-26", nil)
	assertError(t, res, body, http.StatusNotFound, "exchange rate not found")
	res, body = c.do("GET", "/api/exchange-rates?currency=EUR&date=2026-09-29", nil)
	assertError(t, res, body, http.StatusBadGateway, "unavailable")
}

func TestVies(t *testing.T) {
	ts := newTestServer(t)
	withRegistries(ts, api.Deps{VIES: fakeVIES{}})
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.ViesResult](c, http.StatusOK, "GET", "/api/vies/DE811907980", nil)
	if got != (api.ViesResult{VatNo: "DE811907980", CountryCode: "DE", Valid: true}) {
		t.Fatalf("got %+v", got)
	}
	got = doJSON[api.ViesResult](c, http.StatusOK, "GET", "/api/vies/CZ12345678", nil)
	if got.Valid {
		t.Fatalf("got %+v", got)
	}
	res, body := c.do("GET", "/api/vies/US123", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "path.vat_no")
	res, body = c.do("GET", "/api/vies/EL094014201", nil)
	assertError(t, res, body, http.StatusBadGateway, "VIES is unavailable")
	res, body = ts.anon().do("GET", "/api/vies/DE811907980", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

// TestViesRealClient uses the default (cached) client against a fake VIES
// replaying the recorded response.
func TestViesRealClient(t *testing.T) {
	valid, err := os.ReadFile("../vies/testdata/valid_cz.json")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/ms/CZ/vat/27082440" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(valid)
	}))
	defer srv.Close()
	ts := newTestServer(t, func(c *config.Config) { c.ViesURL = srv.URL })
	c := ts.signup("a@example.cz", "Firma A")

	for range 2 {
		got := doJSON[api.ViesResult](c, http.StatusOK, "GET", "/api/vies/CZ27082440", nil)
		if !got.Valid || got.Name != "Alza.cz a.s." || got.Address != "Jankovcova 1522/53\nPRAHA 7 - HOLEŠOVICE\n170 00 PRAHA 7" {
			t.Fatalf("got %+v", got)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 24h cache, calls %d", calls)
	}
	res, body := c.do("GET", "/api/vies/SK2020202020", nil)
	assertError(t, res, body, http.StatusBadGateway, "VIES is unavailable")
}

func TestVatRegistry(t *testing.T) {
	ts := newTestServer(t)
	withRegistries(ts, api.Deps{VatRegistry: fakeVatReg{}})
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.VatRegistryResult](c, http.StatusOK, "GET", "/api/vat-registry/CZ699001234", nil)
	if !got.Registered || got.Reliable == nil || *got.Reliable || got.UnreliableSince != "2024-03-01" ||
		got.Address != "Nádražní 12, 266 01 Beroun" || len(got.PublishedAccounts) != 1 ||
		got.PublishedAccounts[0] != (api.VatRegistryAccount{Number: "19-2000145399/0800", IBAN: "CZ6508000000192000145399", PublishedOn: "2019-05-02"}) {
		t.Fatalf("got %+v", got)
	}
	body := c.mustDo(http.StatusOK, "GET", "/api/vat-registry/12345678", nil)
	if strings.TrimSpace(string(body)) != `{"vat_no":"CZ12345678","registered":false,"reliable":null,"name":"","address":"","street":"","city":"","zip":"","published_accounts":[]}` {
		t.Fatalf("not registered: %s", body)
	}
	res, b := c.do("GET", "/api/vat-registry/abc", nil)
	assertError(t, res, b, http.StatusUnprocessableEntity, "path.dic")
	res, b = c.do("GET", "/api/vat-registry/11111111", nil)
	assertError(t, res, b, http.StatusBadGateway, "registry is unavailable")
	res, b = ts.anon().do("GET", "/api/vat-registry/12345678", nil)
	assertError(t, res, b, http.StatusUnauthorized, "authentication required")
}

// TestVatRegistryRealClient uses the default SOAP client against a fake
// endpoint replaying the recorded MFČR response.
func TestVatRegistryRealClient(t *testing.T) {
	resp, err := os.ReadFile("../vatreg/testdata/status_rozsireny.xml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml;charset=utf-8")
		_, _ = w.Write(resp)
	}))
	defer srv.Close()
	ts := newTestServer(t, func(c *config.Config) { c.VatRegURL = srv.URL })
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.VatRegistryResult](c, http.StatusOK, "GET", "/api/vat-registry/CZ27082440", nil)
	if got.VatNo != "CZ27082440" || got.Reliable == nil || !*got.Reliable || got.Name != "ALZA.CZ A.S." || len(got.PublishedAccounts) != 19 {
		t.Fatalf("got %+v", got)
	}
}
