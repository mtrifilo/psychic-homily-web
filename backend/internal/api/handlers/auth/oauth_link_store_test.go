package auth

import (
	"fmt"
	"testing"
	"time"
)

// TestOAuthLinkIntentStore_TenThousandInsertsStayBoundedAndFlat drives the
// process-wide intent store through storeOAuthLinkIntent: 10,000 live intents
// leave it at capacity, and the median of the last 100 inserts stays within a
// small factor of the median of the first 100.
func TestOAuthLinkIntentStore_TenThousandInsertsStayBoundedAndFlat(t *testing.T) {
	const (
		inserts     = 10000
		window      = 100
		maxSlowdown = 10
	)
	oauthLinkIntentStore.reset()
	t.Cleanup(oauthLinkIntentStore.reset)

	ids := make([]string, inserts)
	for i := range ids {
		ids[i] = fmt.Sprintf("intent-%05d", i)
	}
	durations := make([]time.Duration, inserts)
	reported := 0
	for i, id := range ids {
		putStart := time.Now()
		reported += storeOAuthLinkIntent(id, oauthLinkIntent{
			userID:    1,
			provider:  "google",
			state:     "state",
			expiresAt: time.Now().Add(oauthLinkIntentTTL),
		})
		durations[i] = time.Since(putStart)
	}

	firstMedian := medianDuration(durations[:window])
	lastMedian := medianDuration(durations[inserts-window:])
	t.Logf("median insert %v over the first %d, %v over the last %d; size %d",
		firstMedian, window, lastMedian, window, oauthLinkIntentStore.size())

	if size := oauthLinkIntentStore.size(); size != oauthLinkIntentCapacity {
		t.Errorf("expected the store to stay at capacity %d, got %d", oauthLinkIntentCapacity, size)
	}
	if lastMedian > maxSlowdown*max(firstMedian, time.Microsecond) {
		t.Errorf("median insert grew from %v to %v as the store filled", firstMedian, lastMedian)
	}
	if reported == 0 {
		t.Error("expected the evictions to be reported for logging")
	}
	if _, found := peekOAuthLinkIntent(ids[0]); found {
		t.Error("expected the oldest intent to be evicted")
	}
	if _, found := peekOAuthLinkIntent(ids[inserts-1]); !found {
		t.Error("expected the newest intent to be kept")
	}
	assertRegistryConsistent(t, oauthLinkIntentStore)
}
