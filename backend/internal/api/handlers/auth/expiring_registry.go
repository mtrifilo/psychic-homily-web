package auth

import (
	"container/list"
	"sync"
	"time"
)

// capacityReportInterval is the shortest gap between two capacity reports from
// one bounded store, so a sustained flood logs once a minute rather than once
// per insert.
const capacityReportInterval = time.Minute

// reportThrottle counts capacity events (evictions, refusals) and hands the
// count over for logging at most once per capacityReportInterval.
type reportThrottle struct {
	pending      int
	lastReportAt time.Time
}

// release returns the count of events since the last report, and resets it,
// when there is at least one and capacityReportInterval has passed since that
// report; otherwise 0.
func (t *reportThrottle) release(now time.Time) int {
	if t.pending == 0 || (!t.lastReportAt.IsZero() && now.Sub(t.lastReportAt) < capacityReportInterval) {
		return 0
	}
	released := t.pending
	t.pending = 0
	t.lastReportAt = now
	return released
}

// expiringRegistry is a capacity-bounded, process-local map of values by
// opaque id, each kept for a fixed TTL.
//
// Every entry gets the same TTL from a clock read under the lock, so insertion
// order is expiry order: the front of order is both the oldest entry and the
// next to expire. That lets put expire and evict from the front only, so no
// operation walks the whole registry.
type expiringRegistry[T any] struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	now      func() time.Time

	// entries and order hold the same set: entries indexes order's elements
	// by id, and order holds *registryEntry[T] values oldest first.
	entries map[string]*list.Element
	order   *list.List

	evictions reportThrottle
}

type registryEntry[T any] struct {
	id        string
	value     T
	expiresAt time.Time
}

// newExpiringRegistry panics on a capacity below 1: put always stores the new
// entry, so it needs room for at least one.
func newExpiringRegistry[T any](capacity int, ttl time.Duration, now func() time.Time) *expiringRegistry[T] {
	if capacity < 1 {
		panic("expiringRegistry capacity must be at least 1")
	}
	return &expiringRegistry[T]{
		capacity: capacity,
		ttl:      ttl,
		now:      now,
		entries:  make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

// put stores value under id, replacing any entry already there. It first
// drops expired entries, then evicts the oldest live entries until there is
// room.
//
// evictionsToReport is nonzero when entries were evicted for capacity since
// the last report and at least capacityReportInterval has passed since
// it; the count covers every eviction since that report. The caller logs it;
// put never logs, so it holds the lock only for map and list work.
func (s *expiringRegistry[T]) put(id string, value T) (evictionsToReport int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.removeLocked(id)
	for front := s.order.Front(); front != nil && now.After(entryOf[T](front).expiresAt); front = s.order.Front() {
		s.removeElementLocked(front)
	}
	for s.order.Len() >= s.capacity {
		s.removeElementLocked(s.order.Front())
		s.evictions.pending++
	}

	s.entries[id] = s.order.PushBack(&registryEntry[T]{
		id:        id,
		value:     value,
		expiresAt: now.Add(s.ttl),
	})

	return s.evictions.release(now)
}

// take removes the entry stored under id and returns its value, or false when
// there is none or it has expired. Lookup and removal share one lock hold, so
// of any number of concurrent takes of one id at most one gets the value.
func (s *expiringRegistry[T]) take(id string) (T, bool) {
	value, expired, _, taken := s.takeIf(id, func(T) bool { return true })
	if !taken || expired {
		var zero T
		return zero, false
	}
	return value, true
}

// takeIf removes the entry stored under id and returns its value when match
// accepts that value, expired or not, with expired reporting which. An entry
// match refuses is left in place. found is false only when the registry holds
// nothing under id; taken is false when it holds an entry match refused.
//
// Lookup, match and removal share one lock hold, so of any number of
// concurrent takeIfs of one id at most one takes the entry. match runs under
// that lock: it must not call back into the registry.
func (s *expiringRegistry[T]) takeIf(id string, match func(T) bool) (value T, expired, found, taken bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	element, ok := s.entries[id]
	if !ok {
		return value, false, false, false
	}
	entry := entryOf[T](element)
	if !match(entry.value) {
		return value, false, true, false
	}
	s.removeElementLocked(element)
	return entry.value, s.now().After(entry.expiresAt), true, true
}

// peek returns the value stored under id without consuming it. An expired
// entry is still returned, with expired set, until a put drops it or it is
// removed; found is false only when the registry holds nothing under id.
func (s *expiringRegistry[T]) peek(id string) (value T, expired, found bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	element, ok := s.entries[id]
	if !ok {
		return value, false, false
	}
	entry := entryOf[T](element)
	return entry.value, s.now().After(entry.expiresAt), true
}

func (s *expiringRegistry[T]) removeLocked(id string) {
	if element, ok := s.entries[id]; ok {
		s.removeElementLocked(element)
	}
}

func (s *expiringRegistry[T]) removeElementLocked(element *list.Element) {
	delete(s.entries, entryOf[T](element).id)
	s.order.Remove(element)
}

func entryOf[T any](element *list.Element) *registryEntry[T] {
	return element.Value.(*registryEntry[T])
}
