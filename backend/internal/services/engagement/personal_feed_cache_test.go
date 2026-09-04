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
//
// Overflow drops the map wholesale, so the eviction to assert is that an entry
// stored before the cap is gone afterwards.
func TestPersonalFeedCache_OverflowEvictsOlderEntries(t *testing.T) {
	var c personalFeedCache

	c.store(1, []byte("first"), time.Minute)
	for i := uint(2); i <= personalFeedCacheMaxEntries; i++ {
		c.store(i, []byte("filler"), time.Minute)
	}
	require.Equal(t, personalFeedCacheMaxEntries, c.len(), "cache should be exactly at the cap")

	_, ok := c.load(1)
	require.True(t, ok, "the first entry is still live at the cap")

	c.store(personalFeedCacheMaxEntries+1, []byte("overflow"), time.Minute)

	_, ok = c.load(1)
	assert.False(t, ok, "the entry stored before the cap must have been evicted")
	assert.Equal(t, 1, c.len(), "overflow drops the whole map and keeps the new entry")
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
