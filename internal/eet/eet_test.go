package eet_test

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/eet"
	"github.com/qwerin/nanofaktura/internal/eet/eettest"
)

func TestFormatAmount(t *testing.T) {
	for minor, want := range map[int64]string{25000: "250.00", -18720: "-187.20", 56: "0.56", 0: "0.00", -1: "-0.01", 5: "0.05", 9999999999: "99999999.99"} {
		if got := eet.FormatAmount(minor); got != want {
			t.Errorf("FormatAmount(%d) = %q, want %q", minor, got, want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	winter := time.Date(2027, 1, 9, 3, 25, 28, 0, time.UTC)
	summer := time.Date(2027, 6, 9, 3, 25, 28, 0, time.UTC)
	if got := eet.FormatTime(winter); got != "2027-01-09T04:25:28+01:00" {
		t.Errorf("winter: %s", got)
	}
	if got := eet.FormatTime(summer); got != "2027-06-09T05:25:28+02:00" {
		t.Errorf("summer: %s", got)
	}
}

func message() eet.Message {
	return eet.Message{UUID: "b3a09b52-7c87-4014-a496-4c7a53cf9125", SentAt: time.Date(2027, 1, 9, 12, 0, 0, 0, time.UTC), First: true,
		Sale: eet.Sale{EIC: "CZ00000019", UnitID: "11", RegisterID: "1", SerialNo: "42", SoldAt: time.Date(2027, 1, 9, 11, 59, 0, 0, time.UTC), Amount: 123450}}
}

func testCert(t *testing.T) *eet.Certificate {
	t.Helper()
	cert, err := eet.ParsePKCS12(eettest.PKCS12(t, "CZ00000019", "heslo", time.Now().AddDate(1, 0, 0)), "heslo")
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestCertificate(t *testing.T) {
	p12 := eettest.PKCS12(t, "CZ00000019", "heslo", time.Now().AddDate(1, 0, 0))
	if _, err := eet.ParsePKCS12(p12, "spatne"); err != eet.ErrBadCertificate {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := eet.ParsePKCS12([]byte("not a p12"), ""); err != eet.ErrBadCertificate {
		t.Fatalf("garbage: %v", err)
	}
	cert, err := eet.ParsePKCS12(p12, "heslo")
	if err != nil || cert.EIC() != "CZ00000019" {
		t.Fatalf("parse: %v", err)
	}
	pemText, err := cert.MarshalPEM()
	if err != nil {
		t.Fatal(err)
	}
	back, err := eet.ParsePEM(pemText)
	if err != nil || back.EIC() != "CZ00000019" || back.Key.N.Cmp(cert.Key.N) != 0 {
		t.Fatalf("pem roundtrip: %v", err)
	}
	if _, err := eet.ParsePEM("x"); err == nil {
		t.Fatal("garbage PEM accepted")
	}
}

func TestEnvelope(t *testing.T) {
	cert := testCert(t)
	raw, err := eet.Envelope(cert, message())
	if err != nil {
		t.Fatal(err)
	}
	got, err := eet.VerifyEnvelope(raw)
	if err != nil || got.Subject.CommonName != "CZ00000019" {
		t.Fatalf("verify: %v", err)
	}
	for _, want := range []string{`celk_trzba="1234.50"`, `dat_trzby="2027-01-09T12:59:00+01:00"`, `prvni_zaslani="true"`, `porad_cis="42"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "overeni") {
		t.Error("overeni must be absent outside the verification mode")
	}
	tampered := strings.Replace(string(raw), `celk_trzba="1234.50"`, `celk_trzba="1.00"`, 1)
	if _, err := eet.VerifyEnvelope([]byte(tampered)); err == nil {
		t.Fatal("tampered body verified")
	}

	bad := message()
	bad.Sale.EIC = "12345678"
	if _, err := eet.Envelope(cert, bad); err == nil {
		t.Fatal("invalid EIC accepted")
	}
	bad = message()
	bad.Sale.Amount = eet.MaxAmount + 1
	if _, err := eet.Envelope(cert, bad); err == nil {
		t.Fatal("too large amount accepted")
	}
}

// The official sample message (eet.gov.cz, playground certificate) pins down
// the canonical form: the digest and the signature must come out of the same
// rules Envelope uses (namespaces on the apex element sorted by prefix,
// attributes sorted, empty elements as start + end tag).
func TestOfficialSampleCanonicalForm(t *testing.T) {
	raw, err := os.ReadFile("testdata/sample.v4.req.xml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	find := func(re string) string {
		m := regexp.MustCompile(re).FindStringSubmatch(s)
		if m == nil {
			t.Fatalf("sample: %s not found", re)
		}
		return m[1]
	}
	expand := regexp.MustCompile(`<([a-zA-Z0-9:]+)((?: [^<>]*?)?)/>`)
	const soap, v4, wsu, ds = `xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"`, `xmlns:v4="http://fs.gov.cz/eet/schema/v4"`,
		`xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"`, `xmlns:ds="http://www.w3.org/2000/09/xmldsig#"`

	body := `<soapenv:Body ` + soap + ` ` + v4 + ` ` + wsu + ` wsu:Id="` + find(`<soapenv:Body wsu:Id="([^"]+)"`) + `">` +
		expand.ReplaceAllString(find(`<soapenv:Body[^>]*>(.*)</soapenv:Body>`), `<$1$2></$1>`) + `</soapenv:Body>`
	digest := sha256.Sum256([]byte(body))
	if got, want := base64.StdEncoding.EncodeToString(digest[:]), find(`<ds:DigestValue>([^<]+)<`); got != want {
		t.Fatalf("body digest %s, sample has %s", got, want)
	}

	si := expand.ReplaceAllString(find(`<ds:SignedInfo>(.*)</ds:SignedInfo>`), `<$1$2></$1>`)
	si = regexp.MustCompile(`<ec:InclusiveNamespaces (PrefixList="[^"]*") (xmlns:ec="[^"]*")>`).ReplaceAllString(si, `<ec:InclusiveNamespaces $2 $1>`)
	si = `<ds:SignedInfo ` + ds + ` ` + soap + ` ` + v4 + `>` + si + `</ds:SignedInfo>`
	der, _ := base64.StdEncoding.DecodeString(find(`<wsse:BinarySecurityToken[^>]*>([^<]+)<`))
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := base64.StdEncoding.DecodeString(find(`<ds:SignatureValue>([^<]+)<`))
	hashed := sha256.Sum256([]byte(si))
	if err := rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA256, hashed[:], sig); err != nil {
		t.Fatalf("sample signature: %v", err)
	}
	if cert.Subject.CommonName != "CZ00000019" {
		t.Fatalf("sample certificate CN %q", cert.Subject.CommonName)
	}
}

func TestClient(t *testing.T) {
	srv := eettest.Start(t)
	c, cert, ctx := srv.Client(), testCert(t), context.Background()

	res, err := c.Send(ctx, eet.EnvPlayground, cert, message())
	if err != nil || !res.Confirmed() || !res.Test || !strings.HasSuffix(res.POK, "-ff") || res.TransactionID == "" {
		t.Fatalf("playground: %+v %v", res, err)
	}
	res, err = c.Send(ctx, eet.EnvProduction, cert, message())
	if err != nil || !res.Confirmed() || res.Test || strings.HasSuffix(res.POK, "-ff") {
		t.Fatalf("production: %+v %v", res, err)
	}
	got := srv.Requests()
	if len(got) != 2 || got[1].Amount != "1234.50" || !got[1].Production || !got[1].First || got[1].CertEIC != "CZ00000019" {
		t.Fatalf("requests: %+v", got)
	}

	verify := message()
	verify.Verify = true
	res, err = c.Send(ctx, eet.EnvProduction, cert, verify)
	if err != nil || !res.Verified || res.Confirmed() || res.Rejected() {
		t.Fatalf("verify: %+v %v", res, err)
	}

	srv.SetWarning(true)
	res, _ = c.Send(ctx, eet.EnvProduction, cert, message())
	if len(res.Warnings) != 1 || res.Warnings[0].Code != 1 {
		t.Fatalf("warnings: %+v", res)
	}

	srv.SetMode(eettest.ModeReject)
	res, err = c.Send(ctx, eet.EnvProduction, cert, message())
	if err != nil || !res.Rejected() || res.ErrCode != 4 {
		t.Fatalf("reject: %+v %v", res, err)
	}
	srv.SetMode(eettest.ModeTemporary)
	res, err = c.Send(ctx, eet.EnvProduction, cert, message())
	if err != nil || res.Rejected() || res.Confirmed() || res.ErrCode != -1 {
		t.Fatalf("temporary: %+v %v", res, err)
	}
	srv.SetMode(eettest.ModeDown)
	if _, err = c.Send(ctx, eet.EnvProduction, cert, message()); err == nil {
		t.Fatal("down: expected an error")
	}
	if _, err = c.Send(ctx, "nowhere", cert, message()); err == nil {
		t.Fatal("unknown environment accepted")
	}
}

// TestPlayground sends a real message to the tax authority's developer
// playground. Opt-in: EET_PLAYGROUND_P12=<file> EET_PLAYGROUND_PASSWORD=<pw>
// (shared test certificates from eet.gov.cz → Pro vývojáře).
func TestPlayground(t *testing.T) {
	path := os.Getenv("EET_PLAYGROUND_P12")
	if path == "" {
		t.Skip("EET_PLAYGROUND_P12 not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := eet.ParsePKCS12(data, os.Getenv("EET_PLAYGROUND_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m := eet.Message{UUID: newUUID(t), SentAt: now, First: true,
		Sale: eet.Sale{EIC: cert.EIC(), UnitID: "11", RegisterID: "nanofaktura-test", SerialNo: now.Format("20060102150405"), SoldAt: now, Amount: 12300}}
	res, err := eet.New().Send(context.Background(), eet.EnvPlayground, cert, m)
	t.Logf("sale: %+v %v", res, err)
	if err != nil || !res.Confirmed() || !res.Test {
		t.Fatalf("playground did not confirm the sale")
	}
	m.UUID, m.Verify = newUUID(t), true
	res, err = eet.New().Send(context.Background(), eet.EnvPlayground, cert, m)
	t.Logf("verify: %+v %v", res, err)
	if err != nil || !res.Verified {
		t.Fatalf("playground did not verify the message")
	}
	m.UUID, m.Verify, m.Sale.Amount, m.Sale.SerialNo = newUUID(t), false, -12300, m.Sale.SerialNo+"-S"
	res, err = eet.New().Send(context.Background(), eet.EnvPlayground, cert, m)
	t.Logf("storno: %+v %v", res, err)
	if err != nil || !res.Confirmed() {
		t.Fatalf("playground did not confirm the cancellation")
	}
}
