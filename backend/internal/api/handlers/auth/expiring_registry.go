package auth

import (
	"container/list"
	"sync"
	"time"
)

// registryEvictionReportInterval is the shortest gap between two eviction
// reports from one registry, so a sustained flood logs once a minute rather
// than once per insert.
const registryEvictionReportInterval = time.Minute

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

	evictedSinceReport int
	lastReportAt       time.Time
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
// the last report and at least registryEvictionReportInterval has passed since
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
		s.evictedSinceReport++
	}

	s.entries[id] = s.order.PushBack(&registryEntry[T]{
		id:        id,
		value:     value,
		expiresAt: now.Add(s.ttl),
	})

	if s.evictedSinceReport > 0 && (s.lastReportAt.IsZero() || now.Sub(s.lastReportAt) >= registryEvictionReportInterval) {
		evictionsToReport = s.evictedSinceReport
		s.evictedSinceReport = 0
		s.lastReportAt = now
	}
	return evictionsToReport
}

// take removes the entry stored under id and returns its value, or false when
// there is none or it has expired. Lookup and removal share one lock hold, so
// of any number of concurrent takes of one id at most one gets the value.
func (s *expiringRegistry[T]) take(id string) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var zero T
	element, ok := s.entries[id]
	if !ok {
		return zero, false
	}
	s.removeElementLocked(element)
	entry := entryOf[T](element)
	if s.now().After(entry.expiresAt) {
		return zero, false
	}
	return entry.value, true
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

func (s *expiringRegistry[T]) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(id)
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
