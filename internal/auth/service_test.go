package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

type fixture struct {
	db   *gorm.DB
	svc  *Service
	now  time.Time
	user model.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gdb, err := db.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	f := &fixture{db: gdb, now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	f.svc = NewService(gdb, func() time.Time { return f.now }, nil)
	f.user = model.User{Email: "a@example.cz", Name: "A", PasswordHash: "x"}
	if err := gdb.Create(&f.user).Error; err != nil {
		t.Fatal(err)
	}
	return f
}

func count[T any](t *testing.T, d *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := d.Model(new(T)).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSessionLifecycle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	c, err := f.svc.CreateSession(ctx, f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != SessionCookie || !c.HttpOnly || c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" ||
		!c.Expires.Equal(f.now.Add(SessionTTL)) || len(c.Value) != 43 {
		t.Fatalf("cookie %+v", c)
	}

	// fresh session: resolved, no refresh
	u, refreshed, err := f.svc.userBySession(ctx, c.Value)
	if err != nil || u.ID != f.user.ID || refreshed != nil {
		t.Fatalf("fresh: %v %v %v", u, refreshed, err)
	}

	// less than a day old: still no refresh
	f.now = f.now.Add(23 * time.Hour)
	if _, refreshed, _ := f.svc.userBySession(ctx, c.Value); refreshed != nil {
		t.Fatal("refreshed before a day passed")
	}

	// older than a day: sliding expiry
	f.now = f.now.Add(2 * time.Hour)
	_, refreshed, err = f.svc.userBySession(ctx, c.Value)
	if err != nil || refreshed == nil || !refreshed.Expires.Equal(f.now.Add(SessionTTL)) || refreshed.Value != c.Value {
		t.Fatalf("refresh: %+v %v", refreshed, err)
	}

	// unknown token
	if _, _, err := f.svc.userBySession(ctx, "nope"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown token: %v", err)
	}

	// expired session
	f.now = f.now.Add(SessionTTL + time.Second)
	if _, _, err := f.svc.userBySession(ctx, c.Value); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expired: %v", err)
	}

	// DeleteSession
	c2, _ := f.svc.CreateSession(ctx, f.user.ID)
	if err := f.svc.DeleteSession(ctx, c2.Value); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.userBySession(ctx, c2.Value); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("deleted session still valid: %v", err)
	}
	if err := f.svc.DeleteSession(ctx, "unknown"); err != nil {
		t.Fatalf("delete unknown: %v", err)
	}

	// session of a deleted user
	other := model.User{Email: "gone@example.cz", Name: "G", PasswordHash: "x"}
	f.db.Create(&other)
	c3, _ := f.svc.CreateSession(ctx, other.ID)
	f.db.Delete(&other)
	if _, _, err := f.svc.userBySession(ctx, c3.Value); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("session of deleted user: %v", err)
	}
}

func TestSecureCookieAndClear(t *testing.T) {
	f := newFixture(t)
	svc := NewService(f.db, func() time.Time { return f.now }, func(context.Context) bool { return true })
	c, err := svc.CreateSession(context.Background(), f.user.ID)
	if err != nil || !c.Secure {
		t.Fatalf("secure cookie: %+v %v", c, err)
	}
	cl := svc.ClearCookie(context.Background())
	if cl.MaxAge != -1 || cl.Value != "" || cl.Name != SessionCookie || !cl.Secure {
		t.Fatalf("clear cookie %+v", cl)
	}
}

func TestDeleteOtherSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	keep, _ := f.svc.CreateSession(ctx, f.user.ID)
	f.svc.CreateSession(ctx, f.user.ID)
	f.svc.CreateSession(ctx, f.user.ID)
	other := model.User{Email: "b@example.cz", Name: "B", PasswordHash: "x"}
	f.db.Create(&other)
	f.svc.CreateSession(ctx, other.ID)

	if err := DeleteOtherSessions(f.db, f.user.ID, keep.Value); err != nil {
		t.Fatal(err)
	}
	if n := count[model.Session](t, f.db); n != 2 {
		t.Fatalf("after keep: %d sessions, want 2 (kept + other user's)", n)
	}
	if _, _, err := f.svc.userBySession(ctx, keep.Value); err != nil {
		t.Fatalf("kept session invalid: %v", err)
	}
	if err := DeleteOtherSessions(f.db, f.user.ID, ""); err != nil {
		t.Fatal(err)
	}
	if n := count[model.Session](t, f.db); n != 1 {
		t.Fatalf("after all: %d sessions, want 1", n)
	}
}

