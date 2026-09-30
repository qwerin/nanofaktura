// Package ratelimit is a small in-process keyed token-bucket limiter (per IP,
// per e-mail, per account …). State lives in memory: it resets on restart
// and is not shared between instances, which is enough for a single
// self-hosted process.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter allows Burst events at once per key, refilled at Per/Every.
type Limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	now     func() time.Time
	buckets map[string]*bucket
	calls   int
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a limiter allowing burst events per key at once and then n
// events per every (e.g. New(5, 5, time.Minute, nil): 5 at once, then 5/min).
// now defaults to time.Now.
func New(burst, n int, every time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{rate: float64(n) / every.Seconds(), burst: float64(burst), now: now, buckets: map[string]*bucket{}}
}

// Allow takes one token of key. When none is left it returns false and how
// long until the next token.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b := l.fill(key, now)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration(math.Ceil((1-b.tokens)/l.rate)) * time.Second
	return false, max(wait, time.Second)
}

// Blocked reports whether key has no token left (without taking one).
func (l *Limiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.fill(key, now)
	if b.tokens >= 1 {
		return false, 0
	}
	return true, max(time.Duration(math.Ceil((1-b.tokens)/l.rate))*time.Second, time.Second)
}

// Reset forgets key (e.g. after a successful login).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}

func (l *Limiter) fill(key string, now time.Time) *bucket {
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
		return b
	}
	if d := now.Sub(b.last).Seconds(); d > 0 {
		b.tokens = min(l.burst, b.tokens+d*l.rate)
	}
	b.last = now
	return b
}

// sweep drops full buckets now and then so memory stays bounded by the keys
// active within one refill period.
func (l *Limiter) sweep(now time.Time) {
	l.calls++
	if l.calls%1024 != 0 {
		return
	}
	full := time.Duration(l.burst / l.rate * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= full {
			delete(l.buckets, k)
		}
	}
}
