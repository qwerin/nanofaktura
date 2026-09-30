package api_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/qwerin/nanofaktura/internal/api"
	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

const pw = testPassword

func loginResult(ts *testServer, email, password string) (*client, api.LoginResult) {
	ts.t.Helper()
	c := ts.anon()
	res := doJSON[api.LoginResult](c, http.StatusOK, "POST", "/api/auth/login", api.LoginRequest{Email: email, Password: password})
	return c, res
}

// startTwoFactor logs in with the password and returns the pending challenge.
func startTwoFactor(ts *testServer, email string) (*client, *api.TwoFactorChallenge) {
	ts.t.Helper()
	c, res := loginResult(ts, email, pw)
	if res.TwoFactor == nil || res.Me != nil || c.session != "" {
		ts.t.Fatalf("expected a second factor challenge: %+v (session %q)", res, c.session)
	}
	return c, res.TwoFactor
}

func totpNow(ts *testServer, secret string) string {
	ts.t.Helper()
	code, err := auth.TOTPCode(secret, auth.TOTPStep(ts.now))
	if err != nil {
		ts.t.Fatal(err)
	}
	return code
}

// enableTOTP turns on TOTP for c and returns the secret and recovery codes.
func enableTOTP(ts *testServer, c *client) (string, []string) {
	ts.t.Helper()
	setup := doJSON[api.TOTPSetup](c, http.StatusOK, "POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: pw})
	codes := doJSON[api.RecoveryCodes](c, http.StatusOK, "POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: totpNow(ts, setup.Secret)})
	return setup.Secret, codes.RecoveryCodes
}

func TestTOTPSetup(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	other, _ := loginResult(ts, "a@example.cz", pw) // a second browser

	st := doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if st.Enabled || st.TOTP || len(st.WebAuthn) != 0 || st.RecoveryCodesLeft != 0 {
		t.Fatalf("status: %+v", st)
	}

	res, body := a.do("POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: "123456"})
	assertCode(t, res, body, http.StatusConflict, "totp_not_pending")
	res, body = a.do("POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: "wrong"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "wrong_password")

	setup := doJSON[api.TOTPSetup](a, http.StatusOK, "POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: pw})
	if len(setup.Secret) != 32 || setup.OtpauthURL == "" {
		t.Fatalf("setup: %+v", setup)
	}
	var u model.User
	ts.db.Where("email = ?", "a@example.cz").First(&u)
	if u.TOTPPendingEnc == "" || u.TOTPPendingEnc == setup.Secret || u.TOTPSecretEnc != "" {
		t.Fatalf("stored secret must be encrypted and pending: %+v", u)
	}
	// not yet on: login still needs only the password
	if _, r := loginResult(ts, "a@example.cz", pw); r.Me == nil {
		t.Fatal("pending setup must not require a second factor")
	}

	res, body = a.do("POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: "000000"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "invalid_code")
	codes := doJSON[api.RecoveryCodes](a, http.StatusOK, "POST", "/api/auth/2fa/totp/enable", api.TOTPEnableRequest{Code: totpNow(ts, setup.Secret)})
	if len(codes.RecoveryCodes) != 10 {
		t.Fatalf("recovery codes: %v", codes)
	}

	// enabling logs out the other browser, the current one stays
	a.mustDo(http.StatusOK, "GET", "/api/auth/me", nil)
	res, body = other.do("GET", "/api/auth/me", nil)
	assertCode(t, res, body, http.StatusUnauthorized, "unauthorized")

	st = doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if !st.Enabled || !st.TOTP || st.RecoveryCodesLeft != 10 {
		t.Fatalf("status: %+v", st)
	}
	res, body = a.do("POST", "/api/auth/2fa/totp/setup", api.PasswordConfirm{Password: pw})
	assertCode(t, res, body, http.StatusConflict, "totp_enabled")

	// disabling needs the password and removes the recovery codes
	res, body = a.do("DELETE", "/api/auth/2fa/totp", api.PasswordConfirm{Password: "wrong"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "wrong_password")
	a.mustDo(http.StatusNoContent, "DELETE", "/api/auth/2fa/totp", api.PasswordConfirm{Password: pw})
	st = doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if st.Enabled || st.TOTP || st.RecoveryCodesLeft != 0 {
		t.Fatalf("status after disable: %+v", st)
	}
	if _, r := loginResult(ts, "a@example.cz", pw); r.Me == nil {
		t.Fatal("login after disable must not need a second factor")
	}
	res, body = a.do("POST", "/api/auth/2fa/recovery-codes", api.PasswordConfirm{Password: pw})
	assertCode(t, res, body, http.StatusConflict, "two_factor_disabled")

	// unauthenticated management → 401
	res, body = ts.anon().do("GET", "/api/auth/2fa", nil)
	assertCode(t, res, body, http.StatusUnauthorized, "unauthorized")
}

func TestTOTPLogin(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	secret, recovery := enableTOTP(ts, a)

	c, ch := startTwoFactor(ts, "a@example.cz")
	if !slices.Equal(ch.Methods, []string{"totp", "recovery"}) || !ch.ExpiresAt.Equal(ts.now.Add(10*time.Minute)) {
		t.Fatalf("challenge: %+v", ch)
	}
	res, body := c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: "000000"})
	assertCode(t, res, body, http.StatusUnauthorized, "invalid_code")
	// the code used to enable TOTP cannot be replayed
	res, body = c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	assertCode(t, res, body, http.StatusUnauthorized, "invalid_code")

	ts.now = ts.now.Add(30 * time.Second)
	me := doJSON[api.Me](c, http.StatusOK, "POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	if me.User.Email != "a@example.cz" || c.session == "" {
		t.Fatalf("me: %+v", me)
	}
	c.mustDo(http.StatusOK, "GET", "/api/auth/me", nil)
	// the challenge is single-use
	res, body = ts.anon().do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	assertCode(t, res, body, http.StatusUnauthorized, "two_factor_expired")

	// recovery code: accepted once, in any case/spacing
	c, ch = startTwoFactor(ts, "a@example.cz")
	doJSON[api.Me](c, http.StatusOK, "POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: " " + recovery[0] + " "})
	c, ch = startTwoFactor(ts, "a@example.cz")
	res, body = c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: recovery[0]})
	assertCode(t, res, body, http.StatusUnauthorized, "invalid_code")
	st := doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if st.RecoveryCodesLeft != 9 {
		t.Fatalf("codes left: %d", st.RecoveryCodesLeft)
	}

	// regenerated codes replace the old ones
	fresh := doJSON[api.RecoveryCodes](a, http.StatusOK, "POST", "/api/auth/2fa/recovery-codes", api.PasswordConfirm{Password: pw})
	if len(fresh.RecoveryCodes) != 10 {
		t.Fatalf("fresh codes: %v", fresh)
	}
	c, ch = startTwoFactor(ts, "a@example.cz")
	res, body = c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: recovery[1]})
	assertCode(t, res, body, http.StatusUnauthorized, "invalid_code")
	doJSON[api.Me](c, http.StatusOK, "POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: fresh.RecoveryCodes[0]})

	// wrong password never reaches the second step
	res, body = ts.anon().do("POST", "/api/auth/login", api.LoginRequest{Email: "a@example.cz", Password: "wrong"})
	assertCode(t, res, body, http.StatusUnauthorized, "unauthorized")
}

func TestTwoFactorChallengeLimits(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	secret, _ := enableTOTP(ts, a)
	ts.now = ts.now.Add(time.Minute)

	// 5 wrong attempts kill the challenge
	c, ch := startTwoFactor(ts, "a@example.cz")
	for range 5 {
		res, body := c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: "000000"})
		assertCode(t, res, body, http.StatusUnauthorized, "invalid_code")
	}
	res, body := c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	assertCode(t, res, body, http.StatusUnauthorized, "two_factor_expired")

	// and it expires after 10 minutes
	c, ch = startTwoFactor(ts, "a@example.cz")
	ts.now = ts.now.Add(11 * time.Minute)
	res, body = c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	assertCode(t, res, body, http.StatusUnauthorized, "two_factor_expired")
	res, body = c.do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: "garbage", Code: totpNow(ts, secret)})
	assertCode(t, res, body, http.StatusUnauthorized, "two_factor_expired")

	mustRunJob(ts, "auth-cleanup")
	var n int64
	ts.db.Model(&model.AuthChallenge{}).Count(&n)
	if n != 0 {
		t.Fatalf("challenges after cleanup: %d", n)
	}

	// a password reset keeps 2FA on and cancels pending logins
	_, ch = startTwoFactor(ts, "a@example.cz")
	token := requestReset(ts, "a@example.cz")
	info := doJSON[api.PasswordResetInfo](ts.anon(), http.StatusOK, "GET", "/api/auth/password-reset/"+token, nil)
	if !info.TwoFactor {
		t.Fatal("reset info must mention 2FA")
	}
	ts.anon().mustDo(http.StatusNoContent, "POST", "/api/auth/password-reset/"+token, api.PasswordResetConfirm{Password: "noveheslo123"})
	res, body = ts.anon().do("POST", "/api/auth/login/2fa", api.LoginCodeRequest{Token: ch.Token, Code: totpNow(ts, secret)})
	assertCode(t, res, body, http.StatusUnauthorized, "two_factor_expired")
	_, r := loginResult(ts, "a@example.cz", "noveheslo123")
	if r.TwoFactor == nil {
		t.Fatal("2FA must survive a password reset")
	}
}

