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
