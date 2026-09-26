package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
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

func (s *SMTP) Send(ctx context.Context, m Message) error {
	m, from, rcpt, err := withDefaults(m, s.cfg.From)
	if err != nil {
		return err
	}
	data, err := buildMessage(m, s.now())
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
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
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
		return addr[i+1:]
	}
	return "localhost"
}
