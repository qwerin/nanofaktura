package eet

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

const (
	nsSOAP = "http://schemas.xmlsoap.org/soap/envelope/"
	nsEET  = "http://fs.gov.cz/eet/schema/v4"
	nsDS   = "http://www.w3.org/2000/09/xmldsig#"
	nsWSSE = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	nsWSU  = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"

	algC14N   = "http://www.w3.org/2001/10/xml-exc-c14n#"
	algRSA    = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	algSHA256 = "http://www.w3.org/2001/04/xmlenc#sha256"
	typeB64   = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary"
	typeX509  = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-x509-token-profile-1.0#X509v3"
)

// Certificate is a cash-register certificate with its private key.
type Certificate struct {
	Key  *rsa.PrivateKey
	Cert *x509.Certificate
}

// EIC is the taxpayer the certificate was issued to (its common name).
func (c *Certificate) EIC() string { return c.Cert.Subject.CommonName }

// NotAfter is the end of the certificate's validity (issued for 366 days).
func (c *Certificate) NotAfter() time.Time { return c.Cert.NotAfter }

// ErrBadCertificate: the file is not a PKCS#12 with an RSA key, or the
// password is wrong.
var ErrBadCertificate = errors.New("eet: not a PKCS#12 file with an RSA key, or a wrong password")

// ParsePKCS12 reads the .p12 file downloaded from the certification
// authority (portal MOJE daně → DIS+ → Evidence tržeb).
func ParsePKCS12(data []byte, password string) (*Certificate, error) {
	key, cert, _, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, ErrBadCertificate
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok || cert == nil {
		return nil, ErrBadCertificate
	}
	if pub, ok := cert.PublicKey.(*rsa.PublicKey); !ok || pub.N.Cmp(rsaKey.N) != 0 {
		return nil, ErrBadCertificate
	}
	return &Certificate{Key: rsaKey, Cert: cert}, nil
}

// MarshalPEM serializes the certificate and its key for (encrypted) storage.
func (c *Certificate) MarshalPEM() (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(c.Key)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	_ = pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: c.Cert.Raw})
	_ = pem.Encode(&b, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return b.String(), nil
}

// ParsePEM reads the output of MarshalPEM.
func ParsePEM(s string) (*Certificate, error) {
	var out Certificate
	rest := []byte(s)
	for {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, ErrBadCertificate
			}
			out.Cert = cert
		case "PRIVATE KEY":
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, ErrBadCertificate
			}
			rsaKey, ok := key.(*rsa.PrivateKey)
			if !ok {
				return nil, ErrBadCertificate
			}
			out.Key = rsaKey
		}
	}
	if out.Key == nil || out.Cert == nil {
		return nil, ErrBadCertificate
	}
	return &out, nil
}

// attr renders one attribute; values are escaped as canonical XML requires.
func attr(name, value string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;", "\t", "&#x9;", "\n", "&#xA;", "\r", "&#xD;")
	return " " + name + `="` + r.Replace(value) + `"`
}

// body renders the SOAP Body in its exclusive canonical form (namespaces it
// uses declared on it, attributes sorted, no empty-element tags), so the
// bytes sent are exactly the bytes that are digested.
func body(m Message) string {
	var b strings.Builder
	b.WriteString(`<soapenv:Body xmlns:soapenv="` + nsSOAP + `" xmlns:wsu="` + nsWSU + `" wsu:Id="body">`)
	b.WriteString(`<tns:Trzba xmlns:tns="` + nsEET + `"><tns:Hlavicka`)
	b.WriteString(attr("dat_odesl", FormatTime(m.SentAt)))
	if m.Verify {
		b.WriteString(attr("overeni", "true"))
	}
	b.WriteString(attr("prvni_zaslani", fmt.Sprint(m.First)))
	b.WriteString(attr("uuid_zpravy", m.UUID))
	b.WriteString(`></tns:Hlavicka><tns:Data`)
	b.WriteString(attr("celk_trzba", FormatAmount(m.Sale.Amount)))
	b.WriteString(attr("dat_trzby", FormatTime(m.Sale.SoldAt)))
	b.WriteString(attr("eic_popl", m.Sale.EIC))
	b.WriteString(attr("id_jednotky", m.Sale.UnitID))
	b.WriteString(attr("id_pokl", m.Sale.RegisterID))
	b.WriteString(attr("porad_cis", m.Sale.SerialNo))
	b.WriteString(`></tns:Data></tns:Trzba></soapenv:Body>`)
	return b.String()
}

