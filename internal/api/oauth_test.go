package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

const testVerifier = "verifier-0123456789-0123456789-0123456789-abcdef"

func testChallenge(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// oauthForm POSTs a form-encoded token request (no cookies, like a client backend).
func oauthForm(ts *testServer, form url.Values, basicUser, basicPass string) (*http.Response, map[string]any) {
	ts.t.Helper()
	req := httptest.NewRequest("POST", "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basicUser != "" {
		req.SetBasicAuth(basicUser, basicPass)
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec.Result(), decodeJSON[map[string]any](ts.t, rec.Body.Bytes())
}

// registerClient registers an OAuth client and returns its registration response.
func registerClient(ts *testServer, body map[string]any) map[string]any {
	ts.t.Helper()
	return doJSON[map[string]any](ts.anon(), http.StatusCreated, "POST", "/api/oauth/register", body)
}

func authorizeParams(clientID, redirect string) api.OAuthAuthorizeParams {
	return api.OAuthAuthorizeParams{ClientID: clientID, RedirectURI: redirect, ResponseType: "code", State: "xyz",
		CodeChallenge: testChallenge(testVerifier), CodeChallengeMethod: "S256", Resource: "http://localhost:8080/api/mcp"}
}

// approve runs the consent as c and returns the code from the redirect.
func approve(t *testing.T, c *client, p api.OAuthAuthorizeParams) string {
	t.Helper()
	out := doJSON[api.OAuthRedirect](c, http.StatusOK, "POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: p, Approve: true})
	u, err := url.Parse(out.RedirectTo)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") != p.State || q.Get("iss") != "http://localhost:8080" || q.Get("code") == "" {
		t.Fatalf("redirect: %s", out.RedirectTo)
	}
	return q.Get("code")
}

func TestOAuthMetadata(t *testing.T) {
	ts := newTestServer(t)
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/api/mcp"} {
		m := doJSON[map[string]any](ts.anon(), http.StatusOK, "GET", path, nil)
		if m["resource"] != "http://localhost:8080/api/mcp" || m["authorization_servers"].([]any)[0] != "http://localhost:8080" {
			t.Fatalf("%s: %v", path, m)
		}
	}
	m := doJSON[map[string]any](ts.anon(), http.StatusOK, "GET", "/.well-known/oauth-authorization-server", nil)
	if m["issuer"] != "http://localhost:8080" || m["authorization_endpoint"] != "http://localhost:8080/oauth/authorize" ||
		m["token_endpoint"] != "http://localhost:8080/api/oauth/token" || m["registration_endpoint"] != "http://localhost:8080/api/oauth/register" {
		t.Fatalf("server metadata: %v", m)
	}
	// the MCP endpoint points unauthenticated clients to the metadata
	res, _ := ts.anon().do("POST", "/api/mcp", map[string]any{})
	if res.StatusCode != http.StatusUnauthorized ||
		!strings.Contains(res.Header.Get("WWW-Authenticate"), `resource_metadata="http://localhost:8080/.well-known/oauth-protected-resource/api/mcp"`) {
		t.Fatalf("401: %d %v", res.StatusCode, res.Header)
	}
}

func TestOAuthRegister(t *testing.T) {
	ts := newTestServer(t)
	reg := registerClient(ts, map[string]any{"client_name": " Claude‮ ", "redirect_uris": []string{"https://claude.ai/api/mcp/auth_callback"}})
	if !strings.HasPrefix(reg["client_id"].(string), "nfc_") || reg["client_name"] != "Claude" ||
		reg["token_endpoint_auth_method"] != "none" || reg["client_secret"] != nil {
		t.Fatalf("register: %v", reg)
	}
	for _, uri := range []string{"http://evil.example/cb", "javascript:alert(1)", "https://x.example/cb#frag", "https://u:p@x.example/"} {
		res, body := ts.anon().do("POST", "/api/oauth/register", map[string]any{"redirect_uris": []string{uri}})
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "invalid_redirect_uri") {
			t.Fatalf("%s: %d %s", uri, res.StatusCode, body)
		}
	}
	res, body := ts.anon().do("POST", "/api/oauth/register", map[string]any{"redirect_uris": []string{"https://x.example/cb"}, "grant_types": []string{"password"}})
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "invalid_client_metadata") {
		t.Fatalf("grant type: %d %s", res.StatusCode, body)
	}
}

