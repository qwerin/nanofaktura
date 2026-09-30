// Package smtptest is a small in-process SMTP server for tests of the
// e-mail diagnostics (STARTTLS / implicit TLS with a self-signed
// certificate for 127.0.0.1, AUTH PLAIN/LOGIN, relay refusal).
package smtptest

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Options configure a Server.
type Options struct {
	ImplicitTLS bool   // TLS from the first byte (port 465 style)
	StartTLS    bool   // offer STARTTLS on the plain connection
	User, Pass  string // required credentials; empty = AUTH not offered
	AuthMechs   string // advertised mechanisms, default "PLAIN LOGIN"
	DenyRelay   bool   // answer RCPT TO with 554 relay access denied
	QueueID     string // id in the final DATA reply, default "4ABCDEF123"
}

// Server is a running fake SMTP server.
type Server struct {
	Pool *x509.CertPool // trusts the server's self-signed certificate
	Addr *net.TCPAddr

	o   Options
	ln  net.Listener
	tls *tls.Config

	mu       sync.Mutex
	messages []Message
}

// Message is one accepted message.
type Message struct {
	From string
	To   []string
	Data string
	User string // authenticated user ("" = none)
}

// Start starts a server on 127.0.0.1 (closed with the test).
func Start(t *testing.T, o Options) *Server {
	t.Helper()
	hs := httptest.NewUnstartedServer(nil)
	hs.StartTLS() // only to obtain a certificate for 127.0.0.1
	t.Cleanup(hs.Close)
	cfg := &tls.Config{Certificates: hs.TLS.Certificates}
	pool := x509.NewCertPool()
	pool.AddCert(hs.Certificate())
	if o.AuthMechs == "" {
		o.AuthMechs = "PLAIN LOGIN"
	}
	if o.QueueID == "" {
		o.QueueID = "4ABCDEF123"
	}
	var ln net.Listener
	var err error
	if o.ImplicitTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", cfg)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	s := &Server{Pool: pool, Addr: ln.Addr().(*net.TCPAddr), o: o, ln: ln, tls: cfg}
	go s.serve()
	return s
}

// Messages returns the accepted messages.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

func (s *Server) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer func() { _ = c.Close() }()
	_, isTLS := c.(*tls.Conn)
	r := bufio.NewReader(c)
	w := func(line string) { _, _ = io.WriteString(c, line+"\r\n") }
	readLine := func() (string, bool) {
		l, err := r.ReadString('\n')
		return strings.TrimRight(l, "\r\n"), err == nil
	}
	var cur Message
	var user string
	w("220 fake.example ESMTP ready")
	for {
		line, ok := readLine()
		if !ok {
			return
		}
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			w("250-fake.example hello")
			w("250-SIZE 10240000")
			if s.o.StartTLS && !isTLS {
				w("250-STARTTLS")
			}
			if s.o.User != "" && (isTLS || !s.o.StartTLS) {
				w("250-AUTH " + s.o.AuthMechs)
			}
			w("250 8BITMIME")
		case cmd == "STARTTLS" && s.o.StartTLS && !isTLS:
			w("220 2.0.0 ready to start TLS")
			tc := tls.Server(c, s.tls)
			if err := tc.Handshake(); err != nil {
				return
			}
			c, isTLS, r = tc, true, bufio.NewReader(tc)
		case strings.HasPrefix(cmd, "AUTH PLAIN "):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN "):]))
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 && parts[1] == s.o.User && parts[2] == s.o.Pass {
				user = parts[1]
				w("235 2.7.0 authentication successful")
			} else {
				w("535 5.7.8 authentication credentials invalid")
			}
		case cmd == "AUTH LOGIN":
			w("334 VXNlcm5hbWU6")
			u, _ := readLine()
			w("334 UGFzc3dvcmQ6")
			p, _ := readLine()
			ub, _ := base64.StdEncoding.DecodeString(u)
			pb, _ := base64.StdEncoding.DecodeString(p)
			if string(ub) == s.o.User && string(pb) == s.o.Pass {
				user = string(ub)
				w("235 2.7.0 authentication successful")
			} else {
				w("535 5.7.8 authentication credentials invalid")
			}
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			cur = Message{From: strings.Trim(line[len("MAIL FROM:"):], "<> "), User: user}
			w("250 2.1.0 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if s.o.DenyRelay || (s.o.User != "" && user == "") {
				w("554 5.7.1 <" + strings.Trim(line[len("RCPT TO:"):], "<> ") + ">: Relay access denied")
				continue
			}
			cur.To = append(cur.To, strings.Trim(line[len("RCPT TO:"):], "<> "))
			w("250 2.1.5 ok")
		case cmd == "DATA":
			w("354 end data with <CR><LF>.<CR><LF>")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(strings.TrimPrefix(l, "."))
			}
			cur.Data = b.String()
			s.mu.Lock()
			s.messages = append(s.messages, cur)
			s.mu.Unlock()
			w("250 2.0.0 Ok: queued as " + s.o.QueueID)
		case cmd == "QUIT":
			w("221 2.0.0 bye")
			return
		default:
			w("502 5.5.2 command not recognized")
		}
	}
}
