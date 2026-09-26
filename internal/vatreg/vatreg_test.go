package vatreg_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/vatreg"
)

func TestNormalizeDIC(t *testing.T) {
	for in, want := range map[string]string{"CZ27082440": "27082440", " cz 270 824 40": "27082440", "7001011234": "7001011234"} {
		if got, err := vatreg.NormalizeDIC(in); err != nil || got != want {
			t.Errorf("%q → %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "CZ", "SK2020202020", "1234567", "12345678901", "CZ12a45678"} {
		if _, err := vatreg.NormalizeDIC(bad); !errors.Is(err, vatreg.ErrInvalidDIC) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// fakeRegistry answers like the MFČR SOAP service with recorded responses:
// the real response for 27082440 (+ NENALEZEN for 12345678) and synthetic ones
// following the WSDL for an unreliable payer, maintenance and a SOAP fault.
func fakeRegistry(t *testing.T) (*httptest.Server, *int) {
	calls := 0
	dicRe := regexp.MustCompile(`<roz:dic>(\d+)</roz:dic>`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.Header.Get("SOAPAction") != `"http://adis.mfcr.cz/rozhraniCRPDPH/getStatusNespolehlivyPlatceRozsireny"` {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		m := dicRe.FindSubmatch(body)
		if m == nil {
			http.Error(w, "no dic", http.StatusBadRequest)
			return
		}
		file := map[string]string{
			"27082440": "status_rozsireny.xml", "12345678": "status_rozsireny.xml",
			"699001234": "status_nespolehlivy.xml", "11111111": "status_odstavka.xml", "22222222": "fault.xml",
		}[string(m[1])]
		if file == "" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		b, _ := os.ReadFile("testdata/" + file)
		w.Header().Set("Content-Type", "text/xml;charset=utf-8")
		if file == "fault.xml" {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestClientCheck(t *testing.T) {
	srv, _ := fakeRegistry(t)
	c := vatreg.New(srv.URL)
	ctx := context.Background()

	r, err := c.Check(ctx, "CZ27082440")
	if err != nil {
		t.Fatal(err)
	}
	if r.VatNo != "CZ27082440" || !r.Registered || r.Reliable == nil || !*r.Reliable || r.TaxOffice != "13" {
		t.Fatalf("status %+v", r)
	}
	if r.Name != "ALZA.CZ A.S." || r.Street != "Jankovcova 1522/53" || r.City != "PRAHA 7" || r.Zip != "17000" ||
		r.CityPart != "HOLEŠOVICE (PRAHA 7)" || r.Country != "Česká republika" {
		t.Fatalf("name/address %+v", r)
	}
	if r.Address() != "Jankovcova 1522/53, 170 00 PRAHA 7" {
		t.Fatalf("address %q", r.Address())
	}
	if len(r.Accounts) != 19 {
		t.Fatalf("accounts %d", len(r.Accounts))
	}
	if a := r.Accounts[0]; a != (vatreg.Account{Number: "35-3355550267/0100", IBAN: "CZ1801000000353355550267", PublishedOn: "2013-04-09"}) {
		t.Fatalf("standard account %+v", a)
	}
	if a := r.Accounts[4]; a.Number != "SK2909000000005033845424" || a.IBAN != "SK2909000000005033845424" {
		t.Fatalf("foreign account %+v", a)
	}
	if !r.HasAccount("2531920606/2600") || !r.HasAccount("CZ40 2600 0000 0025 3192 0518") || r.HasAccount("123/0100") || r.HasAccount("") {
		t.Fatal("HasAccount")
	}

	r, err = c.Check(ctx, "12345678")
	if err != nil || r.Registered || r.Reliable != nil || len(r.Accounts) != 0 || r.Name != "" {
		t.Fatalf("not found: %+v %v", r, err)
	}

	r, err = c.Check(ctx, "CZ699001234")
	if err != nil || !r.Registered || r.Reliable == nil || *r.Reliable || r.UnreliableSince != "2024-03-01" || r.Name != "JAN NOVÁK" {
		t.Fatalf("unreliable: %+v %v", r, err)
	}
	if len(r.Accounts) != 1 || r.Accounts[0].Number != "19-2000145399/0800" || r.Accounts[0].IBAN != "CZ6508000000192000145399" {
		t.Fatalf("unreliable accounts %+v", r.Accounts)
	}

	for _, dic := range []string{"11111111", "22222222", "33333333"} {
		if _, err := c.Check(ctx, dic); !errors.Is(err, vatreg.ErrUnavailable) {
			t.Errorf("%s: %v", dic, err)
		}
	}
	if _, err := c.Check(ctx, "abc"); !errors.Is(err, vatreg.ErrInvalidDIC) {
		t.Errorf("invalid: %v", err)
	}
}

func TestCached(t *testing.T) {
	srv, calls := fakeRegistry(t)
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	c := vatreg.NewCached(vatreg.New(srv.URL), 24*time.Hour, func() time.Time { return now })
	ctx := context.Background()
	for _, dic := range []string{"CZ27082440", "27082440"} {
		if r, err := c.Check(ctx, dic); err != nil || !r.Registered {
			t.Fatalf("%+v %v", r, err)
		}
	}
	_, _ = c.Check(ctx, "11111111")
	_, _ = c.Check(ctx, "11111111")
	if *calls != 3 {
		t.Fatalf("calls %d", *calls)
	}
	now = now.Add(25 * time.Hour)
	_, _ = c.Check(ctx, "27082440")
	if *calls != 4 {
		t.Fatalf("calls %d", *calls)
	}
}