func TestAPITokens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	plain, tok, err := f.svc.CreateAPIToken(ctx, f.user.ID, "CI", nil)
	if err != nil || !strings.HasPrefix(plain, APITokenPrefix) || tok.Prefix != plain[:8] || tok.TokenHash == plain {
		t.Fatalf("create: %q %+v %v", plain, tok, err)
	}

	u, err := f.svc.userByAPIToken(ctx, plain)
	if err != nil || u.ID != f.user.ID {
		t.Fatalf("resolve: %v %v", u, err)
	}
	var got model.APIToken
	f.db.First(&got, tok.ID)
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(f.now) {
		t.Fatalf("last_used_at %v", got.LastUsedAt)
	}
	// within a minute: last use not rewritten
	first := *got.LastUsedAt
	f.now = f.now.Add(30 * time.Second)
	f.svc.userByAPIToken(ctx, plain)
	f.db.First(&got, tok.ID)
	if !got.LastUsedAt.Equal(first) {
		t.Fatalf("last_used_at rewritten within a minute: %v", got.LastUsedAt)
	}
	f.now = f.now.Add(time.Minute)
	f.svc.userByAPIToken(ctx, plain)
	f.db.First(&got, tok.ID)
	if !got.LastUsedAt.Equal(f.now) {
		t.Fatalf("last_used_at not updated after a minute: %v", got.LastUsedAt)
	}

	for name, tc := range map[string]string{
		"no prefix": strings.TrimPrefix(plain, APITokenPrefix),
		"unknown":   APITokenPrefix + "unknown",
	} {
		if _, err := f.svc.userByAPIToken(ctx, tc); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// expiring token
	exp := f.now.Add(time.Hour)
	plain2, _, _ := f.svc.CreateAPIToken(ctx, f.user.ID, "short", &exp)
	if _, err := f.svc.userByAPIToken(ctx, plain2); err != nil {
		t.Fatalf("valid expiring token: %v", err)
	}
	f.now = exp
	if _, err := f.svc.userByAPIToken(ctx, plain2); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expired token: %v", err)
	}

	// DeleteExpired removes the expired token and expired sessions, keeps the rest
	f.svc.CreateSession(ctx, f.user.ID) // expires in 30 days
	f.db.Create(&model.Session{UserID: f.user.ID, TokenHash: "old", ExpiresAt: f.now.Add(-time.Second)})
	if err := DeleteExpired(f.db, f.now); err != nil {
		t.Fatal(err)
	}
	if n := count[model.APIToken](t, f.db); n != 1 {
		t.Fatalf("tokens after cleanup: %d, want 1 (the non-expiring one)", n)
	}
	if n := count[model.Session](t, f.db); n != 1 {
		t.Fatalf("sessions after cleanup: %d, want 1", n)
	}
}

