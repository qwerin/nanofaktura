package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("heslo1234")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "heslo1234") || CheckPassword(h, "jine-heslo") || CheckPassword("", "heslo1234") {
		t.Fatal("CheckPassword mismatch")
	}
}

func TestNewToken(t *testing.T) {
	plain, hash := newToken(APITokenPrefix)
	if !strings.HasPrefix(plain, "nf_") || len(plain) != 3+43 || hash != hashToken(plain) || len(hash) != 64 {
		t.Fatalf("bad token %q / %q", plain, hash)
	}
	if other, _ := newToken(APITokenPrefix); other == plain {
		t.Fatal("tokens not random")
	}
}

func TestTOTP(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890")) // RFC 6238 appendix B (SHA-1), last 6 digits
	for unix, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		got, err := TOTPCode(secret, unix/30)
		if err != nil || got != want {
			t.Fatalf("TOTPCode(%d) = %q, %v; want %q", unix, got, err, want)
		}
	}

	now := time.Unix(1234567890, 0)
	prev, _ := TOTPCode(secret, TOTPStep(now)-1)
	step, ok := VerifyTOTP(secret, prev, now, 0)
	if !ok || step != TOTPStep(now)-1 {
		t.Fatalf("previous step not accepted: %d %v", step, ok)
	}
	if _, ok := VerifyTOTP(secret, prev, now, step); ok {
		t.Fatal("replayed code accepted")
	}
	old, _ := TOTPCode(secret, TOTPStep(now)-2)
	if _, ok := VerifyTOTP(secret, old, now, 0); ok {
		t.Fatal("code two steps old accepted")
	}
	if _, ok := VerifyTOTP(secret, "005 924", now, 0); !ok {
		t.Fatal("code with a space rejected")
	}
	if _, ok := VerifyTOTP(secret, "abc", now, 0); ok {
		t.Fatal("garbage accepted")
	}
	if u := TOTPURL("NanoFaktura", "a@b.cz", "ABC"); !strings.HasPrefix(u, "otpauth://totp/NanoFaktura:a@b.cz?") || !strings.Contains(u, "secret=ABC") {
		t.Fatalf("url %q", u)
	}
	if len(NewTOTPSecret()) != 32 {
		t.Fatal("secret length")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes := NewRecoveryCodes()
	if len(codes) != RecoveryCodeCount || len(codes[0]) != 11 || codes[0][5] != '-' || codes[0] == codes[1] {
		t.Fatalf("codes %v", codes)
	}
	if HashRecoveryCode(codes[0]) != HashRecoveryCode(" "+strings.ToUpper(strings.ReplaceAll(codes[0], "-", " "))+" ") {
		t.Fatal("normalization")
	}
}
