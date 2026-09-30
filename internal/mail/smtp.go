package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TLS modes of SMTPConfig.TLS.
const (
	TLSStartTLS = "starttls" // plain connection upgraded with STARTTLS (required), usually port 587
	TLSImplicit = "tls"      // TLS from the first byte, usually port 465
	TLSNone     = "none"     // unencrypted (local relays only; AUTH is refused by net/smtp except on localhost)
)

// SMTPConfig configures an SMTP mailer.
type SMTPConfig struct {
	Host      string
	Port      int
	Username  string // empty = no AUTH
	Password  string
	TLS       string        // TLSStartTLS (default), TLSImplicit or TLSNone
	From      string        // default sender when Message.From is empty
	Timeout   time.Duration // whole conversation when ctx has no deadline; default 30 s
	TLSConfig *tls.Config   // optional (tests: custom RootCAs); ServerName defaults to Host
	DKIM      *DKIMConfig   // optional: sign every message (never logged)
}

// SMTP sends messages over SMTP, one connection per message.
type SMTP struct {
	cfg SMTPConfig
	now func() time.Time
}

func NewSMTP(cfg SMTPConfig) *SMTP {
	if cfg.TLS == "" {
		cfg.TLS = TLSStartTLS
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &SMTP{cfg: cfg, now: time.Now}
}

func (s *SMTP) tlsConfig() *tls.Config {
	c := &tls.Config{}
	if s.cfg.TLSConfig != nil {
		c = s.cfg.TLSConfig.Clone()
	}
	if c.ServerName == "" {
		c.ServerName = s.cfg.Host
	}
	return c
}

// Config returns the configuration (diagnostics reuse it).
func (s *SMTP) Config() SMTPConfig { return s.cfg }

// Compose validates m, renders it as RFC 5322 data (with Date and Message-ID,
// DKIM-signed when cfg.DKIM is set) and returns the data with the envelope
// sender and recipients. Send and the SMTP diagnostics use it.
func Compose(cfg SMTPConfig, m Message, now time.Time) (data []byte, from string, rcpt []string, err error) {
	m, from, rcpt, err = withDefaults(m, cfg.From)
	if err != nil {
		return nil, "", nil, err
	}
	if data, err = buildMessage(m, now); err != nil {
		return nil, "", nil, err
	}
	if cfg.DKIM != nil {
		if data, err = signDKIM(data, cfg.DKIM); err != nil {
			return nil, "", nil, err
		}
	}
	return data, from, rcpt, nil
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	data, from, rcpt, err := Compose(s.cfg, m, s.now())
	if err != nil {
		return err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(s.cfg.Timeout)
	}
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := net.Dialer{Deadline: deadline}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)
	if s.cfg.TLS == TLSImplicit {
		tc := tls.Client(conn, s.tlsConfig())
		if err := tc.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("smtp tls: %w", err)
		}
		conn = tc
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	defer c.Close()
	if s.cfg.TLS == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp: server %s does not support STARTTLS", addr)
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		_, mechs := c.Extension("AUTH")
		if err := c.Auth(chooseAuth(mechs, s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, r := range rcpt {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	return c.Quit()
}

// chooseAuth picks PLAIN, or LOGIN when the server offers only that
// (mechs = the AUTH extension parameter, e.g. "LOGIN XOAUTH2").
func chooseAuth(mechs, user, pass, host string) smtp.Auth {
	if AuthMechanism(mechs) == "LOGIN" {
		return &loginAuth{user: user, pass: pass, host: host}
	}
	return smtp.PlainAuth("", user, pass, host)
}

// AuthMechanism is the mechanism chooseAuth uses for the advertised mechs.
func AuthMechanism(mechs string) string {
	f := strings.Fields(strings.ToUpper(mechs))
	if !slices.Contains(f, "PLAIN") && slices.Contains(f, "LOGIN") {
		return "LOGIN"
	}
	return "PLAIN"
}

// loginAuth implements AUTH LOGIN; like smtp.PlainAuth it refuses to send
// credentials over an unencrypted connection except to localhost.
type loginAuth struct{ user, pass, host string }

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:", "user name", "username":
		return []byte(a.user), nil
	case "password:", "password":
		return []byte(a.pass), nil
	}
	if len(fromServer) == 0 { // some servers send an empty first challenge
		return []byte(a.user), nil
	}
	return nil, errors.New("unexpected AUTH LOGIN challenge")
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

// buildMessage renders m as an RFC 5322 / MIME message (CRLF line endings):
// text and/or HTML (multipart/alternative when both), wrapped in
// multipart/mixed when there are attachments.
func buildMessage(m Message, now time.Time) ([]byte, error) {
	var buf bytes.Buffer
	h := func(k, v string) { fmt.Fprintf(&buf, "%s: %s\r\n", k, v) }

	from, _ := netmail.ParseAddress(m.From) // validated by withDefaults
	h("From", from.String())
	if len(m.To) > 0 {
		h("To", addressList(m.To))
	}
	if len(m.Cc) > 0 {
		h("Cc", addressList(m.Cc))
	}
	if m.ReplyTo != "" {
		rt, _ := netmail.ParseAddress(m.ReplyTo)
		h("Reply-To", rt.String())
	}
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+randomID()+"@"+domainOf(from.Address)+">")
	h("MIME-Version", "1.0")

	if len(m.Attachments) == 0 {
		if err := writeBody(&buf, m); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	mw := multipart.NewWriter(&buf)
	h("Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mw.Boundary()}))
	buf.WriteString("\r\n")
	var body bytes.Buffer
	if err := writeBody(&body, m); err != nil {
		return nil, err
	}
	hdr, rest := splitHeader(body.Bytes())
	pw, err := mw.CreatePart(hdr)
	if err != nil {
		return nil, err
	}
	if _, err := pw.Write(rest); err != nil {
		return nil, err
	}
	for _, a := range m.Attachments {
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		ph := textproto.MIMEHeader{}
		ph.Set("Content-Type", ct)
		ph.Set("Content-Transfer-Encoding", "base64")
		ph.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename}))
		pw, err := mw.CreatePart(ph)
		if err != nil {
			return nil, err
		}
		if err := writeBase64(pw, a.Data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeBody writes the Content-Type header(s), a blank line and the text/HTML
// body (either continuing the message header block or as a MIME part).
func writeBody(buf *bytes.Buffer, m Message) error {
	switch {
	case m.Text != "" && m.HTML != "":
		mw := multipart.NewWriter(buf)
		fmt.Fprintf(buf, "Content-Type: %s\r\n\r\n", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": mw.Boundary()}))
		for _, p := range []struct{ ct, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
			ph := textproto.MIMEHeader{}
			ph.Set("Content-Type", p.ct+"; charset=utf-8")
			ph.Set("Content-Transfer-Encoding", "quoted-printable")
			pw, err := mw.CreatePart(ph)
			if err != nil {
				return err
			}
			if err := writeQP(pw, p.body); err != nil {
				return err
			}
		}
		return mw.Close()
	case m.HTML != "":
		buf.WriteString("Content-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		return writeQP(buf, m.HTML)
	default:
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
		return writeQP(buf, m.Text)
	}
}

// splitHeader splits "K: v\r\n…\r\n\r\nbody" into a MIME header and the body.
func splitHeader(b []byte) (textproto.MIMEHeader, []byte) {
	head, rest, _ := bytes.Cut(b, []byte("\r\n\r\n"))
	h := textproto.MIMEHeader{}
	for _, line := range strings.Split(string(head), "\r\n") {
		if k, v, ok := strings.Cut(line, ": "); ok {
			h.Set(k, v)
		}
	}
	return h, rest
}

func writeQP(w io.Writer, s string) error {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	qw := quotedprintable.NewWriter(w)
	if _, err := qw.Write([]byte(s)); err != nil {
		return err
	}
	return qw.Close()
}

func writeBase64(w io.Writer, data []byte) error {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		if _, err := w.Write([]byte(enc[:76] + "\r\n")); err != nil {
			return err
		}
		enc = enc[76:]
	}
	_, err := w.Write([]byte(enc + "\r\n"))
	return err
}

func addressList(list []string) string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		addr, _ := netmail.ParseAddress(a) // validated by withDefaults
		out = append(out, addr.String())
	}
	return strings.Join(out, ", ")
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return strings.ToLower(addr[i+1:])
	}
	return "localhost"
}