func TestOAuthFlow(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	const redirect = "https://claude.ai/api/mcp/auth_callback"
	reg := registerClient(ts, map[string]any{"client_name": "Claude", "redirect_uris": []string{redirect}})
	clientID := reg["client_id"].(string)
	p := authorizeParams(clientID, redirect)

	// consent page info; bad requests are refused without redirecting
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"code_challenge": {p.CodeChallenge}, "code_challenge_method": {"S256"}, "state": {"xyz"}}
	info := doJSON[api.OAuthConsent](a, http.StatusOK, "GET", "/api/oauth/authorize?"+q.Encode(), nil)
	if info.ClientName != "Claude" || info.RedirectHost != "claude.ai" {
		t.Fatalf("consent: %+v", info)
	}
	bad := p
	bad.RedirectURI = "https://evil.example/cb"
	res, body := a.do("POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: bad, Approve: true})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "oauth_invalid_redirect")
	bad = p
	bad.CodeChallengeMethod = "plain"
	res, body = a.do("POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: bad, Approve: true})
	assertError(t, res, body, http.StatusUnprocessableEntity, "S256")
	bad = p
	bad.Resource = "https://other.example/api/mcp"
	res, body = a.do("POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: bad, Approve: true})
	assertError(t, res, body, http.StatusUnprocessableEntity, "resource")
	res, body = ts.anon().do("POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: p, Approve: true})
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
	// an API token cannot grant access to further applications
	tok := &client{ts: ts, token: apiToken(a)}
	res, body = tok.do("POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: p, Approve: true})
	assertCode(t, res, body, http.StatusForbidden, "oauth_session_required")

	// deny
	denied := doJSON[api.OAuthRedirect](a, http.StatusOK, "POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: p})
	if !strings.HasPrefix(denied.RedirectTo, redirect+"?") || !strings.Contains(denied.RedirectTo, "error=access_denied") || !strings.Contains(denied.RedirectTo, "state=xyz") {
		t.Fatalf("deny: %s", denied.RedirectTo)
	}

	// a wrong verifier spends the code
	code := approve(t, a, p)
	exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {code},
		"redirect_uri": {redirect}, "code_verifier": {strings.Repeat("x", 43)}}
	if res, out := oauthForm(ts, exchange, "", ""); res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier: %d %v", res.StatusCode, out)
	}
	exchange.Set("code_verifier", testVerifier)
	if res, out := oauthForm(ts, exchange, "", ""); res.StatusCode != http.StatusBadRequest || out["error"] != "invalid_grant" {
		t.Fatalf("reused code: %d %v", res.StatusCode, out)
	}
	// wrong redirect_uri and unknown client
	exchange.Set("code", approve(t, a, p))
	exchange.Set("redirect_uri", redirect+"x")
	if res, out := oauthForm(ts, exchange, "", ""); out["error"] != "invalid_grant" {
		t.Fatalf("redirect mismatch: %d %v", res.StatusCode, out)
	}
	if res, out := oauthForm(ts, url.Values{"grant_type": {"authorization_code"}, "client_id": {"nope"}}, "", ""); res.StatusCode != http.StatusUnauthorized || out["error"] != "invalid_client" {
		t.Fatalf("unknown client: %d %v", res.StatusCode, out)
	}
	// expired code
	exchange.Set("code", approve(t, a, p))
	exchange.Set("redirect_uri", redirect)
	ts.now = ts.now.Add(11 * time.Minute)
	if _, out := oauthForm(ts, exchange, "", ""); out["error"] != "invalid_grant" {
		t.Fatalf("expired code: %v", out)
	}

	// success
	exchange.Set("code", approve(t, a, p))
	exchange.Set("resource", "http://localhost:8080/api/mcp/")
	res, out := oauthForm(ts, exchange, "", "")
	if res.StatusCode != http.StatusOK || out["token_type"] != "Bearer" || out["expires_in"] != float64(3600) ||
		res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("token: %d %v", res.StatusCode, out)
	}
	access, refresh := out["access_token"].(string), out["refresh_token"].(string)

	// the access token works with MCP and the API
	s := mcpSession(t, ts, access)
	if accs := mustTool[api.ListResponse[api.Account]](t, s, "list_accounts", nil); accs.Total != 1 {
		t.Fatalf("mcp: %+v", accs)
	}
	// connected applications are listed apart from personal tokens
	grants := doJSON[api.ListResponse[api.OAuthGrantOut]](a, http.StatusOK, "GET", "/api/auth/oauth-grants", nil)
	if grants.Total != 1 || grants.Items[0].ClientName != "Claude" || grants.Items[0].LastUsedAt == nil {
		t.Fatalf("grants: %+v", grants)
	}
	if toks := doJSON[api.ListResponse[api.APIToken]](a, http.StatusOK, "GET", "/api/auth/tokens", nil); toks.Total != 1 {
		t.Fatalf("personal tokens: %+v", toks)
	}

	// refresh rotates the refresh token
	refreshForm := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {refresh}}
	res, out = oauthForm(ts, refreshForm, "", "")
	if res.StatusCode != http.StatusOK || out["refresh_token"] == refresh || out["access_token"] == access {
		t.Fatalf("refresh: %d %v", res.StatusCode, out)
	}
	if _, again := oauthForm(ts, refreshForm, "", ""); again["error"] != "invalid_grant" {
		t.Fatalf("old refresh token: %v", again)
	}
	access2 := out["access_token"].(string)

	// disconnecting revokes every token of the grant
	a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("/api/auth/oauth-grants/%d", grants.Items[0].ID), nil)
	for _, tk := range []string{access, access2} {
		if res, _ := (&client{ts: ts, token: tk}).do("GET", "/api/auth/me", nil); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("revoked token: %d", res.StatusCode)
		}
	}
	if _, out := oauthForm(ts, url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {out["refresh_token"].(string)}}, "", ""); out["error"] != "invalid_grant" {
		t.Fatalf("refresh after disconnect: %v", out)
	}
	// another user cannot delete someone's grant
	b := ts.signup("b@example.cz", "Firma B")
	res, body = b.do("DELETE", "/api/auth/oauth-grants/1", nil)
	assertError(t, res, body, http.StatusNotFound, "not found")
}

