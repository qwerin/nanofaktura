// Package webhooks does the HTTP side of webhook delivery (SPEC §7.10):
// signing, the SSRF-safe client, one POST attempt and the retry schedule.
// Queueing lives in internal/events, the delivery job in internal/api.
//
// Receivers verify a delivery by computing HMAC-SHA256 of the raw body with
// the webhook secret and comparing it with the X-NanoFaktura-Signature
// header ("sha256=<hex>"), e.g. webhooks.Verify(secret, body, header).
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Headers of a delivery.
const (
	HeaderEvent     = "X-NanoFaktura-Event"
	HeaderDelivery  = "X-NanoFaktura-Delivery"
	HeaderSignature = "X-NanoFaktura-Signature"
)

// Timeout of one delivery attempt.
const Timeout = 10 * time.Second

// RetryDelays: after the n-th failed attempt (n = 1…5) the next one follows
// RetryDelays[n-1] later; a delivery failing 1+len(RetryDelays) times is failed.
var RetryDelays = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}

// MaxConsecutiveFailures finally failed deliveries in a row disable a webhook.
const MaxConsecutiveFailures = 20

// ResponseSnippet is how much of the response body is kept.
const ResponseSnippet = 1024

// NextAttempt returns when to retry after `attempts` failed attempts, or
// ok=false when the delivery has failed for good.
func NextAttempt(attempts int, now time.Time) (time.Time, bool) {
	if attempts < 1 || attempts > len(RetryDelays) {
		return time.Time{}, false
	}
	return now.Add(RetryDelays[attempts-1]), true
}

// NewSecret returns a random signing secret ("whsec_" + 48 hex chars).
func NewSecret() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "whsec_" + hex.EncodeToString(b)
}

// Sign returns the signature header value of body: "sha256=<hex HMAC>".
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// Verify checks a signature header value in constant time.
func Verify(secret string, body []byte, signature string) bool {
	return hmac.Equal([]byte(Sign(secret, body)), []byte(signature))
}

// Host returns scheme://host[:port] of a webhook URL (no path, query or
// credentials), for logs and events; "" when unparsable.
func Host(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// ErrForbiddenAddress: the target resolves to a private/loopback/link-local address.
var ErrForbiddenAddress = errors.New("webhook target resolves to a private, loopback or link-local address")

// forbidden reports whether ip must not be called (SSRF protection): every
// IANA special-purpose range (loopback, private, CGNAT, link-local,
// documentation, benchmarking, multicast, reserved …); for IPv6 transition
// addresses (NAT64 64:ff9b::/96, 6to4 2002::/16) the embedded IPv4 decides.
func forbidden(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.Is6() {
		b := ip.As16()
		switch {
		case nat64.Contains(ip):
			return forbidden(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
		case sixToFour.Contains(ip):
			return forbidden(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
		}
	}
	for _, p := range specialPurpose {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

var (
	nat64     = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour = netip.MustParsePrefix("2002::/16")

	// specialPurpose: IANA IPv4/IPv6 special-purpose address registries
	// (not globally reachable or not meant for general traffic).
	specialPurpose = func() []netip.Prefix {
		var out []netip.Prefix
		for _, s := range []string{
			"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
			"192.0.0.0/24", "192.0.2.0/24", "192.31.196.0/24", "192.52.193.0/24", "192.88.99.0/24", "192.168.0.0/16",
			"192.175.48.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
			"::/128", "::1/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32",
			"3fff::/20", "5f00::/16", "fc00::/7", "fe80::/10", "fec0::/10", "ff00::/8",
		} {
			out = append(out, netip.MustParsePrefix(s))
		}
		return out
	}()
)

// allowedPort: webhooks go to 80, 443 or unprivileged ports (not to
// SSH, SMTP, databases … on port numbers below 1024).
func allowedPort(port string) bool {
	if port == "" {
		return true
	}
	n, err := strconv.Atoi(port)
	return err == nil && (n == 80 || n == 443 || (n >= 1024 && n <= 65535))
}

// CheckURL validates a webhook URL: absolute http(s) without credentials
// and, unless allowPrivate, not resolving to a forbidden address (resolution
// errors are reported as well). The dialer of NewClient checks again at
// connect time (DNS may change).
func CheckURL(ctx context.Context, raw string, allowPrivate bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return errors.New("must be an absolute http:// or https:// URL")
	}
	if u.User != nil {
		return errors.New("must not contain credentials")
	}
	if allowPrivate {
		return nil
	}
	if !allowedPort(u.Port()) {
		return errors.New("port must be 80, 443 or 1024–65535")
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		if forbidden(ip) {
			return ErrForbiddenAddress
		}
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(rctx, "ip", host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %s", host)
	}
	for _, ip := range ips {
		if forbidden(ip) {
			return ErrForbiddenAddress
		}
	}
	return nil
}

// NewClient returns the delivery HTTP client: Timeout, no redirects (a 3xx
// is a failed attempt), no proxy, and unless allowPrivate a dialer refusing
// forbidden addresses after DNS resolution.
func NewClient(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: Timeout}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || forbidden(ap.Addr()) || !allowedPort(strconv.Itoa(int(ap.Port()))) {
				return ErrForbiddenAddress
			}
			return nil
		}
	}
	tr := &http.Transport{DialContext: d.DialContext, TLSHandshakeTimeout: Timeout, MaxIdleConns: 10, IdleConnTimeout: time.Minute}
	return &http.Client{
		Timeout:       Timeout,
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Result of one attempt. OK = 2xx response.
type Result struct {
	OK       bool
	Status   int // 0 = no response
	Body     string
	Err      string
	Duration time.Duration
}

// Post sends one delivery attempt of body.
func Post(ctx context.Context, client *http.Client, target, secret, event string, deliveryID uint, body []byte) Result {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Result{Err: err.Error(), Duration: time.Since(start)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NanoFaktura-Webhook/1")
	req.Header.Set(HeaderEvent, event)
	req.Header.Set(HeaderDelivery, strconv.FormatUint(uint64(deliveryID), 10))
	req.Header.Set(HeaderSignature, Sign(secret, body))
	res, err := client.Do(req)
	if err != nil {
		msg := err.Error()
		var ue *url.Error
		if errors.As(err, &ue) {
			msg = ue.Err.Error() // without the URL (it may carry a secret)
		}
		if errors.Is(err, ErrForbiddenAddress) {
			msg = ErrForbiddenAddress.Error()
		}
		return Result{Err: msg, Duration: time.Since(start)}
	}
	defer res.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(res.Body, ResponseSnippet))
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	r := Result{Status: res.StatusCode, Body: string(snippet), Duration: time.Since(start)}
	r.OK = res.StatusCode >= 200 && res.StatusCode < 300
	if !r.OK {
		r.Err = "HTTP " + strconv.Itoa(res.StatusCode)
	}
	return r
}
