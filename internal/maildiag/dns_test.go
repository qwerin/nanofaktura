package maildiag_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/qwerin/nanofaktura/internal/mail"
	"github.com/qwerin/nanofaktura/internal/maildiag"
	"github.com/qwerin/nanofaktura/internal/maildiag/smtptest"
)

var ctx = context.Background()

func spfDNS(records map[string]string) *smtptest.Resolver {
	r := &smtptest.Resolver{TXT: map[string][]string{}, Hosts: map[string][]string{
		"example.cz": {"198.51.100.1"}, "mx1.example.cz": {"198.51.100.25", "2001:db8::25"},
	}, MX: map[string][]*net.MX{"example.cz": {{Host: "mx1.example.cz.", Pref: 10}}}}
	for k, v := range records {
		r.TXT[k] = []string{v}
	}
	return r
}

func TestSPF(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records map[string]string
		ips     []string
		status  maildiag.Status
		want    string
	}{
		{"ip4 pass", map[string]string{"example.cz": "v=spf1 ip4:203.0.113.0/24 -all"}, []string{"203.0.113.10"}, maildiag.OK, "je v SPF povolen"},
		{"include chain", map[string]string{
			"example.cz":              "v=spf1 include:_spf.provider.net ~all",
			"_spf.provider.net":       "v=spf1 include:_netblocks.provider.net include:_other.provider.net ?all",
			"_netblocks.provider.net": "v=spf1 ip4:192.0.2.0/25 ip6:2001:db8:1::/48 -all",
			"_other.provider.net":     "v=spf1 a:relay.provider.net -all",
		}, []string{"2001:db8:1::5"}, maildiag.OK, "je v SPF povolen"},
		{"mx and a with cidr", map[string]string{"example.cz": "v=spf1 a/24 mx -all"}, []string{"198.51.100.200", "2001:db8::25"}, maildiag.OK, "povolen"},
		{"redirect", map[string]string{"example.cz": "v=spf1 redirect=_spf.example.net", "_spf.example.net": "v=spf1 ip4:203.0.113.10 -all"},
			[]string{"203.0.113.10"}, maildiag.OK, "povolen"},
		{"not covered", map[string]string{"example.cz": "v=spf1 ip4:192.0.2.1 -all"}, []string{"203.0.113.10"}, maildiag.Warning, "není v SPF povolena"},
		{"macro uncertain", map[string]string{"example.cz": "v=spf1 exists:%{i}.spf.example.cz -all"}, []string{"203.0.113.10"}, maildiag.Info, "nelze ověřit"},
		{"include without record", map[string]string{"example.cz": "v=spf1 include:gone.example.net -all"}, []string{"203.0.113.10"}, maildiag.Error, "permerror"},
		{"missing", map[string]string{}, nil, maildiag.Warning, "nemá SPF"},
		{"plus all", map[string]string{"example.cz": "v=spf1 +all"}, []string{"203.0.113.10"}, maildiag.Error, "kdokoli"},
		{"no all", map[string]string{"example.cz": "v=spf1 ip4:203.0.113.10"}, []string{"203.0.113.10"}, maildiag.Warning, "nemá koncové"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := spfDNS(tc.records)
			r.Hosts["relay.provider.net"] = []string{"192.0.2.200"}
			c := maildiag.CheckSPF(ctx, r, "example.cz", tc.ips)
			if c.Status != tc.status || !strings.Contains(c.Message, tc.want) {
				t.Fatalf("got %s %q (hint %q, details %v)", c.Status, c.Message, c.Hint, c.Details)
			}
		})
	}

	// two v=spf1 records
	r := spfDNS(nil)
	r.TXT["example.cz"] = []string{"v=spf1 -all", "v=spf1 ip4:1.2.3.4 -all"}
	if c := maildiag.CheckSPF(ctx, r, "example.cz", nil); c.Status != maildiag.Error || !strings.Contains(c.Message, "více SPF") {
		t.Fatalf("multiple: %+v", c)
	}
	// temporary DNS failure → cannot verify, not an error
	r.Fail = map[string]bool{"example.cz": true}
	if c := maildiag.CheckSPF(ctx, r, "example.cz", nil); c.Status != maildiag.Info {
		t.Fatalf("dns failure: %+v", c)
	}
}

