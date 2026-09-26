package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/mail/mailtest"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

// testPassword is the password of every user created by signup.
const testPassword = "heslo1234"

// testServer is an API instance on a fresh in-memory SQLite with a fixed clock.
type testServer struct {
	t       *testing.T
	handler http.Handler
	db      *gorm.DB
	now     time.Time // current time seen by the API; change it to move the clock
	mail    *mailtest.Recorder
	dataDir string // attachment storage (t.TempDir)
}

// newTestServer starts an API with signup allowed. Use opts to tweak the config.
func newTestServer(t *testing.T, opts ...func(*config.Config)) *testServer {
	t.Helper()
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	cfg := config.Config{AllowSignup: true}
	for _, o := range opts {
		o(&cfg)
	}
	ts := &testServer{t: t, db: gdb, now: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC), mail: mailtest.New(), dataDir: t.TempDir()}
	ts.handler, _ = api.New(gdb, cfg, api.Deps{
		Now: func() time.Time { return ts.now }, Mailer: ts.mail, Storage: storage.NewLocal(ts.dataDir),
	})
	return ts
}

// client sends requests as one user. It keeps the session cookie like a
// browser, or sends a bearer token when token is set.
type client struct {
	ts      *testServer
	session string // nf_session cookie value
	token   string // API token (Authorization: Bearer)
	slug    string // slug of the account created at signup
}

// anon returns an unauthenticated client.
func (ts *testServer) anon() *client { return &client{ts: ts} }

// signup registers a user with its own account and returns a logged-in client.
func (ts *testServer) signup(email, accountName string) *client {
	ts.t.Helper()
	c := ts.anon()
	me := doJSON[api.Me](c, http.StatusCreated, "POST", "/api/auth/register", api.RegisterRequest{
		Email: email, Name: "Test " + email, Password: testPassword, AccountName: accountName,
	})
	c.slug = me.Accounts[0].Slug
	return c
}

// memberOf signs up a new user (with its own account) and adds it directly
// to owner's account with role; the returned client acts in owner's account.
func (ts *testServer) memberOf(owner *client, email, role string) *client {
	ts.t.Helper()
	c := ts.signup(email, "Vlastní "+email)
	var acc model.Account
	var user model.User
	if err := ts.db.Where("slug = ?", owner.slug).First(&acc).Error; err != nil {
		ts.t.Fatal(err)
	}
	if err := ts.db.Where("email = ?", email).First(&user).Error; err != nil {
		ts.t.Fatal(err)
	}
	if err := ts.db.Create(&model.Membership{UserID: user.ID, AccountID: acc.ID, Role: role}).Error; err != nil {
		ts.t.Fatal(err)
	}
	c.slug = owner.slug
	return c
}

// acct prefixes path with the client's account: acct("/subjects") → /api/accounts/{slug}/subjects.
func (c *client) acct(path string) string {
	return "/api/accounts/" + c.slug + path
}

// do sends a request with body marshalled as JSON (nil = no body) and returns
// the response and its body. Session cookies set by the server are remembered.
func (c *client) do(method, path string, body any) (*http.Response, []byte) {
	c.ts.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.ts.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.session != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: c.session})
	}
	rec := httptest.NewRecorder()
	c.ts.handler.ServeHTTP(rec, req)
	res := rec.Result()
	for _, ck := range res.Cookies() {
		if ck.Name == auth.SessionCookie {
			c.session = ck.Value
		}
	}
	return res, rec.Body.Bytes()
}

// mustDo is do + a status assertion; returns the body.
func (c *client) mustDo(want int, method, path string, body any) []byte {
	c.ts.t.Helper()
	res, b := c.do(method, path, body)
	if res.StatusCode != want {
		c.ts.t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, res.StatusCode, want, b)
	}
	return b
}

// doJSON is mustDo + decoding the response into T.
func doJSON[T any](c *client, want int, method, path string, body any) T {
	c.ts.t.Helper()
	return decodeJSON[T](c.ts.t, c.mustDo(want, method, path, body))
}

func decodeJSON[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %T: %v; body: %s", v, err, b)
	}
	return v
}

// assertError checks a problem+json error status and that detail contains substr.
func assertError(t *testing.T, res *http.Response, body []byte, status int, substr string) {
	t.Helper()
	if res.StatusCode != status {
		t.Fatalf("status %d, want %d; body: %s", res.StatusCode, status, body)
	}
	if !strings.Contains(string(body), substr) {
		t.Fatalf("body %s does not contain %q", body, substr)
	}
}
