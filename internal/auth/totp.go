package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 default, what authenticator apps support
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (RFC 6238 defaults understood by every authenticator app).
const (
	totpPeriod = 30
	totpDigits = 6
	totpSkew   = 1 // accepted steps before/after the current one
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit TOTP secret (base32, no padding).
func NewTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return b32.EncodeToString(b)
}

// TOTPURL is the otpauth:// URL shown as a QR code to authenticator apps.
func TOTPURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"},
		"digits": {fmt.Sprint(totpDigits)}, "period": {fmt.Sprint(totpPeriod)}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// TOTPCode returns the code of secret for the given time step.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // steps are positive
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%1_000_000), nil
}

// TOTPStep is the time step of t.
func TOTPStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// VerifyTOTP checks code against secret at now (±1 step) and returns the
// matched step. Steps ≤ lastStep are rejected so a code cannot be replayed.
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	for step := cur - totpSkew; step <= cur+totpSkew; step++ {
		if step <= lastStep {
			continue
		}
		want, err := TOTPCode(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// RecoveryCodeCount is how many recovery codes a user gets.
const RecoveryCodeCount = 10

const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789" // no 0/o, 1/l/i

// NewRecoveryCodes returns RecoveryCodeCount codes formatted "xxxxx-xxxxx".
func NewRecoveryCodes() []string {
	codes := make([]string, RecoveryCodeCount)
	b := make([]byte, 10)
	for i := range codes {
		_, _ = rand.Read(b)
		var sb strings.Builder
		for j, c := range b {
			if j == 5 {
				sb.WriteByte('-')
			}
			sb.WriteByte(recoveryAlphabet[int(c)%len(recoveryAlphabet)])
		}
		codes[i] = sb.String()
	}
	return codes
}

// HashRecoveryCode normalizes a recovery code as typed by the user (case,
// spaces, dashes) and returns its storage hash.
func HashRecoveryCode(code string) string {
	code = strings.ToLower(code)
	code = strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code))
	return hashToken(code)
}
