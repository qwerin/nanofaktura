package maildiag

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qwerin/nanofaktura/internal/mail"
)

// spfLookupLimit is the RFC 7208 limit of DNS-querying terms.
const spfLookupLimit = 10

// checkDNS checks the DNS records of the sender domain. smtpIPs are the
// addresses of the SMTP server (for the SPF coverage check; may be empty).
func checkDNS(ctx context.Context, r Resolver, domain string, smtpIPs []string, dk *mail.DKIMConfig, selector string) []DiagCheck {
	return []DiagCheck{
		timed(func() DiagCheck { return checkMX(ctx, r, domain) }),
		timed(func() DiagCheck { return CheckSPF(ctx, r, domain, smtpIPs) }),
		timed(func() DiagCheck { return CheckDMARC(ctx, r, domain) }),
		timed(func() DiagCheck { return CheckDKIM(ctx, r, domain, dk, selector) }),
	}
}

func timed(f func() DiagCheck) DiagCheck {
	start := time.Now()
	c := f()
	if c.Details == nil {
		c.Details = []string{}
	}
	c.DurationMS = time.Since(start).Milliseconds()
	return c
}

func checkMX(ctx context.Context, r Resolver, domain string) DiagCheck {
	c := DiagCheck{ID: "mx", Title: "MX záznam domény " + domain}
	mxs, err := r.LookupMX(ctx, domain)
	if err != nil && !isNotFound(err) {
		c.Status, c.Message, c.Details = Info, "MX záznam se nepodařilo zjistit.", []string{errText(err)}
		return c
	}
	if len(mxs) == 0 {
		hosts, _ := r.LookupHost(ctx, domain)
		c.Status = Warning
		c.Message = "Doména " + domain + " nemá MX záznam — odpovědi na vaše e-maily nemusí být doručeny a některé servery odesílatele bez MX odmítají."
		if len(hosts) == 0 && isNotFound(err) {
			c.Status, c.Message = Error, "Doména odesílatele "+domain+" v DNS neexistuje."
		}
		c.Hint = "Adresa v NANOFAKTURA_MAIL_FROM by měla být na doméně, která poštu i přijímá."
		return c
	}
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	for _, m := range mxs {
		c.Details = append(c.Details, fmt.Sprintf("%d %s", m.Pref, strings.TrimSuffix(m.Host, ".")))
	}
	c.Status, c.Message = OK, "Doména přijímá poštu ("+strings.TrimSuffix(mxs[0].Host, ".")+")."
	return c
}

// ---- SPF ----

// txtWith returns the TXT records of name starting with prefix (case-insensitive,
// followed by a space or the end), found = the name exists.
func txtWith(ctx context.Context, r Resolver, name, prefix string) (recs []string, err error) {
	txts, err := r.LookupTXT(ctx, name)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, t := range txts {
		t = strings.TrimSpace(t)
		if len(t) >= len(prefix) && strings.EqualFold(t[:len(prefix)], prefix) && (len(t) == len(prefix) || t[len(prefix)] == ' ' || t[len(prefix)] == ';') {
			recs = append(recs, t)
		}
	}
	return recs, nil
}

