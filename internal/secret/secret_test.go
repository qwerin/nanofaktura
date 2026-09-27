package secret

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoxRoundTrip(t *testing.T) {
	b := NewRandom()
	c1, err := b.Encrypt("tajny-token")
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := b.Encrypt("tajny-token")
	if c1 == c2 || !strings.HasPrefix(c1, "v1:") || strings.Contains(c1, "tajny") {
		t.Fatalf("ciphertexts %q %q", c1, c2)
	}
	if p, err := b.Decrypt(c1); err != nil || p != "tajny-token" {
		t.Fatalf("decrypt %q %v", p, err)
	}
	if c, _ := b.Encrypt(""); c != "" {
		t.Fatalf("empty: %q", c)
	}
	if p, err := b.Decrypt(""); p != "" || err != nil {
		t.Fatalf("empty decrypt %q %v", p, err)
	}
	other := NewRandom()
	for _, bad := range []string{c1[:len(c1)-2], "plain", "v1:!!!", "v1:AAAA"} {
		if _, err := b.Decrypt(bad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := other.Decrypt(c1); !errors.Is(err, ErrDecrypt) {
		t.Errorf("other key: %v", err)
	}
}

func TestParseKey(t *testing.T) {
	k := GenerateKey()
	for _, s := range []string{hex.EncodeToString(k), base64.StdEncoding.EncodeToString(k), base64.RawURLEncoding.EncodeToString(k),
		" " + base64.StdEncoding.EncodeToString(k) + "\n"} {
		got, err := ParseKey(s)
		if err != nil || string(got) != string(k) {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "short", hex.EncodeToString(k[:16]), base64.StdEncoding.EncodeToString(k[:31])} {
		if _, err := ParseKey(s); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%q: %v", s, err)
		}
	}
	if _, err := New(k[:16]); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("New short: %v", err)
	}
}

func TestLoadOrCreateKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	k1, gen, err := LoadOrCreateKey("", dir)
	if err != nil || !gen || len(k1) != KeySize {
		t.Fatalf("create: %v %v", gen, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, KeyFile)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", fi, err)
	}
	k2, gen, err := LoadOrCreateKey("", dir)
	if err != nil || gen || string(k1) != string(k2) {
		t.Fatalf("reload: %v %v", gen, err)
	}
	env := GenerateKey()
	k3, gen, err := LoadOrCreateKey(hex.EncodeToString(env), dir)
	if err != nil || gen || string(k3) != string(env) {
		t.Fatalf("env: %v %v", gen, err)
	}
	if _, _, err := LoadOrCreateKey("nope", dir); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("bad env: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, KeyFile), []byte("garbage"), 0o600)
	if _, _, err := LoadOrCreateKey("", dir); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("bad file: %v", err)
	}
}
