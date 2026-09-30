package httpsec

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRF(t *testing.T) {
	h := CSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	cases := []struct {
		name   string
		method string
		hdr    map[string]string
		body   string
		status int
		code   string
	}{
		{"GET cross-site passes", "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, "", 204, ""},
		{"HEAD passes", "HEAD", nil, "", 204, ""},
		{"OPTIONS passes", "OPTIONS", map[string]string{"Sec-Fetch-Site": "cross-site"}, "", 204, ""},
		{"same-origin JSON", "POST", map[string]string{"Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}, "{}", 204, ""},
		{"non-browser JSON", "POST", map[string]string{"Content-Type": "application/json; charset=utf-8"}, "{}", 204, ""},
		{"problem+json", "PATCH", map[string]string{"Content-Type": "application/merge-patch+json"}, "{}", 204, ""},
		{"multipart", "POST", map[string]string{"Content-Type": "multipart/form-data; boundary=x"}, "--x--", 204, ""},
		{"empty body without type", "DELETE", map[string]string{"Sec-Fetch-Site": "same-origin"}, "", 204, ""},
		{"cross-site cookie request", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"}, "{}", 403, "cross_origin_request"},
		{"cross-site with bearer", "POST", map[string]string{"Sec-Fetch-Site": "cross-site", "Authorization": "Bearer nf_x", "Content-Type": "application/json"}, "{}", 204, ""},
		{"foreign Origin", "DELETE", map[string]string{"Origin": "https://evil.example", "Host": "app.example"}, "", 403, "cross_origin_request"},
		{"body without Content-Type", "POST", nil, "{}", 415, "unsupported_media_type"},
		{"form body", "POST", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, "a=b", 415, "unsupported_media_type"},
		{"text/plain", "PUT", map[string]string{"Content-Type": "text/plain"}, "{}", 415, "unsupported_media_type"},
		{"garbage Content-Type", "POST", map[string]string{"Content-Type": ";;;"}, "{}", 415, "unsupported_media_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			var r *http.Request
			if body != nil {
				r = httptest.NewRequest(tc.method, "http://app.example/api/x", body)
			} else {
				r = httptest.NewRequest(tc.method, "http://app.example/api/x", nil)
			}
			for k, v := range tc.hdr {
				if k == "Host" {
					r.Host = v
					continue
				}
				r.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d (%s)", rec.Code, tc.status, rec.Body)
			}
			if tc.code == "" {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("content type %q", ct)
			}
			var p struct {
				Status int    `json:"status"`
				Code   string `json:"code"`
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Code != tc.code || p.Status != tc.status || p.Title == "" || p.Detail == "" {
				t.Fatalf("problem %+v %v", p, err)
			}
		})
	}
}

func TestInfoRoundtrip(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if (InfoFrom(r.Context()) != Info{}) {
		t.Fatal("zero info expected without middleware")
	}
	var got Info
	Middleware(Options{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = InfoFrom(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), r)
	if got.IP != "192.0.2.1" || got.HTTPS {
		t.Fatalf("info %+v", got)
	}
	// RemoteAddr without a port
	r.RemoteAddr = "2001:db8::1"
	if a := remoteAddr(r); a.String() != "2001:db8::1" {
		t.Fatalf("remoteAddr %v", a)
	}
}

func TestPublicAPINoStore(t *testing.T) {
	h := Middleware(Options{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for path, want := range map[string]string{"/api/public/invoices/x": "private, no-store", "/api/accounts": "", "/invite/tok": ""} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control %q, want %q", path, got, want)
		}
	}
	if !IsAPI("/api") || IsAPI("/apix") || IsAPI("/") {
		t.Fatal("IsAPI")
	}
}
