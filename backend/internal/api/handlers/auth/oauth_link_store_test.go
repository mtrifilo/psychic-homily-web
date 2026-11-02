package auth

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// storeExpiredOAuthLinkIntent plants an already-expired intent in the
// process-wide store.
func storeExpiredOAuthLinkIntent(id string, intent oauthLinkIntent) {
	oauthLinkIntentStore.putExpired(id, intent)
}

// consumeOAuthLinkIntent removes whatever intent the process-wide store holds
// under id, matching or not, so a test leaves nothing armed behind it.
func consumeOAuthLinkIntent(id string) {
	oauthLinkIntentStore.remove(id)
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

// An intent stays live for oauthLinkIntentTTL from the moment it is stored.
func TestOAuthLinkIntentStore_ExpiresAfterTheIntentTTL(t *testing.T) {
	oauthLinkIntentStore.reset()
	t.Cleanup(oauthLinkIntentStore.reset)

	before := time.Now()
	storeOAuthLinkIntent("ttl-intent", oauthLinkIntent{userID: 1, provider: "google", state: "state"})
	after := time.Now()

	oauthLinkIntentStore.mu.RLock()
	expiresAt := entryOf[oauthLinkIntent](oauthLinkIntentStore.entries["ttl-intent"]).expiresAt
	oauthLinkIntentStore.mu.RUnlock()

	if expiresAt.Before(before.Add(oauthLinkIntentTTL)) || expiresAt.After(after.Add(oauthLinkIntentTTL)) {
		t.Errorf("expiresAt %v is not %v after the store (between %v and %v)",
			expiresAt, oauthLinkIntentTTL, before.Add(oauthLinkIntentTTL), after.Add(oauthLinkIntentTTL))
	}
}

// Of many concurrent callbacks carrying one intent, exactly one takes it; the
// rest find nothing, which the callback handles as an intent it does not hold.
func TestOAuthLinkIntentStore_ConcurrentTakesForOneHandshakeHaveOneWinner(t *testing.T) {
	const takers = 32
	oauthLinkIntentStore.reset()
	t.Cleanup(oauthLinkIntentStore.reset)

	for round := range 50 {
		id := fmt.Sprintf("race-intent-%02d", round)
		storeOAuthLinkIntent(id, oauthLinkIntent{userID: 1, provider: "google", state: "race-state"})

		var wg sync.WaitGroup
		var winners, unheld atomic.Int32
		start := make(chan struct{})
		for range takers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _, found, taken := takeOAuthLinkIntentForHandshake(id, "google", "race-state")
				switch {
				case taken:
					winners.Add(1)
				case !found:
					unheld.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := winners.Load(); got != 1 {
			t.Fatalf("round %d: %d callbacks took the intent, want exactly 1", round, got)
		}
		if got := unheld.Load(); got != takers-1 {
			t.Fatalf("round %d: %d losing callbacks found nothing, want %d", round, got, takers-1)
		}
	}
}

// A callback for another handshake leaves the intent armed for its own.
func TestOAuthLinkIntentStore_TakeForAnotherHandshakeLeavesTheIntent(t *testing.T) {
	oauthLinkIntentStore.reset()
	t.Cleanup(oauthLinkIntentStore.reset)
	storeOAuthLinkIntent("held", oauthLinkIntent{userID: 1, provider: "google", state: "its-state"})

	for _, other := range []struct{ provider, state string }{
		{"google", "another-state"},
		{"github", "its-state"},
	} {
		if _, _, found, taken := takeOAuthLinkIntentForHandshake("held", other.provider, other.state); !found || taken {
			t.Fatalf("take for %s/%s: found %v taken %v, want found and not taken", other.provider, other.state, found, taken)
		}
	}
	intent, expired, found, taken := takeOAuthLinkIntentForHandshake("held", "google", "its-state")
	if !taken || expired || intent.userID != 1 {
		t.Fatalf("its own handshake: intent %+v expired %v found %v taken %v, want it taken live", intent, expired, found, taken)
	}
}
