// Package ttlcache is a tiny concurrency-safe in-memory cache with a fixed
// time-to-live, used by the external registry clients (VIES, VAT registry).
package ttlcache

import (
	"sync"
	"time"
)

// maxEntries bounds memory use; when reached, expired entries are purged and,
// if still full, the whole cache is dropped (lookups are cheap to redo).
const maxEntries = 10000

// Cache maps keys to values that expire TTL after being stored.
type Cache[K comparable, V any] struct {
	TTL time.Duration
	Now func() time.Time // defaults to time.Now

	mu      sync.Mutex
	entries map[K]entry[V]
}

type entry[V any] struct {
	val     V
	expires time.Time
}

// New returns a cache with the given TTL; now may be nil (time.Now).
func New[K comparable, V any](ttl time.Duration, now func() time.Time) *Cache[K, V] {
	if now == nil {
		now = time.Now
	}
	return &Cache[K, V]{TTL: ttl, Now: now, entries: map[K]entry[V]{}}
}

// Get returns the value stored under k if it has not expired yet.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok || !c.Now().Before(e.expires) {
		var zero V
		return zero, false
	}
	return e.val, true
}

// Set stores v under k for TTL.
func (c *Cache[K, V]) Set(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.Now()
	if len(c.entries) >= maxEntries {
		for key, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, key)
			}
		}
		if len(c.entries) >= maxEntries {
			c.entries = map[K]entry[V]{}
		}
	}
	c.entries[k] = entry[V]{val: v, expires: now.Add(c.TTL)}
}