// signedInfo is the canonical ds:SignedInfo referring to the Body.
func signedInfo(digest string) string {
	return `<ds:SignedInfo xmlns:ds="` + nsDS + `">` +
		`<ds:CanonicalizationMethod Algorithm="` + algC14N + `"></ds:CanonicalizationMethod>` +
		`<ds:SignatureMethod Algorithm="` + algRSA + `"></ds:SignatureMethod>` +
		`<ds:Reference URI="#body"><ds:Transforms><ds:Transform Algorithm="` + algC14N + `"></ds:Transform></ds:Transforms>` +
		`<ds:DigestMethod Algorithm="` + algSHA256 + `"></ds:DigestMethod>` +
		`<ds:DigestValue>` + digest + `</ds:DigestValue></ds:Reference></ds:SignedInfo>`
}

// Envelope builds the signed SOAP message: only the Body is signed and the
// certificate travels as a BinarySecurityToken (no timestamp, no addressing —
// extra headers may get the message refused).
func Envelope(cert *Certificate, m Message) ([]byte, error) {
	if cert == nil || cert.Key == nil || cert.Cert == nil {
		return nil, errors.New("eet: missing certificate")
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	b := body(m)
	digest := sha256.Sum256([]byte(b))
	si := signedInfo(base64.StdEncoding.EncodeToString(digest[:]))
	hashed := sha256.Sum256([]byte(si))
	sig, err := rsa.SignPKCS1v15(rand.Reader, cert.Key, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, fmt.Errorf("eet: signing: %w", err)
	}
	out := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="` + nsSOAP + `"><soapenv:Header>` +
		`<wsse:Security xmlns:wsse="` + nsWSSE + `" xmlns:wsu="` + nsWSU + `">` +
		`<wsse:BinarySecurityToken EncodingType="` + typeB64 + `" ValueType="` + typeX509 + `" wsu:Id="cert">` +
		base64.StdEncoding.EncodeToString(cert.Cert.Raw) + `</wsse:BinarySecurityToken>` +
		`<ds:Signature xmlns:ds="` + nsDS + `">` + si +
		`<ds:SignatureValue>` + base64.StdEncoding.EncodeToString(sig) + `</ds:SignatureValue>` +
		`<ds:KeyInfo><wsse:SecurityTokenReference><wsse:Reference URI="#cert" ValueType="` + typeX509 + `"></wsse:Reference>` +
		`</wsse:SecurityTokenReference></ds:KeyInfo></ds:Signature></wsse:Security></soapenv:Header>` +
		b + `</soapenv:Envelope>`
	if len(out) > MaxMessageBytes {
		return nil, errors.New("eet: the message is too large")
	}
	return []byte(out), nil
}

// between returns the first substring of s from start up to and including end.
func between(s, start, end string) (string, bool) {
	i := strings.Index(s, start)
	if i < 0 {
		return "", false
	}
	j := strings.Index(s[i:], end)
	if j < 0 {
		return "", false
	}
	return s[i : i+j+len(end)], true
}

// VerifyEnvelope checks the signature of a message built by Envelope (the
// canonical form this package emits — not arbitrary XML) and returns the
// signing certificate. It serves the fake receiving system of the tests.
func VerifyEnvelope(raw []byte) (*x509.Certificate, error) {
	s := string(raw)
	b, ok1 := between(s, "<soapenv:Body ", "</soapenv:Body>")
	si, ok2 := between(s, "<ds:SignedInfo ", "</ds:SignedInfo>")
	token, ok3 := between(s, "<wsse:BinarySecurityToken ", "</wsse:BinarySecurityToken>")
	sigEl, ok4 := between(s, "<ds:SignatureValue>", "</ds:SignatureValue>")
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil, errors.New("eet: incomplete message")
	}
	digest := sha256.Sum256([]byte(b))
	if !strings.Contains(si, "<ds:DigestValue>"+base64.StdEncoding.EncodeToString(digest[:])+"</ds:DigestValue>") {
		return nil, errors.New("eet: body digest mismatch")
	}
	inner := func(el string) string {
		return el[strings.Index(el, ">")+1 : strings.LastIndex(el, "<")]
	}
	der, err := base64.StdEncoding.DecodeString(inner(token))
	if err != nil {
		return nil, errors.New("eet: invalid certificate token")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.New("eet: invalid certificate token")
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("eet: not an RSA certificate")
	}
	sig, err := base64.StdEncoding.DecodeString(inner(sigEl))
	if err != nil {
		return nil, errors.New("eet: invalid signature value")
	}
	hashed := sha256.Sum256([]byte(si))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sig); err != nil {
		return nil, errors.New("eet: invalid signature")
	}
	return cert, nil
}
