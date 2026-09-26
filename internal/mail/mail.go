// Package mail sends e-mails: the Mailer interface, an SMTP implementation,
// a log mailer for development (no SMTP configured) and a tiny {placeholder}
// template helper. Tests use the recording fake from internal/mail/mailtest.
package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	netmail "net/mail"
	"regexp"
	"strings"
	"sync"
)

// Message is one e-mail. Addresses are RFC 5322 ("a@b.cz" or "Name <a@b.cz>").
// From may be empty — the mailer then uses its configured default sender.
// At least one of Text and HTML should be set.
type Message struct {
	From        string
	ReplyTo     string
	To          []string
	Cc          []string
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
}

// Attachment is a file attached to a Message.
type Attachment struct {
	Filename    string
	ContentType string // e.g. "application/pdf"; empty = application/octet-stream
	Data        []byte
}

// Mailer sends messages.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// ErrNoRecipients is returned for a message without To and Cc.
var ErrNoRecipients = errors.New("mail: no recipients")

// placeholder matches {name} in templates.
var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Render replaces {name} placeholders in tpl with vars[name]. Placeholders
// without a value are left as they are, so typos stay visible.
//
//	mail.Render("Faktura {number} na {total}", map[string]string{"number": "2026-0001", "total": "1 210 Kč"})
func Render(tpl string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		if v, ok := vars[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

// withDefaults fills From and validates the addresses. It returns the
// message, the envelope sender and the envelope recipients (To + Cc).
func withDefaults(m Message, defaultFrom string) (Message, string, []string, error) {
	if m.From == "" {
		m.From = defaultFrom
	}
	from, err := netmail.ParseAddress(m.From)
	if err != nil {
		return m, "", nil, fmt.Errorf("mail: invalid From %q: %w", m.From, err)
	}
	if m.ReplyTo != "" {
		if _, err := netmail.ParseAddress(m.ReplyTo); err != nil {
			return m, "", nil, fmt.Errorf("mail: invalid Reply-To %q: %w", m.ReplyTo, err)
		}
	}
	var rcpt []string
	for _, list := range [][]string{m.To, m.Cc} {
		for _, a := range list {
			addr, err := netmail.ParseAddress(a)
			if err != nil {
				return m, "", nil, fmt.Errorf("mail: invalid recipient %q: %w", a, err)
			}
			rcpt = append(rcpt, addr.Address)
		}
	}
	if len(rcpt) == 0 {
		return m, "", nil, ErrNoRecipients
	}
	return m, from.Address, rcpt, nil
}

// LogMailer prints messages instead of sending them (development without SMTP).
type LogMailer struct {
	mu   sync.Mutex
	w    io.Writer
	from string
}

// NewLogMailer writes a readable dump of every message to w.
func NewLogMailer(w io.Writer, defaultFrom string) *LogMailer {
	return &LogMailer{w: w, from: defaultFrom}
}

func (l *LogMailer) Send(_ context.Context, m Message) error {
	m, _, _, err := withDefaults(m, l.from)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "----- mail (not sent, no SMTP configured) -----\nFrom: %s\nTo: %s\n", m.From, strings.Join(m.To, ", "))
	if len(m.Cc) > 0 {
		fmt.Fprintf(&b, "Cc: %s\n", strings.Join(m.Cc, ", "))
	}
	if m.ReplyTo != "" {
		fmt.Fprintf(&b, "Reply-To: %s\n", m.ReplyTo)
	}
	fmt.Fprintf(&b, "Subject: %s\n\n", m.Subject)
	if m.Text != "" {
		b.WriteString(m.Text)
	} else {
		b.WriteString(m.HTML)
	}
	b.WriteString("\n")
	for _, a := range m.Attachments {
		fmt.Fprintf(&b, "[attachment %s, %s, %d bytes]\n", a.Filename, a.ContentType, len(a.Data))
	}
	b.WriteString("-----------------------------------------------\n")
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = io.WriteString(l.w, b.String())
	return err
}
