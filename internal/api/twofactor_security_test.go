package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/api"
)

func TestPasswordResetLimits(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	ts.signup("a@example.cz", "Firma A")
	anon := ts.anon()
	for i := range 5 {
		anon.mustDo(http.StatusNoContent, "POST", "/api/auth/password-reset", api.PasswordResetRequest{Email: fmt.Sprintf("u%d@example.cz", i)})
	}
	res, body := anon.do("POST", "/api/auth/password-reset", api.PasswordResetRequest{Email: "a@example.cz"})
	assertRateLimited(t, res, body) // per IP
	if len(ts.mail.Messages()) != 0 {
		t.Fatal("no e-mail may be sent when limited")
	}

	for range 20 {
		res, body = anon.do("GET", "/api/auth/password-reset/guess", nil)
		assertCode(t, res, body, http.StatusNotFound, "not_found")
	}
	res, body = anon.do("GET", "/api/auth/password-reset/guess", nil)
	assertRateLimited(t, res, body)
	res, body = anon.do("POST", "/api/auth/password-reset/guess", api.PasswordResetConfirm{Password: "noveheslo123"})
	assertRateLimited(t, res, body)
}

func TestPasswordResetByteLimit(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")
	token := requestReset(ts, "a@example.cz")
	long := strings.Repeat("ř", 40) // 80 bytes
	res, body := ts.anon().do("POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: long})
	assertCode(t, res, body, http.StatusUnprocessableEntity, api.CodePasswordTooLong)
}

// New challenges must not reset the per-user budget of wrong second factors.
func TestTwoFactorLoginLimit(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	secret, _ := enableTOTP(ts, a)
	ts.now = ts.now.Add(30e9)
	for range 2 {
		c, ch := startTwoFactor(ts, "a@example.cz")
		for range 5 {
			res, body := c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: "000000"})
			assertCode(t, res, body, http.StatusUnauthorized, "invalid_code")
		}
	}
	c, ch := startTwoFactor(ts, "a@example.cz")
	res, body := c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	assertRateLimited(t, res, body)
	if c.session != "" {
		t.Fatal("logged in despite the limit")
	}
}

func TestTwoFactorManagementLimits(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")
	for range 5 {
		res, body := a.do("POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: "wrong"})
		assertCode(t, res, body, http.StatusUnprocessableEntity, "wrong_password")
	}
	res, body := a.do("POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: pw})
	assertRateLimited(t, res, body) // shared with the password change

	b := ts.signup("b@example.cz", "Firma B")
	b.mustDo(http.StatusOK, "POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: pw})
	for range 10 {
		res, body = b.do("POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: "000000"})
		assertCode(t, res, body, http.StatusUnprocessableEntity, "invalid_code")
	}
	res, body = b.do("POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: "000000"})
	assertRateLimited(t, res, body)
}

func TestTwoFactorSecretsNotEchoed(t *testing.T) {
	ts := newTestServer(t)
	secretish := strings.Repeat("7", 40)
	res, body := ts.anon().do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: secretish + "x", Code: secretish})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "validation_failed")
	if strings.Contains(string(body), secretish) {
		t.Fatalf("submitted secret echoed: %s", body)
	}
}

func TestTwoFactorCSRF(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: pw}},
		{"POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: "123456"}},
		{"DELETE", "/api/auth/2fa/totp", api.PasswordConfirm{Password: pw}},
		{"POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: pw}},
		{"POST", "/api/auth/2fa/webauthn", api.WebAuthnRegisterRequest{Token: "t", Name: "k", Credential: []byte(`{}`)}},
		{"DELETE", "/api/auth/2fa/webauthn/1", api.PasswordConfirm{Password: pw}},
		{"POST", "/api/auth/2fa/recovery-codes", api.PasswordConfirm{Password: pw}},
	} {
		res, body := a.raw(jsonReq(r.method, r.path, r.body, "Sec-Fetch-Site", "cross-site"))
		assertCode(t, res, body, http.StatusForbidden, api.CodeCrossOrigin)
	}
	// nothing happened
	st := doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if st.Enabled {
		t.Fatalf("status: %+v", st)
	}
}