// ---- WebAuthn with a software authenticator ----

const testOrigin = "http://localhost:8080" // origin of the default public URL
const testRPID = "localhost"

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// softKey is a minimal "none"-attestation ES256 authenticator (like a YubiKey).
type softKey struct {
	t     *testing.T
	priv  *ecdsa.PrivateKey
	id    []byte
	count uint32
}

func newSoftKey(t *testing.T) *softKey {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softKey{t: t, priv: priv, id: id}
}

func (k *softKey) challenge(options json.RawMessage) string {
	var o struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(options, &o); err != nil || o.Challenge == "" {
		k.t.Fatalf("options %s: %v", options, err)
	}
	return o.Challenge
}

func clientData(typ, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin, "crossOrigin": false})
	return b
}

func (k *softKey) authData(flags byte, attested []byte) []byte {
	rp := sha256.Sum256([]byte(testRPID))
	out := append(rp[:], flags)
	out = binary.BigEndian.AppendUint32(out, k.count)
	return append(out, attested...)
}

// create answers navigator.credentials.create() options.
func (k *softKey) create(options json.RawMessage, origin string) json.RawMessage {
	pub := k.priv.PublicKey
	x, y := make([]byte, 32), make([]byte, 32)
	pub.X.FillBytes(x)
	pub.Y.FillBytes(y)
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		k.t.Fatal(err)
	}
	attested := make([]byte, 16) // AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(k.id)))
	attested = append(append(attested, k.id...), cose...)
	att, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": k.authData(0x41, attested)})
	if err != nil {
		k.t.Fatal(err)
	}
	cd := clientData("webauthn.create", k.challenge(options), origin)
	b, _ := json.Marshal(map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key",
		"response": map[string]any{"clientDataJSON": b64(cd), "attestationObject": b64(att)}})
	return b
}

