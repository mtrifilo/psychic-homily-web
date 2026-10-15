package auth

import (
	"container/list"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// reset empties the registry, including its eviction-report state.
func (s *expiringRegistry[T]) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*list.Element)
	s.order.Init()
	s.evictedSinceReport = 0
	s.lastReportAt = time.Time{}
}

func (s *expiringRegistry[T]) size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.order.Len()
}

// get reads without consuming and refuses an expired entry: what a holder
// of id could still take.
func (s *expiringRegistry[T]) get(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero T
	element, ok := s.entries[id]
	if !ok {
		return zero, false
	}
	entry := entryOf[T](element)
	if s.now().After(entry.expiresAt) {
		return zero, false
	}
	return entry.value, true
}

// fakeClock is a manually advanced clock for a registry under test. It is not
// safe for concurrent use; the concurrent test runs on time.Now.
type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

// assertRegistryConsistent checks that entries and order hold the same ids,
// order sorted oldest first by expiry, and that the registry is within
// capacity.
func assertRegistryConsistent[T any](t *testing.T, s *expiringRegistry[T]) {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.entries) != s.order.Len() {
		t.Fatalf("entries has %d ids, order has %d", len(s.entries), s.order.Len())
	}
	if s.order.Len() > s.capacity {
		t.Fatalf("registry holds %d entries, capacity %d", s.order.Len(), s.capacity)
	}
	var previous time.Time
	for element := s.order.Front(); element != nil; element = element.Next() {
		entry := entryOf[T](element)
		if s.entries[entry.id] != element {
			t.Fatalf("entries[%q] does not point at its order element", entry.id)
		}
		if entry.expiresAt.Before(previous) {
			t.Fatalf("order is not sorted by expiry at %q", entry.id)
		}
		previous = entry.expiresAt
	}
}

func TestExpiringRegistry_RefusesCapacityBelowOne(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a capacity of 0 to panic")
		}
	}()
	newExpiringRegistry[string](0, cliCallbackTTL, time.Now)
}

