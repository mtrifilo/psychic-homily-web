package auth

import (
	"container/list"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"psychic-homily-backend/internal/testlog"
)

// reset empties the registry, including its eviction-report state.
func (s *cliCallbackRegistry) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*list.Element)
	s.order.Init()
	s.evictedSinceReport = 0
	s.lastReportAt = time.Time{}
}

func (s *cliCallbackRegistry) size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.order.Len()
}

func cleanCLICallbackStore() {
	cliCallbackStore.reset()
}

// storeExpiredCLICallback plants an already-expired entry in the process-wide
// store. It goes to the front of order, which keeps order sorted by expiry.
func storeExpiredCLICallback(id, callbackURL string) {
	cliCallbackStore.mu.Lock()
	defer cliCallbackStore.mu.Unlock()
	cliCallbackStore.removeLocked(id)
	cliCallbackStore.entries[id] = cliCallbackStore.order.PushFront(&cliCallbackEntry{
		id:          id,
		callbackURL: callbackURL,
		expiresAt:   time.Now().Add(-time.Minute),
	})
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
func assertRegistryConsistent(t *testing.T, s *cliCallbackRegistry) {
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
		entry := element.Value.(*cliCallbackEntry)
		if s.entries[entry.id] != element {
			t.Fatalf("entries[%q] does not point at its order element", entry.id)
		}
		if entry.expiresAt.Before(previous) {
			t.Fatalf("order is not sorted by expiry at %q", entry.id)
		}
		previous = entry.expiresAt
	}
}

func TestCLICallbackStore_StoreAndRetrieve(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("test-id-1", "http://localhost:9999/callback")

	url, ok := getCLICallback("test-id-1")
	if !ok {
		t.Fatal("expected callback to be found")
	}
	if url != "http://localhost:9999/callback" {
		t.Errorf("expected callback URL, got %q", url)
	}
}

func TestCLICallbackStore_NotFound(t *testing.T) {
	defer cleanCLICallbackStore()

	_, ok := getCLICallback("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent key")
	}
}

func TestCLICallbackStore_Expired(t *testing.T) {
	defer cleanCLICallbackStore()

	storeExpiredCLICallback("expired-id", "http://expired.example.com")

	_, ok := getCLICallback("expired-id")
	if ok {
		t.Error("expected expired callback to not be found")
	}
}

func TestCLICallbackStore_Delete(t *testing.T) {
	defer cleanCLICallbackStore()

	storeCLICallback("del-id", "http://localhost/cb")

	_, ok := getCLICallback("del-id")
	if !ok {
		t.Fatal("expected callback to exist before delete")
	}

	deleteCLICallback("del-id")

	_, ok = getCLICallback("del-id")
	if ok {
		t.Error("expected callback to be gone after delete")
	}
	assertRegistryConsistent(t, cliCallbackStore)
}

func TestCLICallbackStore_CleansExpiredOnStore(t *testing.T) {
	defer cleanCLICallbackStore()

	storeExpiredCLICallback("old-id", "http://old.example.com")

	storeCLICallback("new-id", "http://new.example.com")

	cliCallbackStore.mu.RLock()
	_, oldExists := cliCallbackStore.entries["old-id"]
	_, newExists := cliCallbackStore.entries["new-id"]
	cliCallbackStore.mu.RUnlock()

	if oldExists {
		t.Error("expected expired entry to be cleaned up")
	}
	if !newExists {
		t.Error("expected new entry to exist")
	}
	assertRegistryConsistent(t, cliCallbackStore)
}

