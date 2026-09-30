package maildiag_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-msgauth/dkim"

	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/maildiag"
	"github.com/qwerin/nanofaktura/internal/maildiag/smtptest"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// baseDNS is a well configured example.cz.
func baseDNS() *smtptest.Resolver {
	return &smtptest.Resolver{
		Hosts: map[string][]string{"mail.example.cz": {"203.0.113.10"}},
		TXT: map[string][]string{
			"example.cz":        {"v=spf1 ip4:203.0.113.0/24 -all", "google-site-verification=x"},
			"_dmarc.example.cz": {"v=DMARC1; p=quarantine; rua=mailto:dmarc@example.cz"},
		},
		MX: map[string][]*net.MX{"example.cz": {{Host: "mx2.example.cz.", Pref: 20}, {Host: "mx1.example.cz.", Pref: 10}}},
	}
}

func run(t *testing.T, srv *smtptest.Server, mode, user, pass string, tlsCfg *tls.Config, dk *mail.DKIMConfig) *maildiag.DiagReport {
	t.Helper()
	cfg := mail.SMTPConfig{Host: "127.0.0.1", Port: srv.Addr.Port, TLS: mode, Username: user, Password: pass,
		From: "NanoFaktura <faktury@example.cz>", TLSConfig: tlsCfg, DKIM: dk}
	return maildiag.Run(context.Background(), maildiag.Options{
		SMTP: cfg, To: "prijemce@example.com", Message: maildiag.TestMessage("https://f.example.cz", cfg, "prijemce@example.com", now),
		Resolver: baseDNS(), Now: func() time.Time { return now }, Timeout: 10 * time.Second,
	})
}

func step(t *testing.T, r *maildiag.DiagReport, id string) maildiag.DiagCheck {
	t.Helper()
	for _, c := range append(r.SMTP, r.DNS...) {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no step %s in %+v", id, r.SMTP)
	return maildiag.DiagCheck{}
}

func noSecret(t *testing.T, r *maildiag.DiagReport, secret string) {
	t.Helper()
	for _, c := range append(r.SMTP, r.DNS...) {
		all := c.Message + c.Hint + strings.Join(c.Details, "\n")
		if strings.Contains(all, secret) {
			t.Fatalf("report leaks the password in %s: %s", c.ID, all)
		}
	}
}

func TestDiagnosePlain(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{})
	r := run(t, srv, mail.TLSNone, "", "", nil, nil)
	if !r.Sent || r.QueueID != "4ABCDEF123" || !strings.Contains(r.ServerReply, "queued as") {
		t.Fatalf("not sent: %+v", r)
	}
	for _, id := range []string{"resolve", "connect", "banner", "ehlo", "mail_from", "rcpt_to", "data"} {
		if c := step(t, r, id); c.Status != maildiag.OK {
			t.Fatalf("%s: %+v", id, c)
		}
	}
	if c := step(t, r, "starttls"); c.Status != maildiag.Warning {
		t.Fatalf("unencrypted not warned: %+v", c)
	}
	if c := step(t, r, "auth"); c.Status != maildiag.Info {
		t.Fatalf("auth: %+v", c)
	}
	if c := step(t, r, "ehlo"); !strings.Contains(c.Message, "SIZE") || !strings.Contains(strings.Join(c.Details, "\n"), "C: EHLO") {
		t.Fatalf("ehlo: %+v", c)
	}
	msgs := srv.Messages()
	if len(msgs) != 1 || msgs[0].From != "faktury@example.cz" || !strings.Contains(msgs[0].Data, "Subject: =?utf-8?q?Testovac") ||
		!strings.Contains(msgs[0].Data, "Zobrazit origin") {
		t.Fatalf("message: %+v", msgs)
	}
	// DNS checks ran for the sender domain; local SMTP address → SPF coverage "cannot verify"
	if c := step(t, r, "mx"); c.Status != maildiag.OK || c.Details[0] != "10 mx1.example.cz" {
		t.Fatalf("mx: %+v", c)
	}
	if c := step(t, r, "spf"); c.Status != maildiag.Info || !strings.Contains(c.Message, "nelze ověřit") {
		t.Fatalf("spf: %+v", c)
	}
}

func TestDiagnoseStartTLSAndAuth(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{StartTLS: true, User: "u@example.cz", Pass: "tajne-heslo-123", AuthMechs: "LOGIN"})
	r := run(t, srv, mail.TLSStartTLS, "u@example.cz", "tajne-heslo-123", &tls.Config{RootCAs: srv.Pool}, nil)
	if !r.Sent {
		t.Fatalf("not sent: %+v", r.SMTP)
	}
	c := step(t, r, "starttls")
	if c.Status != maildiag.OK || !strings.Contains(strings.Join(c.Details, "\n"), "Verze: TLS 1.3") || !strings.Contains(strings.Join(c.Details, "\n"), "Platný do:") {
		t.Fatalf("starttls: %+v", c)
	}
	if a := step(t, r, "auth"); a.Status != maildiag.OK || !strings.Contains(a.Message, "LOGIN") {
		t.Fatalf("auth: %+v", a)
	}
	noSecret(t, r, "tajne-heslo-123")
	noSecret(t, r, "dGFqbmUtaGVzbG8tMTIz") // base64 of the password
	if srv.Messages()[0].User != "u@example.cz" {
		t.Fatal("not authenticated")
	}
}