func TestCleanup(t *testing.T) {
	f := newFixture(t)
	now := f.now
	uid := f.user.ID
	used := now.Add(-2 * time.Hour)
	rows := []any{
		&model.Session{UserID: uid, TokenHash: "s-old", ExpiresAt: now.Add(-time.Minute)},
		&model.Session{UserID: uid, TokenHash: "s-new", ExpiresAt: now.Add(time.Minute)},
		&model.AuthChallenge{UserID: uid, Purpose: model.ChallengeLogin, TokenHash: "c-old", ExpiresAt: now},
		&model.AuthChallenge{UserID: uid, Purpose: model.ChallengeLogin, TokenHash: "c-new", ExpiresAt: now.Add(time.Minute)},
		// reset links are kept one day after expiry
		&model.PasswordReset{UserID: uid, TokenHash: "r-old", ExpiresAt: now.Add(-25 * time.Hour), UsedAt: &used},
		&model.PasswordReset{UserID: uid, TokenHash: "r-recent", ExpiresAt: now.Add(-23 * time.Hour)},
		&model.EmailVerification{UserID: uid, TokenHash: "v-old", Email: "a@example.cz", ExpiresAt: now.Add(-25 * time.Hour)},
		&model.EmailVerification{UserID: uid, TokenHash: "v-recent", Email: "a@example.cz", ExpiresAt: now.Add(-time.Hour)},
	}
	for _, r := range rows {
		if err := f.db.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Cleanup(f.db, now); err != nil {
		t.Fatal(err)
	}
	for name, n := range map[string]int64{
		"sessions":      count[model.Session](t, f.db),
		"challenges":    count[model.AuthChallenge](t, f.db),
		"resets":        count[model.PasswordReset](t, f.db),
		"verifications": count[model.EmailVerification](t, f.db),
	} {
		if n != 1 {
			t.Errorf("%s: %d left, want 1", name, n)
		}
	}
}

func TestResetSecondFactor(t *testing.T) {
	f := newFixture(t)
	uid := f.user.ID
	f.db.Model(&f.user).Updates(map[string]any{"totp_secret_enc": "enc", "totp_pending_enc": "p", "totp_last_step": 42})
	f.db.Create(&model.RecoveryCode{UserID: uid, CodeHash: "h"})
	f.db.Create(&model.WebAuthnCredential{UserID: uid, Name: "key", CredentialID: "c1", Data: "{}"})
	f.db.Create(&model.AuthChallenge{UserID: uid, Purpose: model.ChallengeLogin, TokenHash: "t", ExpiresAt: f.now.Add(time.Hour)})
	other := model.User{Email: "b@example.cz", Name: "B", PasswordHash: "x"}
	f.db.Create(&other)
	f.db.Create(&model.RecoveryCode{UserID: other.ID, CodeHash: "h2"})

	if err := ResetSecondFactor(f.db, uid); err != nil {
		t.Fatal(err)
	}
	var u model.User
	f.db.First(&u, uid)
	if u.TOTPSecretEnc != "" || u.TOTPPendingEnc != "" || u.TOTPLastStep != 0 {
		t.Fatalf("totp not reset: %+v", u)
	}
	if count[model.WebAuthnCredential](t, f.db) != 0 || count[model.AuthChallenge](t, f.db) != 0 {
		t.Fatal("credentials/challenges not removed")
	}
	if n := count[model.RecoveryCode](t, f.db); n != 1 {
		t.Fatalf("recovery codes left: %d, want 1 (other user's)", n)
	}
}

func TestSecrets(t *testing.T) {
	plain, hash := NewSecret()
	if HashSecret(plain) != hash || strings.HasPrefix(plain, APITokenPrefix) || len(hash) != 64 {
		t.Fatalf("secret %q %q", plain, hash)
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	if UserFrom(ctx) != nil || AccountFrom(ctx) != nil || RoleFrom(ctx) != "" {
		t.Fatal("empty context not empty")
	}
	acc := &model.Account{ID: 7}
	ctx = WithAccount(ctx, acc, model.RoleAdmin)
	if AccountFrom(ctx) != acc || RoleFrom(ctx) != model.RoleAdmin || UserFrom(ctx) != nil {
		t.Fatal("WithAccount")
	}
	if err := RequireRole(ctx, model.RoleOwner, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	err := RequireRole(ctx, model.RoleOwner)
	var se huma.StatusError
	if !errors.As(err, &se) || se.GetStatus() != http.StatusForbidden ||
		!strings.Contains(err.Error(), "your role (admin) cannot do this; allowed roles: owner") {
		t.Fatalf("RequireRole: %v", err)
	}
}

func TestAllow(t *testing.T) {
	op := &huma.Operation{Description: "List."}
	Allow(model.RoleOwner, model.RoleAccountant)(op)
	if got := AllowedRoles(op); strings.Join(got, ",") != "owner,accountant" {
		t.Fatalf("roles %v", got)
	}
	if !strings.HasSuffix(op.Description, "\n\nAllowed roles: owner, accountant.") || op.Extensions[ExtRoles] == nil {
		t.Fatalf("docs: %q %v", op.Description, op.Extensions)
	}
	if !roleAllowed(op, model.RoleOwner) || roleAllowed(op, model.RoleMember) {
		t.Fatal("roleAllowed")
	}
	if AllowedRoles(nil) != nil || AllowedRoles(&huma.Operation{}) != nil || !roleAllowed(&huma.Operation{}, "anything") {
		t.Fatal("undeclared operation must allow everyone")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("unknown role did not panic")
		}
	}()
	Allow("superuser")
}

// TestMiddlewares drives RequireUser + RequireAccount through a real huma API.
func TestMiddlewares(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := model.Account{Slug: "firma", Name: "Firma"}
	f.db.Create(&acc)
	f.db.Create(&model.Membership{UserID: f.user.ID, AccountID: acc.ID, Role: model.RoleAccountant})
	foreign := model.Account{Slug: "cizi", Name: "Cizí"}
	f.db.Create(&foreign)

	_, api := humatest.New(t)
	api.UseMiddleware(f.svc.RequireUser(api))
	type out struct {
		Body struct {
			User string `json:"user"`
			Acc  string `json:"acc"`
			Role string `json:"role"`
		}
	}
	h := func(ctx context.Context, _ *struct {
		Slug string `path:"slug"`
	}) (*out, error) {
		o := &out{}
		o.Body.User = UserFrom(ctx).Email
		if a := AccountFrom(ctx); a != nil {
			o.Body.Acc = a.Slug
		}
		o.Body.Role = RoleFrom(ctx)
		return o, nil
	}
	grp := huma.NewGroup(api, "/a/{slug}")
	grp.UseMiddleware(f.svc.RequireAccount(api))
	huma.Get(grp, "/read", h)
	huma.Post(grp, "/write", h, ForEditors)

	cookie, _ := f.svc.CreateSession(ctx, f.user.ID)
	token, _, _ := f.svc.CreateAPIToken(ctx, f.user.ID, "t", nil)
	sess := "Cookie: " + SessionCookie + "=" + cookie.Value
	bearer := "Authorization: Bearer " + token

	cases := []struct {
		name   string
		method string
		path   string
		hdr    []any
		status int
		body   string
	}{
		{"anonymous", "GET", "/a/firma/read", nil, 401, "authentication required"},
		{"bad scheme", "GET", "/a/firma/read", []any{"Authorization: Basic abc"}, 401, "authentication required"},
		{"bad bearer", "GET", "/a/firma/read", []any{"Authorization: Bearer nf_nope"}, 401, "authentication required"},
		{"empty cookie", "GET", "/a/firma/read", []any{"Cookie: " + SessionCookie + "="}, 401, "authentication required"},
		{"cookie", "GET", "/a/firma/read", []any{sess}, 200, `"role":"accountant"`},
		{"bearer", "GET", "/a/firma/read", []any{bearer}, 200, `"acc":"firma"`},
		{"bearer wins over bad cookie", "GET", "/a/firma/read", []any{bearer, "Cookie: " + SessionCookie + "=bad"}, 200, `"user":"a@example.cz"`},
		{"non-member", "GET", "/a/cizi/read", []any{sess}, 404, "account not found"},
		{"unknown account", "GET", "/a/nic/read", []any{sess}, 404, "account not found"},
		{"role denied", "POST", "/a/firma/write", []any{sess}, 403, "your role (accountant) cannot do this; allowed roles: owner, admin, member"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := api.Do(tc.method, tc.path, tc.hdr...)
			r, buf := rec.Result(), rec.Body
			if r.StatusCode != tc.status || !strings.Contains(buf.String(), tc.body) {
				t.Fatalf("got %d %s; want %d containing %q", r.StatusCode, buf, tc.status, tc.body)
			}
		})
	}

	// an old session is refreshed and the new cookie is sent
	f.now = f.now.Add(25 * time.Hour)
	r := api.Get("/a/firma/read", sess).Result()
	if r.StatusCode != 200 || !strings.Contains(r.Header.Get("Set-Cookie"), SessionCookie+"="+cookie.Value) {
		t.Fatalf("refresh cookie: %d %q", r.StatusCode, r.Header.Get("Set-Cookie"))
	}

	// a DB failure is a 500, not a 401
	sqlDB, _ := f.db.DB()
	sqlDB.Close()
	if r := api.Get("/a/firma/read", sess).Result(); r.StatusCode != http.StatusInternalServerError {
		t.Fatalf("closed DB: %d", r.StatusCode)
	}
}