// CheckSPF checks the SPF record of domain: exactly one v=spf1 record, the
// "all" qualifier, the 10-lookup limit and (best effort) whether smtpIPs are
// allowed by it.
func CheckSPF(ctx context.Context, r Resolver, domain string, smtpIPs []string) DiagCheck {
	c := DiagCheck{ID: "spf", Title: "SPF (kdo smí posílat za " + domain + ")", Details: []string{}}
	recs, err := txtWith(ctx, r, domain, "v=spf1")
	if err != nil {
		c.Status, c.Message, c.Details = Info, "SPF záznam se nepodařilo zjistit (DNS neodpovídá).", []string{errText(err)}
		return c
	}
	switch len(recs) {
	case 0:
		c.Status, c.Message = Warning, "Doména nemá SPF záznam — příjemci nemohou ověřit, že smíte posílat za "+domain+"."
		c.Hint = "Přidejte TXT záznam na " + domain + ", např. „v=spf1 include:<SPF vašeho poskytovatele> -all“ (přesnou hodnotu uvádí poskytovatel SMTP)."
		return c
	case 1:
	default:
		c.Status, c.Message, c.Details = Error, "Doména má více SPF záznamů — příjemci to vyhodnotí jako chybu (permerror).", recs
		c.Hint = "Sloučte záznamy do jediného TXT záznamu „v=spf1 … -all“."
		return c
	}
	c.Details = append(c.Details, "Záznam: "+recs[0])
	status := OK
	var msgs, hints []string

	// the "all" qualifier of the top-level record
	all := ""
	hasRedirect := false
	for _, t := range strings.Fields(recs[0])[1:] {
		lt := strings.ToLower(t)
		if strings.HasPrefix(lt, "redirect=") {
			hasRedirect = true
		}
		if strings.TrimLeft(lt, "+-~?") == "all" {
			all = lt
			if !strings.ContainsAny(all[:1], "+-~?") {
				all = "+" + all
			}
		}
	}
	switch all {
	case "-all":
		msgs = append(msgs, "Ostatní servery jsou odmítnuty (-all).")
	case "~all":
		msgs = append(msgs, "Ostatní servery jsou označeny jako podezřelé (~all).")
	case "?all":
		status = worse(status, Warning)
		msgs = append(msgs, "Záznam končí ?all — ostatní servery nejsou nijak omezeny.")
		hints = append(hints, "Zakončete záznam ~all nebo -all.")
	case "+all":
		status = worse(status, Error)
		msgs = append(msgs, "Záznam končí +all — posílat za vaši doménu smí kdokoli.")
		hints = append(hints, "Nahraďte +all za -all (nebo ~all).")
	default:
		if !hasRedirect {
			status = worse(status, Warning)
			msgs = append(msgs, "Záznam nemá koncové pravidlo all.")
			hints = append(hints, "Zakončete záznam -all nebo ~all.")
		}
	}

	n := countSPFLookups(ctx, r, domain, 0, map[string]bool{})
	c.Details = append(c.Details, fmt.Sprintf("DNS dotazů při vyhodnocení: %d z %d", n, spfLookupLimit))
	if n > spfLookupLimit {
		status = worse(status, Error)
		msgs = append(msgs, fmt.Sprintf("Záznam vyžaduje %d DNS dotazů, limit je %d — příjemci ho vyhodnotí jako chybu (permerror).", n, spfLookupLimit))
		hints = append(hints, "Omezte počet include/a/mx (nahraďte je rozsahy ip4:/ip6:, odeberte nepoužívané služby).")
	}

	// coverage of the SMTP server's addresses
	switch {
	case len(smtpIPs) == 0:
		status = worse(status, Info)
		msgs = append(msgs, "Zda SPF povoluje váš SMTP server, nelze ověřit (adresa serveru není známa).")
	default:
		var pass, fail, unsure, perm []string
		for _, s := range smtpIPs {
			ip, err := netip.ParseAddr(s)
			if err != nil {
				continue
			}
			if ip.IsLoopback() || ip.IsPrivate() {
				unsure = append(unsure, s+" (lokální adresa)")
				continue
			}
			e := &spfEval{ctx: ctx, r: r}
			res := e.eval(ip.Unmap(), domain, 0)
			c.Details = append(c.Details, "Vyhodnocení pro "+s+": "+res)
			switch res {
			case "pass":
				pass = append(pass, s)
			case "fail", "softfail", "neutral":
				fail = append(fail, s)
			case "permerror":
				perm = append(perm, s)
			default:
				unsure = append(unsure, s)
			}
		}
		switch {
		case len(perm) > 0:
			status = worse(status, Error)
			msgs = append(msgs, "Vyhodnocení SPF končí chybou (permerror) — příjemci záznam nepoužijí.")
			hints = append(hints, "Zkontrolujte, že každá doména v include: a redirect= má vlastní SPF záznam a že záznam nemá překlep.")
		case len(fail) > 0:
			status = worse(status, Warning)
			msgs = append(msgs, "Adresa SMTP serveru "+strings.Join(fail, ", ")+" není v SPF povolena.")
			hints = append(hints, "Pokud poskytovatel odesílá poštu z jiných serverů než z přihlašovacího (běžné u velkých služeb), "+
				"rozhodne až test u příjemce (Gmail „Zobrazit originál“ → spf=pass). Jinak do SPF přidejte ip4:/ip6: adresu serveru nebo include: poskytovatele.")
		case len(unsure) > 0:
			status = worse(status, Info)
			msgs = append(msgs, "Zda SPF povoluje SMTP server ("+strings.Join(unsure, ", ")+"), nelze ověřit.")
			hints = append(hints, "Ověřte výsledek v testovacím e-mailu (Gmail „Zobrazit originál“ → spf=pass).")
		default:
			msgs = append(msgs, "SMTP server ("+strings.Join(pass, ", ")+") je v SPF povolen.")
		}
	}
	c.Status, c.Message, c.Hint = status, strings.Join(msgs, " "), strings.Join(hints, " ")
	return c
}

