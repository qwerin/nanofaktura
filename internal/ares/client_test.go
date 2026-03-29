package ares

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchByIC_OK(t *testing.T) {
	// Mockujeme ARES server lokálně – testy nesmí záviset na síti.
	// Číselná pole (psc, cisloDomovni, cisloOrientacni) jsou v ARES jako JSON number.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := aresResponse{
			ICO:           "27082440",
			ObchodniJmeno: "Ukázková s.r.o.",
			DIC:           "CZ27082440",
		}
		resp.Sidlo.UliceNazev = "Václavské náměstí"
		resp.Sidlo.CisloDomovni = 1
		resp.Sidlo.CisloOrientacni = 2
		resp.Sidlo.ObecNazev = "Praha"
		resp.Sidlo.PSC = 11000

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{
		Transport: rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
	}}

	subj, err := c.FetchByIC(context.Background(), "27082440")
	if err != nil {
		t.Fatalf("neočekávaná chyba: %v", err)
	}
	if subj.Name != "Ukázková s.r.o." {
		t.Errorf("Name = %q, want %q", subj.Name, "Ukázková s.r.o.")
	}
	if subj.Street != "Václavské náměstí 1/2" {
		t.Errorf("Street = %q, want %q", subj.Street, "Václavské náměstí 1/2")
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

func TestFetchByIC_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{
		Transport: rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
	}}

	_, err := c.FetchByIC(context.Background(), "00000000")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestFetchByIC_Live volá živé ARES API – spusť jen ručně: go test -run TestFetchByIC_Live -v
func TestFetchByIC_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("live test přeskočen v short módu")
	}

	c := New(nil)
	subj, err := c.FetchByIC(context.Background(), "25356275")
	if err != nil {
		t.Fatalf("ARES chyba: %v", err)
	}

	t.Logf("IC:     %s", subj.IC)
	t.Logf("Name:   %s", subj.Name)
	t.Logf("DIC:    %s", subj.DIC)
	t.Logf("Street: %s", subj.Street)
	t.Logf("City:   %s", subj.City)
	t.Logf("ZIP:    %s", subj.ZIP)

	if subj.Name == "" {
		t.Error("Name je prázdné")
	}
	if subj.Street == "" {
		t.Error("Street je prázdné")
	}
	if subj.ZIP == "" {
		t.Error("ZIP je prázdné")
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
	req.URL.Host = t.base[len("http://"):]
	if t.inner == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.inner.RoundTrip(req)
}