// TestOAuthNativeClient: loopback redirect on any port, confidential client with a secret.
func TestOAuthNativeClient(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	reg := registerClient(ts, map[string]any{"client_name": "CLI", "redirect_uris": []string{"http://127.0.0.1:3000/callback"},
		"token_endpoint_auth_method": "client_secret_basic"})
	clientID, secret := reg["client_id"].(string), reg["client_secret"].(string)
	p := authorizeParams(clientID, "http://127.0.0.1:53111/callback")
	p.Resource = ""
	code := approve(t, a, p)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {p.RedirectURI}, "code_verifier": {testVerifier}}
	if res, out := oauthForm(ts, form, clientID, "wrong"); res.StatusCode != http.StatusUnauthorized || out["error"] != "invalid_client" {
		t.Fatalf("wrong secret: %d %v", res.StatusCode, out)
	}
	form.Set("code", approve(t, a, p))
	if res, out := oauthForm(ts, form, clientID, secret); res.StatusCode != http.StatusOK || out["access_token"] == nil {
		t.Fatalf("token: %d %v", res.StatusCode, out)
	}
	// another path on loopback is not allowed
	p.RedirectURI = "http://127.0.0.1:53111/other"
	res, body := a.do("POST", "/api/oauth/authorize", api.OAuthDecision{OAuthAuthorizeParams: p, Approve: true})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "oauth_invalid_redirect")
}

func TestOAuthCleanupAndLimits(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	registerClient(ts, map[string]any{"redirect_uris": []string{"https://x.example/cb"}})
	ts.now = ts.now.Add(8 * 24 * time.Hour)
	runJob(ts, "auth-cleanup")
	var n int64
	ts.db.Model(&model.OAuthClient{}).Count(&n)
	if n != 0 {
		t.Fatalf("unused client kept: %d", n)
	}
	for i := range 20 {
		if res, out := oauthForm(ts, url.Values{"grant_type": {"refresh_token"}, "client_id": {"nope"}}, "", ""); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %v", i, res.StatusCode, out)
		}
	}
	if res, _ := oauthForm(ts, url.Values{"grant_type": {"refresh_token"}, "client_id": {"nope"}}, "", ""); res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", res.StatusCode)
	}
}
