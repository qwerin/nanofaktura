package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/ares"
	"github.com/qwerin/nanofaktura/internal/config"
)

// fakeARES returns canned results; unknown IČOs are not found, "00000019" is an outage.
type fakeARES map[string]ares.Result

func (f fakeARES) Lookup(_ context.Context, ico string) (*ares.Result, error) {
	if ico == "00000019" {
		return nil, errors.New("connection refused")
	}
	if ico == "00000027" {
		return nil, fmt.Errorf("ares: %w", context.DeadlineExceeded)
	}
	r, ok := f[ico]
	if !ok {
		return nil, ares.ErrNotFound
	}
	return &r, nil
}

// withARES rebuilds the test server's API with a replaced ARES dependency.
func withARES(ts *testServer, a api.ARES) {
	ts.handler, _ = api.New(ts.db, config.Config{AllowSignup: true},
		api.Deps{Now: func() time.Time { return ts.now }, ARES: a})
}

func TestAresLookup(t *testing.T) {
	ts := newTestServer(t)
	withARES(ts, fakeARES{"27074358": {RegistrationNo: "27074358", VatNo: "CZ27074358", Name: "Firma a.s.",
		Street: "Budějovická 778/3a", City: "Praha 4", Zip: "14000", Country: "CZ"}})
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.AresSubject](c, http.StatusOK, "GET", "/api/ares/27074358", nil)
	want := api.AresSubject{RegistrationNo: "27074358", VatNo: "CZ27074358", Name: "Firma a.s.",
		Street: "Budějovická 778/3a", City: "Praha 4", Zip: "14000", Country: "CZ"}
	if got != want {
		t.Fatalf("got %+v", got)
	}

	res, body := c.do("GET", "/api/ares/27074359", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "path.ico")
	res, body = c.do("GET", "/api/ares/abc", nil)
	assertError(t, res, body, http.StatusUnprocessableEntity, "path.ico")
	res, body = c.do("GET", "/api/ares/26168685", nil)
	assertError(t, res, body, http.StatusNotFound, "not found")
	res, body = c.do("GET", "/api/ares/0000019", nil) // 7 digits, padded
	assertError(t, res, body, http.StatusBadGateway, "ARES is unavailable")
	res, body = c.do("GET", "/api/ares/00000027", nil)
	assertCode(t, res, body, http.StatusGatewayTimeout, "upstream_timeout")
	res, body = ts.anon().do("GET", "/api/ares/27074358", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

// TestAresLookupRealClient runs the default client against a fake ARES server
// configured through NANOFAKTURA_ARES_URL (cfg.AresURL).
func TestAresLookupRealClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ekonomicke-subjekty/27074358" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"ico":"27074358","obchodniJmeno":"Firma a.s.","sidlo":{"kodStatu":"CZ","nazevObce":"Praha",
			"nazevUlice":"Budějovická","cisloDomovni":778,"psc":14000}}`))
	}))
	defer srv.Close()
	ts := newTestServer(t, func(c *config.Config) { c.AresURL = srv.URL + "/ekonomicke-subjekty" })
	c := ts.signup("a@example.cz", "Firma A")

	got := doJSON[api.AresSubject](c, http.StatusOK, "GET", "/api/ares/27074358", nil)
	if got.Name != "Firma a.s." || got.Street != "Budějovická 778" || got.City != "Praha" || got.Zip != "14000" {
		t.Fatalf("got %+v", got)
	}
	res, body := c.do("GET", "/api/ares/26168685", nil)
	assertError(t, res, body, http.StatusNotFound, "not found")
}