func TestExpiringRegistry_ExpiresAfterTTL(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](10, cliCallbackTTL, clock.Now)

	store.put("a", "http://localhost/a")

	clock.Advance(cliCallbackTTL)
	if _, ok := store.get("a"); !ok {
		t.Fatal("expected the entry to be usable at exactly its TTL")
	}

	clock.Advance(time.Nanosecond)
	if _, ok := store.get("a"); ok {
		t.Fatal("expected the entry to be refused once past its TTL")
	}

	store.put("b", "http://localhost/b")
	if store.size() != 1 {
		t.Fatalf("expected the expired entry to be dropped on the next put, size %d", store.size())
	}
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_CapacityEvictsOldestKeepsNewest(t *testing.T) {
	clock := newFakeClock()
	const capacity = 3
	store := newExpiringRegistry[string](capacity, cliCallbackTTL, clock.Now)

	for i := range 5 {
		store.put(fmt.Sprintf("id-%d", i), fmt.Sprintf("http://localhost/%d", i))
		clock.Advance(time.Second)
		if store.size() > capacity {
			t.Fatalf("size %d exceeds capacity %d after put %d", store.size(), capacity, i)
		}
	}

	for _, evicted := range []string{"id-0", "id-1"} {
		if _, ok := store.get(evicted); ok {
			t.Errorf("expected %s to be evicted", evicted)
		}
	}
	for i := 2; i < 5; i++ {
		id := fmt.Sprintf("id-%d", i)
		url, ok := store.get(id)
		if !ok {
			t.Errorf("expected %s to be kept", id)
			continue
		}
		if want := fmt.Sprintf("http://localhost/%d", i); url != want {
			t.Errorf("%s: got %q, want %q", id, url, want)
		}
	}
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_ExpiredEntriesMakeRoomBeforeEviction(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](2, cliCallbackTTL, clock.Now)

	store.put("stale", "http://localhost/stale")
	clock.Advance(cliCallbackTTL / 2)
	store.put("live", "http://localhost/live")
	clock.Advance(cliCallbackTTL/2 + time.Second)

	if evicted := store.put("new", "http://localhost/new"); evicted != 0 {
		t.Fatalf("expected the expired entry to make room without an eviction, got %d evicted", evicted)
	}
	if _, ok := store.get("live"); !ok {
		t.Error("expected the live entry to survive")
	}
	if _, ok := store.get("new"); !ok {
		t.Error("expected the new entry to be stored")
	}
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_RestoringAnIDReplacesItWithoutEviction(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](2, cliCallbackTTL, clock.Now)

	store.put("a", "http://localhost/a1")
	clock.Advance(time.Second)
	store.put("b", "http://localhost/b")
	clock.Advance(time.Second)

	if evicted := store.put("a", "http://localhost/a2"); evicted != 0 {
		t.Fatalf("expected replacing an id to evict nothing, got %d", evicted)
	}
	if url, _ := store.get("a"); url != "http://localhost/a2" {
		t.Errorf("expected the replacement URL, got %q", url)
	}

	// "a" is now the newest, so the next put at capacity evicts "b".
	store.put("c", "http://localhost/c")
	if _, ok := store.get("b"); ok {
		t.Error("expected b, now the oldest, to be evicted")
	}
	if _, ok := store.get("a"); !ok {
		t.Error("expected the replaced a to be kept")
	}
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_RemoveFreesCapacity(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](2, cliCallbackTTL, clock.Now)

	store.put("a", "http://localhost/a")
	store.put("b", "http://localhost/b")
	store.remove("a")

	if evicted := store.put("c", "http://localhost/c"); evicted != 0 {
		t.Fatalf("expected a removed entry to free its slot, got %d evicted", evicted)
	}
	if _, ok := store.get("b"); !ok {
		t.Error("expected b to be kept")
	}
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_EvictionsReportedAtMostOncePerInterval(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](1, cliCallbackTTL, clock.Now)

	if evicted := store.put("a", "http://localhost/a"); evicted != 0 {
		t.Fatalf("no eviction yet, got report %d", evicted)
	}
	if evicted := store.put("b", "http://localhost/b"); evicted != 1 {
		t.Fatalf("expected the first eviction to be reported at once, got %d", evicted)
	}

	// Two more evictions inside the interval are held back.
	for _, id := range []string{"c", "d"} {
		clock.Advance(registryEvictionReportInterval / 4)
		if evicted := store.put(id, "http://localhost/"+id); evicted != 0 {
			t.Fatalf("expected no report inside the interval, got %d", evicted)
		}
	}

	// The first put once the interval has passed reports both.
	clock.Advance(registryEvictionReportInterval / 2)
	if evicted := store.put("e", "http://localhost/e"); evicted != 3 {
		t.Fatalf("expected the held-back evictions plus this one (3), got %d", evicted)
	}

	// Without further evictions there is nothing to report.
	clock.Advance(registryEvictionReportInterval)
	store.remove("e")
	if evicted := store.put("f", "http://localhost/f"); evicted != 0 {
		t.Fatalf("expected no report without new evictions, got %d", evicted)
	}
}

func medianDuration(samples []time.Duration) time.Duration {
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	return sorted[len(sorted)/2]
}

// TestExpiringRegistry_InsertCostDoesNotScaleWithSize inserts 10,000
// distinct live entries into a registry that holds all of them and into one at
// the production capacity. A put does constant work, so the median of the last
// 100 puts stays within a small factor of the median of the first 100, and all
// 10,000 finish inside insertBound. A put that walked the store would make the
// last puts on the 10,000-entry registry orders of magnitude slower than the
// first. Medians keep a GC pause in either window from deciding the result.
func TestExpiringRegistry_InsertCostDoesNotScaleWithSize(t *testing.T) {
	const (
		inserts     = 10000
		window      = 100
		maxSlowdown = 10
		insertBound = time.Second
	)

	for _, capacity := range []int{inserts, cliCallbackCapacity, oauthLinkIntentCapacity} {
		t.Run(fmt.Sprintf("capacity_%d", capacity), func(t *testing.T) {
			store := newExpiringRegistry[string](capacity, cliCallbackTTL, time.Now)
			ids := make([]string, inserts)
			for i := range ids {
				ids[i] = fmt.Sprintf("id-%05d", i)
			}

			durations := make([]time.Duration, inserts)
			start := time.Now()
			for i, id := range ids {
				putStart := time.Now()
				store.put(id, "http://localhost:8888/cb")
				durations[i] = time.Since(putStart)
			}
			elapsed := time.Since(start)

			firstMedian := medianDuration(durations[:window])
			lastMedian := medianDuration(durations[inserts-window:])
			t.Logf("%d puts in %v; median put %v over the first %d, %v over the last %d; size %d",
				inserts, elapsed, firstMedian, window, lastMedian, window, store.size())

			if elapsed > insertBound {
				t.Errorf("%d puts took %v, over the %v bound", inserts, elapsed, insertBound)
			}
			if lastMedian > maxSlowdown*max(firstMedian, time.Microsecond) {
				t.Errorf("median put grew from %v to %v as the store filled", firstMedian, lastMedian)
			}
			if want := min(inserts, capacity); store.size() != want {
				t.Errorf("expected size %d, got %d", want, store.size())
			}
			assertRegistryConsistent(t, store)
		})
	}
}

