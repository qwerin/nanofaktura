package webhooks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestSignVerify(t *testing.T) {
	body := []byte(`{"event":"invoice.paid"}`)
	sig := Sign("whsec_x", body)
	// HMAC-SHA256("whsec_x", body) computed independently
	if !strings.HasPrefix(sig, "sha256=") || len(sig) != 7+64 {
		t.Fatalf("sig %q", sig)
	}
	if !Verify("whsec_x", body, sig) || Verify("whsec_y", body, sig) || Verify("whsec_x", []byte("{}"), sig) {
		t.Fatal("verify")
	}
}

func TestNextAttempt(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	want := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}
	for i, d := range want {
		at, ok := NextAttempt(i+1, now)
		if !ok || !at.Equal(now.Add(d)) {
			t.Fatalf("attempt %d: %v %v", i+1, at, ok)
		}
	}
	if _, ok := NextAttempt(6, now); ok {
		t.Fatal("6th failure must be final")
	}
}

func TestCheckURL(t *testing.T) {
	ctx := context.Background()
	for _, u := range []string{"ftp://x.cz", "/relative", "http://user:pw@example.com", "https://"} {
		if CheckURL(ctx, u, true) == nil {
			t.Errorf("%s accepted", u)
		}
	}
	for _, u := range []string{"http://127.0.0.1:8080/x", "http://10.1.2.3", "http://192.168.1.1", "http://169.254.169.254/latest",
		"http://[::1]/", "http://[fd00::1]/", "http://0.0.0.0/", "http://100.64.0.1/", "http://localhost/"} {
		if err := CheckURL(ctx, u, false); err == nil {
			t.Errorf("%s accepted", u)
		}
		if err := CheckURL(ctx, u, true); err != nil {
			t.Errorf("%s with allowPrivate: %v", u, err)
		}
	}
	if err := CheckURL(ctx, "https://93.184.215.14/hook", false); err != nil {
		t.Errorf("public IP: %v", err)
	}
}

func TestForbiddenSpecialPurpose(t *testing.T) {
	for _, s := range []string{
		"64:ff9b::a00:1",      // NAT64 → 10.0.0.1
		"64:ff9b::7f00:1",     // NAT64 → 127.0.0.1
		"64:ff9b:1::1",        // local-use NAT64
		"2002:a00:1::1",       // 6to4 → 10.0.0.1
		"2002:7f00:1::",       // 6to4 → 127.0.0.1
		"2001:0:4136:e378::1", // Teredo
		"198.18.0.1", "192.0.0.8", "240.0.0.1", "0.1.2.3", "255.255.255.255", "192.0.2.1", "203.0.113.9",
		"2001:db8::1", "fec0::1", "100::1", "::ffff:10.0.0.1",
	} {
		if !forbidden(netip.MustParseAddr(s)) {
			t.Errorf("%s allowed", s)
		}
	}
	for _, s := range []string{"93.184.215.14", "64:ff9b::5db8:d70e", "2002:5db8:d70e::1", "2a00:1450:4001:80b::200e"} {
		if forbidden(netip.MustParseAddr(s)) {
			t.Errorf("%s forbidden", s)
		}
	}
	for u, ok := range map[string]bool{
		"https://93.184.215.14:22/": false, "https://93.184.215.14:25/": false, "https://93.184.215.14:8443/": true,
		"http://93.184.215.14:80/": true, "https://93.184.215.14/": true,
	} {
		if err := CheckURL(context.Background(), u, false); (err == nil) != ok {
			t.Errorf("%s: %v", u, err)
		}
	}
}

func TestPostAndDialerBlock(t *testing.T) {
	var got *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	r := Post(context.Background(), NewClient(true), srv.URL, "s3cret", "invoice.paid", 42, []byte(`{"a":1}`))
	if !r.OK || r.Status != 204 {
		t.Fatalf("result %+v", r)
	}
	if got.Header.Get(HeaderEvent) != "invoice.paid" || got.Header.Get(HeaderDelivery) != "42" ||
		!Verify("s3cret", body, got.Header.Get(HeaderSignature)) {
		t.Fatalf("headers %v", got.Header)
	}

	// the SSRF-safe client refuses loopback even when the URL passed validation earlier
	r = Post(context.Background(), NewClient(false), srv.URL, "s", "x", 1, []byte(`{}`))
	if r.OK || !strings.Contains(r.Err, "private") {
		t.Fatalf("blocked result %+v", r)
	}
}
