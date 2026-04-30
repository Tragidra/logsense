package analyze

import (
	"sync"
	"time"

	"github.com/Tragidra/loglens/model"
)

type cacheKey struct {
	fingerprint string
	windowStart int64 // unix seconds
}

type cacheEntry struct {
	analysis  *model.Analysis
	expiresAt time.Time
}

// cache is a small in-memory store keyed by (fingerprint, window). It evicts expired entries lazily on Get
// and bounds capacity by dropping the oldest entry when full.
type cache struct {
	mu      sync.Mutex
	entries map[cacheKey]cacheEntry
	cap     int
	ttl     time.Duration
}

func newCache(capacity int, ttl time.Duration) *cache {
	if capacity <= 0 {
		capacity = 256
	}
	return &cache{
		entries: make(map[cacheKey]cacheEntry, capacity),
		cap:     capacity,
		ttl:     ttl,
	}
}

func (c *cache) Get(fp string, windowStart time.Time) *model.Analysis {
	k := cacheKey{fp, windowStart.Unix()}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok {
		return nil
	}
	if time.Now().After(e.expiresAt) {
		delete(c.entries, k)
		return nil
	}
	return e.analysis
}

func (c *cache) Set(fp string, windowStart time.Time, a *model.Analysis) {
	k := cacheKey{fp, windowStart.Unix()}
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.cap {
		var oldestKey cacheKey
		var oldestExp time.Time
		first := true
		for kk, ee := range c.entries {
			if time.Now().After(ee.expiresAt) {
				oldestKey = kk
				break
			}
			if first || ee.expiresAt.Before(oldestExp) {
				oldestKey = kk
				oldestExp = ee.expiresAt
				first = false
			}
		}
		delete(c.entries, oldestKey)
	}

	c.entries[k] = cacheEntry{analysis: a, expiresAt: time.Now().Add(c.ttl)}
}

func (c *cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
