// Package maildiag diagnoses outgoing e-mail step by step (SPEC §3.2): the
// SMTP conversation with the configured server (DNS, TCP, banner, EHLO,
// TLS, AUTH, MAIL/RCPT/DATA) ending with a real test message, and the DNS
// records of the sender domain (MX, SPF, DMARC, DKIM). Every step reports a
// status, a Czech message and a hint. Passwords are never part of a report.
//
// Resolver and dialer are injectable (tests use fakes and smtptest.Server).
package maildiag

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	netmail "net/mail"
	"net/textproto"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qwerin/nanofaktura/internal/mail"
)

// Status of one check. Severity: ok < info < warning < error.
type Status string

const (
	OK      Status = "ok"
	Info    Status = "info"
	Warning Status = "warning"
	Error   Status = "error"
)

func (s Status) rank() int { return slices.Index([]Status{OK, Info, Warning, Error}, s) }

// worse returns the more severe of a and b.
func worse(a, b Status) Status {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// DiagCheck is one step of the SMTP conversation or one DNS check.
type DiagCheck struct {
	ID         string   `json:"id" doc:"Stable step id (resolve, connect, tls, banner, ehlo, starttls, auth, mail_from, rcpt_to, data, config; mx, spf, dmarc, dkim)"`
	Title      string   `json:"title"`
	Status     Status   `json:"status" enum:"ok,info,warning,error"`
	Message    string   `json:"message" doc:"Czech explanation"`
	Hint       string   `json:"hint,omitempty" doc:"Czech advice what to do"`
	Details    []string `json:"details" nullable:"false" doc:"Technical details (server replies, records); never passwords"`
	DurationMS int64    `json:"duration_ms"`
}

// DiagReport is the result of Run.
type DiagReport struct {
	Status      Status      `json:"status" enum:"ok,info,warning,error" doc:"Worst status of all checks"`
	Sent        bool        `json:"sent" doc:"The server accepted the test message"`
	QueueID     string      `json:"queue_id,omitempty" doc:"Queue id from the server's final reply, when recognisable"`
	ServerReply string      `json:"server_reply,omitempty" doc:"Final reply of the server to DATA"`
	From        string      `json:"from"`
	To          string      `json:"to"`
	SMTPHost    string      `json:"smtp_host"`
	SMTPPort    int         `json:"smtp_port"`
	TLSMode     string      `json:"tls_mode"`
	DKIMSigned  bool        `json:"dkim_signed" doc:"The test message was DKIM-signed by the application"`
	SMTP        []DiagCheck `json:"smtp" nullable:"false"`
	DNS         []DiagCheck `json:"dns" nullable:"false"`
	StartedAt   time.Time   `json:"started_at"`
	DurationMS  int64       `json:"duration_ms"`
}

// Resolver is the DNS subset used (net.DefaultResolver satisfies it).
// Missing names must return a *net.DNSError with IsNotFound.
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

// DialFunc opens the TCP connection to the SMTP server.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Options configure Run.
type Options struct {
	SMTP         mail.SMTPConfig // Host empty = SMTP not configured (only DNS checks run)
	To           string          // recipient of the test message
	Message      mail.Message    // the test message (From empty = SMTP.From; To is set from To)
	DKIMSelector string          // selector to check when the application does not sign (optional)
	HeloName     string          // EHLO name; default "localhost"
	Resolver     Resolver        // default net.DefaultResolver
	Dial         DialFunc        // default net.Dialer
	Timeout      time.Duration   // whole run; default 60 s
	Now          func() time.Time
}

// selectorRe validates a user-supplied DKIM selector.
var selectorRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62})$`)

// ValidSelector reports whether s is a syntactically valid DKIM selector.
func ValidSelector(s string) bool { return selectorRe.MatchString(s) && !strings.Contains(s, "..") }

// Run executes the diagnosis. It never returns an error: problems are checks.
func Run(ctx context.Context, o Options) *DiagReport {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	if o.Dial == nil {
		var d net.Dialer
		o.Dial = d.DialContext
	}
	if o.Timeout == 0 {
		o.Timeout = 60 * time.Second
	}
	if o.HeloName == "" {
		o.HeloName = "localhost"
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	start := time.Now()
	r := &DiagReport{To: o.To, SMTPHost: o.SMTP.Host, SMTPPort: o.SMTP.Port, TLSMode: defaultStr(o.SMTP.TLS, mail.TLSStartTLS),
		StartedAt: o.Now(), SMTP: []DiagCheck{}, DNS: []DiagCheck{}, DKIMSigned: o.SMTP.DKIM != nil}
	from := o.Message.From
	if from == "" {
		from = o.SMTP.From
	}
	fromAddr, fromErr := netmail.ParseAddress(from)
	if fromErr == nil {
		r.From = fromAddr.Address
	} else {
		r.From = from
	}

	var ips []string
	if o.SMTP.Host == "" {
		r.SMTP = append(r.SMTP, DiagCheck{ID: "config", Title: "Nastavení SMTP", Status: Error,
			Message: "SMTP není nastavené — e-maily se jen vypisují do logu serveru a nikomu nedorazí.",
			Hint: "Nastavte proměnné prostředí NANOFAKTURA_SMTP_HOST, NANOFAKTURA_SMTP_PORT (587 nebo 465), " +
				"NANOFAKTURA_SMTP_TLS (starttls|tls), NANOFAKTURA_SMTP_USER, NANOFAKTURA_SMTP_PASSWORD a NANOFAKTURA_MAIL_FROM a server restartujte.",
			Details: []string{}})
	} else if fromErr != nil {
		r.SMTP = append(r.SMTP, DiagCheck{ID: "config", Title: "Nastavení SMTP", Status: Error,
			Message: "Adresa odesílatele není platná.", Hint: "Opravte NANOFAKTURA_MAIL_FROM, např. „Firma <faktury@firma.cz>“.",
			Details: []string{fromErr.Error()}})
	} else {
		s := &smtpSession{o: o, r: r, from: fromAddr.Address}
		s.run(ctx)
		ips = s.ips
	}

	if fromErr == nil {
		r.DNS = checkDNS(ctx, o.Resolver, domainOf(fromAddr.Address), ips, o.SMTP.DKIM, o.DKIMSelector)
	}
	r.Status = OK
	for _, c := range append(slices.Clone(r.SMTP), r.DNS...) {
		r.Status = worse(r.Status, c.Status)
	}
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}

// smtpSession runs the SMTP conversation and appends its steps to r.SMTP.
type smtpSession struct {
	o    Options
	r    *DiagReport
	from string
	ips  []string

	conn  net.Conn
	tp    *textproto.Conn
	tls   bool
	exts  map[string]string
	trace []string // transcript of the current step
}

// step starts a timed check; finish it with done.
type step struct {
	s     *smtpSession
	c     DiagCheck
	start time.Time
}

func (s *smtpSession) begin(id, title string) *step {
	s.trace = nil
	return &step{s: s, c: DiagCheck{ID: id, Title: title}, start: time.Now()}
}

func (st *step) done(status Status, msg, hint string, details ...string) bool {
	st.c.Status, st.c.Message, st.c.Hint = status, msg, hint
	st.c.Details = []string{}
	for _, d := range append(slices.Clone(st.s.trace), details...) {
		if d != "" {
			st.c.Details = append(st.c.Details, d)
		}
	}
	st.c.DurationMS = time.Since(st.start).Milliseconds()
	st.s.r.SMTP = append(st.s.r.SMTP, st.c)
	return status != Error
}

func (s *smtpSession) run(ctx context.Context) {
	defer func() {
		if s.conn != nil {
			_ = s.conn.Close()
		}
	}()
	if !s.resolve(ctx) || !s.connect(ctx) {
		return
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = s.conn.SetDeadline(dl)
	}
	if s.r.TLSMode == mail.TLSImplicit && !s.handshake(ctx, "tls", "Šifrované spojení (TLS)") {
		return
	}
	s.tp = textproto.NewConn(s.conn)
	if !s.banner() || !s.ehlo("ehlo", "Představení (EHLO)") {
		return
	}
	switch s.r.TLSMode {
	case mail.TLSStartTLS:
		if !s.starttls(ctx) {
			return
		}
	case mail.TLSNone:
		st := s.begin("starttls", "Šifrování")
		st.done(Warning, "Spojení není šifrované (NANOFAKTURA_SMTP_TLS=none).",
			"Nešifrované spojení používejte jen k lokálnímu relay serveru. U poskytovatele nastavte starttls (port 587) nebo tls (port 465).")
	}
	if !s.auth() || !s.envelope() {
		return
	}
	s.data()
	_, _, _ = s.cmd("QUIT")
}

func (s *smtpSession) resolve(ctx context.Context) bool {
	st := s.begin("resolve", "Překlad názvu serveru (DNS)")
	host := s.o.SMTP.Host
	if ip := net.ParseIP(host); ip != nil {
		s.ips = []string{ip.String()}
		return st.done(OK, "Server je zadán IP adresou "+host+".", "")
	}
	ips, err := s.o.Resolver.LookupHost(ctx, host)
	if err != nil || len(ips) == 0 {
		msg := "Název " + host + " se nepodařilo přeložit na IP adresu."
		if isNotFound(err) {
			msg = "Název " + host + " v DNS neexistuje."
		}
		return st.done(Error, msg, "Zkontrolujte NANOFAKTURA_SMTP_HOST (překlep?) a DNS server, který používá počítač s NanoFakturou.", errText(err))
	}
	s.ips = ips
	return st.done(OK, host+" → "+strings.Join(ips, ", "), "")
}

func (s *smtpSession) connect(ctx context.Context) bool {
	st := s.begin("connect", "Spojení TCP")
	addr := net.JoinHostPort(s.o.SMTP.Host, strconv.Itoa(s.o.SMTP.Port))
	conn, err := s.o.Dial(ctx, "tcp", addr)
	if err != nil {
		hint := "Ověřte port (587 = STARTTLS, 465 = TLS) a že firewall nebo poskytovatel hostingu neblokuje odchozí spojení na tento port."
		msg := "Nepodařilo se spojit s " + addr + "."
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			msg = "Spojení s " + addr + " vypršelo (server neodpovídá)."
		} else if strings.Contains(err.Error(), "refused") {
			msg = "Server " + addr + " spojení odmítl (na portu nic neběží)."
		}
		return st.done(Error, msg, hint, errText(err))
	}
	s.conn = conn
	return st.done(OK, "Spojeno s "+addr+" ("+conn.RemoteAddr().String()+").", "")
}

func (s *smtpSession) banner() bool {
	st := s.begin("banner", "Uvítání serveru")
	code, _, err := s.read()
	if err != nil {
		return st.done(Error, "Server neposlal uvítání.", s.tlsModeHint(), errText(err))
	}
	if code != 220 {
		return st.done(Error, "Server odmítl spojení.", "Server odpověděl chybou hned po připojení — zkuste to později nebo kontaktujte poskytovatele.")
	}
	return st.done(OK, "Server se ohlásil.", "")
}

// tlsModeHint explains the usual port/TLS mix-up.
func (s *smtpSession) tlsModeHint() string {
	if s.r.TLSMode == mail.TLSImplicit {
		return "Port 587 obvykle vyžaduje NANOFAKTURA_SMTP_TLS=starttls, ne tls."
	}
	return "Port 465 vyžaduje NANOFAKTURA_SMTP_TLS=tls (šifrování od začátku spojení)."
}

func (s *smtpSession) ehlo(id, title string) bool {
	st := s.begin(id, title)
	code, msg, err := s.cmd("EHLO %s", s.o.HeloName)
	if err != nil || code != 250 {
		return st.done(Error, "Server nepřijal příkaz EHLO.", "Server možná nepodporuje ESMTP nebo odmítá název "+s.o.HeloName+".", errText(err))
	}
	s.exts = map[string]string{}
	lines := strings.Split(msg, "\n")
	for _, l := range lines[1:] {
		k, v, _ := strings.Cut(l, " ")
		s.exts[strings.ToUpper(k)] = v
	}
	return st.done(OK, "Server podporuje: "+strings.Join(lines[1:], ", ")+".", "")
}

func (s *smtpSession) starttls(ctx context.Context) bool {
	if _, ok := s.exts["STARTTLS"]; !ok {
		st := s.begin("starttls", "Šifrování (STARTTLS)")
		return st.done(Error, "Server nenabízí STARTTLS, spojení by nebylo šifrované.",
			"Na portu 465 nastavte NANOFAKTURA_SMTP_TLS=tls. Pro lokální relay bez šifrování NANOFAKTURA_SMTP_TLS=none.")
	}
	st := s.begin("starttls", "Šifrování (STARTTLS)")
	code, _, err := s.cmd("STARTTLS")
	if err != nil || code != 220 {
		return st.done(Error, "Server odmítl zahájit šifrování.", "", errText(err))
	}
	if !s.handshake(ctx, "starttls", "Šifrování (STARTTLS)", s.trace...) {
		return false
	}
	s.tp = textproto.NewConn(s.conn)
	return s.ehlo("ehlo_tls", "Představení po zašifrování (EHLO)")
}

// handshake upgrades s.conn to TLS and reports version, cipher and the
// certificate (verified manually so its details are known even on failure).
func (s *smtpSession) handshake(ctx context.Context, id, title string, pre ...string) bool {
	st := s.begin(id, title)
	s.trace = pre
	base := &tls.Config{}
	if s.o.SMTP.TLSConfig != nil {
		base = s.o.SMTP.TLSConfig.Clone()
	}
	name := base.ServerName
	if name == "" {
		name = s.o.SMTP.Host
	}
	roots := base.RootCAs
	var verifyErr error
	var leaf *x509.Certificate
	cfg := base.Clone()
	cfg.ServerName = name
	cfg.InsecureSkipVerify = true // verified below, like crypto/tls would
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			verifyErr = errors.New("no certificate")
			return verifyErr
		}
		leaf = cs.PeerCertificates[0]
		inter := x509.NewCertPool()
		for _, c := range cs.PeerCertificates[1:] {
			inter.AddCert(c)
		}
		_, verifyErr = leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: roots, Intermediates: inter})
		return verifyErr
	}
	tc := tls.Client(s.conn, cfg)
	err := tc.HandshakeContext(ctx)
	var details []string
	if leaf != nil {
		details = append(details, certDetails(leaf, s.o.Now())...)
	}
	if err != nil {
		if verifyErr != nil {
			msg, hint := certProblem(verifyErr, name)
			return st.done(Error, msg, hint, append(details, errText(verifyErr))...)
		}
		return st.done(Error, "Šifrované spojení se nepodařilo navázat.", s.tlsModeHint(), append(details, errText(err))...)
	}
	s.conn, s.tls = tc, true
	cs := tc.ConnectionState()
	details = append([]string{"Verze: " + tls.VersionName(cs.Version), "Šifra: " + tls.CipherSuiteName(cs.CipherSuite)}, details...)
	status, msg, hint := OK, "Spojení je šifrované ("+tls.VersionName(cs.Version)+"), certifikát je platný pro "+name+".", ""
	if cs.Version < tls.VersionTLS12 {
		status, hint = Warning, "Server používá zastaralou verzi TLS; požádejte poskytovatele o TLS 1.2 nebo novější."
	}
	if days := int(leaf.NotAfter.Sub(s.o.Now()).Hours() / 24); days < 14 {
		status = worse(status, Warning)
		msg += fmt.Sprintf(" Certifikát ale vyprší za %d dní.", days)
		hint = "Certifikát serveru brzy vyprší — pokud jde o váš server, obnovte ho."
	}
	return st.done(status, msg, hint, details...)
}

func certDetails(c *x509.Certificate, now time.Time) []string {
	d := []string{
		"Certifikát: " + c.Subject.String(),
		"Vydal: " + c.Issuer.String(),
		fmt.Sprintf("Platný do: %s (zbývá %d dní)", c.NotAfter.UTC().Format("2006-01-02"), int(c.NotAfter.Sub(now).Hours()/24)),
	}
	if len(c.DNSNames) > 0 {
		d = append(d, "Názvy: "+strings.Join(c.DNSNames, ", "))
	}
	return d
}

func certProblem(err error, name string) (msg, hint string) {
	var ua x509.UnknownAuthorityError
	var he x509.HostnameError
	var ci x509.CertificateInvalidError
	switch {
	case errors.As(err, &ua):
		return "Certifikát serveru vydala nedůvěryhodná autorita (např. certifikát podepsaný sám sebou).",
			"Použijte certifikát od veřejné autority (např. Let's Encrypt), nebo se připojte k serveru poskytovatele pod jeho oficiálním názvem."
	case errors.As(err, &he):
		return "Certifikát serveru nepatří k názvu " + name + ".",
			"Do NANOFAKTURA_SMTP_HOST zadejte přesně ten název, na který je certifikát vystaven (viz Názvy v detailu)."
	case errors.As(err, &ci) && ci.Reason == x509.Expired:
		return "Certifikát serveru vypršel.", "Certifikát je potřeba na serveru obnovit."
	}
	return "Certifikát serveru se nepodařilo ověřit.", ""
}

func (s *smtpSession) auth() bool {
	st := s.begin("auth", "Přihlášení (AUTH)")
	mechs, offered := s.exts["AUTH"]
	user := s.o.SMTP.Username
	if user == "" {
		hint := ""
		if offered {
			hint = "Server nabízí přihlášení. Pokud odmítne adresáta, nastavte NANOFAKTURA_SMTP_USER a NANOFAKTURA_SMTP_PASSWORD."
		}
		return st.done(Info, "Bez přihlášení (NANOFAKTURA_SMTP_USER není nastaven).", hint)
	}
	if !offered {
		return st.done(Error, "Server nenabízí přihlášení (AUTH).",
			"Server možná přihlášení povoluje až po zašifrování (NANOFAKTURA_SMTP_TLS=starttls/tls), nebo přihlášení nepodporuje — pak NANOFAKTURA_SMTP_USER nenastavujte.")
	}
	if !s.tls && !isLocal(s.o.SMTP.Host) {
		return st.done(Error, "Heslo se po nešifrovaném spojení neposílá.",
			"Nastavte NANOFAKTURA_SMTP_TLS=starttls (port 587) nebo tls (port 465).")
	}
	mech := mail.AuthMechanism(mechs)
	detail := "Mechanismus: " + mech + " (nabízené: " + mechs + "), uživatel: " + user
	var code int
	var err error
	switch mech {
	case "LOGIN":
		code, _, err = s.cmd("AUTH LOGIN")
		if err == nil && code == 334 {
			code, _, err = s.secret(base64.StdEncoding.EncodeToString([]byte(user)), "<uživatel>")
		}
		if err == nil && code == 334 {
			code, _, err = s.secret(base64.StdEncoding.EncodeToString([]byte(s.o.SMTP.Password)), "<heslo>")
		}
	default:
		code, _, err = s.secret("AUTH PLAIN "+base64.StdEncoding.EncodeToString([]byte("\x00"+user+"\x00"+s.o.SMTP.Password)), "AUTH PLAIN <skryto>")
	}
	if err != nil && code == 0 {
		return st.done(Error, "Přihlášení se nepodařilo dokončit.", "", detail, errText(err))
	}
	if code != 235 {
		msg := "Server přihlášení odmítl."
		if code == 535 {
			msg = "Server odmítl jméno nebo heslo."
		}
		return st.done(Error, msg, "Zkontrolujte NANOFAKTURA_SMTP_USER a NANOFAKTURA_SMTP_PASSWORD. Některé služby (Gmail, Microsoft 365) vyžadují heslo pro aplikace.", detail)
	}
	return st.done(OK, "Přihlášení proběhlo ("+mech+").", "", detail)
}

func (s *smtpSession) envelope() bool {
	st := s.begin("mail_from", "Odesílatel (MAIL FROM)")
	code, _, err := s.cmd("MAIL FROM:<%s>", s.from)
	if err != nil || code != 250 {
		return st.done(Error, "Server odmítl odesílatele "+s.from+".",
			"Adresa v NANOFAKTURA_MAIL_FROM musí patřit přihlášenému účtu nebo doméně, pro kterou smí server odesílat.", errText(err))
	}
	st.done(OK, "Odesílatel "+s.from+" přijat.", "")
	st = s.begin("rcpt_to", "Příjemce (RCPT TO)")
	code, msg, err := s.cmd("RCPT TO:<%s>", s.o.To)
	if err != nil || (code != 250 && code != 251) {
		m := "Server odmítl příjemce " + s.o.To + "."
		hint := "Zkontrolujte adresu příjemce."
		if code == 550 || code == 551 || code == 553 || code == 554 || strings.Contains(strings.ToLower(msg), "relay") {
			m = "Server odmítl předat e-mail dál (relay denied)."
			hint = "Server posílá poštu jen přihlášeným uživatelům nebo povoleným adresám — nastavte NANOFAKTURA_SMTP_USER a heslo, " +
				"nebo u relay serveru povolte IP adresu NanoFaktury."
		}
		return st.done(Error, m, hint, errText(err))
	}
	return st.done(OK, "Příjemce "+s.o.To+" přijat.", "")
}

var queueIDRe = regexp.MustCompile(`(?i)(?:queued as|queue id|id=|<)\s*([A-Za-z0-9._@-]+)`)

func (s *smtpSession) data() bool {
	st := s.begin("data", "Odeslání zprávy (DATA)")
	msg := s.o.Message
	msg.To = []string{s.o.To}
	data, _, _, err := mail.Compose(s.o.SMTP, msg, s.o.Now())
	if err != nil {
		return st.done(Error, "Zprávu se nepodařilo sestavit.", "", errText(err))
	}
	code, _, err := s.cmd("DATA")
	if err != nil || code != 354 {
		return st.done(Error, "Server odmítl přijmout obsah zprávy.", "", errText(err))
	}
	w := s.tp.DotWriter()
	if _, err := w.Write(data); err != nil {
		return st.done(Error, "Odeslání obsahu se přerušilo.", "", errText(err))
	}
	if err := w.Close(); err != nil {
		return st.done(Error, "Odeslání obsahu se přerušilo.", "", errText(err))
	}
	s.trace = append(s.trace, fmt.Sprintf("C: <zpráva, %d B>", len(data)))
	code, reply, err := s.read()
	if err != nil || code != 250 {
		return st.done(Error, "Server zprávu nepřijal.", "Odpověď serveru v detailu často říká proč (spam filtr, velikost, zakázaný odesílatel).", errText(err))
	}
	s.r.Sent, s.r.ServerReply = true, reply
	if m := queueIDRe.FindStringSubmatch(reply); m != nil {
		s.r.QueueID = strings.TrimRight(m[1], ">")
	}
	text := "Server zprávu převzal k doručení."
	if s.r.QueueID != "" {
		text += " ID ve frontě: " + s.r.QueueID + "."
	}
	return st.done(OK, text, "Zkontrolujte schránku příjemce (i složku spam). Převzetí serverem ještě neznamená doručení.")
}

// cmd sends a command and reads the reply; the transcript records both.
func (s *smtpSession) cmd(format string, args ...any) (int, string, error) {
	line := fmt.Sprintf(format, args...)
	return s.send(line, line)
}

// secret sends a line containing credentials; the transcript shows shown.
func (s *smtpSession) secret(line, shown string) (int, string, error) { return s.send(line, shown) }

func (s *smtpSession) send(line, shown string) (int, string, error) {
	s.trace = append(s.trace, "C: "+shown)
	if err := s.tp.PrintfLine("%s", line); err != nil {
		return 0, "", err
	}
	return s.read()
}

func (s *smtpSession) read() (int, string, error) {
	code, msg, err := s.tp.ReadResponse(0)
	for i, l := range strings.Split(msg, "\n") {
		sep := "-"
		if i == strings.Count(msg, "\n") {
			sep = " "
		}
		if code != 0 {
			s.trace = append(s.trace, fmt.Sprintf("S: %d%s%s", code, sep, l))
		}
	}
	if err != nil {
		var te *textproto.Error
		if errors.As(err, &te) {
			return te.Code, te.Msg, nil
		}
		return code, msg, err
	}
	return code, msg, nil
}

func isLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isNotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return "Chyba: " + err.Error()
}

func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return strings.ToLower(addr[i+1:])
	}
	return ""
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