// countSPFLookups counts the DNS-querying terms of domain's SPF record tree
// (include, a, mx, ptr, exists, redirect) — the RFC limit is 10.
func countSPFLookups(ctx context.Context, r Resolver, domain string, depth int, seen map[string]bool) int {
	if depth > 10 || seen[domain] {
		return 0
	}
	seen[domain] = true
	recs, err := txtWith(ctx, r, domain, "v=spf1")
	if err != nil || len(recs) != 1 {
		return 0
	}
	n := 0
	for _, t := range strings.Fields(recs[0])[1:] {
		mech, arg := splitTerm(t)
		switch mech {
		case "a", "mx", "ptr", "exists":
			n++
		case "include", "redirect":
			n++
			if !strings.Contains(arg, "%") {
				n += countSPFLookups(ctx, r, strings.ToLower(arg), depth+1, seen)
			}
		}
	}
	return n
}

// splitTerm splits an SPF term into the lowercase mechanism/modifier name and
// its argument ("include:x" → include, x; "redirect=x"; "a:x/24" → a, x/24; "?all" → all).
func splitTerm(t string) (mech, arg string) {
	t = strings.TrimLeft(t, "+-~?")
	if k, v, ok := strings.Cut(t, "="); ok && !strings.ContainsAny(k, ":/") {
		return strings.ToLower(k), v
	}
	i := strings.IndexAny(t, ":/")
	if i < 0 {
		return strings.ToLower(t), ""
	}
	if t[i] == ':' {
		return strings.ToLower(t[:i]), t[i+1:]
	}
	return strings.ToLower(t[:i]), t[i:]
}

// spfEval evaluates an SPF record for one IP (RFC 7208 subset). Results:
// pass, fail, softfail, neutral, none, permerror, uncertain (macros, ptr,
// exists, DNS errors — we do not guess).
type spfEval struct {
	ctx     context.Context
	r       Resolver
	lookups int
}

