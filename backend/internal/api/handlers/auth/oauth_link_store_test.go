package auth

import (
	"fmt"
	"testing"
)

// storeExpiredOAuthLinkIntent plants an already-expired intent in the
// process-wide store.
func storeExpiredOAuthLinkIntent(id string, intent oauthLinkIntent) {
	oauthLinkIntentStore.putExpired(id, intent)
}

// TestOAuthLinkIntentStore_TenThousandInsertsStayBounded drives the
// process-wide intent store through storeOAuthLinkIntent: 10,000 live intents
// leave it at capacity, oldest evicted first, with the evictions reported for
// logging. TestExpiringRegistry_InsertCostDoesNotScaleWithSize times the same
// put.
func TestOAuthLinkIntentStore_TenThousandInsertsStayBounded(t *testing.T) {
	const inserts = 10000
	oauthLinkIntentStore.reset()
	t.Cleanup(oauthLinkIntentStore.reset)

	reported := 0
	for i := range inserts {
		reported += storeOAuthLinkIntent(fmt.Sprintf("intent-%05d", i), oauthLinkIntent{
			userID:   1,
			provider: "google",
			state:    "state",
		})
	}

	if size := oauthLinkIntentStore.size(); size != oauthLinkIntentCapacity {
		t.Errorf("expected the store to stay at capacity %d, got %d", oauthLinkIntentCapacity, size)
	}
	if reported == 0 {
		t.Error("expected the evictions to be reported for logging")
	}
	if _, _, found := peekOAuthLinkIntent("intent-00000"); found {
		t.Error("expected the oldest intent to be evicted")
	}
	if _, _, found := peekOAuthLinkIntent(fmt.Sprintf("intent-%05d", inserts-1)); !found {
		t.Error("expected the newest intent to be kept")
	}
	assertRegistryConsistent(t, oauthLinkIntentStore)
}
