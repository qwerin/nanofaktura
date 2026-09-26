package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestAPITokens(t *testing.T) {
	ts := newTestServer(t)
	c := ts.signup("a@example.cz", "Firma A")

	created := doJSON[api.CreatedAPIToken](c, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "CLI"})
	if !strings.HasPrefix(created.Token, "nf_") || created.Prefix != created.Token[:8] || created.Name != "CLI" {
		t.Fatalf("token: %+v", created)
	}

	// bearer auth works without a cookie and records last use
	bot := &client{ts: ts, token: created.Token}
	me := doJSON[api.Me](bot, http.StatusOK, "GET", "/api/auth/me", nil)
	if me.User.Email != "a@example.cz" {
		t.Fatalf("me: %+v", me)
	}
	doJSON[api.Account](bot, http.StatusOK, "GET", c.acct(""), nil)

	list := doJSON[api.ListResponse[api.APIToken]](c, http.StatusOK, "GET", "/api/auth/tokens", nil)
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].LastUsedAt == nil || list.Page != 1 || list.PerPage != 50 {
		t.Fatalf("list: %+v", list)
	}
	if strings.Contains(string(c.mustDo(http.StatusOK, "GET", "/api/auth/tokens", nil)), created.Token) {
		t.Fatal("plaintext token leaked in list")
	}

	// revoked token no longer authenticates
	c.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("/api/auth/tokens/%d", created.ID), nil)
	res, body := bot.do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
	res, body = c.do("DELETE", fmt.Sprintf("/api/auth/tokens/%d", created.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "token not found")
}

func TestAPITokenIsolation(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	tok := doJSON[api.CreatedAPIToken](a, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "A"})

	list := doJSON[api.ListResponse[api.APIToken]](b, http.StatusOK, "GET", "/api/auth/tokens", nil)
	if list.Total != 0 {
		t.Fatalf("b sees a's tokens: %+v", list)
	}
	res, body := b.do("DELETE", fmt.Sprintf("/api/auth/tokens/%d", tok.ID), nil)
	assertError(t, res, body, http.StatusNotFound, "token not found")
}

func TestAPITokenInvalid(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")
	for _, tok := range []string{"nf_bogus", "bogus"} {
		res, body := (&client{ts: ts, token: tok}).do("GET", "/api/auth/me", nil)
		assertError(t, res, body, http.StatusUnauthorized, "authentication required")
	}
}

func TestAPITokenRequiresAuth(t *testing.T) {
	ts := newTestServer(t)
	res, body := ts.anon().do("POST", "/api/auth/tokens", api.APITokenCreate{Name: "x"})
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
}
