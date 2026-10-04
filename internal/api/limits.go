package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/httpsec"
	"github.com/qwerin/nanofaktura/internal/ratelimit"
)

// limits are the in-process rate limits (SPEC §5 NANOFAKTURA_DISABLE_RATE_LIMIT).
// Keys: client IP (httpsec.InfoFrom; X-Forwarded-For only from trusted
// proxies), e-mail, user or account ID.
type limits struct {
	loginIP    *ratelimit.Limiter // every login attempt per IP
	loginEmail *ratelimit.Limiter // failed logins per e-mail
	register   *ratelimit.Limiter // registrations per IP
	invitation *ratelimit.Limiter // invitation lookups/accepts per IP
	public     *ratelimit.Limiter // public invoice link requests per IP
	password   *ratelimit.Limiter // wrong current passwords per user
	mail       *ratelimit.Limiter // user-triggered e-mails per account
	export     *ratelimit.Limiter // heavy exports (PDF ZIP, backup) per account
	resetMail  *ratelimit.Limiter // password reset requests per IP and per e-mail
	resetLink  *ratelimit.Limiter // password reset link lookups/confirms per IP
	twoFactor  *ratelimit.Limiter // wrong second factors (login, TOTP enable) per user
	verifyMail *ratelimit.Limiter // e-mail verification links sent per user
	verifyLink *ratelimit.Limiter // verification link lookups/confirms per IP
	emailTest  *ratelimit.Limiter // SMTP diagnostics (test e-mails) per instance admin
	mcpAuth    *ratelimit.Limiter // invalid API tokens on the MCP endpoint per IP
}

func newLimits(now func() time.Time) limits {
	return limits{
		loginIP:    ratelimit.New(20, 2, time.Minute, now),
		loginEmail: ratelimit.New(5, 5, 15*time.Minute, now),
		register:   ratelimit.New(5, 10, time.Hour, now),
		invitation: ratelimit.New(20, 20, 10*time.Minute, now),
		public:     ratelimit.New(60, 60, time.Minute, now),
		password:   ratelimit.New(5, 5, 15*time.Minute, now),
		mail:       ratelimit.New(50, 50, time.Hour, now),
		export:     ratelimit.New(10, 10, time.Hour, now),
		resetMail:  ratelimit.New(5, 5, time.Hour, now),
		resetLink:  ratelimit.New(20, 20, 10*time.Minute, now),
		twoFactor:  ratelimit.New(10, 10, time.Hour, now),
		verifyMail: ratelimit.New(3, 3, time.Hour, now),
		verifyLink: ratelimit.New(20, 20, 10*time.Minute, now),
		emailTest:  ratelimit.New(10, 10, time.Hour, now),
		mcpAuth:    ratelimit.New(20, 20, 10*time.Minute, now),
	}
}

// clientIP of the current request ("" outside HTTP, e.g. jobs).
func clientIP(ctx context.Context) string { return httpsec.InfoFrom(ctx).IP }

// rateLimit takes one token of key from l → 429 rate_limited with
// Retry-After when exhausted.
func (s *server) rateLimit(l *ratelimit.Limiter, key string) error {
	if s.cfg.DisableRateLimit {
		return nil
	}
	if ok, wait := l.Allow(key); !ok {
		return tooManyRequests(wait)
	}
	return nil
}

// rateBlocked is rateLimit without taking a token (limits counting failures only).
func (s *server) rateBlocked(l *ratelimit.Limiter, key string) error {
	if s.cfg.DisableRateLimit {
		return nil
	}
	if blocked, wait := l.Blocked(key); blocked {
		return tooManyRequests(wait)
	}
	return nil
}

// rateFail records a failure of key (see rateBlocked).
func (s *server) rateFail(l *ratelimit.Limiter, key string) {
	if !s.cfg.DisableRateLimit {
		l.Allow(key)
	}
}

// acquireExport reserves a heavy export (PDF ZIP, backup) of the current
// account: one at a time per account, a few per instance, and at most
// limits.export per hour → 429 export_in_progress / rate_limited. release
// must be called when the export ends (also when it fails).
func (s *server) acquireExport(ctx context.Context) (release func(), err error) {
	id := auth.AccountFrom(ctx).ID
	if _, busy := s.exportBusy.LoadOrStore(id, struct{}{}); busy {
		return nil, apiError(http.StatusTooManyRequests, CodeExportBusy, "another export of this account is running; try again when it finishes")
	}
	select {
	case s.exportSlots <- struct{}{}:
	default:
		s.exportBusy.Delete(id)
		return nil, huma.ErrorWithHeaders(apiError(http.StatusTooManyRequests, CodeExportBusy, "too many exports are running; try again in a minute"),
			http.Header{"Retry-After": {"60"}})
	}
	if err := s.rateLimit(s.limits.export, accountKey(ctx)); err != nil {
		<-s.exportSlots
		s.exportBusy.Delete(id)
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			<-s.exportSlots
			s.exportBusy.Delete(id)
		})
	}, nil
}

// slowUpload is the operation option of large uploads: the body may take d
// to arrive and the response deadline (server WriteTimeout) moves with it.
func slowUpload(d time.Duration) func(*huma.Operation) {
	return func(o *huma.Operation) {
		o.BodyReadTimeout = d
		o.Middlewares = append(o.Middlewares, func(hc huma.Context, next func(huma.Context)) {
			extendWriteDeadline(hc, d+5*time.Minute)
			next(hc)
		})
	}
}

// extendWriteDeadline pushes the server's WriteTimeout of a streamed
// response forward (long exports write for minutes).
func extendWriteDeadline(hc huma.Context, d time.Duration) {
	_, w := humachi.Unwrap(hc)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
}

func tooManyRequests(wait time.Duration) error {
	secs := int(math.Ceil(wait.Seconds()))
	return huma.ErrorWithHeaders(
		apiError(http.StatusTooManyRequests, CodeRateLimited, fmt.Sprintf("too many requests; retry in %d s", secs)),
		http.Header{"Retry-After": {strconv.Itoa(secs)}})
}
