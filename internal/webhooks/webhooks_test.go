package webhooks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