func (e *spfEval) eval(ip netip.Addr, domain string, depth int) string {
	if depth > 10 {
		return "permerror"
	}
	recs, err := txtWith(e.ctx, e.r, domain, "v=spf1")
	if err != nil {
		return "uncertain"
	}
	if len(recs) == 0 {
		return "none"
	}
	if len(recs) > 1 {
		return "permerror"
	}
	redirect := ""
	for _, t := range strings.Fields(recs[0])[1:] {
		q := byte('+')
		if strings.ContainsRune("+-~?", rune(t[0])) {
			q = t[0]
		}
		mech, arg := splitTerm(t)
		if strings.Contains(arg, "%") {
			return "uncertain"
		}
		var match bool
		switch mech {
		case "redirect":
			redirect = strings.ToLower(arg)
			continue
		case "exp":
			continue
		case "all":
			match = true
		case "ip4", "ip6":
			p, err := parsePrefix(arg)
			if err != nil {
				return "permerror"
			}
			match = p.Contains(ip)
		case "a", "mx":
			if e.lookups++; e.lookups > spfLookupLimit {
				return "permerror"
			}
			target, c4, c6 := splitCIDR(arg, domain)
			hosts := []string{target}
			if mech == "mx" {
				mxs, err := e.r.LookupMX(e.ctx, target)
				if err != nil && !isNotFound(err) {
					return "uncertain"
				}
				hosts = hosts[:0]
				for i, m := range mxs {
					if i == 10 {
						break
					}
					hosts = append(hosts, strings.TrimSuffix(m.Host, "."))
				}
			}
			for _, h := range hosts {
				addrs, err := e.r.LookupHost(e.ctx, h)
				if err != nil && !isNotFound(err) {
					return "uncertain"
				}
				for _, a := range addrs {
					if hostMatches(ip, a, c4, c6) {
						match = true
					}
				}
			}
		case "include":
			if e.lookups++; e.lookups > spfLookupLimit {
				return "permerror"
			}
			switch res := e.eval(ip, strings.ToLower(arg), depth+1); res {
			case "pass":
				match = true
			case "fail", "softfail", "neutral":
			case "none":
				return "permerror"
			default:
				return res
			}
		case "ptr", "exists":
			return "uncertain"
		default:
			if strings.Contains(t, "=") {
				continue // unknown modifier
			}
			return "permerror"
		}
		if match {
			return map[byte]string{'+': "pass", '-': "fail", '~': "softfail", '?': "neutral"}[q]
		}
	}
	if redirect != "" {
		if e.lookups++; e.lookups > spfLookupLimit {
			return "permerror"
		}
		res := e.eval(ip, redirect, depth+1)
		if res == "none" {
			return "permerror"
		}
		return res
	}
	return "neutral"
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// splitCIDR parses the argument of a/mx: "[domain][/c4][//c6]".
func splitCIDR(arg, def string) (target string, c4, c6 int) {
	c4, c6 = 32, 128
	target = arg
	if i := strings.Index(arg, "/"); i >= 0 {
		target, arg = arg[:i], arg[i:] // "/24", "/24//64" or "//64"
		if j := strings.Index(arg, "//"); j >= 0 {
			if n, err := strconv.Atoi(arg[j+2:]); err == nil {
				c6 = n
			}
			arg = arg[:j]
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(arg, "/")); err == nil {
			c4 = n
		}
	}
	if target == "" {
		target = def
	}
	return strings.ToLower(target), c4, c6
}

func hostMatches(ip netip.Addr, addr string, c4, c6 int) bool {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return false
	}
	a = a.Unmap()
	if a.Is4() != ip.Is4() {
		return false
	}
	bits := c6
	if a.Is4() {
		bits = c4
	}
	p, err := a.Prefix(bits)
	return err == nil && p.Contains(ip)
}

// ---- DMARC ----

