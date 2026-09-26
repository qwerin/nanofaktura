package auth

import (
	"strings"
	"testing"
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
