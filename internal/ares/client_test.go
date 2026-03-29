package ares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchByIC_OK(t *testing.T) {
	// Mockujeme ARES server lokálně – testy nesmí záviset na síti
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := aresResponse{
			ICO:           "27082440",
			ObchodniJmeno: "Ukázková s.r.o.",
			DIC:           "CZ27082440",
		}
		resp.Sidlo.UliceNazev = "Václavské náměstí"
		resp.Sidlo.CisloDomovniOrientacni = "1"
		resp.Sidlo.ObecNazev = "Praha"
		resp.Sidlo.PSC = "11000"

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Přesměrujeme klienta na mock server
	c := &Client{http: srv.Client()}
	// Nahradíme base URL mock serverem pomocí custom RoundTripper
	c.http = &http.Client{
		Transport: rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
	}

	subj, err := c.FetchByIC(context.Background(), "27082440")
	if err != nil {
		t.Fatalf("neočekávaná chyba: %v", err)
	}
	if subj.Name != "Ukázková s.r.o." {
		t.Errorf("Name = %q, want %q", subj.Name, "Ukázková s.r.o.")
	}
	if subj.Street != "Václavské náměstí 1" {
		t.Errorf("Street = %q, want %q", subj.Street, "Václavské náměstí 1")
	}
	if subj.ZIP != "11000" {
		t.Errorf("ZIP = %q, want %q", subj.ZIP, "11000")
	}
}

func TestFetchByIC_ShortIC(t *testing.T) {
	c := New(nil)
	_, err := c.FetchByIC(context.Background(), "123")
	if err == nil {
		t.Error("očekávána chyba pro krátké IČO, žádná nebyla vrácena")
	}
}

// rewriteTransport přesměruje všechny požadavky na testovací server.
type rewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = req.URL.Host // zachováme – mock server to přepíše automaticky
	// Jednoduché přepsání: nahradíme host mock serverem
	req.URL.Host = t.base[len("http://"):]
	if t.inner == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.inner.RoundTrip(req)
}