// CheckDMARC checks the _dmarc record of domain (or of its parent domain).
func CheckDMARC(ctx context.Context, r Resolver, domain string) DiagCheck {
	c := DiagCheck{ID: "dmarc", Title: "DMARC (pravidla pro neověřené e-maily)", Details: []string{}}
	names := []string{"_dmarc." + domain}
	if labels := strings.Split(domain, "."); len(labels) > 2 {
		names = append(names, "_dmarc."+strings.Join(labels[len(labels)-2:], "."))
	}
	var recs []string
	var name string
	for _, n := range names {
		rs, err := txtWith(ctx, r, n, "v=DMARC1")
		if err != nil {
			c.Status, c.Message, c.Details = Info, "DMARC záznam se nepodařilo zjistit (DNS neodpovídá).", []string{errText(err)}
			return c
		}
		if len(rs) > 0 {
			recs, name = rs, n
			break
		}
	}
	example := "v=DMARC1; p=none; rua=mailto:dmarc@" + domain
	if len(recs) == 0 {
		c.Status, c.Message = Warning, "Doména nemá DMARC záznam. Gmail a další velcí příjemci ho u odesílatelů vyžadují a bez něj častěji řadí poštu do spamu."
		c.Hint = "Přidejte TXT záznam _dmarc." + domain + " s hodnotou „" + example + "“ a po ověření, že SPF a DKIM procházejí, zpřísněte p=quarantine."
		return c
	}
	if len(recs) > 1 {
		c.Status, c.Message, c.Details = Error, "Doména má více DMARC záznamů — příjemci je ignorují.", recs
		c.Hint = "Ponechte jediný záznam na " + name + "."
		return c
	}
	c.Details = append(c.Details, name+": "+recs[0])
	tags := parseTags(recs[0])
	p := strings.ToLower(tags["p"])
	switch p {
	case "reject", "quarantine":
		c.Status, c.Message = OK, "DMARC je nastaven (p="+p+")."
	case "none":
		c.Status, c.Message = Info, "DMARC je nastaven jen pro sledování (p=none) — neověřené e-maily se nijak neomezují."
		c.Hint = "Až budou SPF a DKIM spolehlivě procházet, zvažte p=quarantine."
	default:
		c.Status, c.Message = Error, "DMARC záznam nemá platné pravidlo p= (none, quarantine nebo reject)."
		c.Hint = "Opravte záznam, např. „" + example + "“."
		return c
	}
	if tags["rua"] == "" {
		c.Hint = strings.TrimSpace(c.Hint + " Doplňte rua=mailto:… a budete dostávat přehledy, kdo za vaši doménu posílá.")
	} else {
		c.Details = append(c.Details, "Přehledy (rua): "+tags["rua"])
	}
	if name != "_dmarc."+domain {
		c.Details = append(c.Details, "Použit záznam nadřazené domény.")
	}
	return c
}

// parseTags parses "k=v; k2=v2" (DMARC, DKIM records); keys lowercase.
func parseTags(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, "=")
		if ok {
			out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return out
}

// ---- DKIM ----

