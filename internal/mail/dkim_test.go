package mail

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-msgauth/dkim"
)

func pemKey(t *testing.T, k any, pkcs1 bool) string {
	t.Helper()
	if pkcs1 {
		return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k.(*rsa.PrivateKey))}))
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestParseDKIMKey(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	small, _ := rsa.GenerateKey(rand.Reader, 1024)
	_, edKey, _ := ed25519.GenerateKey(rand.Reader)

	if _, err := ParseDKIMKey(pemKey(t, rsaKey, true)); err != nil {
		t.Fatalf("PKCS#1: %v", err)
	}
	// a key pasted into one env line with literal \n
	oneLine := strings.ReplaceAll(pemKey(t, rsaKey, false), "\n", `\n`)
	if k, err := ParseDKIMKey(oneLine); err != nil || DKIMKeyType(k) != "rsa" {
		t.Fatalf("PKCS#8 one line: %v", err)
	}
	if k, err := ParseDKIMKey(pemKey(t, edKey, false)); err != nil || DKIMKeyType(k) != "ed25519" {
		t.Fatalf("ed25519: %v", err)
	}
	if _, err := ParseDKIMKey(pemKey(t, small, false)); err == nil || !strings.Contains(err.Error(), "2048") {
		t.Fatalf("1024-bit key accepted: %v", err)
	}
	for _, bad := range []string{"", "garbage", "-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----"} {
		_, err := ParseDKIMKey(bad)
		if err == nil {
			t.Fatalf("%q accepted", bad)
		}
		if strings.Contains(err.Error(), "AAAA") {
			t.Fatalf("error leaks key material: %v", err)
		}
	}
	if rec := DKIMRecord(edKey); !strings.HasPrefix(rec, "v=DKIM1; k=ed25519; p=") {
		t.Fatalf("record %q", rec)
	}
}

func TestComposeDKIMSignatureVerifies(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, edKey, _ := ed25519.GenerateKey(rand.Reader)
	for name, key := range map[string]crypto.Signer{"rsa": rsaKey, "ed25519": edKey} {
		t.Run(name, func(t *testing.T) {
			cfg := SMTPConfig{From: "Firma <faktury@example.cz>", DKIM: &DKIMConfig{Domain: "example.cz", Selector: "nf", Key: key}}
			data, from, rcpt, err := Compose(cfg, Message{To: []string{"klient@example.com"}, Cc: []string{"kopie@example.com"},
				Subject: "Faktura č. 1 – ěščř", Text: "Dobrý den,\npříliš žluťoučký kůň.\n",
				Attachments: []Attachment{{Filename: "f.pdf", Data: []byte("%PDF")}}}, time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if from != "faktury@example.cz" || len(rcpt) != 2 {
				t.Fatalf("envelope %s %v", from, rcpt)
			}
			head, _, _ := bytes.Cut(data, []byte("\r\n\r\n"))
			for _, h := range []string{"DKIM-Signature:", "\r\nDate: ", "\r\nMessage-ID: <", "@example.cz>"} {
				if !bytes.Contains(head, []byte(h)) {
					t.Fatalf("header %q missing:\n%s", h, head)
				}
			}
			if !bytes.Contains(head, []byte("c=relaxed/relaxed")) {
				t.Fatalf("not relaxed/relaxed:\n%s", head)
			}
			record := DKIMRecord(key)
			verifs, err := dkim.VerifyWithOptions(bytes.NewReader(data), &dkim.VerifyOptions{
				LookupTXT: func(domain string) ([]string, error) {
					if domain != "nf._domainkey.example.cz" {
						return nil, errors.New("unexpected lookup " + domain)
					}
					return []string{record}, nil
				},
			})
			if err != nil || len(verifs) != 1 || verifs[0].Err != nil {
				t.Fatalf("verify: %v %+v", err, verifs)
			}
			for _, h := range []string{"from", "to", "cc", "subject", "date", "message-id", "mime-version", "content-type"} {
				found := false
				for _, k := range verifs[0].HeaderKeys {
					found = found || strings.EqualFold(k, h)
				}
				if !found {
					t.Fatalf("header %s not signed: %v", h, verifs[0].HeaderKeys)
				}
			}
			// a modified body must fail
			tampered := bytes.Replace(data, []byte("%PDF"), []byte("%PDX"), 1)
			tampered = bytes.Replace(tampered, []byte("JVBERg=="), []byte("JVBEWA=="), 1)
			verifs, _ = dkim.VerifyWithOptions(bytes.NewReader(tampered), &dkim.VerifyOptions{
				LookupTXT: func(string) ([]string, error) { return []string{record}, nil }})
			if len(verifs) != 1 || verifs[0].Err == nil {
				t.Fatal("tampered message verified")
			}
		})
	}
}
