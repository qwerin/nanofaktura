package ttlcache

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := New[string, int](time.Hour, func() time.Time { return now })
	if _, ok := c.Get("a"); ok {
		t.Fatal("empty cache hit")
	}
	c.Set("a", 1)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("got %v %v", v, ok)
	}
	now = now.Add(59 * time.Minute)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expired too early")
	}
	now = now.Add(time.Minute)
	if _, ok := c.Get("a"); ok {
		t.Fatal("not expired after TTL")
	}
}

func TestCacheBounded(t *testing.T) {
	c := New[int, int](time.Hour, nil)
	for i := 0; i < maxEntries+10; i++ {
		c.Set(i, i)
	}
	if len(c.entries) > maxEntries {
		t.Fatalf("cache grew to %d", len(c.entries))
	}
	if v, ok := c.Get(maxEntries + 9); !ok || v != maxEntries+9 {
		t.Fatal("latest entry missing")
	}
}