func TestSPFLookupLimit(t *testing.T) {
	recs := map[string]string{}
	var incs []string
	for i := range 11 {
		name := fmt.Sprintf("_s%d.provider.net", i)
		incs = append(incs, "include:"+name)
		recs[name] = "v=spf1 ip4:192.0.2." + fmt.Sprint(i) + " -all"
	}
	recs["example.cz"] = "v=spf1 " + strings.Join(incs, " ") + " -all"
	c := maildiag.CheckSPF(ctx, spfDNS(recs), "example.cz", []string{"192.0.2.10"})
	if c.Status != maildiag.Error || !strings.Contains(c.Message, "11 DNS dotazů") {
		t.Fatalf("limit: %+v", c)
	}
	// nested includes count too
	recs = map[string]string{"example.cz": "v=spf1 include:a.net include:b.net -all",
		"a.net": "v=spf1 a mx include:c.net -all", "b.net": "v=spf1 a mx a:x.net mx:y.net -all", "c.net": "v=spf1 a mx ptr exists:z.net -all"}
	c = maildiag.CheckSPF(ctx, spfDNS(recs), "example.cz", nil)
	if c.Status != maildiag.Error || !strings.Contains(strings.Join(c.Details, " "), "13 z 10") {
		t.Fatalf("nested: %+v", c)
	}
	recs["b.net"] = "v=spf1 -all"
	if c = maildiag.CheckSPF(ctx, spfDNS(recs), "example.cz", nil); strings.Contains(c.Message, "limit") {
		t.Fatalf("9 lookups flagged: %+v", c)
	}
}

func TestDMARC(t *testing.T) {
	r := &smtptest.Resolver{TXT: map[string][]string{"_dmarc.example.cz": {"v=DMARC1; p=reject; rua=mailto:d@example.cz"}}}
	if c := maildiag.CheckDMARC(ctx, r, "example.cz"); c.Status != maildiag.OK || !strings.Contains(c.Message, "p=reject") {
		t.Fatalf("reject: %+v", c)
	}
	// subdomain falls back to the organizational domain
	if c := maildiag.CheckDMARC(ctx, r, "faktury.example.cz"); c.Status != maildiag.OK || !strings.Contains(strings.Join(c.Details, " "), "nadřazené") {
		t.Fatalf("parent: %+v", c)
	}
	r.TXT["_dmarc.example.cz"] = []string{"v=DMARC1; p=none"}
	if c := maildiag.CheckDMARC(ctx, r, "example.cz"); c.Status != maildiag.Info || !strings.Contains(c.Hint, "rua=") {
		t.Fatalf("none: %+v", c)
	}
	r.TXT["_dmarc.example.cz"] = []string{"v=DMARC1; rua=mailto:x@example.cz"}
	if c := maildiag.CheckDMARC(ctx, r, "example.cz"); c.Status != maildiag.Error {
		t.Fatalf("no policy: %+v", c)
	}
	if c := maildiag.CheckDMARC(ctx, r, "jina.cz"); c.Status != maildiag.Warning || !strings.Contains(c.Hint, "_dmarc.jina.cz") {
		t.Fatalf("missing: %+v", c)
	}
}

func TestDKIMRecordCheck(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	dk := &mail.DKIMConfig{Domain: "example.cz", Selector: "nf", Key: key}
	r := &smtptest.Resolver{TXT: map[string][]string{"nf._domainkey.example.cz": {mail.DKIMRecord(key)}}}

	if c := maildiag.CheckDKIM(ctx, r, "example.cz", dk, ""); c.Status != maildiag.OK || !strings.Contains(c.Message, "odpovídá") {
		t.Fatalf("match: %+v", c)
	}
	// From on another domain → DMARC alignment fails
	if c := maildiag.CheckDKIM(ctx, r, "jina.cz", dk, ""); c.Status != maildiag.Warning {
		t.Fatalf("alignment: %+v", c)
	}
	r.TXT["nf._domainkey.example.cz"] = []string{mail.DKIMRecord(other)}
	if c := maildiag.CheckDKIM(ctx, r, "example.cz", dk, ""); c.Status != maildiag.Error || !strings.Contains(c.Message, "neodpovídá") ||
		!strings.Contains(c.Hint, mail.DKIMRecord(key)) {
		t.Fatalf("mismatch: %+v", c)
	}

	// no app signing: user-supplied selector of the provider
	der, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	r.TXT["google._domainkey.example.cz"] = []string{"v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)}
	r.TXT["ed._domainkey.example.cz"] = []string{"v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub)}
	r.TXT["bad._domainkey.example.cz"] = []string{"v=DKIM1; k=rsa; p=Zm9v"}
	r.TXT["revoked._domainkey.example.cz"] = []string{"v=DKIM1; p="}
	for sel, want := range map[string]maildiag.Status{"google": maildiag.OK, "ed": maildiag.OK, "bad": maildiag.Error,
		"revoked": maildiag.Error, "missing": maildiag.Error} {
		if c := maildiag.CheckDKIM(ctx, r, "example.cz", nil, sel); c.Status != want {
			t.Fatalf("selector %s: %+v", sel, c)
		}
	}
	if c := maildiag.CheckDKIM(ctx, r, "example.cz", nil, ""); c.Status != maildiag.Info {
		t.Fatalf("not configured: %+v", c)
	}
	if !maildiag.ValidSelector("google") || !maildiag.ValidSelector("s1.2026") || maildiag.ValidSelector("a b") || maildiag.ValidSelector("../x") {
		t.Fatal("ValidSelector")
	}
}
