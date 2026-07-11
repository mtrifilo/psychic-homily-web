package catalog

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// Unit tests for the chartsCache mechanics — the injectable clock makes
// expiry deterministic without sleeping.

func cacheAt(t0 time.Time) (*chartsCache, *time.Time) {
	c := newChartsCache()
	now := t0
	c.now = func() time.Time { return now }
	return c, &now
}

func TestChartsCached_HitAndExpiry(t *testing.T) {
	t0 := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	c, now := cacheAt(t0)

	calls := 0
	fetch := func() (int, error) {
		calls++
		return 42, nil
	}

	for i := 0; i < 3; i++ {
		v, err := chartsCached(c, "k", time.Minute, fetch)
		if err != nil || v != 42 {
			t.Fatalf("unexpected: %v %v", v, err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 fetch (then hits), got %d", calls)
	}

	*now = t0.Add(61 * time.Second)
	if v, _ := chartsCached(c, "k", time.Minute, fetch); v != 42 {
		t.Fatal("refetch after expiry failed")
	}
	if calls != 2 {
		t.Fatalf("expired entry must refetch, got %d calls", calls)
	}
}

func TestChartsCached_KeyIsolation(t *testing.T) {
	c, _ := cacheAt(time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC))
	va, _ := chartsCached(c, "a", time.Minute, func() (int, error) { return 1, nil })
	vb, _ := chartsCached(c, "b", time.Minute, func() (int, error) { return 2, nil })
	if va != 1 || vb != 2 {
		t.Fatalf("key isolation broken: %d %d", va, vb)
	}
}

func TestChartsCached_NilCacheBypasses(t *testing.T) {
	calls := 0
	for i := 0; i < 2; i++ {
		v, err := chartsCached(nil, "k", time.Minute, func() (int, error) {
			calls++
			return 42, nil
		})
		if err != nil || v != 42 {
			t.Fatalf("unexpected: %v %v", v, err)
		}
	}
	if calls != 2 {
		t.Fatalf("nil cache must bypass (fetch every call), got %d calls", calls)
	}
}

func TestChartsCached_CachesSuccessNotError(t *testing.T) {
	c, _ := cacheAt(time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC))
	calls := 0
	for i := 0; i < 2; i++ {
		if _, err := chartsCached(c, "k", time.Minute, func() (int, error) {
			calls++
			return 0, fmt.Errorf("transient")
		}); err == nil {
			t.Fatal("expected error")
		}
	}
	if calls != 2 {
		t.Fatalf("errors must not be cached, got %d calls", calls)
	}
	// A failed fetch must not leave an empty entry pinned in the map.
	if len(c.entries) != 0 {
		t.Fatalf("failed fetches must not grow the map, have %d entries", len(c.entries))
	}

	for i := 0; i < 3; i++ {
		v, err := chartsCached(c, "k", time.Minute, func() (int, error) {
			calls++
			return 7, nil
		})
		if err != nil || v != 7 {
			t.Fatalf("unexpected: %v %v", v, err)
		}
	}
	if calls != 3 {
		t.Fatalf("success must be cached after first fetch (2 failing + 1 ok), got %d calls", calls)
	}
}

// TestChartsCached_OverflowSkipsCachingKeepsHotKeys: a full cache of FRESH
// entries never evicts — new (overflow) keys just run uncached, so a client
// minting junk offset keys can't wipe the hot masthead entries.
func TestChartsCached_OverflowSkipsCachingKeepsHotKeys(t *testing.T) {
	t0 := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	c, now := cacheAt(t0)

	hotCalls := 0
	hotFetch := func() (string, error) {
		hotCalls++
		return "masthead", nil
	}
	if _, err := chartsCached(c, "summary|quarter", time.Minute, hotFetch); err != nil {
		t.Fatal(err)
	}

	// Fill the rest of the cache with fresh entries.
	for i := 0; len(c.entries) < chartsCacheMaxEntries; i++ {
		if _, err := chartsCached(c, fmt.Sprintf("junk|%d", i), time.Hour, func() (int, error) { return i, nil }); err != nil {
			t.Fatal(err)
		}
	}

	// Overflow keys run uncached — twice each — and never enter the map.
	overflowCalls := 0
	for i := 0; i < 2; i++ {
		if _, err := chartsCached(c, "overflow|1", time.Hour, func() (int, error) {
			overflowCalls++
			return 1, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if overflowCalls != 2 {
		t.Fatalf("overflow key must bypass the cache, got %d calls", overflowCalls)
	}

	// The hot key survived: still a hit, no refetch.
	if v, _ := chartsCached(c, "summary|quarter", time.Minute, hotFetch); v != "masthead" || hotCalls != 1 {
		t.Fatalf("hot key must survive overflow pressure (calls=%d)", hotCalls)
	}

	// Once entries expire, the sweep makes room again.
	*now = t0.Add(2 * time.Hour)
	if _, err := chartsCached(c, "post-sweep", time.Hour, func() (int, error) { return 9, nil }); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.entries["post-sweep"]; !ok {
		t.Fatal("sweep at cap must admit new keys once entries expire")
	}
}

// TestChartsCached_SingleFlight: concurrent misses on one key run ONE fetch;
// the rest wait and read the cached value (no thundering herd on expiry).
func TestChartsCached_SingleFlight(t *testing.T) {
	c, _ := cacheAt(time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC))

	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})
	fetch := func() (int, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		return 42, nil
	}

	const n = 8
	var wg sync.WaitGroup
	results := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := chartsCached(c, "k", time.Minute, fetch)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			results[i] = v
		}(i)
	}
	// Let the goroutines contend, then release the single in-flight fetch.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if calls != 1 {
		t.Fatalf("expected exactly 1 fetch across %d concurrent misses, got %d", n, calls)
	}
	for i, v := range results {
		if v != 42 {
			t.Fatalf("goroutine %d got %d", i, v)
		}
	}
}

