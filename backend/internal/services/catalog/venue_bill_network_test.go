package catalog

// Unit coverage for the venue bill-network node cap (PSY-1461). The cap is
// the mechanism that bounds the frontend's synchronous warmup cost, so its
// ranking semantics are pinned here as pure-function tests (no DB); the
// end-to-end wiring is covered by VenueBillNetworkIntegrationSuite in
// internal/api/handlers/catalog.

import (
	"testing"
	"time"
)

// TestCapVenueBillArtistsUnderCapIsNoop: rosters at or under the ceiling
// pass through untouched — no artist or per-show membership is dropped.
func TestCapVenueBillArtistsUnderCapIsNoop(t *testing.T) {
	artists := map[uint]*venueBillArtistAggregate{
		1: {ID: 1, AtVenueShowCount: 5},
		2: {ID: 2, AtVenueShowCount: 1},
	}
	byShow := map[uint]map[uint]struct{}{
		10: {1: {}, 2: {}},
	}

	capVenueBillArtists(artists, byShow)

	if len(artists) != 2 {
		t.Fatalf("expected 2 artists untouched, got %d", len(artists))
	}
	if len(byShow[10]) != 2 {
		t.Fatalf("expected show membership untouched, got %d", len(byShow[10]))
	}
}

// TestCapVenueBillArtistsRanking: over-cap rosters keep the top
// venueBillMaxNodes by (at-venue show count desc, last played desc, ID asc),
// and the per-show artist sets are filtered to the survivors so the pair
// build can't resurrect a dropped artist as an edge endpoint.
func TestCapVenueBillArtistsRanking(t *testing.T) {
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	older := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)

	artists := make(map[uint]*venueBillArtistAggregate)
	// Two regulars — highest show counts, must survive.
	artists[1] = &venueBillArtistAggregate{ID: 1, AtVenueShowCount: 9, LastPlayedAt: older}
	artists[2] = &venueBillArtistAggregate{ID: 2, AtVenueShowCount: 4, LastPlayedAt: older}
	// One-off artists: IDs 3..152 (150 of them), all count=1. IDs 3..151
	// played recently; ID 152 played long ago — the last-played tiebreak
	// must drop it before any recent one-off despite equal show counts.
	for id := uint(3); id <= 152; id++ {
		lastPlayed := newer
		if id == 152 {
			lastPlayed = older
		}
		artists[id] = &venueBillArtistAggregate{ID: id, AtVenueShowCount: 1, LastPlayedAt: lastPlayed}
	}
	// 152 artists total → 2 over the cap. Expected drops: ID 152 (oldest
	// last-played among count=1) and ID 151 (highest ID among the remaining
	// equal-count, equal-date one-offs).
	byShow := map[uint]map[uint]struct{}{
		20: {1: {}, 2: {}, 151: {}, 152: {}},
	}

	capVenueBillArtists(artists, byShow)

	if len(artists) != venueBillMaxNodes {
		t.Fatalf("expected %d artists after cap, got %d", venueBillMaxNodes, len(artists))
	}
	for _, id := range []uint{1, 2, 3, 150} {
		if _, ok := artists[id]; !ok {
			t.Errorf("artist %d should survive the cap", id)
		}
	}
	for _, id := range []uint{151, 152} {
		if _, ok := artists[id]; ok {
			t.Errorf("artist %d should be dropped by the cap", id)
		}
	}
	if got := len(byShow[20]); got != 2 {
		t.Errorf("expected show 20 filtered to the 2 surviving members, got %d", got)
	}
	for _, id := range []uint{1, 2} {
		if _, ok := byShow[20][id]; !ok {
			t.Errorf("surviving artist %d should remain in show 20's set", id)
		}
	}
}

// TestCapVenueBillArtistsDeterministic: equal count + equal last-played
// ranks by ID ascending, so repeated calls over identically-shaped maps
// (whose iteration order is random) keep the same survivors.
func TestCapVenueBillArtistsDeterministic(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	build := func() map[uint]*venueBillArtistAggregate {
		m := make(map[uint]*venueBillArtistAggregate)
		for id := uint(1); id <= uint(venueBillMaxNodes)+5; id++ {
			m[id] = &venueBillArtistAggregate{ID: id, AtVenueShowCount: 1, LastPlayedAt: when}
		}
		return m
	}

	for run := 0; run < 3; run++ {
		artists := build()
		capVenueBillArtists(artists, map[uint]map[uint]struct{}{})
		for id := uint(1); id <= uint(venueBillMaxNodes); id++ {
			if _, ok := artists[id]; !ok {
				t.Fatalf("run %d: expected lowest-ID artists kept, missing %d", run, id)
			}
		}
	}
}