// get answers navigator.credentials.get() options.
func (k *softKey) get(options json.RawMessage, origin string) json.RawMessage {
	k.count++
	ad := k.authData(0x01, nil)
	cd := clientData("webauthn.get", k.challenge(options), origin)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(slices.Clone(ad), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	if err != nil {
		k.t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key",
		"response": map[string]any{"clientDataJSON": b64(cd), "authenticatorData": b64(ad), "signature": b64(sig)}})
	return b
}

func registerKey(ts *testServer, c *client, k *softKey, name string) api.WebAuthnKeyCreated {
	ts.t.Helper()
	opts := doJSON[api.WebAuthnRegistrationOptions](c, http.StatusOK, "POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: pw})
	return doJSON[api.WebAuthnKeyCreated](c, http.StatusCreated, "POST", "/api/auth/2fa/webauthn",
		api.WebAuthnRegisterRequest{Token: opts.Token, Name: name, Credential: k.create(opts.Options, testOrigin)})
}

func loginWithKey(ts *testServer, k *softKey, origin string) (*client, *http.Response, []byte) {
	ts.t.Helper()
	c, ch := startTwoFactor(ts, "a@example.cz")
	opts := doJSON[api.WebAuthnOptions](c, http.StatusOK, "POST", "/api/auth/login/webauthn/options", api.ChallengeRequest{Token: ch.Token})
	res, body := c.do("POST", "/api/auth/login/webauthn", api.WebAuthnLoginRequest{Token: ch.Token, Credential: k.get(opts.Options, origin)})
	return c, res, body
}

func TestWebAuthn(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	b := ts.signup("b@example.cz", "Firma B")
	yubi := newSoftKey(t)

	res, body := a.do("POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: "wrong"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "wrong_password")

	// a wrong origin fails verification and consumes the ceremony
	opts := doJSON[api.WebAuthnRegistrationOptions](a, http.StatusOK, "POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: pw})
	res, body = a.do("POST", "/api/auth/2fa/webauthn", api.WebAuthnRegisterRequest{Token: opts.Token, Name: "YubiKey",
		Credential: yubi.create(opts.Options, "https://evil.example")})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "webauthn_failed")
	res, body = a.do("POST", "/api/auth/2fa/webauthn", api.WebAuthnRegisterRequest{Token: opts.Token, Name: "YubiKey",
		Credential: yubi.create(opts.Options, testOrigin)})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "webauthn_failed")
	// another user's ceremony token is useless
	opts = doJSON[api.WebAuthnRegistrationOptions](a, http.StatusOK, "POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: pw})
	res, body = b.do("POST", "/api/auth/2fa/webauthn", api.WebAuthnRegisterRequest{Token: opts.Token, Name: "cizí",
		Credential: yubi.create(opts.Options, testOrigin)})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "webauthn_failed")

	created := registerKey(ts, a, yubi, "YubiKey 5C")
	if created.Key.Name != "YubiKey 5C" || len(created.RecoveryCodes) != 10 {
		t.Fatalf("created: %+v", created)
	}
	// a second key adds no new recovery codes; registering the same key again is refused
	phone := newSoftKey(t)
	if second := registerKey(ts, a, phone, "Telefon"); len(second.RecoveryCodes) != 0 {
		t.Fatalf("second key: %+v", second)
	}
	opts = doJSON[api.WebAuthnRegistrationOptions](a, http.StatusOK, "POST", "/api/auth/2fa/webauthn/options", api.PasswordConfirm{Password: pw})
	var excl struct {
		ExcludeCredentials []struct{ ID string } `json:"excludeCredentials"`
	}
	_ = json.Unmarshal(opts.Options, &excl)
	if len(excl.ExcludeCredentials) != 2 {
		t.Fatalf("registered keys must be excluded: %s", opts.Options)
	}
	res, body = a.do("POST", "/api/auth/2fa/webauthn", api.WebAuthnRegisterRequest{Token: opts.Token, Name: "znovu",
		Credential: yubi.create(opts.Options, testOrigin)})
	assertCode(t, res, body, http.StatusConflict, "already_exists")

	st := doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if !st.Enabled || st.TOTP || len(st.WebAuthn) != 2 || st.RecoveryCodesLeft != 10 {
		t.Fatalf("status: %+v", st)
	}

	// login with the key
	_, ch := startTwoFactor(ts, "a@example.cz")
	if !slices.Equal(ch.Methods, []string{"webauthn", "recovery"}) {
		t.Fatalf("methods: %v", ch.Methods)
	}
	c, res, body := loginWithKey(ts, yubi, testOrigin)
	if res.StatusCode != http.StatusOK || c.session == "" {
		t.Fatalf("webauthn login: %d %s", res.StatusCode, body)
	}
	st = doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if st.WebAuthn[0].LastUsedAt == nil {
		t.Fatalf("last used not recorded: %+v", st.WebAuthn[0])
	}
	// wrong origin / unknown key fail
	_, res, body = loginWithKey(ts, yubi, "https://evil.example")
	assertCode(t, res, body, http.StatusUnauthorized, "webauthn_failed")
	_, res, body = loginWithKey(ts, newSoftKey(t), testOrigin)
	assertCode(t, res, body, http.StatusUnauthorized, "webauthn_failed")
	// a cloned key (sign counter going backwards) is refused
	yubi.count = 0
	_, res, body = loginWithKey(ts, yubi, testOrigin)
	assertCode(t, res, body, http.StatusUnauthorized, "webauthn_failed")

	// deleting: other users' keys are 404, the last key takes the recovery codes with it
	res, body = b.do("DELETE", fmt.Sprintf("/api/auth/2fa/webauthn/%d", created.Key.ID), api.PasswordConfirm{Password: pw})
	assertCode(t, res, body, http.StatusNotFound, "not_found")
	res, body = a.do("DELETE", fmt.Sprintf("/api/auth/2fa/webauthn/%d", created.Key.ID), api.PasswordConfirm{Password: "wrong"})
	assertCode(t, res, body, http.StatusUnprocessableEntity, "wrong_password")
	for _, k := range st.WebAuthn {
		a.mustDo(http.StatusNoContent, "DELETE", fmt.Sprintf("/api/auth/2fa/webauthn/%d", k.ID), api.PasswordConfirm{Password: pw})
	}
	st = doJSON[api.TwoFactorStatus](a, http.StatusOK, "GET", "/api/auth/2fa", nil)
	if st.Enabled || st.RecoveryCodesLeft != 0 {
		t.Fatalf("status after delete: %+v", st)
	}
}

func TestWebAuthnLoginWithoutKey(t *testing.T) {
	ts := newTestServer(t)
	a := ts.signup("a@example.cz", "Firma A")
	enableTOTP(ts, a)
	_, ch := startTwoFactor(ts, "a@example.cz")
	res, body := ts.anon().do("POST", "/api/auth/login/webauthn/options", api.ChallengeRequest{Token: ch.Token})
	assertCode(t, res, body, http.StatusUnauthorized, "webauthn_failed")
	res, body = ts.anon().do("POST", "/api/auth/login/webauthn/options", api.ChallengeRequest{Token: "nope"})
	assertCode(t, res, body, http.StatusUnauthorized, "two_factor_expired")
}
