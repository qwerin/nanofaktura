package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Second factor (SPEC §3.1): TOTP, WebAuthn security keys, recovery codes.

const (
	challengeTTL         = 10 * time.Minute
	challengeMaxAttempts = 5
	totpIssuer           = "NanoFaktura"
)

// Second factor methods offered at login.
const (
	MethodTOTP     = "totp"
	MethodWebAuthn = "webauthn"
	MethodRecovery = "recovery"
)

type TwoFactorChallenge struct {
	Token     string    `json:"token" doc:"Pass to /api/auth/login/2fa or /api/auth/login/webauthn*"`
	Methods   []string  `json:"methods" nullable:"false" enum:"totp,webauthn,recovery"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LoginResult is the outcome of POST /api/auth/login: either the user is
// logged in (Me + session cookie) or a second factor is required.
type LoginResult struct {
	Me        *Me                 `json:"me,omitempty" doc:"Logged-in user; missing when a second factor is required"`
	TwoFactor *TwoFactorChallenge `json:"two_factor,omitempty"`
}

type loginOutput struct {
	SetCookie string `header:"Set-Cookie"`
	Body      LoginResult
}

type LoginCodeRequest struct {
	Token string `json:"token" maxLength:"100"`
	Code  string `json:"code" maxLength:"32" doc:"6-digit code from the authenticator app or a recovery code"`
}

type ChallengeRequest struct {
	Token string `json:"token" maxLength:"100"`
}

type WebAuthnLoginRequest struct {
	Token      string          `json:"token" maxLength:"100"`
	Credential json.RawMessage `json:"credential" doc:"PublicKeyCredential from navigator.credentials.get(), JSON-serialized (base64url)"`
}

type WebAuthnOptions struct {
	Options json.RawMessage `json:"options" doc:"PublicKeyCredentialRequestOptions (binary fields base64url)"`
}

type TwoFactorStatus struct {
	Enabled           bool          `json:"enabled"`
	TOTP              bool          `json:"totp"`
	WebAuthn          []WebAuthnKey `json:"webauthn" nullable:"false"`
	RecoveryCodesLeft int           `json:"recovery_codes_left"`
}

type PasswordConfirm struct {
	Password string `json:"password" maxLength:"72" doc:"Current password"`
}

type TOTPSetup struct {
	Secret     string `json:"secret" doc:"Base32 secret for manual entry"`
	OtpauthURL string `json:"otpauth_url" doc:"otpauth:// URL for the QR code"`
}

type TOTPEnableRequest struct {
	Code string `json:"code" maxLength:"16"`
}

type RecoveryCodes struct {
	RecoveryCodes []string `json:"recovery_codes" nullable:"false" doc:"Shown only now; empty when the user already has codes"`
}

type WebAuthnRegistrationOptions struct {
	Token   string          `json:"token"`
	Options json.RawMessage `json:"options" doc:"PublicKeyCredentialCreationOptions (binary fields base64url)"`
}

type WebAuthnRegisterRequest struct {
	Token      string          `json:"token" maxLength:"100"`
	Name       string          `json:"name" minLength:"1" maxLength:"100"`
	Credential json.RawMessage `json:"credential" doc:"PublicKeyCredential from navigator.credentials.create(), JSON-serialized (base64url)"`
}

type WebAuthnKeyCreated struct {
	Key           WebAuthnKey `json:"key"`
	RecoveryCodes []string    `json:"recovery_codes" nullable:"false"`
}

func (s *server) registerTwoFactor(public, authed huma.API) {
	huma.Post(public, "/api/auth/login/2fa", s.loginWithCode)
	huma.Post(public, "/api/auth/login/webauthn/options", s.loginWebAuthnOptions)
	huma.Post(public, "/api/auth/login/webauthn", s.loginWithWebAuthn)

	huma.Get(authed, "/api/auth/2fa", s.getTwoFactor)
	huma.Post(authed, "/api/auth/2fa/totp/setup", s.setupTOTP)
	huma.Post(authed, "/api/auth/2fa/totp/enable", s.enableTOTP)
	huma.Delete(authed, "/api/auth/2fa/totp", s.disableTOTP, status(http.StatusNoContent))
	huma.Post(authed, "/api/auth/2fa/webauthn/options", s.webAuthnRegistrationOptions)
	huma.Post(authed, "/api/auth/2fa/webauthn", s.registerWebAuthn, status(http.StatusCreated))
	huma.Delete(authed, "/api/auth/2fa/webauthn/{id}", s.deleteWebAuthn, status(http.StatusNoContent))
	huma.Post(authed, "/api/auth/2fa/recovery-codes", s.regenerateRecoveryCodes)
}

// ---- helpers ----

func hasSecondFactor(db *gorm.DB, user *model.User) (bool, error) {
	if user.TOTPSecretEnc != "" {
		return true, nil
	}
	var n int64
	err := db.Model(&model.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&n).Error
	return n > 0, dbErrOrNil(err, "security keys")
}

func unusedRecoveryCodes(db *gorm.DB, userID uint) (int, error) {
	var n int64
	err := db.Model(&model.RecoveryCode{}).Where("user_id = ? AND used_at IS NULL", userID).Count(&n).Error
	return int(n), dbErrOrNil(err, "recovery codes")
}

// loginMethods lists the second factors user can log in with (empty = none).
func loginMethods(db *gorm.DB, user *model.User) ([]string, error) {
	methods := []string{}
	if user.TOTPSecretEnc != "" {
		methods = append(methods, MethodTOTP)
	}
	var keys int64
	if err := db.Model(&model.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&keys).Error; err != nil {
		return nil, dbErr(err, "security keys")
	}
	if keys > 0 {
		methods = append(methods, MethodWebAuthn)
	}
	if len(methods) == 0 {
		return methods, nil
	}
	left, err := unusedRecoveryCodes(db, user.ID)
	if err != nil {
		return nil, err
	}
	if left > 0 {
		methods = append(methods, MethodRecovery)
	}
	return methods, nil
}

func wrongPassword() error {
	return apiError(http.StatusUnprocessableEntity, CodeWrongPassword, "validation failed",
		&huma.ErrorDetail{Location: "body.password", Message: "password is incorrect"})
}

func invalidCode() error {
	return apiError(http.StatusUnprocessableEntity, CodeInvalidCode, "validation failed",
		&huma.ErrorDetail{Location: "body.code", Message: "code is incorrect"})
}

func challengeExpired() error {
	return apiError(http.StatusUnauthorized, CodeTwoFactorExpired, "login has expired, log in with your password again")
}

// checkCurrentPassword re-reads the user (the context copy may be stale) and
// verifies password.
func (s *server) checkCurrentPassword(ctx context.Context, password string) (*model.User, error) {
	var user model.User
	if err := s.db.WithContext(ctx).First(&user, auth.UserFrom(ctx).ID).Error; err != nil {
		return nil, dbErr(err, "user")
	}
	key := userKey(user.ID)
	if err := s.rateBlocked(s.limits.password, key); err != nil {
		return nil, err
	}
	if !auth.CheckPassword(user.PasswordHash, password) {
		s.rateFail(s.limits.password, key)
		return nil, wrongPassword()
	}
	return &user, nil
}

func userKey(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func (s *server) webAuthnRP() (*webauthn.WebAuthn, error) {
	if s.webauthnErr != nil {
		return nil, huma.Error500InternalServerError("security keys are not available: check NANOFAKTURA_PUBLIC_URL", s.webauthnErr)
	}
	return s.webauthn, nil
}

// newChallenge stores a challenge for userID and returns its token.
func (s *server) newChallenge(tx *gorm.DB, userID uint, purpose string, sd *webauthn.SessionData) (string, *model.AuthChallenge, error) {
	plain, hash := auth.NewSecret()
	now := s.deps.Now()
	ch := model.AuthChallenge{UserID: userID, Purpose: purpose, TokenHash: hash, ExpiresAt: now.Add(challengeTTL), CreatedAt: now}
	if sd != nil {
		var err error
		if ch.WebAuthnSession, err = encodeSession(sd); err != nil {
			return "", nil, err
		}
	}
	if err := tx.Create(&ch).Error; err != nil {
		return "", nil, dbErr(err, "challenge")
	}
	return plain, &ch, nil
}

// lockChallenge loads a live challenge of purpose by token (row locked);
// nil when unknown or expired.
func (s *server) lockChallenge(tx *gorm.DB, token, purpose string) (*model.AuthChallenge, error) {
	var ch model.AuthChallenge
	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Where("token_hash = ? AND purpose = ? AND expires_at > ?", auth.HashSecret(token), purpose, s.deps.Now()).
		First(&ch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, dbErr(err, "challenge")
	}
	return &ch, nil
}

// failAttempt counts a wrong second factor; the challenge dies after
// challengeMaxAttempts.
func failAttempt(tx *gorm.DB, ch *model.AuthChallenge) error {
	if ch.Attempts+1 >= challengeMaxAttempts {
		return dbErrOrNil(tx.Delete(ch).Error, "challenge")
	}
	return dbErrOrNil(tx.Model(ch).Update("attempts", gorm.Expr("attempts + 1")).Error, "challenge")
}

// ensureRecoveryCodes creates recovery codes when the user has no unused one
// and returns the new plaintext codes (nil otherwise).
func (s *server) ensureRecoveryCodes(tx *gorm.DB, userID uint) ([]string, error) {
	left, err := unusedRecoveryCodes(tx, userID)
	if err != nil || left > 0 {
		return nil, err
	}
	return s.replaceRecoveryCodes(tx, userID)
}

func (s *server) replaceRecoveryCodes(tx *gorm.DB, userID uint) ([]string, error) {
	if err := tx.Where("user_id = ?", userID).Delete(&model.RecoveryCode{}).Error; err != nil {
		return nil, dbErr(err, "recovery codes")
	}
	codes := auth.NewRecoveryCodes()
	rows := make([]model.RecoveryCode, len(codes))
	for i, c := range codes {
		rows[i] = model.RecoveryCode{UserID: userID, CodeHash: auth.HashRecoveryCode(c), CreatedAt: s.deps.Now()}
	}
	return codes, dbErrOrNil(tx.Create(&rows).Error, "recovery codes")
}

// afterFactorRemoved deletes the recovery codes once no factor is left.
func afterFactorRemoved(tx *gorm.DB, user *model.User) error {
	has, err := hasSecondFactor(tx, user)
	if err != nil || has {
		return err
	}
	return dbErrOrNil(tx.Where("user_id = ?", user.ID).Delete(&model.RecoveryCode{}).Error, "recovery codes")
}

// ---- login ----

func (s *server) login(ctx context.Context, in *struct{ Body LoginRequest }) (*loginOutput, error) {
	email := normalizeEmail(in.Body.Email)
	if err := s.rateLimit(s.limits.loginIP, clientIP(ctx)); err != nil {
		return nil, err
	}
	if err := s.rateBlocked(s.limits.loginEmail, email); err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	var user model.User
	err := db.Where("email = ?", email).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, dbErr(err, "user")
	}
	if !auth.CheckPassword(user.PasswordHash, in.Body.Password) {
		s.rateFail(s.limits.loginEmail, email)
		return nil, huma.Error401Unauthorized("invalid email or password")
	}
	s.limits.loginEmail.Reset(email)
	methods, err := loginMethods(db, &user)
	if err != nil {
		return nil, err
	}
	if len(methods) == 0 {
		out, err := s.startSession(ctx, &user)
		if err != nil {
			return nil, err
		}
		return &loginOutput{SetCookie: out.SetCookie, Body: LoginResult{Me: &out.Body}}, nil
	}
	token, ch, err := s.newChallenge(db, user.ID, model.ChallengeLogin, nil)
	if err != nil {
		return nil, err
	}
	return &loginOutput{Body: LoginResult{TwoFactor: &TwoFactorChallenge{Token: token, Methods: methods, ExpiresAt: ch.ExpiresAt}}}, nil
}

// finishLogin runs verify inside a transaction on the locked login challenge.
// verify returns false for a wrong factor (counted as a failed attempt,
// answered with failErr); on success the challenge is removed and the session starts.
func (s *server) finishLogin(ctx context.Context, token string, failErr error,
	verify func(tx *gorm.DB, user *model.User, ch *model.AuthChallenge) (bool, error),
) (*meWithCookie, error) {
	if err := s.rateLimit(s.limits.loginIP, clientIP(ctx)); err != nil {
		return nil, err
	}
	var user model.User
	var fail error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ch, err := s.lockChallenge(tx, token, model.ChallengeLogin)
		if err != nil {
			return err
		}
		if ch == nil {
			fail = challengeExpired()
			return nil
		}
		// wrong factors are limited per user too: new challenges (password
		// known) must not give a TOTP guesser 5 fresh attempts each
		if err := s.rateBlocked(s.limits.twoFactor, userKey(ch.UserID)); err != nil {
			fail = err
			return nil
		}
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&user, ch.UserID).Error; err != nil {
			return dbErr(err, "user")
		}
		ok, err := verify(tx, &user, ch)
		if err != nil {
			return err
		}
		if !ok {
			fail = failErr
			s.rateFail(s.limits.twoFactor, userKey(ch.UserID))
			return failAttempt(tx, ch) // committed: the attempt counts
		}
		return dbErrOrNil(tx.Delete(ch).Error, "challenge")
	})
	if err != nil {
		return nil, err
	}
	if fail != nil {
		return nil, fail
	}
	return s.startSession(ctx, &user)
}

func (s *server) loginWithCode(ctx context.Context, in *struct{ Body LoginCodeRequest }) (*meWithCookie, error) {
	fail := apiError(http.StatusUnauthorized, CodeInvalidCode, "invalid code")
	return s.finishLogin(ctx, in.Body.Token, fail, func(tx *gorm.DB, user *model.User, _ *model.AuthChallenge) (bool, error) {
		now := s.deps.Now()
		code := strings.TrimSpace(in.Body.Code)
		if user.TOTPSecretEnc != "" && len(strings.ReplaceAll(code, " ", "")) == 6 {
			secret, err := s.deps.Secrets.Decrypt(user.TOTPSecretEnc)
			if err != nil {
				return false, err
			}
			if step, ok := auth.VerifyTOTP(secret, code, now, user.TOTPLastStep); ok {
				return true, dbErrOrNil(tx.Model(user).Update("totp_last_step", step).Error, "user")
			}
			return false, nil
		}
		res := tx.Model(&model.RecoveryCode{}).
			Where("user_id = ? AND code_hash = ? AND used_at IS NULL", user.ID, auth.HashRecoveryCode(code)).
			Update("used_at", now)
		if res.Error != nil {
			return false, dbErr(res.Error, "recovery codes")
		}
		return res.RowsAffected == 1, nil
	})
}

func (s *server) loginWebAuthnOptions(ctx context.Context, in *struct{ Body ChallengeRequest }) (*Out[WebAuthnOptions], error) {
	if err := s.rateLimit(s.limits.loginIP, clientIP(ctx)); err != nil {
		return nil, err
	}
	rp, err := s.webAuthnRP()
	if err != nil {
		return nil, err
	}
	var out WebAuthnOptions
	var fail error
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ch, err := s.lockChallenge(tx, in.Body.Token, model.ChallengeLogin)
		if err != nil {
			return err
		}
		if ch == nil {
			fail = challengeExpired()
			return nil
		}
		var user model.User
		if err := tx.First(&user, ch.UserID).Error; err != nil {
			return dbErr(err, "user")
		}
		wu, err := loadWAUser(tx, &user)
		if err != nil {
			return err
		}
		if len(wu.creds) == 0 {
			fail = apiError(http.StatusUnauthorized, CodeWebAuthnFailed, "no security key is registered")
			return nil
		}
		assertion, sd, err := rp.BeginLogin(wu, webauthn.WithUserVerification(protocol.VerificationDiscouraged))
		if err != nil {
			return err
		}
		if out.Options, err = json.Marshal(assertion.Response); err != nil {
			return err
		}
		enc, err := encodeSession(sd)
		if err != nil {
			return err
		}
		return dbErrOrNil(tx.Model(ch).Update("web_authn_session", enc).Error, "challenge")
	})
	if err != nil {
		return nil, err
	}
	if fail != nil {
		return nil, fail
	}
	return &Out[WebAuthnOptions]{Body: out}, nil
}

func (s *server) loginWithWebAuthn(ctx context.Context, in *struct{ Body WebAuthnLoginRequest }) (*meWithCookie, error) {
	rp, err := s.webAuthnRP()
	if err != nil {
		return nil, err
	}
	fail := apiError(http.StatusUnauthorized, CodeWebAuthnFailed, "security key verification failed")
	return s.finishLogin(ctx, in.Body.Token, fail, func(tx *gorm.DB, user *model.User, ch *model.AuthChallenge) (bool, error) {
		if ch.WebAuthnSession == "" {
			return false, nil
		}
		sd, err := decodeSession(ch.WebAuthnSession)
		if err != nil {
			return false, err
		}
		parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Body.Credential)
		if err != nil {
			return false, nil
		}
		wu, err := loadWAUser(tx, user)
		if err != nil {
			return false, err
		}
		cred, err := rp.ValidateLogin(wu, sd, parsed)
		if err != nil || cred.Authenticator.CloneWarning {
			return false, nil
		}
		row := wu.row(cred)
		if row == nil {
			return false, nil
		}
		data, err := encodeCredential(cred)
		if err != nil {
			return false, err
		}
		err = tx.Model(row).Updates(map[string]any{"data": data, "last_used_at": s.deps.Now()}).Error
		return true, dbErrOrNil(err, "security key")
	})
}

// ---- management ----

func (s *server) getTwoFactor(ctx context.Context, _ *struct{}) (*Out[TwoFactorStatus], error) {
	db := s.db.WithContext(ctx)
	var user model.User
	if err := db.First(&user, auth.UserFrom(ctx).ID).Error; err != nil {
		return nil, dbErr(err, "user")
	}
	var keys []model.WebAuthnCredential
	if err := db.Where("user_id = ?", user.ID).Order("id").Find(&keys).Error; err != nil {
		return nil, dbErr(err, "security keys")
	}
	out := TwoFactorStatus{TOTP: user.TOTPSecretEnc != "", WebAuthn: make([]WebAuthnKey, len(keys))}
	for i := range keys {
		out.WebAuthn[i] = toWebAuthnKey(&keys[i])
	}
	out.Enabled = out.TOTP || len(keys) > 0
	left, err := unusedRecoveryCodes(db, user.ID)
	if err != nil {
		return nil, err
	}
	out.RecoveryCodesLeft = left
	return &Out[TwoFactorStatus]{Body: out}, nil
}

func (s *server) setupTOTP(ctx context.Context, in *struct{ Body PasswordConfirm }) (*Out[TOTPSetup], error) {
	user, err := s.checkCurrentPassword(ctx, in.Body.Password)
	if err != nil {
		return nil, err
	}
	if user.TOTPSecretEnc != "" {
		return nil, conflict(CodeTOTPEnabled, "authenticator app is already enabled")
	}
	secret := auth.NewTOTPSecret()
	enc, err := s.deps.Secrets.Encrypt(secret)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(user).Update("totp_pending_enc", enc).Error; err != nil {
		return nil, dbErr(err, "user")
	}
	return &Out[TOTPSetup]{Body: TOTPSetup{Secret: secret, OtpauthURL: auth.TOTPURL(totpIssuer, user.Email, secret)}}, nil
}

func (s *server) enableTOTP(ctx context.Context, in *struct {
	Session string `cookie:"nf_session"`
	Body    TOTPEnableRequest
}) (*Out[RecoveryCodes], error) {
	key := userKey(auth.UserFrom(ctx).ID)
	if err := s.rateBlocked(s.limits.twoFactor, key); err != nil {
		return nil, err
	}
	codes := []string{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&user, auth.UserFrom(ctx).ID).Error; err != nil {
			return dbErr(err, "user")
		}
		if user.TOTPSecretEnc != "" {
			return conflict(CodeTOTPEnabled, "authenticator app is already enabled")
		}
		if user.TOTPPendingEnc == "" {
			return conflict(CodeTOTPNotPending, "start the authenticator app setup first")
		}
		secret, err := s.deps.Secrets.Decrypt(user.TOTPPendingEnc)
		if err != nil {
			return err
		}
		step, ok := auth.VerifyTOTP(secret, in.Body.Code, s.deps.Now(), 0)
		if !ok {
			s.rateFail(s.limits.twoFactor, key)
			return invalidCode()
		}
		err = tx.Model(&user).Updates(map[string]any{
			"totp_secret_enc": user.TOTPPendingEnc, "totp_pending_enc": "", "totp_last_step": step,
		}).Error
		if err != nil {
			return dbErr(err, "user")
		}
		return s.afterFactorAdded(tx, user.ID, in.Session, &codes)
	})
	if err != nil {
		return nil, err
	}
	return &Out[RecoveryCodes]{Body: RecoveryCodes{RecoveryCodes: codes}}, nil
}

// afterFactorAdded creates recovery codes if needed (into *codes) and logs
// out the user's other browsers.
func (s *server) afterFactorAdded(tx *gorm.DB, userID uint, session string, codes *[]string) error {
	created, err := s.ensureRecoveryCodes(tx, userID)
	if err != nil {
		return err
	}
	if created != nil {
		*codes = created
	}
	return dbErrOrNil(auth.DeleteOtherSessions(tx, userID, session), "sessions")
}

func (s *server) disableTOTP(ctx context.Context, in *struct{ Body PasswordConfirm }) (*NoContent, error) {
	user, err := s.checkCurrentPassword(ctx, in.Body.Password)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(user).Updates(map[string]any{"totp_secret_enc": "", "totp_pending_enc": "", "totp_last_step": 0}).Error
		if err != nil {
			return dbErr(err, "user")
		}
		user.TOTPSecretEnc = ""
		return afterFactorRemoved(tx, user)
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) webAuthnRegistrationOptions(ctx context.Context, in *struct{ Body PasswordConfirm }) (*Out[WebAuthnRegistrationOptions], error) {
	rp, err := s.webAuthnRP()
	if err != nil {
		return nil, err
	}
	user, err := s.checkCurrentPassword(ctx, in.Body.Password)
	if err != nil {
		return nil, err
	}
	var out WebAuthnRegistrationOptions
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		wu, err := loadWAUser(tx, user)
		if err != nil {
			return err
		}
		exclude := make([]protocol.CredentialDescriptor, len(wu.creds))
		for i := range wu.creds {
			exclude[i] = wu.creds[i].Descriptor()
		}
		creation, sd, err := rp.BeginRegistration(wu,
			webauthn.WithExclusions(exclude),
			webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
				ResidentKey: protocol.ResidentKeyRequirementDiscouraged, UserVerification: protocol.VerificationDiscouraged,
			}),
			webauthn.WithConveyancePreference(protocol.PreferNoAttestation))
		if err != nil {
			return err
		}
		if out.Options, err = json.Marshal(creation.Response); err != nil {
			return err
		}
		out.Token, _, err = s.newChallenge(tx, user.ID, model.ChallengeWebAuthnRegister, sd)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[WebAuthnRegistrationOptions]{Body: out}, nil
}

func (s *server) registerWebAuthn(ctx context.Context, in *struct {
	Session string `cookie:"nf_session"`
	Body    WebAuthnRegisterRequest
}) (*Out[WebAuthnKeyCreated], error) {
	rp, err := s.webAuthnRP()
	if err != nil {
		return nil, err
	}
	var fail string // set when the ceremony fails; the challenge is consumed anyway
	out := WebAuthnKeyCreated{RecoveryCodes: []string{}}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		me := auth.UserFrom(ctx)
		ch, err := s.lockChallenge(tx, in.Body.Token, model.ChallengeWebAuthnRegister)
		if err != nil {
			return err
		}
		if ch == nil || ch.UserID != me.ID {
			fail = "registration has expired, start again"
			return nil
		}
		if err := tx.Delete(ch).Error; err != nil { // single-use whatever the outcome
			return dbErr(err, "challenge")
		}
		sd, err := decodeSession(ch.WebAuthnSession)
		if err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, me.ID).Error; err != nil {
			return dbErr(err, "user")
		}
		wu, err := loadWAUser(tx, &user)
		if err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialCreationResponseBytes(in.Body.Credential)
		if err != nil {
			fail = "invalid credential"
			return nil
		}
		cred, err := rp.CreateCredential(wu, sd, parsed)
		if err != nil {
			fail = "security key verification failed"
			return nil
		}
		data, err := encodeCredential(cred)
		if err != nil {
			return err
		}
		row := model.WebAuthnCredential{UserID: user.ID, Name: strings.TrimSpace(in.Body.Name),
			CredentialID: credentialKey(cred.ID), Data: data, CreatedAt: s.deps.Now()}
		if err := tx.Create(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return conflict(CodeAlreadyExists, "this security key is already registered")
			}
			return dbErr(err, "security key")
		}
		out.Key = toWebAuthnKey(&row)
		return s.afterFactorAdded(tx, user.ID, in.Session, &out.RecoveryCodes)
	})
	if err != nil {
		return nil, err
	}
	if fail != "" {
		return nil, apiError(http.StatusUnprocessableEntity, CodeWebAuthnFailed, "validation failed",
			&huma.ErrorDetail{Location: "body.credential", Message: fail})
	}
	return &Out[WebAuthnKeyCreated]{Body: out}, nil
}

func (s *server) deleteWebAuthn(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body PasswordConfirm
}) (*NoContent, error) {
	user, err := s.checkCurrentPassword(ctx, in.Body.Password)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND user_id = ?", in.ID, user.ID).Delete(&model.WebAuthnCredential{})
		if res.Error != nil {
			return dbErr(res.Error, "security key")
		}
		if res.RowsAffected == 0 {
			return notFound("security key")
		}
		return afterFactorRemoved(tx, user)
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

func (s *server) regenerateRecoveryCodes(ctx context.Context, in *struct{ Body PasswordConfirm }) (*Out[RecoveryCodes], error) {
	user, err := s.checkCurrentPassword(ctx, in.Body.Password)
	if err != nil {
		return nil, err
	}
	var codes []string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		has, err := hasSecondFactor(tx, user)
		if err != nil {
			return err
		}
		if !has {
			return conflict(CodeTwoFactorDisabled, "two-factor authentication is not enabled")
		}
		codes, err = s.replaceRecoveryCodes(tx, user.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Out[RecoveryCodes]{Body: RecoveryCodes{RecoveryCodes: codes}}, nil
}
