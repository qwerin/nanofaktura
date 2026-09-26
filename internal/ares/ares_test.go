package ares_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/ares"
)

func TestNormalizeICO(t *testing.T) {
	tests := []struct{ in, want string }{
		{"27074358", "27074358"},
		{" 2707 4358 ", "27074358"},
		{"00006947", "00006947"},
		{"6947", ""}, // too short
		{"0006947", "00006947"},
		{"25596641", "25596641"}, // checksum remainder 0 → check digit 1
		{"27074359", ""},
		{"2707435a", ""},
		{"123456789", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got, err := ares.NormalizeICO(tt.in)
		if tt.want == "" {
			if !errors.Is(err, ares.ErrInvalidICO) {
				t.Errorf("NormalizeICO(%q) = %q, %v; want ErrInvalidICO", tt.in, got, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeICO(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

// fakeARES serves canned responses keyed by IČO.
func fakeARES(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/ekonomicke-subjekty/") {
			http.NotFound(w, r)
			return
		}
		ico := strings.TrimPrefix(r.URL.Path, "/ekonomicke-subjekty/")
		switch ico {
		case "00000019":
			http.Error(w, `{"kod":"CHYBA"}`, http.StatusInternalServerError)
			return
		}
		body, ok := responses[ico]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"kod":"NENALEZENO","popis":"Ekonomický subjekt nenalezen"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLookup(t *testing.T) {
	srv := fakeARES(t, map[string]string{
		"27074358": `{"ico":"27074358","obchodniJmeno":"Firma a.s.","dic":"CZ27074358",
			"sidlo":{"kodStatu":"CZ","nazevObce":"Praha","nazevCastiObce":"Michle","nazevMestskeCastiObvodu":"Praha 4",
			"nazevUlice":"Budějovická","cisloDomovni":778,"cisloOrientacni":3,"cisloOrientacniPismeno":"a","psc":14000,
			"textovaAdresa":"Budějovická 778/3a, Michle, 14000 Praha 4"}}`,
		// village without streets, no DIČ
		"25596641": `{"ico":"25596641","obchodniJmeno":"Jan Novák",
			"sidlo":{"kodStatu":"CZ","nazevObce":"Lhota","nazevCastiObce":"Dolní Lhota","cisloDomovni":12,"psc":6601}}`,
		// only a text address
		"00006947": `{"ico":"00006947","obchodniJmeno":"Úřad","sidlo":{"textovaAdresa":"Letenská 525/15, Malá Strana, 118 00 Praha 1"}}`,
		// evidence number, orientation number only
		"45274649": `{"ico":"45274649","obchodniJmeno":"Chata s.r.o.",
			"sidlo":{"kodStatu":"CZ","nazevObce":"Brno","nazevMestskeCastiObvodu":"Brno-střed","nazevUlice":"Nádražní","cisloDomovni":5,"typCisloDomovni":2,"psc":60200}}`,
	})
	c := ares.New(srv.URL + "/ekonomicke-subjekty/")

	tests := []struct {
		ico  string
		want ares.Result
	}{
		{"27074358", ares.Result{RegistrationNo: "27074358", VatNo: "CZ27074358", Name: "Firma a.s.",
			Street: "Budějovická 778/3a", City: "Praha 4", Zip: "14000", Country: "CZ"}},
		{"25596641", ares.Result{RegistrationNo: "25596641", Name: "Jan Novák",
			Street: "Dolní Lhota 12", City: "Lhota", Zip: "06601", Country: "CZ"}},
		{"6947", ares.Result{}}, // invalid, checked below
		{"00006947", ares.Result{RegistrationNo: "00006947", Name: "Úřad",
			Street: "Letenská 525/15", City: "Praha 1", Zip: "11800", Country: "CZ"}},
		{"45274649", ares.Result{RegistrationNo: "45274649", Name: "Chata s.r.o.",
			Street: "Nádražní č. ev. 5", City: "Brno-střed", Zip: "60200", Country: "CZ"}},
	}
	for _, tt := range tests {
		got, err := c.Lookup(context.Background(), tt.ico)
		if tt.want.RegistrationNo == "" {
			if !errors.Is(err, ares.ErrInvalidICO) {
				t.Errorf("Lookup(%s): %v, want ErrInvalidICO", tt.ico, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("Lookup(%s): %v", tt.ico, err)
			continue
		}
		if *got != tt.want {
			t.Errorf("Lookup(%s) = %+v\nwant %+v", tt.ico, *got, tt.want)
		}
	}

	if _, err := c.Lookup(context.Background(), "26168685"); !errors.Is(err, ares.ErrNotFound) {
		t.Errorf("unknown IČO: %v", err)
	}
	_, err := c.Lookup(context.Background(), "00000019")
	if err == nil || errors.Is(err, ares.ErrNotFound) || errors.Is(err, ares.ErrInvalidICO) {
		t.Errorf("upstream error: %v", err)
	}

	down := ares.New("http://127.0.0.1:1")
	if _, err := down.Lookup(context.Background(), "27074358"); err == nil || errors.Is(err, ares.ErrNotFound) {
		t.Errorf("unreachable: %v", err)
	}
}

func TestParseTextAddressFallbacks(t *testing.T) {
	srv := fakeARES(t, map[string]string{
		"27074358": `{"ico":"27074358","obchodniJmeno":"X","sidlo":{"textovaAdresa":"Nějaká adresa bez PSČ"}}`,
	})
	got, err := ares.New(srv.URL+"/ekonomicke-subjekty").Lookup(context.Background(), "27074358")
	if err != nil {
		t.Fatal(err)
	}
	if got.Street != "Nějaká adresa bez PSČ" || got.City != "" || got.Zip != "" || got.Country != "CZ" {
		t.Fatalf("got %+v", got)
	}
}
