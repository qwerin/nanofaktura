package api_test

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/maildiag"
	"github.com/qwerin/nanofaktura/internal/maildiag/smtptest"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

var verifyLinkRe = regexp.MustCompile(`http://localhost:8080/verify-email/([A-Za-z0-9_-]+)`)

func withAdmins(emails ...string) func(*config.Config) {
	return func(c *config.Config) { c.AdminEmails = emails }
}

// verify marks email verified directly in the DB.
func verify(ts *testServer, email string) {
	ts.t.Helper()
	if err := ts.db.Model(&model.User{}).Where("email = ?", email).Update("email_verified_at", ts.now).Error; err != nil {
		ts.t.Fatal(err)
	}
}

// requestVerification asks for a verification link as c and returns its token.
func requestVerification(c *client) string {
	c.ts.t.Helper()
	c.mustDo(http.StatusNoContent, "POST", "/api/auth/me/verify-email", nil)
	m, ok := c.ts.mail.Last()
	match := verifyLinkRe.FindStringSubmatch(m.Text)
	if !ok || match == nil {
		c.ts.t.Fatalf("no verification link in %q", m.Text)
	}
	return match[1]
}

func me(c *client) api.Me { return doJSON[api.Me](c, http.StatusOK, "GET", "/api/auth/me", nil) }