func TestExpiringRegistry_ConcurrentUseStaysBoundedAndConsistent(t *testing.T) {
	const capacity = 50
	store := newExpiringRegistry[string](capacity, cliCallbackTTL, time.Now)

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 500 {
				id := fmt.Sprintf("w%d-%d", worker, i)
				store.put(id, "http://localhost/"+id)
				store.get(id)
				store.get(fmt.Sprintf("w%d-%d", (worker+1)%8, i))
				if i%3 == 0 {
					store.remove(id)
				}
				if size := store.size(); size > capacity {
					t.Errorf("size %d exceeds capacity %d", size, capacity)
					return
				}
			}
		}()
	}
	wg.Wait()
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_TakeConsumes(t *testing.T) {
	store := newExpiringRegistry[string](2, cliCallbackTTL, newFakeClock().Now)
	store.put("a", "http://localhost/a")

	if url, ok := store.take("a"); !ok || url != "http://localhost/a" {
		t.Fatalf("expected the first take to return the entry, got %q, %v", url, ok)
	}
	if url, ok := store.take("a"); ok {
		t.Fatalf("expected the second take to find nothing, got %q", url)
	}
	if store.size() != 0 {
		t.Errorf("expected take to free the slot, size %d", store.size())
	}
	assertRegistryConsistent(t, store)
}

func TestExpiringRegistry_TakeRefusesAndDropsAnExpiredEntry(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](2, cliCallbackTTL, clock.Now)
	store.put("a", "http://localhost/a")

	clock.Advance(cliCallbackTTL + time.Nanosecond)
	if url, ok := store.take("a"); ok {
		t.Fatalf("expected an expired entry to be refused, got %q", url)
	}
	if store.size() != 0 {
		t.Errorf("expected the expired entry to be dropped, size %d", store.size())
	}
	assertRegistryConsistent(t, store)
}

// Of many concurrent takes of one id, exactly one gets the entry.
func TestExpiringRegistry_ConcurrentTakesHaveOneWinner(t *testing.T) {
	const takers = 32
	for round := range 50 {
		store := newExpiringRegistry[string](4, cliCallbackTTL, time.Now)
		store.put("id", "http://localhost/cb")

		var wg sync.WaitGroup
		var winners sync.Map
		start := make(chan struct{})
		for taker := range takers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if _, ok := store.take("id"); ok {
					winners.Store(taker, true)
				}
			}()
		}
		close(start)
		wg.Wait()

		count := 0
		winners.Range(func(any, any) bool { count++; return true })
		if count != 1 {
			t.Fatalf("round %d: %d takers got the entry, want exactly 1", round, count)
		}
	}
}

func TestExpiringRegistry_PeekDoesNotConsumeAndSeesExpiredEntries(t *testing.T) {
	clock := newFakeClock()
	store := newExpiringRegistry[string](2, cliCallbackTTL, clock.Now)
	store.put("a", "http://localhost/a")

	for range 2 {
		if url, ok := store.peek("a"); !ok || url != "http://localhost/a" {
			t.Fatalf("expected peek to return the entry and leave it, got %q, %v", url, ok)
		}
	}

	clock.Advance(cliCallbackTTL + time.Nanosecond)
	if _, ok := store.peek("a"); !ok {
		t.Fatal("expected peek to still see an expired entry no put has dropped")
	}

	store.put("b", "http://localhost/b")
	if _, ok := store.peek("a"); ok {
		t.Fatal("expected the expired entry to be gone after the next put")
	}
	if _, ok := store.peek("missing"); ok {
		t.Fatal("expected peek of an unknown id to find nothing")
	}
}
