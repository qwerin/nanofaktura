package mail

import (
	"context"
	"crypto/tls"
	"net"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/qwerin/nanofaktura/internal/maildiag/smtptest"
)

func sendVia(t *testing.T, srv *smtptest.Server, cfg SMTPConfig) error {
	t.Helper()
	cfg.Host, cfg.Port = "127.0.0.1", srv.Addr.Port
	cfg.TLSConfig = &tls.Config{RootCAs: srv.Pool}
	if cfg.From == "" {
		cfg.From = "nf@example.cz"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return NewSMTP(cfg).Send(ctx, Message{To: []string{"a@example.cz"}, Subject: "S", Text: "x"})
}

func TestSMTPAuthLogin(t *testing.T) {
	srv := smtptest.Start(t, smtptest.Options{StartTLS: true, User: "user", Pass: "pw", AuthMechs: "LOGIN"})
	if err := sendVia(t, srv, SMTPConfig{TLS: TLSStartTLS, Username: "user", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	msgs := srv.Messages()
	if len(msgs) != 1 || msgs[0].User != "user" || msgs[0].To[0] != "a@example.cz" {
		t.Fatalf("messages %+v", msgs)
	}
}

func TestSMTPSendErrors(t *testing.T) {
	cases := []struct {
		name string
		opts smtptest.Options
		cfg  SMTPConfig
		want string
	}{
		{"wrong password", smtptest.Options{StartTLS: true, User: "user", Pass: "pw"}, SMTPConfig{TLS: TLSStartTLS, Username: "user", Password: "bad"}, "smtp auth"},
		{"relay denied", smtptest.Options{DenyRelay: true}, SMTPConfig{TLS: TLSNone}, "smtp RCPT TO a@example.cz"},
		{"implicit TLS against plain server", smtptest.Options{}, SMTPConfig{TLS: TLSImplicit, Timeout: time.Second}, "smtp tls"},
		{"no recipients", smtptest.Options{}, SMTPConfig{TLS: TLSNone}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := smtptest.Start(t, tc.opts)
			if tc.want == "" {
				// invalid message: rejected before dialing
				cfg := tc.cfg
				cfg.Host, cfg.Port, cfg.From = "127.0.0.1", srv.Addr.Port, "nf@example.cz"
				if err := NewSMTP(cfg).Send(context.Background(), Message{Text: "x"}); err == nil {
					t.Fatal("message without recipients accepted")
				}
				return
			}
			err := sendVia(t, srv, tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "bad") && tc.cfg.Password == "bad" {
				t.Fatalf("password leaked into the error: %v", err)
			}
		})
	}
}

func TestSMTPDialError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	s := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: port, TLS: TLSNone, From: "nf@example.cz", Timeout: time.Second})
	if err := s.Send(context.Background(), Message{To: []string{"a@example.cz"}, Text: "x"}); err == nil || !strings.Contains(err.Error(), "smtp dial") {
		t.Fatalf("err = %v", err)
	}
	if c := s.Config(); c.Port != port || c.TLS != TLSNone || c.Timeout != time.Second {
		t.Fatalf("config %+v", c)
	}
	if c := NewSMTP(SMTPConfig{}).Config(); c.TLS != TLSStartTLS || c.Timeout != 30*time.Second {
		t.Fatalf("defaults %+v", c)
	}
}

func TestLoginAuth(t *testing.T) {
	a := &loginAuth{user: "u", pass: "p", host: "mail.example.cz"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "mail.example.cz", TLS: false}); err == nil {
		t.Fatal("credentials over plaintext to a remote host")
	}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "other.example.cz", TLS: true}); err == nil {
		t.Fatal("wrong host accepted")
	}
	if mech, resp, err := a.Start(&smtp.ServerInfo{Name: "mail.example.cz", TLS: true}); mech != "LOGIN" || resp != nil || err != nil {
		t.Fatalf("start: %q %v %v", mech, resp, err)
	}
	local := &loginAuth{user: "u", pass: "p", host: "localhost"}
	if _, _, err := local.Start(&smtp.ServerInfo{Name: "localhost"}); err != nil {
		t.Fatalf("localhost plaintext: %v", err)
	}
	for challenge, want := range map[string]string{"Username:": "u", "User Name": "u", "username": "u", "Password:": "p", " password ": "p", "": "u"} {
		got, err := a.Next([]byte(challenge), true)
		if err != nil || string(got) != want {
			t.Errorf("Next(%q) = %q, %v; want %q", challenge, got, err, want)
		}
	}
	if _, err := a.Next([]byte("Something?"), true); err == nil {
		t.Error("unexpected challenge accepted")
	}
	if got, err := a.Next([]byte("whatever"), false); got != nil || err != nil {
		t.Errorf("done: %q %v", got, err)
	}
	for name, want := range map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true, "mail.example.cz": false} {
		if isLocalhost(name) != want {
			t.Errorf("isLocalhost(%s)", name)
		}
	}
	for mechs, want := range map[string]string{"": "PLAIN", "PLAIN LOGIN": "PLAIN", "login xoauth2": "LOGIN", "CRAM-MD5": "PLAIN"} {
		if got := AuthMechanism(mechs); got != want {
			t.Errorf("AuthMechanism(%q) = %s", mechs, got)
		}
	}
	if _, ok := chooseAuth("LOGIN", "u", "p", "h").(*loginAuth); !ok {
		t.Error("chooseAuth LOGIN")
	}
	if _, ok := chooseAuth("PLAIN LOGIN", "u", "p", "h").(*loginAuth); ok {
		t.Error("chooseAuth PLAIN")
	}
}