// TestChartsCached_RefreshInFlightNotSwept: an expired entry being REFRESHED
// is marked in-flight (zero expires) and must survive an at-cap sweep — the
// hot masthead key mid-refresh is exactly the entry the cap policy protects.
func TestChartsCached_RefreshInFlightNotSwept(t *testing.T) {
	t0 := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	c, now := cacheAt(t0)

	// Fill the hot key, then let it expire.
	if _, err := chartsCached(c, "hot", time.Minute, func() (string, error) { return "v1", nil }); err != nil {
		t.Fatal(err)
	}
	*now = t0.Add(2 * time.Minute)

	// Start a refresh that blocks mid-fetch.
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan string)
	go func() {
		v, _ := chartsCached(c, "hot", time.Minute, func() (string, error) {
			close(started)
			<-release
			return "v2", nil
		})
		done <- v
	}()
	<-started

	// While the refresh is in flight, an at-cap sweep runs (fill to cap and
	// force acquire's sweep path). The hot entry must NOT be deleted.
	for i := 0; len(c.entries) < chartsCacheMaxEntries; i++ {
		c.entries[fmt.Sprintf("filler-%d", i)] = &chartsCacheEntry{expires: t0.Add(time.Second)} // long-expired
	}
	c.acquire("trigger-sweep")
	c.mu.Lock()
	_, hotSurvived := c.entries["hot"]
	c.mu.Unlock()
	if !hotSurvived {
		t.Fatal("in-flight refresh entry must survive the at-cap sweep")
	}

	close(release)
	if v := <-done; v != "v2" {
		t.Fatalf("refresh result lost: %q", v)
	}
	// And the refreshed value is served from cache afterwards.
	calls := 0
	v, _ := chartsCached(c, "hot", time.Minute, func() (string, error) { calls++; return "v3", nil })
	if v != "v2" || calls != 0 {
		t.Fatalf("refreshed value must be cached (got %q, %d fetches)", v, calls)
	}
}

// TestChartsCached_PanicDropsEntry: a panicking fetch must not leak a
// permanently unsweepable zero-expires slot; the panic propagates.
func TestChartsCached_PanicDropsEntry(t *testing.T) {
	c, _ := cacheAt(time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC))

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic must propagate")
			}
		}()
		_, _ = chartsCached(c, "boom", time.Minute, func() (int, error) { panic("fetch exploded") })
	}()

	if len(c.entries) != 0 {
		t.Fatalf("panicking fetch must drop its entry, have %d", len(c.entries))
	}
}

// TestChartsCached_FailedRefreshStaysSweepable: when a refresh errors, the
// stale entry must be expired-but-nonzero (sweepable, retried next request) —
// not zero-expires, which would squat a slot as fake in-flight forever.
func TestChartsCached_FailedRefreshStaysSweepable(t *testing.T) {
	t0 := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	c, now := cacheAt(t0)

	if _, err := chartsCached(c, "k", time.Minute, func() (string, error) { return "v1", nil }); err != nil {
		t.Fatal(err)
	}
	*now = t0.Add(2 * time.Minute)
	if _, err := chartsCached(c, "k", time.Minute, func() (string, error) { return "", fmt.Errorf("refresh failed") }); err == nil {
		t.Fatal("expected refresh error")
	}

	c.mu.Lock()
	entry := c.entries["k"]
	c.mu.Unlock()
	if entry == nil {
		t.Fatal("filled entry must survive a failed refresh (stale value kept)")
	}
	if entry.expires.IsZero() {
		t.Fatal("failed refresh must not leave the entry marked in-flight (unsweepable)")
	}

	// Next request retries the fetch (error was not cached).
	v, err := chartsCached(c, "k", time.Minute, func() (string, error) { return "v2", nil })
	if err != nil || v != "v2" {
		t.Fatalf("retry after failed refresh: %q %v", v, err)
	}
}

// TestChartsCache_ConcurrentAtCap exercises the sweep/fill interleaving under
// the race detector: concurrent fetch completions (expires writes) while
// at-cap acquires sweep (expires reads). Run with -race locally/CI.
func TestChartsCache_ConcurrentAtCap(t *testing.T) {
	c := newChartsCache() // real clock: expiry values irrelevant, the race is the point

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := fmt.Sprintf("k-%d-%d", g, i%64)
				ttl := time.Duration(i%3) * time.Millisecond // some entries expire immediately
				_, _ = chartsCached(c, key, ttl, func() (int, error) {
					if i%7 == 0 {
						return 0, fmt.Errorf("transient")
					}
					return i, nil
				})
			}
		}(g)
	}
	wg.Wait()
}
