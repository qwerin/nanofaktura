// Package secret encrypts small secrets stored in the database (bank API
// tokens …) with AES-256-GCM.
//
// The key comes from NANOFAKTURA_SECRET_KEY (32 bytes, base64 or hex). When
// it is not set, LoadOrCreateKey generates one and persists it in
// NANOFAKTURA_DATA_DIR/secret.key, so a single-node install works out of the
// box; losing that file makes the stored secrets unreadable.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// KeySize is the AES-256 key length in bytes.
const KeySize = 32

// KeyFile is the name of the generated key file inside the data directory.
const KeyFile = "secret.key"

// prefix marks the ciphertext format version.
const prefix = "v1:"

var (
	// ErrInvalidKey: the key is not 32 bytes of base64 or hex.
	ErrInvalidKey = errors.New("secret: key must be 32 bytes encoded as base64 or hex")
	// ErrDecrypt: the ciphertext is malformed or was encrypted with another key.
	ErrDecrypt = errors.New("secret: cannot decrypt (wrong key or corrupted value)")
)

// Box encrypts and decrypts strings with one key. It is safe for concurrent use.
type Box struct {
	aead cipher.AEAD
}

// New returns a Box for a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// NewRandom returns a Box with a fresh random key (values do not survive a
// restart; for tests and tools that never persist secrets).
func NewRandom() *Box {
	b, err := New(GenerateKey())
	if err != nil {
		panic(err)
	}
	return b
}

// GenerateKey returns 32 random bytes.
func GenerateKey() []byte {
	k := make([]byte, KeySize)
	_, _ = rand.Read(k) // never fails (crypto/rand)
	return k
}

// ParseKey decodes a key given as 64 hex characters or base64 (standard or
// URL alphabet, with or without padding).
func ParseKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if len(s) == 2*KeySize {
		if k, err := hex.DecodeString(s); err == nil {
			return k, nil
		}
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if k, err := enc.DecodeString(s); err == nil && len(k) == KeySize {
			return k, nil
		}
	}
	return nil, ErrInvalidKey
}

// LoadOrCreateKey returns the key from envValue when non-empty, otherwise
// from dataDir/secret.key, generating and writing that file (0600) when it
// does not exist yet; generated reports the latter so the caller can warn.
func LoadOrCreateKey(envValue, dataDir string) (key []byte, generated bool, err error) {
	if strings.TrimSpace(envValue) != "" {
		k, err := ParseKey(envValue)
		if err != nil {
			return nil, false, fmt.Errorf("NANOFAKTURA_SECRET_KEY: %w", err)
		}
		return k, false, nil
	}
	path := filepath.Join(dataDir, KeyFile)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		k, err := ParseKey(string(data))
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", path, err)
		}
		return k, false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, false, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, false, err
	}
	k := GenerateKey()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, false, err
	}
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(k) + "\n"); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	if err := f.Close(); err != nil {
		return nil, false, err
	}
	return k, true, nil
}

// Encrypt returns "v1:" + base64url(nonce | ciphertext). The empty string
// stays empty (no secret stored).
func (b *Box) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt; "" decrypts to "".
func (b *Box) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	raw, ok := strings.CutPrefix(value, prefix)
	if !ok {
		return "", ErrDecrypt
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(data) < b.aead.NonceSize() {
		return "", ErrDecrypt
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, data[:n], data[n:], nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}
