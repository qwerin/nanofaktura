package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
)

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	h := doJSON[api.Health](ts.anon(), http.StatusOK, "GET", "/api/health", nil)
	if h.Status != "ok" {
		t.Fatalf("status = %q", h.Status)
	}
}

func TestAuthStatus(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.AllowSignup = false })
	st := doJSON[api.AuthStatus](ts.anon(), http.StatusOK, "GET", "/api/auth/status", nil)
	if !st.SignupAllowed || st.HasUsers {
		t.Fatalf("empty instance: %+v", st)
	}
	ts.signup("a@example.cz", "Firma A")
	st = doJSON[api.AuthStatus](ts.anon(), http.StatusOK, "GET", "/api/auth/status", nil)
	if st.SignupAllowed || !st.HasUsers {
		t.Fatalf("after first user: %+v", st)
	}
}

func TestRegister(t *testing.T) {
	ts := newTestServer(t)
	c := ts.anon()
	res, body := c.do("POST", "/api/auth/register", api.RegisterRequest{
		Email: " Jan@Example.CZ ", Name: "Jan", Password: testPassword, AccountName: "Účetní Žluťoučký s.r.o.",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	ck := res.Cookies()[0]
	if ck.Name != "nf_session" || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Secure {
		t.Fatalf("bad cookie: %+v", ck)
	}
	me := decodeJSON[api.Me](t, body)
	if me.User.Email != "jan@example.cz" || me.User.Name != "Jan" {
		t.Fatalf("user: %+v", me.User)
	}
	if len(me.Accounts) != 1 || me.Accounts[0] != (api.MeAccount{Slug: "ucetni-zlutoucky-s-r-o", Name: "Účetní Žluťoučký s.r.o.", Role: "owner"}) {
		t.Fatalf("accounts: %+v", me.Accounts)
	}
	// session cookie works
	doJSON[api.Me](c, http.StatusOK, "GET", "/api/auth/me", nil)
}

func TestRegisterErrors(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")

	res, body := ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{
		Email: "A@example.cz", Name: "A", Password: testPassword, AccountName: "X",
	})
	assertError(t, res, body, http.StatusConflict, "already registered")

	res, body = ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{
		Email: "b@example.cz", Name: "B", Password: "short", AccountName: "X",
	})
	assertError(t, res, body, http.StatusUnprocessableEntity, "password")

	res, body = ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{
		Email: "not-an-email", Name: "B", Password: testPassword, AccountName: "X",
	})
	assertError(t, res, body, http.StatusUnprocessableEntity, "email")
}

func TestRegisterDisabledAfterFirstUser(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.AllowSignup = false })
	ts.signup("a@example.cz", "Firma A") // first user is always allowed
	res, body := ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{
		Email: "b@example.cz", Name: "B", Password: testPassword, AccountName: "Firma B",
	})
	assertError(t, res, body, http.StatusForbidden, "signup is disabled")
}

func TestLogin(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")

	c := ts.anon()
	me := doJSON[api.Me](c, http.StatusOK, "POST", "/api/auth/login", api.LoginRequest{Email: "A@Example.cz", Password: testPassword})
	if me.User.Email != "a@example.cz" || len(me.Accounts) != 1 {
		t.Fatalf("me: %+v", me)
	}
	if c.session == "" {
		t.Fatal("no session cookie")
	}
	doJSON[api.Me](c, http.StatusOK, "GET", "/api/auth/me", nil)
}

func TestLoginWrongCredentials(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")
	for _, req := range []api.LoginRequest{
		{Email: "a@example.cz", Password: "wrong-password"},
		{Email: "nobody@example.cz", Password: testPassword},
	} {
		c := ts.anon()
		res, body := c.do("POST", "/api/auth/login", req)
		assertError(t, res, body, http.StatusUnauthorized, "invalid email or password")
		if c.session != "" {
			t.Fatal("session cookie set on failed login")
		}
	}
}

func TestLogout(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")
	old := c.session

	c.mustDo(http.StatusNoContent, "POST", "/api/auth/logout", nil)
	if c.session != "" {
		t.Fatal("cookie not cleared")
	}
	// the old cookie value is no longer valid
	c.session = old
	res, body := c.do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

func TestSessionExpiry(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	// used after 20 days: expiry slides forward and the cookie is refreshed
	ts.now = ts.now.Add(20 * 24 * time.Hour)
	res, _ := c.do("GET", "/api/auth/me", nil)
	if res.StatusCode != http.StatusOK || len(res.Cookies()) != 1 {
		t.Fatalf("status %d, cookies %v", res.StatusCode, res.Cookies())
	}
	var sess model.Session
	ts.db.First(&sess)
	if want := ts.now.Add(30 * 24 * time.Hour); !sess.ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", sess.ExpiresAt, want)
	}

	// idle for more than 30 days: expired
	ts.now = ts.now.Add(31 * 24 * time.Hour)
	res, body := c.do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

func TestMeRequiresAuth(t *testing.T) {
	ts := newTestServer(t)
	res, body := ts.anon().do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")

	c := ts.anon()
	c.session = "forged"
	res, body = c.do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}

func TestPatchMe(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	me := doJSON[api.Me](c, http.StatusOK, "PATCH", "/api/auth/me", map[string]any{"name": "Nové jméno"})
	if me.User.Name != "Nové jméno" {
		t.Fatalf("name = %q", me.User.Name)
	}

	res, body := c.do("PATCH", "/api/auth/me", map[string]any{"password": "noveheslo123"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "current password")
	res, body = c.do("PATCH", "/api/auth/me", map[string]any{"password": "noveheslo123", "current_password": "wrong"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "current password")

	c.mustDo(http.StatusOK, "PATCH", "/api/auth/me", map[string]any{"password": "noveheslo123", "current_password": testPassword})
	res, body = ts.anon().do("POST", "/api/auth/login", api.LoginRequest{Email: "a@example.cz", Password: testPassword})
	assertError(t, res, body, http.StatusUnauthorized, "invalid")
	doJSON[api.Me](ts.anon(), http.StatusOK, "POST", "/api/auth/login", api.LoginRequest{Email: "a@example.cz", Password: "noveheslo123"})
}

func TestOpenAPIDocumentsSlugParam(t *testing.T) {
	ts := newTestServer(t)
	body := string(ts.anon().mustDo(http.StatusOK, "GET", "/api/openapi.json", nil))
	if !strings.Contains(body, `"/api/accounts/{slug}"`) || !strings.Contains(body, `"name":"slug"`) {
		t.Fatal("slug path parameter missing from OpenAPI")
	}
}
