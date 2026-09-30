package httpsec

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseProxies(t *testing.T) {
	p, err := ParseProxies(" 10.0.0.0/8, 192.168.1.5 ,::1")
	if err != nil || len(p) != 3 || p[1].String() != "192.168.1.5/32" || p[2].String() != "::1/128" {
		t.Fatalf("%v %v", p, err)
	}
	if _, err := ParseProxies("nonsense"); err == nil {
		t.Fatal("invalid accepted")
	}
	if p, err := ParseProxies(""); err != nil || len(p) != 0 {
		t.Fatalf("empty: %v %v", p, err)
	}
}

func TestRequestInfo(t *testing.T) {
	proxies, _ := ParseProxies("10.0.0.0/8")
	req := func(remote string, h ...string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for i := 0; i+1 < len(h); i += 2 {
			r.Header.Add(h[i], h[i+1])
		}
		return r
	}
	cases := []struct {
		r     *http.Request
		ip    string
		https bool
	}{
		// untrusted peer: headers ignored
		{req("203.0.113.5:1234", "X-Forwarded-For", "1.2.3.4", "X-Forwarded-Proto", "https"), "203.0.113.5", false},
		// trusted proxy: rightmost untrusted hop
		{req("10.0.0.2:80", "X-Forwarded-For", "6.6.6.6, 198.51.100.7", "X-Forwarded-Proto", "https"), "198.51.100.7", true},
		// chain of trusted proxies
		{req("10.0.0.2:80", "X-Forwarded-For", "198.51.100.7, 10.1.1.1"), "198.51.100.7", false},
		// several header lines
		{req("10.0.0.2:80", "X-Forwarded-For", "6.6.6.6", "X-Forwarded-For", "198.51.100.8"), "198.51.100.8", false},
		// garbage stops the walk
		{req("10.0.0.2:80", "X-Forwarded-For", "junk"), "10.0.0.2", false},
		{req("[::ffff:10.0.0.3]:80", "X-Forwarded-For", "198.51.100.9"), "198.51.100.9", false},
	}
	for i, c := range cases {
		got := RequestInfo(c.r, proxies)
		if got.IP != c.ip || got.HTTPS != c.https {
			t.Errorf("case %d: %+v, want %s %v", i, got, c.ip, c.https)
		}
	}
	r := req("203.0.113.5:1")
	r.TLS = &tls.ConnectionState{}
	if !RequestInfo(r, nil).HTTPS {
		t.Error("TLS not detected")
	}
}

func TestMiddlewareHeaders(t *testing.T) {
	h := Middleware(Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for path, want := range map[string]string{"/": spaCSP, "/a/firma/invoices": spaCSP, "/api/auth/me": apiCSP} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("%s: CSP %q", path, got)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: %v", path, rec.Header())
		}
	}
	if !strings.Contains(spaCSP, "frame-ancestors 'none'") || strings.Contains(spaCSP, "unsafe-eval") ||
		strings.Contains(spaCSP, "script-src 'self' 'unsafe-inline'") {
		t.Fatal("spa CSP too weak")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/p/tokentoken", nil))
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("public link referrer: %v", rec.Header())
	}
	rec = httptest.NewRecorder()
	Middleware(Options{PublicHTTPS: true})(h).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("no HSTS for an https public URL")
	}
}
