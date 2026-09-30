package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New(3, 1, time.Minute, func() time.Time { return now })
	for i := range 3 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("attempt %d refused", i)
		}
	}
	ok, wait := l.Allow("a")
	if ok || wait != time.Minute {
		t.Fatalf("4th: %v %v", ok, wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("other key refused")
	}
	if blocked, _ := l.Blocked("a"); !blocked {
		t.Fatal("a not blocked")
	}
	now = now.Add(30 * time.Second)
	if ok, wait := l.Allow("a"); ok || wait != 30*time.Second {
		t.Fatalf("after 30 s: %v %v", ok, wait)
	}
	now = now.Add(31 * time.Second)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("refilled token refused")
	}
	l.Reset("a")
	if blocked, _ := l.Blocked("a"); blocked {
		t.Fatal("reset key blocked")
	}
}

func TestSweep(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New(1, 1, time.Second, func() time.Time { return now })
	for i := range 2000 {
		now = now.Add(time.Second)
		l.Allow(string(rune(i)))
	}
	if len(l.buckets) > 1100 {
		t.Fatalf("%d buckets kept", len(l.buckets))
	}
}
