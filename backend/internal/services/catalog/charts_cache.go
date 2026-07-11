package catalog

import (
	"strconv"
	"sync"
	"time"
)

// Chart payloads are stale-tolerant by nature: every public chart surface is
// a ranked aggregate over slow-moving data, and the all_time window is a
// full-history aggregation per request — the cache is the primary cost
// lever, with the expression indexes (charts cost-lever migration) as the
// fallback. TTL tiers: the masthead summary + ticker are the heaviest
// per-request calls and tolerate 60s staleness invisibly, so they get the
// shortest TTL; module pages get longer. The authed personal stats endpoint
// is per-user private data and is deliberately NOT cached.
//
// This cache and radio_now_playing.go's nowPlayingCache share the same
// shape (per-entry single-flight, errors never cached, injectable clock) but
// differ where their key spaces differ: station IDs are domain-bounded, chart
// keys are client-controlled and need the entry cap below. Consolidate into
// one shared cache only when a third consumer appears.
const (
	chartsModuleTTL   = 5 * time.Minute
	chartsMastheadTTL = time.Minute

	// chartsCacheMaxEntries bounds memory: offset/limit are client-controlled,
	// so key cardinality is unbounded. When the cap is hit, expired entries
	// are swept; if the map is still full, the NEW key is simply not cached
	// (the request runs uncached) — overflow traffic must never evict the hot
	// masthead/teaser entries the cache exists to protect. 512 comfortably
	// covers every organic key (6 modules x 3 windows x the handful of page
	// shapes the frontend requests, plus masthead keys).
	chartsCacheMaxEntries = 512
)

// chartsCacheEntry is one cached payload. The per-entry mutex serializes
// fetches: while one request computes, concurrent requests for the same key
// wait and then read the fresh value — no thundering herd on TTL expiry of a
// hot key (the masthead summary expires every 60s by design).
type chartsCacheEntry struct {
	mu      sync.Mutex
	value   any
	expires time.Time
}

// chartsCache is a small in-process TTL cache for chart payloads. Cached
// values are shared across requests — every consumer must treat them as
// immutable (the chart handlers copy into response structs and never mutate
// contract values). A nil *chartsCache is a valid no-op cache: chartsCached
// bypasses it entirely, which is how the integration test suite (which
// constructs ChartsService directly) keeps per-test DB isolation.
type chartsCache struct {
	mu      sync.Mutex
	entries map[string]*chartsCacheEntry
	now     func() time.Time // injectable for expiry tests
}

func newChartsCache() *chartsCache {
	return &chartsCache{
		entries: make(map[string]*chartsCacheEntry),
		now:     time.Now,
	}
}

// acquire returns the entry for key, creating it if there's room. A nil
// return means the cache is full of fresh entries — the caller runs uncached
// rather than evicting a hot key.
func (c *chartsCache) acquire(key string) *chartsCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok {
		return entry
	}
	if len(c.entries) >= chartsCacheMaxEntries {
		now := c.now()
		for k, e := range c.entries {
			// Unfilled entries (a fetch in flight) have zero expires and are
			// never swept; e.mu is deliberately not taken here — deleting the
			// map slot is safe while a fetch fills the (now detached) entry.
			if e.expires.IsZero() || now.Before(e.expires) {
				continue
			}
			delete(c.entries, k)
		}
		if len(c.entries) >= chartsCacheMaxEntries {
			return nil
		}
	}
	entry := &chartsCacheEntry{}
	c.entries[key] = entry
	return entry
}

// dropIfUnfilled removes a never-filled entry after a failed fetch so junk
// keys (probing clients) can't grow the map with permanently empty entries.
func (c *chartsCache) dropIfUnfilled(key string, entry *chartsCacheEntry) {
	if entry.value != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries[key] == entry {
		delete(c.entries, key)
	}
}

// chartsCached returns the cached value for key when fresh, else runs fetch
// (single-flight per key) and caches its result. Errors are never cached — a
// transient DB failure can't be pinned for a TTL; a stale value is served
// only until its expiry, never refreshed-from-error. Free function because
// Go methods can't take type parameters.
func chartsCached[T any](c *chartsCache, key string, ttl time.Duration, fetch func() (T, error)) (T, error) {
	if c == nil {
		return fetch()
	}
	entry := c.acquire(key)
	if entry == nil {
		return fetch()
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.value != nil && c.now().Before(entry.expires) {
		if typed, ok := entry.value.(T); ok {
			return typed, nil
		}
	}

	v, err := fetch()
	if err != nil {
		c.dropIfUnfilled(key, entry)
		var zero T
		return zero, err
	}
	entry.value = v
	entry.expires = c.now().Add(ttl)
	return v, nil
}

// pagedChartRows bundles a module page with its window-total so the pair
// caches (and returns) atomically.
type pagedChartRows[T any] struct {
	rows  []T
	total int
}

// cachedChartPage is the single owner of the windowed modules' cache-key
// scheme (module|window|limit|offset) and the pagedChartRows pack/unpack.
// NOTE: pages cache independently, so two pages of one window can come from
// snapshots up to a TTL apart — totals (and most-anticipated's mode) are
// per-response facts, not cross-page guarantees.
func cachedChartPage[T any](c *chartsCache, module string, window string, limit, offset int, fetch func() ([]T, int, error)) ([]T, int, error) {
	key := module + "|" + window + "|" + strconv.Itoa(limit) + "|" + strconv.Itoa(offset)
	page, err := chartsCached(c, key, chartsModuleTTL, func() (pagedChartRows[T], error) {
		rows, total, err := fetch()
		return pagedChartRows[T]{rows: rows, total: total}, err
	})
	return page.rows, page.total, err
}
