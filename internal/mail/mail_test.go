package mail

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http/httptest"
	netmail "net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRender(t *testing.T) {
	got := Render("Faktura {number} ({total}), {unknown} {}", map[string]string{"number": "2026-0001", "total": "1 210 Kč"})
	if got != "Faktura 2026-0001 (1 210 Kč), {unknown} {}" {
		t.Fatalf("Render = %q", got)
	}
}

func TestLogMailer(t *testing.T) {
	var b bytes.Buffer
	l := NewLogMailer(&b, "NanoFaktura <nf@example.cz>")
	err := l.Send(context.Background(), Message{To: []string{"a@example.cz"}, Subject: "Ahoj", Text: "tělo",
		Attachments: []Attachment{{Filename: "f.pdf", ContentType: "application/pdf", Data: []byte("x")}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"From: NanoFaktura <nf@example.cz>", "To: a@example.cz", "Subject: Ahoj", "tělo", "[attachment f.pdf, application/pdf, 1 bytes]"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("log output lacks %q:\n%s", want, b.String())
		}
	}
	if err := l.Send(context.Background(), Message{Subject: "x"}); !errors.Is(err, ErrNoRecipients) {
		t.Fatalf("no recipients: %v", err)
	}
	if err := l.Send(context.Background(), Message{To: []string{"not an address"}}); err == nil {
		t.Fatal("expected invalid recipient error")
	}
}

func TestBuildMessage(t *testing.T) {
	m := Message{
		From: "Firma s.r.o. <f@example.cz>", ReplyTo: "reply@example.cz",
		To: []string{"Jan Novák <jan@example.cz>"}, Cc: []string{"cc@example.cz"},
		Subject: "Faktura č. 2026-0001", Text: "Dobrý den,\nposíláme fakturu.", HTML: "<p>Dobrý den</p>",
		Attachments: []Attachment{{Filename: "faktura 2026-0001.pdf", ContentType: "application/pdf", Data: bytes.Repeat([]byte("%PDF"), 100)}},
	}
	raw, err := buildMessage(m, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	subj, _ := dec.DecodeHeader(msg.Header.Get("Subject"))
	to, _ := msg.Header.AddressList("To")
	if subj != m.Subject || len(to) != 1 || to[0].Name != "Jan Novák" || msg.Header.Get("Cc") != "<cc@example.cz>" ||
		msg.Header.Get("Reply-To") != "<reply@example.cz>" || !strings.HasSuffix(msg.Header.Get("Message-ID"), "@example.cz>") {
		t.Fatalf("headers: %v (subject %q)", msg.Header, subj)
	}
	mt, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if mt != "multipart/mixed" {
		t.Fatalf("content type %q", mt)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	alt, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	amt, aparams, _ := mime.ParseMediaType(alt.Header.Get("Content-Type"))
	if amt != "multipart/alternative" {
		t.Fatalf("first part %q", amt)
	}
	ar := multipart.NewReader(alt, aparams["boundary"])
	var bodies []string
	for {
		p, err := ar.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p) // multipart.Reader decodes quoted-printable
		bodies = append(bodies, p.Header.Get("Content-Type")+"|"+string(b))
	}
	if len(bodies) != 2 || bodies[0] != "text/plain; charset=utf-8|Dobrý den,\r\nposíláme fakturu." || bodies[1] != "text/html; charset=utf-8|<p>Dobrý den</p>" {
		t.Fatalf("bodies: %q", bodies)
	}
	att, err := mr.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, att))
	if att.FileName() != "faktura 2026-0001.pdf" || att.Header.Get("Content-Type") != "application/pdf" || !bytes.Equal(data, m.Attachments[0].Data) {
		t.Fatalf("attachment %q %v %d bytes", att.FileName(), att.Header, len(data))
	}

	// text only → single part
	raw, _ = buildMessage(Message{From: "f@example.cz", To: []string{"a@example.cz"}, Text: "x"}, time.Now())
	msg, _ = netmail.ReadMessage(bytes.NewReader(raw))
	if msg.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("text-only content type: %q", msg.Header.Get("Content-Type"))
	}
}

