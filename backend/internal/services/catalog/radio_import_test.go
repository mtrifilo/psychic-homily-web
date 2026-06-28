package catalog

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestRetryTransientConflict exercises the bounded retry-on-transient-conflict
// loop in isolation (no DB): it must retry on a Postgres 40001 (serialization
// failure) OR a 40P01 (deadlock), cap at playUpsertMaxAttempts, and pass any
// other error (or success) straight through.
//
// There is no integration test for the real upsert's retry path because a 40001
// cannot arise on INSERT … ON CONFLICT DO NOTHING under the production READ
// COMMITTED isolation, and a 40P01 deadlock — while possible at READ COMMITTED —
// can't be induced deterministically here (the per-station advisory lock
// serializes same-station writes; the retry is mostly defense-in-depth for a
// future higher isolation level — see retryTransientConflict). The loop's
// behavior is therefore verified via a stub op, and the detection predicates it
// relies on are covered by shared.TestIsSerializationFailure / TestIsDeadlock
// (incl. the TranslateError-survival cases).
func TestRetryTransientConflict(t *testing.T) {
	serErr := &pgconn.PgError{Code: "40001"}      // serialization_failure
	deadlockErr := &pgconn.PgError{Code: "40P01"} // deadlock_detected

	t.Run("succeeds on first attempt", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error { calls++; return nil })
		if err != nil {
			t.Fatalf("want nil err, got %v", err)
		}
		if calls != 1 {
			t.Errorf("want 1 call, got %d", calls)
		}
	})

	t.Run("retries past transient 40001 then succeeds", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error {
			calls++
			if calls < playUpsertMaxAttempts {
				return serErr
			}
			return nil
		})
		if err != nil {
			t.Fatalf("want nil err after transient 40001s, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("want %d calls, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("retries on a 40P01 deadlock too", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error {
			calls++
			if calls < playUpsertMaxAttempts {
				return deadlockErr
			}
			return nil
		})
		if err != nil {
			t.Fatalf("want nil err after transient deadlocks, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("deadlock must be retried; want %d calls, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("surfaces persistent 40001 after max attempts", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error { calls++; return serErr })
		if !errors.Is(err, serErr) {
			t.Fatalf("want the 40001 surfaced, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("want %d attempts before surfacing, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("non-conflict error returns immediately, unchanged", func(t *testing.T) {
		boom := errors.New("FK violation")
		calls := 0
		err := retryTransientConflict(func() error { calls++; return boom })
		if !errors.Is(err, boom) {
			t.Fatalf("want the original error unchanged, got %v", err)
		}
		if calls != 1 {
			t.Errorf("non-conflict error must not retry; got %d calls", calls)
		}
	})
}

// TestFetchSince covers the incremental-fetch lower-bound logic (PSY-1230). The
// load-bearing case is "recent last fetch" — before the floor, a forward-only
// `since` let weekly shows slip permanently behind the window. The provider-side
// filter that consumes `since` is covered separately by
// TestWFMU_ParseArchivePage_SinceFilter.
func TestFetchSince(t *testing.T) {
	// Fixed clock (mid-day) so we also assert the floor is normalized to midnight.
	now := time.Date(2026, 6, 26, 18, 30, 0, 0, time.UTC)
	today := now.UTC().Truncate(24 * time.Hour)           // 2026-06-26 00:00 UTC
	floor := today.AddDate(0, 0, -fetchLookbackFloorDays) // 2026-05-12 00:00 UTC (45d)

	t.Run("default floor is the deliberately-chosen 45 days (PSY-1241/PSY-1268)", func(t *testing.T) {
		// Pinned to the literal so an accidental change to the DEFAULT trips this test.
		// 45d covers the stage roster's max regular cadence (~31d) with margin; retune
		// deliberately (and update this line), or override at runtime via
		// RADIO_FETCH_LOOKBACK_FLOOR_DAYS — see TestResolveFetchLookbackFloorDays.
		if fetchLookbackFloorDays != 45 {
			t.Errorf("fetchLookbackFloorDays = %d, want 45", fetchLookbackFloorDays)
		}
	})

	t.Run("nil last fetch (cold start) uses the same floor as a steady-state fetch", func(t *testing.T) {
		// A first fetch must never look back less than a subsequent one — that is the
		// one place a monthly show's latest episode could be missed before the floor
		// takes over.
		got := fetchSince(nil, now, fetchLookbackFloorDays)
		if !got.Equal(floor) {
			t.Errorf("cold-start since must equal the floor: got %v, want %v", got, floor)
		}
	})

	t.Run("recent last fetch is floored to UTC midnight (the weekly-show stall)", func(t *testing.T) {
		// A fetch 2h ago would, unfloored, skip a weekly show that aired days ago.
		// The floor must win, and it must be midnight-aligned (not carry 18:30).
		recent := now.Add(-2 * time.Hour)
		got := fetchSince(&recent, now, fetchLookbackFloorDays)
		if !got.Equal(floor) {
			t.Errorf("recent last fetch must be floored: got %v, want floor %v", got, floor)
		}
		if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 {
			t.Errorf("since must be midnight-aligned, got %v", got)
		}
	})

	t.Run("last fetch older than the floor still wins (re-enabled station / recovered outage)", func(t *testing.T) {
		// Older than the 45d floor, so the catch-up branch returns the true lastFetch
		// rather than clamping forward to the floor.
		old := now.AddDate(0, 0, -60)
		got := fetchSince(&old, now, fetchLookbackFloorDays)
		if !got.Equal(old) {
			t.Errorf("stale last fetch must win: got %v, want %v", got, old)
		}
	})

	t.Run("a wider configured floorDays widens the window for both branches (PSY-1268)", func(t *testing.T) {
		// RADIO_FETCH_LOOKBACK_FLOOR_DAYS=90 → `since` reaches back 90d, and the
		// cold-start branch tracks it.
		wide := today.AddDate(0, 0, -90)
		recent := now.Add(-2 * time.Hour)
		if got := fetchSince(&recent, now, 90); !got.Equal(wide) {
			t.Errorf("recent fetch with floorDays=90: got %v, want %v", got, wide)
		}
		if got := fetchSince(nil, now, 90); !got.Equal(wide) {
			t.Errorf("cold-start with floorDays=90: got %v, want %v", got, wide)
		}
		// The catch-up boundary moves with the param: a 60d-old lastFetch wins at the
		// default 45d floor (tested above) but is INSIDE a 90d floor, so the floor wins.
		old60 := now.AddDate(0, 0, -60)
		if got := fetchSince(&old60, now, 90); !got.Equal(wide) {
			t.Errorf("60d-old lastFetch under a 90d floor must clamp to the floor: got %v, want %v", got, wide)
		}
	})
}

// TestResolveFetchLookbackFloorDays covers the RADIO_FETCH_LOOKBACK_FLOOR_DAYS env
// override (PSY-1268): a positive value within [1, maxFetchLookbackFloorDays] wins;
// anything else (unset/empty, 0, negative, over-max, garbage) falls back to the
// fetchLookbackFloorDays default — a 0/negative floor would reintroduce the PSY-1230
// permanent-skip bug and an over-max one would make KEXP page its whole archive. The
// default VALUE itself is pinned to the literal in TestFetchSince.
func TestResolveFetchLookbackFloorDays(t *testing.T) {
	cases := []struct {
		name, env string
		want      int
	}{
		{"unset/empty uses default", "", fetchLookbackFloorDays},
		{"valid override wins", "90", 90},
		{"at the max wins", "365", maxFetchLookbackFloorDays},
		{"over the max is ignored (KEXP archive-walk guard)", "100000", fetchLookbackFloorDays},
		{"zero is ignored (no floor would reintroduce PSY-1230)", "0", fetchLookbackFloorDays},
		{"negative is ignored", "-5", fetchLookbackFloorDays},
		{"garbage is ignored", "soon", fetchLookbackFloorDays},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RADIO_FETCH_LOOKBACK_FLOOR_DAYS", tc.env)
			if got := resolveFetchLookbackFloorDays(); got != tc.want {
				t.Errorf("resolveFetchLookbackFloorDays() with env=%q = %d, want %d", tc.env, got, tc.want)
			}
		})
	}
}

// TestShouldAdvanceLastFetch covers the per-run gate that keeps a wholly-failed
// run from advancing last_playlist_fetch_at (PSY-1241). Holding the timestamp stale
// on a total failure — whether the fetch failed or every fetched episode failed to
// persist — is what lets fetchSince's catch-up branch re-scan the true gap on
// recovery.
func TestShouldAdvanceLastFetch(t *testing.T) {
	cases := []struct {
		name                                  string
		attempts, success, returned, imported int
		want                                  bool
	}{
		{"no fetchable shows advances (nothing to catch up on)", 0, 0, 0, 0, true},
		{"all shows succeeded and imported advances", 5, 5, 20, 20, true},
		{"partial fetch success advances", 5, 3, 12, 12, true},
		{"fetched but nothing new (empty) advances", 5, 5, 0, 0, true},
		{"partial import progress advances", 5, 5, 8, 3, true},
		{"total fetch failure holds the timestamp", 5, 0, 0, 0, false},
		{"single show fetch outage holds the timestamp", 1, 0, 0, 0, false},
		{"fetched but every import failed holds the timestamp", 5, 5, 8, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldAdvanceLastFetch(tc.attempts, tc.success, tc.returned, tc.imported)
			if got != tc.want {
				t.Errorf("shouldAdvanceLastFetch(%d, %d, %d, %d) = %v, want %v",
					tc.attempts, tc.success, tc.returned, tc.imported, got, tc.want)
			}
		})
	}
}

// TestFetchWindow_AdmitsMonthlyAndBiweeklyCadence proves the 45-day floor admits
// the cadences it was raised to cover (PSY-1241 AC: biweekly/monthly inclusion). It
// composes fetchSince (which floors `since`) with episodeInWindow (the shared
// inclusive-lower-bound predicate) to prove the floor WIDTH covers these cadences;
// a regression in either, or in the floor value, that would re-skip them trips here.
// It does not drive the NTS provider's own filter (that path compares
// episodeFilterTime's StartsAt instant — covered by the NTS provider tests); the
// lower-bound boundary semantics are equivalent because `since` is midnight-
// truncated and an episode's AirDate is its StartsAt calendar day.
func TestFetchWindow_AdmitsMonthlyAndBiweeklyCadence(t *testing.T) {
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * time.Hour) // steady state: lastFetch ~recent → since = floor
	since := fetchSince(&recent, now, fetchLookbackFloorDays)
	until := now

	inWindow := func(daysAgo int) bool {
		airDate := now.AddDate(0, 0, -daysAgo).Format("2006-01-02")
		return episodeInWindow(RadioEpisodeImport{AirDate: airDate}, since, until)
	}

	// Cadences the floor must cover: biweekly (~14d), the dominant NTS monthly (~28d),
	// and the observed monthly tail (~42d), with a day of margin (44d).
	for _, daysAgo := range []int{14, 28, 42, 44} {
		if !inWindow(daysAgo) {
			t.Errorf("episode aired %dd ago must fall inside the 45-day floored window (since=%v)", daysAgo, since)
		}
	}
	// Beyond the floor, an episode is correctly outside the steady-state window.
	if inWindow(46) {
		t.Errorf("episode aired 46d ago must be outside the 45-day floor (since=%v)", since)
	}
}