func TestDiagnoseImplicitTLS(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{ImplicitTLS: true, User: "u", Pass: "p4ssword!"})
	r := run(t, srv, mail.TLSImplicit, "u", "p4ssword!", &tls.Config{RootCAs: srv.Pool}, nil)
	if !r.Sent || step(t, r, "tls").Status != maildiag.OK || !strings.Contains(step(t, r, "auth").Message, "PLAIN") {
		t.Fatalf("implicit: %+v", r.SMTP)
	}
	noSecret(t, r, "p4ssword!")
}

func TestDiagnoseSelfSignedCertificate(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{StartTLS: true})
	r := run(t, srv, mail.TLSStartTLS, "", "", nil, nil) // system roots: the test certificate is not trusted
	c := step(t, r, "starttls")
	if r.Sent || c.Status != maildiag.Error || !strings.Contains(c.Message, "nedůvěryhodná") ||
		!strings.Contains(strings.Join(c.Details, "\n"), "Vydal:") {
		t.Fatalf("self-signed: %+v", c)
	}
	if r.Status != maildiag.Error {
		t.Fatalf("overall %s", r.Status)
	}
}

func TestDiagnoseAuthFailure(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{StartTLS: true, User: "u", Pass: "right"})
	r := run(t, srv, mail.TLSStartTLS, "u", "wrong-secret-pw", &tls.Config{RootCAs: srv.Pool}, nil)
	c := step(t, r, "auth")
	if r.Sent || c.Status != maildiag.Error || !strings.Contains(c.Message, "jméno nebo heslo") {
		t.Fatalf("auth fail: %+v", c)
	}
	noSecret(t, r, "wrong-secret-pw")
	if len(srv.Messages()) != 0 {
		t.Fatal("message sent after failed auth")
	}
}

func TestDiagnoseRelayDenied(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{DenyRelay: true})
	r := run(t, srv, mail.TLSNone, "", "", nil, nil)
	c := step(t, r, "rcpt_to")
	if r.Sent || c.Status != maildiag.Error || !strings.Contains(c.Message, "relay denied") || !strings.Contains(strings.Join(c.Details, "\n"), "554") {
		t.Fatalf("relay: %+v", c)
	}
}

func TestDiagnoseConnectionErrors(t *testing.T) {
	cfg := mail.SMTPConfig{Host: "smtp.nowhere.cz", Port: 587, From: "a@example.cz"}
	r := maildiag.Run(context.Background(), maildiag.Options{SMTP: cfg, To: "b@example.com", Resolver: baseDNS()})
	if c := step(t, r, "resolve"); c.Status != maildiag.Error || !strings.Contains(c.Message, "neexistuje") {
		t.Fatalf("resolve: %+v", c)
	}
	cfg.Host = "mail.example.cz"
	r = maildiag.Run(context.Background(), maildiag.Options{SMTP: cfg, To: "b@example.com", Resolver: baseDNS(),
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("connect: connection refused")
		}})
	if c := step(t, r, "connect"); c.Status != maildiag.Error || !strings.Contains(c.Message, "odmítl") {
		t.Fatalf("connect: %+v", c)
	}
}

func TestDiagnoseNotConfigured(t *testing.T) {
	r := maildiag.Run(context.Background(), maildiag.Options{SMTP: mail.SMTPConfig{From: "a@example.cz"}, To: "b@example.com", Resolver: baseDNS()})
	c := step(t, r, "config")
	if r.Sent || r.Status != maildiag.Error || !strings.Contains(c.Message, "SMTP není nastavené") || !strings.Contains(c.Hint, "NANOFAKTURA_SMTP_HOST") {
		t.Fatalf("not configured: %+v", r)
	}
	if len(r.DNS) != 4 {
		t.Fatalf("DNS checks should still run: %+v", r.DNS)
	}
}

func TestDiagnoseDKIMSignedMessage(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	dk := &mail.DKIMConfig{Domain: "example.cz", Selector: "nf", Key: key}
	srv := smtptest.Start(t, smtptest.Options{})
	r := run(t, srv, mail.TLSNone, "", "", nil, dk)
	if !r.Sent || !r.DKIMSigned {
		t.Fatalf("%+v", r)
	}
	if c := step(t, r, "dkim"); c.Status != maildiag.Error || !strings.Contains(c.Hint, mail.DKIMRecord(key)) {
		t.Fatalf("missing DKIM record not reported with the record to publish: %+v", c)
	}
	data := srv.Messages()[0].Data
	verifs, err := dkim.VerifyWithOptions(bytes.NewReader([]byte(data)), &dkim.VerifyOptions{
		LookupTXT: func(string) ([]string, error) { return []string{mail.DKIMRecord(key)}, nil }})
	if err != nil || len(verifs) != 1 || verifs[0].Err != nil {
		t.Fatalf("delivered message does not verify: %v %+v", err, verifs)
	}
}