// CheckDKIM verifies the DKIM record: of the application's signing key when
// dk is set (record exists and its public key matches), otherwise of the
// user-supplied selector (record exists and holds a usable key).
func CheckDKIM(ctx context.Context, r Resolver, fromDomain string, dk *mail.DKIMConfig, selector string) DiagCheck {
	c := DiagCheck{ID: "dkim", Title: "DKIM (podpis e-mailů)", Details: []string{}}
	domain := fromDomain
	if dk != nil {
		selector, domain = dk.Selector, dk.Domain
	}
	if selector == "" {
		c.Status = Info
		c.Message = "Aplikace e-maily nepodepisuje (DKIM není nastaven)."
		c.Hint = "Pokud e-maily podepisuje váš SMTP poskytovatel, zadejte jeho selektor a ověříme záznam. " +
			"Jinak můžete zapnout podpis v aplikaci (NANOFAKTURA_DKIM_DOMAIN, NANOFAKTURA_DKIM_SELECTOR, NANOFAKTURA_DKIM_PRIVATE_KEY)."
		return c
	}
	name := selector + "._domainkey." + domain
	c.Title = "DKIM (" + name + ")"
	expected := ""
	if dk != nil {
		expected = mail.DKIMRecord(dk.Key)
		c.Details = append(c.Details, "Podepisuje aplikace: d="+dk.Domain+", s="+dk.Selector+", klíč "+mail.DKIMKeyType(dk.Key))
	}
	txts, err := r.LookupTXT(ctx, name)
	if err != nil && !isNotFound(err) {
		c.Status, c.Message = Info, "DKIM záznam se nepodařilo zjistit (DNS neodpovídá)."
		c.Details = append(c.Details, errText(err))
		return c
	}
	var rec string
	for _, t := range txts {
		if tags := parseTags(t); tags["p"] != "" || strings.Contains(strings.ToLower(t), "v=dkim1") {
			rec = strings.TrimSpace(t)
			break
		}
	}
	if rec == "" {
		c.Status, c.Message = Error, "Záznam "+name+" v DNS neexistuje — podpis nepůjde ověřit."
		if expected != "" {
			c.Hint = "Vytvořte TXT záznam " + name + " s hodnotou: " + expected
		} else {
			c.Hint = "Zkontrolujte selektor (najdete ho v hlavičce DKIM-Signature: s=… přijatého e-mailu) nebo záznam vytvořte podle návodu poskytovatele."
		}
		return c
	}
	c.Details = append(c.Details, "Záznam: "+abbreviate(rec, 120))
	tags := parseTags(rec)
	p := strings.Join(strings.Fields(tags["p"]), "")
	if p == "" {
		c.Status, c.Message = Error, "Klíč v záznamu je prázdný (p=) — klíč byl zneplatněn."
		return c
	}
	if expected != "" {
		want := parseTags(expected)["p"]
		if p != want {
			c.Status, c.Message = Error, "Veřejný klíč v DNS neodpovídá privátnímu klíči, kterým aplikace podepisuje."
			c.Hint = "Nahraďte hodnotu záznamu " + name + " za: " + expected
			return c
		}
		c.Status, c.Message = OK, "Záznam existuje a odpovídá podpisovému klíči aplikace."
		if fromDomain != domain && !strings.HasSuffix(fromDomain, "."+domain) {
			c.Status = Warning
			c.Message += " Doména podpisu " + domain + " se ale neshoduje s doménou odesílatele " + fromDomain + ", takže DMARC neprojde."
			c.Hint = "Nastavte NANOFAKTURA_DKIM_DOMAIN na doménu adresy v NANOFAKTURA_MAIL_FROM."
		}
		return c
	}
	desc, status, hint := describeKey(strings.ToLower(defaultStr(tags["k"], "rsa")), p)
	c.Status, c.Message, c.Hint = status, desc, hint
	return c
}

// describeKey validates the p= value of a DKIM record.
func describeKey(kind, p string) (string, Status, string) {
	raw, err := base64.StdEncoding.DecodeString(p)
	if err != nil {
		return "Klíč v záznamu není platné base64 (chyba při kopírování do DNS?).", Error, "Zkopírujte hodnotu záznamu znovu, bez mezer a uvozovek uvnitř."
	}
	switch kind {
	case "ed25519":
		if len(raw) != ed25519.PublicKeySize {
			return "Klíč Ed25519 v záznamu má špatnou délku.", Error, ""
		}
		return "Záznam existuje a obsahuje platný klíč (Ed25519).", OK, ""
	case "rsa":
		pub, err := x509.ParsePKIXPublicKey(raw)
		if err != nil {
			pub, err = x509.ParsePKCS1PublicKey(raw)
		}
		rk, ok := pub.(*rsa.PublicKey)
		if err != nil || !ok {
			return "Klíč v záznamu není platný RSA klíč.", Error, "Zkopírujte hodnotu záznamu znovu z nastavení poskytovatele."
		}
		bits := rk.N.BitLen()
		switch {
		case bits < 1024:
			return fmt.Sprintf("Klíč RSA má jen %d bitů — příjemci ho odmítají.", bits), Error, "Vygenerujte klíč o délce 2048 bitů."
		case bits < 2048:
			return fmt.Sprintf("Záznam existuje, klíč RSA má %d bitů.", bits), Warning, "Doporučená délka je 2048 bitů."
		}
		return fmt.Sprintf("Záznam existuje a obsahuje platný klíč (RSA %d b).", bits), OK, ""
	}
	return "Neznámý typ klíče k=" + kind + ".", Warning, ""
}

func abbreviate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
