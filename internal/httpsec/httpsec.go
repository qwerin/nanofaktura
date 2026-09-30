// Package httpsec holds the HTTP hardening shared by the API and the SPA
// server: client IP / HTTPS detection behind trusted proxies, security
// response headers (CSP, HSTS …) and CSRF protection for cookie sessions.
package httpsec

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseProxies parses NANOFAKTURA_TRUSTED_PROXIES: comma separated IPs or
// CIDR prefixes ("" = none).
func ParseProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if p, err := netip.ParsePrefix(f); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(f)
		if err != nil {
			return nil, fmt.Errorf("invalid IP or CIDR %q", f)
		}
		out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
	}
	return out, nil
}

// Info is what the middleware learned about the request.
type Info struct {
	IP    string // client IP (X-Forwarded-For only from trusted proxies)
	HTTPS bool   // TLS, or X-Forwarded-Proto: https from a trusted proxy
}

type infoKey struct{}

// InfoFrom returns the request info stored by Middleware (zero without it).
func InfoFrom(ctx context.Context) Info {
	i, _ := ctx.Value(infoKey{}).(Info)
	return i
}

// WithInfo stores i in ctx (tests, custom servers).
func WithInfo(ctx context.Context, i Info) context.Context {
	return context.WithValue(ctx, infoKey{}, i)
}

func trusted(a netip.Addr, proxies []netip.Prefix) bool {
	a = a.Unmap()
	for _, p := range proxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

// RequestInfo computes Info. Forwarded headers are honoured only when the
// direct peer is a trusted proxy; X-Forwarded-For is walked from the right
// skipping trusted hops, so a client cannot spoof its address by sending
// the header itself.
func RequestInfo(r *http.Request, proxies []netip.Prefix) Info {
	peer := remoteAddr(r)
	info := Info{IP: peer.String(), HTTPS: r.TLS != nil}
	if !peer.IsValid() || !trusted(peer, proxies) {
		return info
	}
	if p := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); p == "https" {
		info.HTTPS = true
	}
	var hops []string
	for _, h := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(h, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		info.IP = a.Unmap().String()
		if !trusted(a, proxies) {
			break
		}
	}
	return info
}

// Options configure Middleware.
type Options struct {
	TrustedProxies []netip.Prefix
	// PublicHTTPS: NANOFAKTURA_PUBLIC_URL is https (HSTS on every response).
	PublicHTTPS bool
}

// SPA content security policy. The built app needs only same-origin scripts
// (no inline script; the theme bootstrap is /theme-init.js), inline style
// attributes (React style props), data:/blob: images (QR, previews) and blob:
// frames (PDF preview of a downloaded blob).
const spaCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-src 'self' blob:; " +
	"worker-src 'self'; manifest-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// apiCSP applies to API responses: nothing renders there except files, which
// may be framed by the app itself (PDF preview) but never by other sites.
const apiCSP = "frame-ancestors 'self'"

// PermissionsPolicy disables browser features the app never uses.
const PermissionsPolicy = "camera=(), microphone=(), geolocation=(), payment=(), usb=(), serial=(), bluetooth=(), " +
	"magnetometer=(), gyroscope=(), accelerometer=(), browsing-topics=()"

// Middleware stores Info in the request context and sets the security
// headers. Handlers may override them (e.g. attachment downloads set a
// sandbox CSP). Idempotent: wrapping twice sets the same values.
func Middleware(o Options) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := RequestInfo(r, o.TrustedProxies)
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Permissions-Policy", PermissionsPolicy)
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if IsAPI(r.URL.Path) {
				h.Set("Content-Security-Policy", apiCSP)
				h.Set("X-Frame-Options", "SAMEORIGIN")
				h.Set("Referrer-Policy", "no-referrer")
				if strings.HasPrefix(r.URL.Path, "/api/public/") {
					h.Set("Cache-Control", "private, no-store") // invoice data behind a secret link
				}
			} else {
				h.Set("Content-Security-Policy", spaCSP)
				h.Set("X-Frame-Options", "DENY")
				if strings.HasPrefix(r.URL.Path, "/p/") || strings.HasPrefix(r.URL.Path, "/invite/") {
					h.Set("Referrer-Policy", "no-referrer") // the URL carries a token
				} else {
					h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
				}
			}
			if info.HTTPS || o.PublicHTTPS {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r.WithContext(WithInfo(r.Context(), info)))
		})
	}
}

// IsAPI reports whether path is served by the API.
func IsAPI(path string) bool { return path == "/api" || strings.HasPrefix(path, "/api/") }

// CSRF protects cookie-authenticated state-changing requests:
//
//   - requests carrying an Authorization header (API tokens) are exempt —
//     browsers cannot attach it cross-site without a CORS preflight, which
//     the API never grants;
//   - otherwise the browser's Sec-Fetch-Site / Origin must say same-origin
//     (http.CrossOriginProtection; requests without either header, i.e.
//     non-browser clients, pass);
//   - a body must declare its Content-Type (JSON or multipart), so an
//     "untyped" cross-site body is never parsed as JSON.
func CSRF(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			if err := cop.Check(r); err != nil {
				WriteProblem(w, http.StatusForbidden, "cross_origin_request", "cross-origin request rejected")
				return
			}
		}
		if r.ContentLength != 0 && r.Header.Get("Content-Type") == "" {
			WriteProblem(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "request body without Content-Type")
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "" {
			mt, _, err := mime.ParseMediaType(ct)
			if err != nil || !(mt == "application/json" || strings.HasSuffix(mt, "+json") || mt == "multipart/form-data") {
				WriteProblem(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json or multipart/form-data")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// WriteProblem writes an RFC 9457 problem+json body in the API error format.
func WriteProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title": http.StatusText(status), "status": status, "detail": detail, "code": code,
	})
}
