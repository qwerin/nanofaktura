package api_test

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/model"
)

var resetLinkRe = regexp.MustCompile(`/reset-password/([A-Za-z0-9_-]+)`)

// requestReset asks for a password reset of email and returns the token from
// the e-mail ("" when no e-mail was sent).
func requestReset(ts *testServer, email string) string {
	ts.t.Helper()
	before := len(ts.mail.Messages())
	ts.anon().mustDo(http.StatusNoContent, "POST", "/api/auth/password-reset", api.PasswordResetRequest{Email: email})
	msgs := ts.mail.Messages()
	if len(msgs) == before {
		return ""
	}
	m := msgs[len(msgs)-1]
	match := resetLinkRe.FindStringSubmatch(m.Text)
	if match == nil {
		ts.t.Fatalf("no reset link in %q", m.Text)
	}
	return match[1]
}

func TestPasswordReset(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")

	// unknown e-mail: the same 204, nothing sent
	if tok := requestReset(ts, "nobody@example.cz"); tok != "" {
		t.Fatal("e-mail sent for an unknown address")
	}

	token := requestReset(ts, " A@Example.cz ")
	if token == "" {
		t.Fatal("no reset e-mail")
	}
	if m, _ := ts.mail.Last(); len(m.To) != 1 || m.To[0] != "a@example.cz" {
		t.Fatalf("mail to %v", m.To)
	}
	// throttled for 5 minutes
	if requestReset(ts, "a@example.cz") != "" {
		t.Fatal("second e-mail within the throttle window")
	}
	ts.now = ts.now.Add(6 * time.Minute)
	second := requestReset(ts, "a@example.cz")
	if second == "" || second == token {
		t.Fatal("no new e-mail after the throttle window")
	}

	info := doJSON[api.PasswordResetInfo](ts.anon(), http.StatusOK, "GET", "/api/auth/password-reset/"+token, nil)
	if info.Email != "a@example.cz" || info.TwoFactor {
		t.Fatalf("info: %+v", info)
	}
	res, body := ts.anon().do("GET", "/api/auth/password-reset/unknown-token", nil)
	assertCode(t, res, body, http.StatusNotFound, "not_found")

	res, body = ts.anon().do("POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: "short"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "validation_failed")

	ts.anon().mustDo(http.StatusNoContent, "POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: "noveheslo123"})

	// every session is logged out, the old password is gone, the new one works
	res, body = a.do("GET", "/api/auth/me", nil)
	assertCode(t, res, body, http.StatusUnauthorized, "unauthorized")
	res, body = ts.anon().do("POST", "/api/auth/login", api.LoginRequest{Email: "a@example.cz", Password: testPassword})
	assertCode(t, res, body, http.StatusUnauthorized, "unauthorized")
	login := doJSON[api.LoginResult](ts.anon(), http.StatusOK, "POST", "/api/auth/login", api.LoginRequest{Email: "a@example.cz", Password: "noveheslo123"})
	if login.Me == nil {
		t.Fatalf("login: %+v", login)
	}

	// the used link says so, the other link no longer exists
	res, body = ts.anon().do("GET", "/api/auth/password-reset/"+token, nil)
	assertCode(t, res, body, http.StatusGone, "reset_expired")
	res, body = ts.anon().do("POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: "jineheslo123"})
	assertCode(t, res, body, http.StatusGone, "reset_expired")
	res, body = ts.anon().do("GET", "/api/auth/password-reset/"+second, nil)
	assertCode(t, res, body, http.StatusNotFound, "not_found")
}

func TestPasswordResetExpires(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")
	token := requestReset(ts, "a@example.cz")
	ts.now = ts.now.Add(61 * time.Minute)
	res, body := ts.anon().do("GET", "/api/auth/password-reset/"+token, nil)
	assertCode(t, res, body, http.StatusGone, "reset_expired")
	res, body = ts.anon().do("POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: "noveheslo123"})
	assertCode(t, res, body, http.StatusGone, "reset_expired")

	// the cleanup job removes the link a day after it expired
	mustRunJob(ts, "auth-cleanup")
	var n int64
	ts.db.Model(&model.PasswordReset{}).Count(&n)
	if n != 1 {
		t.Fatalf("resets after cleanup: %d", n)
	}
	ts.now = ts.now.Add(25 * time.Hour)
	mustRunJob(ts, "auth-cleanup")
	ts.db.Model(&model.PasswordReset{}).Count(&n)
	if n != 0 {
		t.Fatalf("resets after cleanup: %d", n)
	}
}
