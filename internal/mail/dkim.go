package mail

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/emersion/go-msgauth/dkim"
)

// DKIMConfig enables DKIM signing of outgoing messages (relaxed/relaxed,
// SHA-256). Key is an RSA (≥ 2048 bits) or Ed25519 private key.
type DKIMConfig struct {
	Domain   string
	Selector string
	Key      crypto.Signer
}

// dkimHeaders are the signed header fields (absent ones are "oversigned", so
// adding them later breaks the signature).
var dkimHeaders = []string{
	"From", "To", "Cc", "Reply-To", "Subject", "Date", "Message-ID",
	"MIME-Version", "Content-Type", "Content-Transfer-Encoding",
}

// ParseDKIMKey parses a PEM private key (PKCS#8 "PRIVATE KEY" with RSA or
// Ed25519, or PKCS#1 "RSA PRIVATE KEY"). Literal "\n" sequences are accepted
// as line breaks (keys pasted into a single-line environment variable).
// Errors never contain key material.
func ParseDKIMKey(pemText string) (crypto.Signer, error) {
	pemText = strings.ReplaceAll(strings.TrimSpace(pemText), `\n`, "\n")
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("DKIM key: no PEM block found")
	}
	var key any
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("DKIM key: unsupported PEM type %q (use PRIVATE KEY or RSA PRIVATE KEY)", block.Type)
	}
	if err != nil {
		return nil, errors.New("DKIM key: invalid private key")
	}
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.N.BitLen() < 2048 {
			return nil, fmt.Errorf("DKIM key: RSA key has %d bits, at least 2048 required", k.N.BitLen())
		}
		return k, nil
	case ed25519.PrivateKey:
		return k, nil
	default:
		return nil, errors.New("DKIM key: only RSA and Ed25519 keys are supported")
	}
}

// DKIMKeyType is "rsa" or "ed25519" (the k= tag of the DNS record).
func DKIMKeyType(k crypto.Signer) string {
	if _, ok := k.Public().(ed25519.PublicKey); ok {
		return "ed25519"
	}
	return "rsa"
}

// dkimPublicKeyData is the p= value of the DNS record for k: base64 of the
// PKIX public key (RSA) or of the raw 32-byte key (Ed25519, RFC 8463).
func dkimPublicKeyData(pub crypto.PublicKey) (string, error) {
	switch p := pub.(type) {
	case ed25519.PublicKey:
		return base64.StdEncoding.EncodeToString(p), nil
	case *rsa.PublicKey:
		der, err := x509.MarshalPKIXPublicKey(p)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(der), nil
	}
	return "", errors.New("unsupported key type")
}

// DKIMRecord is the TXT record to publish at <selector>._domainkey.<domain>.
func DKIMRecord(k crypto.Signer) string {
	p, err := dkimPublicKeyData(k.Public())
	if err != nil {
		return ""
	}
	return "v=DKIM1; k=" + DKIMKeyType(k) + "; p=" + p
}

// signDKIM prepends a DKIM-Signature header to the rendered message.
func signDKIM(data []byte, c *DKIMConfig) ([]byte, error) {
	var out bytes.Buffer
	err := dkim.Sign(&out, bytes.NewReader(data), &dkim.SignOptions{
		Domain: c.Domain, Selector: c.Selector, Signer: c.Key, Hash: crypto.SHA256,
		HeaderCanonicalization: dkim.CanonicalizationRelaxed,
		BodyCanonicalization:   dkim.CanonicalizationRelaxed,
		HeaderKeys:             dkimHeaders,
	})
	if err != nil {
		return nil, fmt.Errorf("dkim sign: %w", err)
	}
	return out.Bytes(), nil
}