func TestCLICallbackRegistry_ExpiresAfterTTL(t *testing.T) {
	clock := newFakeClock()
	store := newCLICallbackRegistry(10, cliCallbackTTL, clock.Now)

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

func TestCLICallbackRegistry_CapacityEvictsOldestKeepsNewest(t *testing.T) {
	clock := newFakeClock()
	const capacity = 3
	store := newCLICallbackRegistry(capacity, cliCallbackTTL, clock.Now)

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

func TestCLICallbackRegistry_ExpiredEntriesMakeRoomBeforeEviction(t *testing.T) {
	clock := newFakeClock()
	store := newCLICallbackRegistry(2, cliCallbackTTL, clock.Now)

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

func TestCLICallbackRegistry_RestoringAnIDReplacesItWithoutEviction(t *testing.T) {
	clock := newFakeClock()
	store := newCLICallbackRegistry(2, cliCallbackTTL, clock.Now)

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

func TestCLICallbackRegistry_RemoveFreesCapacity(t *testing.T) {
	clock := newFakeClock()
	store := newCLICallbackRegistry(2, cliCallbackTTL, clock.Now)

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

func TestCLICallbackRegistry_EvictionsReportedAtMostOncePerInterval(t *testing.T) {
	clock := newFakeClock()
	store := newCLICallbackRegistry(1, cliCallbackTTL, clock.Now)

	if evicted := store.put("a", "http://localhost/a"); evicted != 0 {
		t.Fatalf("no eviction yet, got report %d", evicted)
	}
	if evicted := store.put("b", "http://localhost/b"); evicted != 1 {
		t.Fatalf("expected the first eviction to be reported at once, got %d", evicted)
	}

	// Two more evictions inside the interval are held back.
	for _, id := range []string{"c", "d"} {
		clock.Advance(cliCallbackEvictionReportInterval / 4)
		if evicted := store.put(id, "http://localhost/"+id); evicted != 0 {
			t.Fatalf("expected no report inside the interval, got %d", evicted)
		}
	}

	// The first put once the interval has passed reports both.
	clock.Advance(cliCallbackEvictionReportInterval / 2)
	if evicted := store.put("e", "http://localhost/e"); evicted != 3 {
		t.Fatalf("expected the held-back evictions plus this one (3), got %d", evicted)
	}

	// Without further evictions there is nothing to report.
	clock.Advance(cliCallbackEvictionReportInterval)
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

// TestCLICallbackRegistry_InsertCostDoesNotScaleWithSize inserts 10,000
// distinct live entries into a registry that holds all of them and into one at
// the production capacity. A put does constant work, so the median of the last
// 100 puts stays within a small factor of the median of the first 100, and all
// 10,000 finish inside insertBound. A put that walked the store would make the
// last puts on the 10,000-entry registry orders of magnitude slower than the
// first. Medians keep a GC pause in either window from deciding the result.
func TestCLICallbackRegistry_InsertCostDoesNotScaleWithSize(t *testing.T) {
	const (
		inserts     = 10000
		window      = 100
		maxSlowdown = 10
		insertBound = time.Second
	)

	for _, capacity := range []int{inserts, cliCallbackCapacity} {
		t.Run(fmt.Sprintf("capacity_%d", capacity), func(t *testing.T) {
			store := newCLICallbackRegistry(capacity, cliCallbackTTL, time.Now)
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

func TestCLICallbackRegistry_ConcurrentUseStaysBoundedAndConsistent(t *testing.T) {
	const capacity = 50
	store := newCLICallbackRegistry(capacity, cliCallbackTTL, time.Now)

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

// TestOAuthLoginHTTPHandler_CLICallbackAtCapacityEvictsOldestAndLogsOnce drives
// the login handler against a full process-wide store: the start succeeds, the
// oldest pending callback makes room, and the eviction is logged for the first
// start and not again inside the report interval.
func TestOAuthLoginHTTPHandler_CLICallbackAtCapacityEvictsOldestAndLogsOnce(t *testing.T) {
	cleanCLICallbackStore()
	t.Cleanup(cleanCLICallbackStore)

	for i := range cliCallbackCapacity {
		storeCLICallback(fmt.Sprintf("fill-%04d", i), "http://localhost:8888/fill")
	}

	handler := NewOAuthHTTPHandler(nil, nil, nil)
	start := func() (*httptest.ResponseRecorder, string) {
		w, req := oauthLoginRequest("google")
		req.URL.RawQuery = "cli_callback=http://localhost:8888/cli-cb"
		output := testlog.Capture(t, func() {
			handler.OAuthLoginHTTPHandler(w, testlog.Request(t, req))
		})
		return w, output
	}

	w, output := start()
	callbackID := cliCallbackID(t, w)
	if url, ok := getCLICallback(callbackID); !ok || url != "http://localhost:8888/cli-cb" {
		t.Fatalf("expected the new start's callback to be stored, got %q, %v", url, ok)
	}
	if _, ok := getCLICallback("fill-0000"); ok {
		t.Error("expected the oldest pending callback to be evicted")
	}
	if _, ok := getCLICallback("fill-0001"); !ok {
		t.Error("expected only the oldest pending callback to be evicted")
	}
	if size := cliCallbackStore.size(); size != cliCallbackCapacity {
		t.Errorf("expected the store to stay at capacity %d, got %d", cliCallbackCapacity, size)
	}
	for _, want := range []string{
		"msg=oauth_cli_callback_store_evicting",
		"evicted=1",
		fmt.Sprintf("capacity=%d", cliCallbackCapacity),
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in the log; captured log:\n%s", want, output)
		}
	}
	if strings.Contains(output, callbackID) {
		t.Errorf("the login handler logged the minted callback id; captured log:\n%s", output)
	}

	var cookieMaxAge int
	for _, c := range w.Result().Cookies() {
		if c.Name == "cli_callback_id" {
			cookieMaxAge = c.MaxAge
		}
	}
	if want := int(cliCallbackTTL / time.Second); cookieMaxAge != want {
		t.Errorf("expected the cli_callback_id cookie MaxAge %d, got %d", want, cookieMaxAge)
	}

	_, output = start()
	if !strings.Contains(output, "msg=oauth_cli_callback_stored") {
		t.Fatalf("expected the second start to log its stored callback; captured log:\n%s", output)
	}
	if strings.Contains(output, "msg=oauth_cli_callback_store_evicting") {
		t.Errorf("expected no second eviction log inside the report interval; captured log:\n%s", output)
	}
}
