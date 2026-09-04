package engagement

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A feed token is public and its holder controls the polling, so the key space
// is user-driven and the cache is what a slow leak looks like: one entry per
// user who ever polled, expiring only on that same user's next request.
func TestPersonalFeedCache_StaysAtTheCap(t *testing.T) {
	var c personalFeedCache

	for i := uint(0); i < personalFeedCacheMaxEntries*3; i++ {
		c.store(i, []byte("BEGIN:VCALENDAR"), time.Minute)
	}

	assert.Equal(t, personalFeedCacheMaxEntries, c.len(),
		"an unbounded cache grows with every user who has ever polled")
}

// A new key at the cap costs ONE entry, not the whole map. The distinction is
// the point: these routes are exempt from the public-read limiter, so a
// whole-map drop would let a handful of accounts evict every real subscriber's
// feed on demand.
func TestPersonalFeedCache_OverflowEvictsOneEntry(t *testing.T) {
	var c personalFeedCache

	// Distinct TTLs, so the entry closest to expiring is unambiguous. Entry 1 is
	// the soonest and is the one eviction must choose.
	for i := uint(1); i <= personalFeedCacheMaxEntries; i++ {
		c.store(i, []byte("filler"), time.Duration(i)*time.Minute)
	}
	require.Equal(t, personalFeedCacheMaxEntries, c.len(), "cache should be exactly at the cap")

	c.store(personalFeedCacheMaxEntries+1, []byte("overflow"), time.Hour)

	assert.Equal(t, personalFeedCacheMaxEntries, c.len(),
		"one new key must cost one entry, not the map")

	_, ok := c.load(1)
	assert.False(t, ok, "the entry closest to expiring is the one evicted")
	_, ok = c.load(2)
	assert.True(t, ok, "every other subscriber keeps their entry")
	_, ok = c.load(personalFeedCacheMaxEntries + 1)
	assert.True(t, ok, "the new entry is cached")
}

// Re-storing a key already in the cache is the ordinary case once the cache is
// full: calendar clients poll on a schedule and refresh their own entry. It must
// evict nobody.
func TestPersonalFeedCache_RefreshAtTheCapEvictsNothing(t *testing.T) {
	var c personalFeedCache

	for i := uint(1); i <= personalFeedCacheMaxEntries; i++ {
		c.store(i, []byte("filler"), time.Minute)
	}
	require.Equal(t, personalFeedCacheMaxEntries, c.len())

	c.store(1, []byte("refreshed"), time.Minute)

	assert.Equal(t, personalFeedCacheMaxEntries, c.len(),
		"a refresh at the cap must not evict")
	for i := uint(1); i <= personalFeedCacheMaxEntries; i++ {
		_, ok := c.load(i)
		require.True(t, ok, "entry %d was evicted by a refresh of an existing key", i)
	}
}

// An expired entry is dead weight, so it is the one eviction spends before it
// touches a live one.
func TestPersonalFeedCache_OverflowPrefersExpiredEntries(t *testing.T) {
	var c personalFeedCache

	c.store(1, []byte("stale"), -time.Second)
	for i := uint(2); i <= personalFeedCacheMaxEntries; i++ {
		c.store(i, []byte("filler"), time.Hour)
	}
	require.Equal(t, personalFeedCacheMaxEntries, c.len())

	c.store(personalFeedCacheMaxEntries+1, []byte("overflow"), time.Hour)

	_, ok := c.load(1)
	assert.False(t, ok, "the expired entry is gone")
	for i := uint(2); i <= personalFeedCacheMaxEntries; i++ {
		_, ok := c.load(i)
		require.True(t, ok, "live entry %d must survive while an expired one exists", i)
	}
}

func TestPersonalFeedCache_ExpiredEntryIsAMiss(t *testing.T) {
	var c personalFeedCache

	c.store(7, []byte("stale"), -time.Second)
	_, ok := c.load(7)
	assert.False(t, ok, "an expired entry must not be served")
	assert.Zero(t, c.len(), "a miss on an expired entry must also drop it")
}

func TestPersonalFeedCache_HandsOutCopies(t *testing.T) {
	var c personalFeedCache

	original := []byte("BEGIN:VCALENDAR")
	c.store(3, original, time.Minute)

	// Mutating the slice the caller stored must not reach the cache.
	original[0] = 'X'

	got, ok := c.load(3)
	require.True(t, ok)
	assert.Equal(t, "BEGIN:VCALENDAR", string(got), "store must copy")

	// Nor must mutating what a read handed back.
	got[0] = 'Y'
	again, ok := c.load(3)
	require.True(t, ok)
	assert.Equal(t, "BEGIN:VCALENDAR", string(again), "load must copy")
}

func TestPersonalFeedCache_DeleteInvalidates(t *testing.T) {
	var c personalFeedCache

	c.store(5, []byte("payload"), time.Minute)
	c.delete(5)

	_, ok := c.load(5)
	assert.False(t, ok, "a regenerated token must not serve the old feed")
}

// Calendar clients poll on their own schedule, so the cache is shared mutable
// state on the hot path. Run under -race, this is what proves the locking is
// real rather than plausible.
func TestPersonalFeedCache_ConcurrentAccessIsSafe(t *testing.T) {
	var c personalFeedCache

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := uint(n % 8)
			c.store(id, []byte("BEGIN:VCALENDAR"), time.Minute)
			c.load(id)
			c.delete(id)
			c.len()
		}(i)
	}
	wg.Wait()
}
