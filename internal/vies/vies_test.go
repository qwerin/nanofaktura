package vies_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/vies"
)

func TestSplit(t *testing.T) {
	for _, tc := range []struct{ in, cc, num string }{
		{"CZ27082440", "CZ", "27082440"},
		{" cz 270 824 40 ", "CZ", "27082440"},
		{"GR094014201", "EL", "094014201"},
		{"EL094014201", "EL", "094014201"},
		{"NL123456789B01", "NL", "123456789B01"},
		{"ATU12345678", "AT", "U12345678"},
	} {
		cc, num, err := vies.Split(tc.in)
		if err != nil || cc != tc.cc || num != tc.num {
			t.Errorf("Split(%q) = %s %s %v", tc.in, cc, num, err)
		}
	}
	for _, bad := range []string{"", "CZ", "27082440", "US123456789", "CZ12345678901234", "CZ123/45"} {
		if _, _, err := vies.Split(bad); !errors.Is(err, vies.ErrInvalidInput) {
			t.Errorf("Split(%q): %v", bad, err)
		}
	}
}

// fakeVIES replays responses recorded from the real API (testdata/*.json).
func fakeVIES(t *testing.T) (*httptest.Server, *int) {
	calls := 0
	routes := map[string]struct {
		status int
		file   string
	}{
		"/ms/CZ/vat/27082440":  {200, "valid_cz.json"},
		"/ms/CZ/vat/12345678":  {200, "invalid.json"},
		"/ms/DE/vat/811907980": {200, "valid_de_no_details.json"},
		"/ms/EL/vat/094014201": {200, "ms_unavailable.json"},
		"/ms/CZ/vat/ABC":       {400, "bad_request_400.json"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		rt, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
			return
		}
		b, err := os.ReadFile("testdata/" + rt.file)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestClientCheck(t *testing.T) {
	srv, _ := fakeVIES(t)
	c := vies.New(srv.URL)
	ctx := context.Background()

	r, err := c.Check(ctx, "CZ27082440")
	if err != nil {
		t.Fatal(err)
	}
	want := vies.Result{CountryCode: "CZ", VatNumber: "27082440", Valid: true, Name: "Alza.cz a.s.",
		Address: "Jankovcova 1522/53\nPRAHA 7 - HOLEŠOVICE\n170 00 PRAHA 7"}
	if *r != want || r.VatNo() != "CZ27082440" {
		t.Fatalf("got %+v", r)
	}

	r, err = c.Check(ctx, "DE811907980")
	if err != nil || !r.Valid || r.Name != "" || r.Address != "" {
		t.Fatalf("DE: %+v %v", r, err)
	}
	r, err = c.Check(ctx, "CZ12345678")
	if err != nil || r.Valid {
		t.Fatalf("invalid: %+v %v", r, err)
	}
	if _, err = c.Check(ctx, "GR094014201"); !errors.Is(err, vies.ErrUnavailable) {
		t.Fatalf("MS_UNAVAILABLE: %v", err)
	}
	if _, err = c.Check(ctx, "CZABC"); !errors.Is(err, vies.ErrInvalidInput) {
		t.Fatalf("400: %v", err)
	}
	if _, err = c.Check(ctx, "SK1234567890"); !errors.Is(err, vies.ErrUnavailable) {
		t.Fatalf("504: %v", err)
	}
	if _, err = c.Check(ctx, "XX1"); !errors.Is(err, vies.ErrInvalidInput) {
		t.Fatalf("local validation: %v", err)
	}
	if _, err = vies.New("http://127.0.0.1:1").Check(ctx, "CZ27082440"); !errors.Is(err, vies.ErrUnavailable) {
		t.Fatalf("down: %v", err)
	}
}

func TestCached(t *testing.T) {
	srv, calls := fakeVIES(t)
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	c := vies.NewCached(vies.New(srv.URL), 24*time.Hour, func() time.Time { return now })
	ctx := context.Background()
	for _, in := range []string{"CZ27082440", "cz 27082440", "CZ27082440"} {
		if r, err := c.Check(ctx, in); err != nil || r.Name != "Alza.cz a.s." {
			t.Fatalf("%+v %v", r, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("calls %d", *calls)
	}
	_, _ = c.Check(ctx, "EL094014201")
	_, _ = c.Check(ctx, "EL094014201")
	if *calls != 3 {
		t.Fatalf("errors must not be cached, calls %d", *calls)
	}
	now = now.Add(24 * time.Hour)
	_, _ = c.Check(ctx, "CZ27082440")
	if *calls != 4 {
		t.Fatalf("expired entry not refetched, calls %d", *calls)
	}
}
