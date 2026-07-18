package catalog

import (
	"strconv"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// Chart payloads are stale-tolerant by nature: every public chart surface is
// a ranked aggregate over slow-moving data, and the all_time window is a
// full-history aggregation per request — the cache is the primary cost
// lever, with the expression indexes (charts cost-lever migration) as the
// fallback. TTL tiers: the masthead summary + ticker are the heaviest
// per-request calls and tolerate 60s staleness invisibly, so they get the
// shortest TTL; module pages get longer. The masthead pair lives in its OWN
// cache instance so the modules' client-controlled key space can never
// starve the hot masthead keys of a slot. The authed personal stats endpoint
// is per-user private data and is deliberately NOT cached.
//
// This cache and radio_now_playing.go's nowPlayingCache share the same
// shape (per-entry single-flight, errors never cached, injectable clock) but
// differ where their key spaces differ: station IDs are domain-bounded, chart
// keys are client-controlled and need the entry cap below. Consolidate into
// one shared cache only when a third consumer appears.
const (
	chartsModuleTTL         = 5 * time.Minute
	chartsMastheadTTL       = time.Minute
	chartsClosedCalendarTTL = 24 * time.Hour

	// chartsCacheMaxEntries bounds memory: offset/limit are client-controlled
	// and scene, while gated to real CBSA codes (chartSceneExists — the gate
	// is what keeps this key space bounded; junk scenes never reach the
	// cache), still multiplies the key population. When the cap is hit,
	// expired entries are swept; if the map is still full of FRESH entries,
	// the NEW key is simply not cached (the request runs uncached) —
	// overflow traffic never evicts a fresh entry, but an EXPIRED organic
	// entry's slot is claimable by any traffic. Sizing: 7 modules x accepted
	// rolling/calendar windows x page shapes x [global + every switcher metro] plus
	// per-(module,window,scene) count keys and the scoped summary/ticker
	// keys chartsCacheFor routes here — calendar values are grammar-, launch-,
	// and future-gated before reaching this layer; a few dozen active scenes
	// lands in the low thousands of keys, so 4096 keeps organic traffic cached.
	// Worst-case memory is cap x a full limit=100 page (tens of KB with
	// name-enrichment slices), i.e. low hundreds of MB only if every slot
	// held a max-size page — real pages are the front page's limit=10 shape
	// for all but drill-down traffic. The masthead instance holds only the
	// GLOBAL summary/ticker/scenes keys, all domain-bounded by request
	// validation and the chartsCacheFor routing rule.
	chartsCacheMaxEntries = 4096
)

func chartWindowTTL(c *chartsCache, window contracts.ChartWindow, currentTTL time.Duration) time.Duration {
	_, end, ok := window.OrDefault().CalendarBounds()
	if !ok {
		return currentTTL
	}
	now := time.Now().UTC()
	if c != nil {
		now = c.now().UTC()
	}
	if !end.After(now) {
		return chartsClosedCalendarTTL
	}
	return currentTTL
}

// chartsCacheEntry is one cached payload. The per-entry mutex serializes
// fetches: while one request computes, concurrent requests for the same key
// wait and then read the fresh value — no thundering herd on TTL expiry of a
// hot key (the masthead summary expires every 60s by design).
//
// Locking discipline for `expires`: WRITES hold BOTH entry.mu and c.mu
// (setExpires); the hit check reads it under entry.mu, the at-cap sweep
// reads it under c.mu — each reader synchronizes with the writer through
// its own lock. `value` is only ever touched under entry.mu.
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
// rather than evicting an existing key.
func (c *chartsCache) acquire(key string) *chartsCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok {
		return entry
	}
	if len(c.entries) >= chartsCacheMaxEntries {
		now := c.now()
		for k, e := range c.entries {
			// Zero expires marks an in-flight fetch (initial fill OR a
			// refresh — chartsCached zeroes it before refetching), so a
			// hot entry is never evicted once marked. (An expired entry
			// can be swept in the beat before its refresher marks it;
			// the refresh then fills an orphaned entry — benign: the
			// value is still returned, just not cached.) Reading
			// e.expires here is synchronized through c.mu (see
			// setExpires).
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

// setExpires is the single writer of entry.expires. The caller must hold
// entry.mu; taking c.mu as well is what lets acquire's sweep read expires
// under c.mu alone without a data race. Lock order entry.mu -> c.mu is
// consistent across the file (dropEntry), so no deadlock.
func (c *chartsCache) setExpires(entry *chartsCacheEntry, t time.Time) {
	c.mu.Lock()
	entry.expires = t
	c.mu.Unlock()
}

// dropEntry removes the entry's map slot (the caller holds entry.mu). Used
// when a fetch fails or panics before ever filling the entry, so junk keys
// (probing clients) and panicking fetches can't leak permanently unsweepable
// slots.
func (c *chartsCache) dropEntry(key string, entry *chartsCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries[key] == entry {
		delete(c.entries, key)
	}
}

// chartsCached returns the cached value for key when fresh, else runs fetch
// (single-flight per key) and caches its result. Errors are never cached — a
// never-filled entry is dropped (retry recreates it), and a failed REFRESH
// re-marks the stale entry as expired-but-sweepable so it neither serves
// stale forever nor squats a slot as fake in-flight. A panicking fetch drops
// the entry and re-panics. Free function because Go methods can't take type
// parameters.
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
		// A type mismatch here is always a programming error (two call sites
		// colliding on one key); it degrades to refetch-every-time rather
		// than a panic — visible as a perpetual cache miss, never bad data.
		if typed, ok := entry.value.(T); ok {
			return typed, nil
		}
	}

	// Mark the fetch in flight (zero expires) so the at-cap sweep never
	// evicts an entry that is being refreshed.
	c.setExpires(entry, time.Time{})

	v, err := func() (v T, err error) {
		defer func() {
			if r := recover(); r != nil {
				c.dropEntry(key, entry)
				panic(r)
			}
		}()
		return fetch()
	}()
	if err != nil {
		if entry.value == nil {
			c.dropEntry(key, entry)
		} else {
			// Failed refresh: keep the slot honest — expired (sweepable,
			// refetch next request), not zero (which would mean in-flight).
			c.setExpires(entry, c.now().Add(-time.Nanosecond))
		}
		var zero T
		return zero, err
	}
	entry.value = v
	c.setExpires(entry, c.now().Add(ttl))
	return v, nil
}

// pagedChartRows bundles a module page with its window-total so the pair
// caches (and returns) atomically.
type pagedChartRows[T any] struct {
	rows  []T
	total int
}

// cachedChartPage is the single owner of the windowed modules' cache-key
// scheme (module|window|scene|limit|offset) and the pagedChartRows
// pack/unpack. scene is "" for the global (unscoped) page — the empty segment
// keeps global and scoped keys disjoint. Scene is client-controlled like
// offset/limit, so its key cardinality rides on the same entry cap +
// run-uncached overflow rule (shape validation at the HTTP layer bounds it to
// short numeric strings).
// NOTE: pages cache independently, so two pages of one window can come from
// snapshots up to a TTL apart — totals (and most-anticipated's mode) are
// per-response facts, not cross-page guarantees.
func cachedChartPage[T any](c *chartsCache, module string, window contracts.ChartWindow, scene string, limit, offset int, fetch func() ([]T, int, error)) ([]T, int, error) {
	normalizedWindow := window.OrDefault()
	key := module + "|" + string(normalizedWindow) + "|" + scene + "|" + strconv.Itoa(limit) + "|" + strconv.Itoa(offset)
	page, err := chartsCached(c, key, chartWindowTTL(c, normalizedWindow, chartsModuleTTL), func() (pagedChartRows[T], error) {
		rows, total, err := fetch()
		return pagedChartRows[T]{rows: rows, total: total}, err
	})
	return page.rows, page.total, err
}

// chartCountKey is the offset-independent cache key for a module's full-set
// count — the beyond-the-end re-count caches under it so a client walking
// junk offsets pays the count aggregation once per TTL, not per request.
// scene follows the cachedChartPage segment convention ("" = global).
func chartCountKey(module, window, scene string) string {
	return "count|" + module + "|" + window + "|" + scene
}