// fakeSMTP is a minimal SMTP server for tests.
type fakeSMTP struct {
	t        *testing.T
	ln       net.Listener
	tls      *tls.Config // used for STARTTLS (and implicit TLS when the listener is TLS)
	startTLS bool

	mu   sync.Mutex
	from string
	rcpt []string
	data string
	auth string
}

func newFakeSMTP(t *testing.T, implicitTLS, startTLS bool) (*fakeSMTP, *x509.CertPool) {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	ts.StartTLS() // only to obtain a certificate for 127.0.0.1
	t.Cleanup(ts.Close)
	cfg := &tls.Config{Certificates: ts.TLS.Certificates}
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())

	var ln net.Listener
	var err error
	if implicitTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", cfg)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	f := &fakeSMTP{t: t, ln: ln, tls: cfg, startTLS: startTLS}
	go f.serve()
	return f, pool
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(c)
	}
}

func (f *fakeSMTP) handle(c net.Conn) {
	defer c.Close()
	_, isTLS := c.(*tls.Conn)
	r := bufio.NewReader(c)
	w := func(s string) { _, _ = io.WriteString(c, s+"\r\n") }
	w("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if f.startTLS && !isTLS {
				w("250-fake")
				w("250 STARTTLS")
			} else {
				w("250-fake")
				w("250 AUTH PLAIN")
			}
		case cmd == "STARTTLS":
			w("220 go ahead")
			tc := tls.Server(c, f.tls)
			if err := tc.Handshake(); err != nil {
				return
			}
			c, isTLS, r = tc, true, bufio.NewReader(tc)
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			f.mu.Lock()
			f.auth = strings.TrimSpace(line[len("AUTH PLAIN"):])
			f.mu.Unlock()
			w("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			f.mu.Lock()
			f.from = strings.Trim(line[len("MAIL FROM:"):], "<> ")
			f.mu.Unlock()
			w("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			f.mu.Lock()
			f.rcpt = append(f.rcpt, strings.Trim(line[len("RCPT TO:"):], "<> "))
			f.mu.Unlock()
			w("250 ok")
		case cmd == "DATA":
			w("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			w("250 queued")
		case cmd == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func TestSMTPSend(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     string
		implicit bool
		startTLS bool
		user     string
	}{
		{"none", TLSNone, false, false, ""},
		{"starttls+auth", TLSStartTLS, false, true, "user"},
		{"implicit tls+auth", TLSImplicit, true, false, "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, pool := newFakeSMTP(t, tc.implicit, tc.startTLS)
			s := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: f.port(), TLS: tc.mode, Username: tc.user, Password: "secret",
				From: "NanoFaktura <nf@example.cz>", TLSConfig: &tls.Config{RootCAs: pool}})
			err := s.Send(context.Background(), Message{To: []string{"Jan <jan@example.cz>"}, Cc: []string{"cc@example.cz"}, Subject: "Test", Text: "Ahoj"})
			if err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.from != "nf@example.cz" || strings.Join(f.rcpt, ",") != "jan@example.cz,cc@example.cz" ||
				!strings.Contains(f.data, "Subject: Test") || !strings.Contains(f.data, "Ahoj") {
				t.Fatalf("got from=%q rcpt=%v data=%q", f.from, f.rcpt, f.data)
			}
			if tc.user != "" {
				dec, _ := base64.StdEncoding.DecodeString(f.auth)
				if string(dec) != "\x00user\x00secret" {
					t.Fatalf("auth %q", dec)
				}
			}
		})
	}
}

func TestSMTPStartTLSRequired(t *testing.T) {
	f, _ := newFakeSMTP(t, false, false) // server without STARTTLS
	s := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: f.port(), From: "nf@example.cz"})
	err := s.Send(context.Background(), Message{To: []string{"a@example.cz"}, Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err = %v", err)
	}
}
