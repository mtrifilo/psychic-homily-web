package auth

import (
	"container/list"
	"sync"
	"time"
)

const (
	// cliCallbackTTL is how long a stored CLI callback stays usable. The
	// cli_callback_id cookie lives exactly as long.
	cliCallbackTTL = 5 * time.Minute

	// cliCallbackCapacity bounds the store. Past it the oldest entry is
	// evicted, so a flood of CLI starts costs the oldest pending CLI sign-in
	// its loopback redirect rather than growing the process's memory.
	cliCallbackCapacity = 1000

	// cliCallbackEvictionReportInterval is the shortest gap between two
	// eviction reports, so a sustained flood logs once a minute rather than
	// once per start.
	cliCallbackEvictionReportInterval = time.Minute
)

// cliCallbackStore holds CLI callback URLs by the opaque id parked in the
// cli_callback_id cookie, for the length of one OAuth round trip.
// Process-local: a callback that lands on a restarted process finds nothing
// and falls back to the web flow.
var cliCallbackStore = newCLICallbackRegistry(cliCallbackCapacity, cliCallbackTTL, time.Now)

// cliCallbackRegistry is a capacity-bounded map of CLI callbacks with a fixed
// TTL.
//
// Every entry gets the same TTL from a clock read under the lock, so insertion
// order is expiry order: the front of order is both the oldest entry and the
// next to expire. That lets put expire and evict from the front only, so no
// operation walks the whole store.
type cliCallbackRegistry struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	now      func() time.Time

	// entries and order hold the same set: entries indexes order's elements
	// by id, and order holds *cliCallbackEntry values oldest first.
	entries map[string]*list.Element
	order   *list.List

	evictedSinceReport int
	lastReportAt       time.Time
}

type cliCallbackEntry struct {
	id          string
	callbackURL string
	expiresAt   time.Time
}

// newCLICallbackRegistry panics on a capacity below 1: put always stores the
// new entry, so it needs room for at least one.
func newCLICallbackRegistry(capacity int, ttl time.Duration, now func() time.Time) *cliCallbackRegistry {
	if capacity < 1 {
		panic("cliCallbackRegistry capacity must be at least 1")
	}
	return &cliCallbackRegistry{
		capacity: capacity,
		ttl:      ttl,
		now:      now,
		entries:  make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

// put stores callbackURL under id, replacing any entry already there. It first
// drops expired entries, then evicts the oldest live entries until there is
// room.
//
// evictionsToReport is nonzero when entries were evicted for capacity since
// the last report and at least cliCallbackEvictionReportInterval has passed
// since it; the count covers every eviction since that report. The caller logs
// it; put never logs, so it holds the lock only for map and list work.
func (s *cliCallbackRegistry) put(id, callbackURL string) (evictionsToReport int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.removeLocked(id)
	for front := s.order.Front(); front != nil && now.After(front.Value.(*cliCallbackEntry).expiresAt); front = s.order.Front() {
		s.removeElementLocked(front)
	}
	for s.order.Len() >= s.capacity {
		s.removeElementLocked(s.order.Front())
		s.evictedSinceReport++
	}

	s.entries[id] = s.order.PushBack(&cliCallbackEntry{
		id:          id,
		callbackURL: callbackURL,
		expiresAt:   now.Add(s.ttl),
	})

	if s.evictedSinceReport > 0 && (s.lastReportAt.IsZero() || now.Sub(s.lastReportAt) >= cliCallbackEvictionReportInterval) {
		evictionsToReport = s.evictedSinceReport
		s.evictedSinceReport = 0
		s.lastReportAt = now
	}
	return evictionsToReport
}

// get returns the callback stored under id, or false when there is none or it
// has expired. It does not consume the entry; remove does.
func (s *cliCallbackRegistry) get(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	element, ok := s.entries[id]
	if !ok {
		return "", false
	}
	entry := element.Value.(*cliCallbackEntry)
	if s.now().After(entry.expiresAt) {
		return "", false
	}
	return entry.callbackURL, true
}

func (s *cliCallbackRegistry) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(id)
}

func (s *cliCallbackRegistry) removeLocked(id string) {
	if element, ok := s.entries[id]; ok {
		s.removeElementLocked(element)
	}
}

func (s *cliCallbackRegistry) removeElementLocked(element *list.Element) {
	delete(s.entries, element.Value.(*cliCallbackEntry).id)
	s.order.Remove(element)
}

// storeCLICallback stores callbackURL under id in the process-wide store. Its
// result is put's evictionsToReport.
func storeCLICallback(id, callbackURL string) (evictionsToReport int) {
	return cliCallbackStore.put(id, callbackURL)
}

func getCLICallback(id string) (string, bool) {
	return cliCallbackStore.get(id)
}

func deleteCLICallback(id string) {
	cliCallbackStore.remove(id)
}