func TestInstanceAdminAccess(t *testing.T) {
	ts := newTestServer(t, withAdmins("admin@example.cz"))
	admin := ts.signup("Admin@Example.cz", "Správa")
	other := ts.signup("other@example.cz", "Jiná")
	verify(ts, "other@example.cz")

	res, body := ts.anon().do("GET", "/api/admin/status", nil)
	assertError(t, res, body, http.StatusUnauthorized, "authentication required")
	// verified but not listed
	res, body = other.do("GET", "/api/admin/status", nil)
	assertCode(t, res, body, http.StatusForbidden, api.CodeNotInstanceAdmin)
	if m := me(other); m.InstanceAdmin || m.InstanceAdminPending || !m.EmailVerified {
		t.Fatalf("other: %+v", m)
	}
	// listed but unverified: no rights, only the hint
	res, body = admin.do("GET", "/api/admin/status", nil)
	assertCode(t, res, body, http.StatusForbidden, api.CodeNotInstanceAdmin)
	res, body = admin.do("POST", "/api/admin/email-test", api.EmailTestRequest{})
	assertCode(t, res, body, http.StatusForbidden, api.CodeNotInstanceAdmin)
	if m := me(admin); m.InstanceAdmin || !m.InstanceAdminPending || m.EmailVerified {
		t.Fatalf("pending admin: %+v", m)
	}

	// verification link → admin
	token := requestVerification(admin)
	v := doJSON[api.EmailVerificationInfo](ts.anon(), http.StatusOK, "POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: token})
	if !v.Verified || v.Email != "admin@example.cz" {
		t.Fatalf("verify: %+v", v)
	}
	if m := me(admin); !m.InstanceAdmin || m.InstanceAdminPending || !m.EmailVerified {
		t.Fatalf("admin: %+v", m)
	}
	st := doJSON[api.InstanceStatus](admin, http.StatusOK, "GET", "/api/admin/status", nil)
	if st.Users != 2 || st.Accounts != 2 || st.DBDriver != "sqlite" || st.Version == "" || st.Config.SMTPConfigured ||
		st.Config.RateLimitEnabled || len(st.Config.AdminEmails) != 1 || len(st.Warnings) == 0 || st.Config.SecretKeySource != "file" {
		t.Fatalf("status: %+v", st)
	}
	// API tokens of the admin work too; others still do not
	other.mustDo(http.StatusForbidden, "GET", "/api/admin/users", nil)
}

// An account owner can invite any address and the invitee registers it —
// without the verification this would hand out admin rights (SPEC §3.2).
func TestInstanceAdminInvitationTakeover(t *testing.T) {
	ts := newTestServer(t, withAdmins("boss@example.cz"))
	owner := ts.signup("attacker@example.cz", "Útočník")
	_, token := invite(owner, "boss@example.cz", "member")
	taken := ts.anon()
	doJSON[api.Me](taken, http.StatusCreated, "POST", "/api/auth/register", api.RegisterRequest{
		Email: "boss@example.cz", Name: "Falešný", Password: testPassword, InvitationToken: token})
	if m := me(taken); m.InstanceAdmin || m.EmailVerified {
		t.Fatalf("invited registration became admin: %+v", m)
	}
	res, body := taken.do("GET", "/api/admin/status", nil)
	assertCode(t, res, body, http.StatusForbidden, api.CodeNotInstanceAdmin)
	// the verification link goes to the real mailbox, never to the attacker
	requestVerification(taken)
	if m, _ := ts.mail.Last(); len(m.To) != 1 || m.To[0] != "boss@example.cz" {
		t.Fatalf("verification sent to %v", m.To)
	}
	res, body = taken.do("GET", "/api/admin/users", nil)
	assertCode(t, res, body, http.StatusForbidden, api.CodeNotInstanceAdmin)
}

func TestEmailVerification(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	anon := ts.anon()

	res, body := anon.do("POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: "neexistuje"})
	assertError(t, res, body, http.StatusNotFound, "not found")

	// a newer link replaces the older one
	old := requestVerification(a)
	token := requestVerification(a)
	res, body = anon.do("POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: old})
	assertError(t, res, body, http.StatusNotFound, "not found")

	// expired after 24 h
	ts.now = ts.now.Add(25 * time.Hour)
	res, body = anon.do("POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: token})
	assertCode(t, res, body, http.StatusGone, api.CodeVerificationExpired)

	token = requestVerification(a)
	doJSON[api.EmailVerificationInfo](anon, http.StatusOK, "POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: token})
	// single use
	res, body = anon.do("POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: token})
	assertCode(t, res, body, http.StatusGone, api.CodeVerificationExpired)
	if !me(a).EmailVerified {
		t.Fatal("not verified")
	}
	// already verified → 409
	res, body = a.do("POST", "/api/auth/me/verify-email", nil)
	assertCode(t, res, body, http.StatusConflict, api.CodeEmailVerified)

	// the token is stored hashed
	var v model.EmailVerification
	ts.db.Last(&v)
	if v.TokenHash == token || v.UsedAt == nil || v.Email != "a@example.cz" {
		t.Fatalf("stored: %+v", v)
	}
}

func TestEmailVerificationSecurity(t *testing.T) {
	ts := newTestServer(t, withRateLimit)
	a := ts.signup("a@example.cz", "Firma A")

	// cookie-authenticated cross-site request → CSRF
	res, body := a.raw(jsonReq("POST", "/api/auth/me/verify-email", nil, "Sec-Fetch-Site", "cross-site"))
	assertCode(t, res, body, http.StatusForbidden, api.CodeCrossOrigin)

	// 3 links per hour per user
	for range 3 {
		requestVerification(a)
	}
	res, body = a.do("POST", "/api/auth/me/verify-email", nil)
	assertRateLimited(t, res, body)

	// link confirmations are limited per IP (guessing)
	for range 20 {
		ts.anon().do("POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: "x"})
	}
	res, body = ts.anon().do("POST", "/api/auth/verify-email", api.EmailVerificationConfirm{Token: "x"})
	assertRateLimited(t, res, body)
}

func TestPasswordResetVerifiesEmail(t *testing.T) {
	ts := newTestServer(t)
	ts.signup("a@example.cz", "Firma A")
	token := requestReset(ts, "a@example.cz")
	ts.anon().mustDo(http.StatusNoContent, "POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: "noveheslo123"})
	var u model.User
	ts.db.Where("email = ?", "a@example.cz").First(&u)
	if u.EmailVerifiedAt == nil {
		t.Fatal("password reset did not verify the e-mail")
	}
}

func TestAdminUsers(t *testing.T) {
	ts := newTestServer(t, withAdmins("admin@example.cz"))
	admin := ts.signup("admin@example.cz", "Správa")
	verify(ts, "admin@example.cz")
	ts.signup("b@example.cz", "Firma B")
	var b model.User
	ts.db.Where("email = ?", "b@example.cz").First(&b)
	ts.db.Model(&b).Update("totp_secret_enc", "enc")

	list := doJSON[api.ListResponse[api.AdminUser]](admin, http.StatusOK, "GET", "/api/admin/users", nil)
	if list.Total != 2 || !list.Items[0].InstanceAdmin || list.Items[1].Email != "b@example.cz" || !list.Items[1].TwoFactor ||
		list.Items[1].Accounts != 1 || list.Items[1].EmailVerifiedAt != nil {
		t.Fatalf("users: %+v", list)
	}
	if l := doJSON[api.ListResponse[api.AdminUser]](admin, http.StatusOK, "GET", "/api/admin/users?query=B@EX", nil); l.Total != 1 {
		t.Fatalf("query: %+v", l)
	}

	path := fmt.Sprintf("/api/admin/users/%d", b.ID)
	admin.mustDo(http.StatusNoContent, "POST", path+"/send-verification", nil)
	if m, _ := ts.mail.Last(); m.To[0] != "b@example.cz" || !verifyLinkRe.MatchString(m.Text) {
		t.Fatalf("mail: %+v", m)
	}
	admin.mustDo(http.StatusNoContent, "POST", path+"/reset-2fa", nil)
	admin.mustDo(http.StatusNoContent, "POST", path+"/verify-email", nil)
	ts.db.First(&b, b.ID)
	if b.TOTPSecretEnc != "" || b.EmailVerifiedAt == nil {
		t.Fatalf("user: %+v", b)
	}
	res, body := admin.do("POST", path+"/send-verification", nil)
	assertCode(t, res, body, http.StatusConflict, api.CodeEmailVerified)
	res, body = admin.do("POST", "/api/admin/users/9999/reset-2fa", nil)
	assertError(t, res, body, http.StatusNotFound, "user not found")
}

// emailTestServer is an admin test server whose SMTP points at srv (plain).
func emailTestServer(t *testing.T, srv *smtptest.Server, opts ...func(*config.Config)) (*testServer, *client) {
	t.Helper()
	opts = append([]func(*config.Config){withAdmins("admin@example.cz"), func(c *config.Config) {
		if srv != nil {
			c.SMTPHost, c.SMTPPort, c.SMTPTLS, c.MailFrom = "127.0.0.1", srv.Addr.Port, mail.TLSNone, "NanoFaktura <faktury@example.cz>"
		}
		c.PublicURL = "https://faktury.example.cz"
	}}, opts...)
	ts := newTestServer(t, opts...)
	resolver := &smtptest.Resolver{
		TXT: map[string][]string{
			"example.cz":        {"v=spf1 ip4:203.0.113.0/24 -all"},
			"_dmarc.example.cz": {"v=DMARC1; p=none"},
		},
		MX: map[string][]*net.MX{"example.cz": {{Host: "mx.example.cz.", Pref: 10}}},
	}
	ts.handler, _ = api.New(ts.db, ts.cfg, api.Deps{Now: func() time.Time { return ts.now }, Mailer: ts.mail,
		Storage: storage.NewLocal(ts.dataDir), Secrets: ts.secrets, CNB: &fakeRates{}, Resolver: resolver})
	admin := ts.signup("admin@example.cz", "Správa")
	verify(ts, "admin@example.cz")
	return ts, admin
}

func TestAdminEmailTest(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{})
	ts, admin := emailTestServer(t, srv)

	r := doJSON[maildiag.DiagReport](admin, http.StatusOK, "POST", "/api/admin/email-test", api.EmailTestRequest{DKIMSelector: "google"})
	if !r.Sent || r.To != "admin@example.cz" || r.From != "faktury@example.cz" || r.QueueID == "" || len(r.SMTP) == 0 || len(r.DNS) != 4 {
		t.Fatalf("report: %+v", r)
	}
	msgs := srv.Messages()
	if len(msgs) != 1 || msgs[0].To[0] != "admin@example.cz" || !strings.Contains(msgs[0].Data, "https://faktury.example.cz") ||
		!strings.Contains(msgs[0].Data, "Message-ID: <") || !strings.Contains(msgs[0].Data, "Date: ") {
		t.Fatalf("message: %+v", msgs)
	}
	// the admin's mailer is not used for the test (it goes straight over SMTP)
	if len(ts.mail.Messages()) != 0 {
		t.Fatal("test went through the mailer")
	}
	r = doJSON[maildiag.DiagReport](admin, http.StatusOK, "POST", "/api/admin/email-test", api.EmailTestRequest{To: "Jiny@Example.com"})
	if r.To != "jiny@example.com" {
		t.Fatalf("to: %+v", r.To)
	}
	res, body := admin.do("POST", "/api/admin/email-test", api.EmailTestRequest{DKIMSelector: "bad selector!"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "DKIM selector")
	res, body = admin.do("POST", "/api/admin/email-test", map[string]string{"to": "not-an-email"})
	assertError(t, res, body, http.StatusUnprocessableEntity, "validation failed")
}

func TestAdminEmailTestRelayDenied(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{DenyRelay: true})
	_, admin := emailTestServer(t, srv)
	r := doJSON[maildiag.DiagReport](admin, http.StatusOK, "POST", "/api/admin/email-test", api.EmailTestRequest{})
	if r.Sent || r.Status != maildiag.Error {
		t.Fatalf("report: %+v", r)
	}
	for _, c := range r.SMTP {
		if c.ID == "rcpt_to" && (c.Status != maildiag.Error || !strings.Contains(c.Message, "relay")) {
			t.Fatalf("rcpt: %+v", c)
		}
	}
}

func TestAdminEmailTestNotConfigured(t *testing.T) {
	_, admin := emailTestServer(t, nil, func(c *config.Config) { c.MailFrom = "faktury@example.cz" })
	r := doJSON[maildiag.DiagReport](admin, http.StatusOK, "POST", "/api/admin/email-test", api.EmailTestRequest{})
	if r.Sent || len(r.SMTP) != 1 || r.SMTP[0].ID != "config" || !strings.Contains(r.SMTP[0].Message, "SMTP není nastavené") ||
		!strings.Contains(r.SMTP[0].Hint, "NANOFAKTURA_SMTP_HOST") || len(r.DNS) != 4 {
		t.Fatalf("report: %+v", r)
	}
}

func TestAdminEmailTestRateLimit(t *testing.T) {
	_, admin := emailTestServer(t, nil, withRateLimit, func(c *config.Config) { c.MailFrom = "faktury@example.cz" })
	for range 10 {
		admin.mustDo(http.StatusOK, "POST", "/api/admin/email-test", api.EmailTestRequest{})
	}
	res, body := admin.do("POST", "/api/admin/email-test", api.EmailTestRequest{})
	assertRateLimited(t, res, body)
}
