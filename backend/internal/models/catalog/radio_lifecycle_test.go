package catalog

import (
	"testing"
	"time"
)

// TestComputeEpisodeStatus locks the episode lifecycle state machine (PSY-1152):
// status derives from the FROZEN air window + playlist completeness + now, and a
// windowless or unbounded episode is NEVER falsely "live" (the PSY-1128 bug).
func TestComputeEpisodeStatus(t *testing.T) {
	start := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC) // Tue 9am
	end := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)  // Tue noon
	during := time.Date(2026, 6, 16, 10, 30, 0, 0, time.UTC)
	before := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	after := time.Date(2026, 6, 16, 17, 54, 0, 0, time.UTC) // 5:54pm — the PSY-1128 moment

	ptr := func(tm time.Time) *time.Time { return &tm }

	cases := []struct {
		name          string
		starts, ends  *time.Time
		playlistState string
		now           time.Time
		want          string
	}{
		// Bounded window (KEXP).
		{"before window → scheduled", ptr(start), ptr(end), RadioPlaylistStatePending, before, RadioEpisodeStatusScheduled},
		{"inside window → live", ptr(start), ptr(end), RadioPlaylistStatePending, during, RadioEpisodeStatusLive},
		{"at start (inclusive) → live", ptr(start), ptr(end), RadioPlaylistStatePending, start, RadioEpisodeStatusLive},
		{"at end (inclusive) → live", ptr(start), ptr(end), RadioPlaylistStatePending, end, RadioEpisodeStatusLive},
		{"after window, pending → aired (the PSY-1128 fix: NOT live at 5:54pm)", ptr(start), ptr(end), RadioPlaylistStatePending, after, RadioEpisodeStatusAired},
		{"after window, complete → archived", ptr(start), ptr(end), RadioPlaylistStateComplete, after, RadioEpisodeStatusArchived},

		// Start but no end (NTS): never live; settles once started.
		{"start-only, before → scheduled", ptr(start), nil, RadioPlaylistStatePending, before, RadioEpisodeStatusScheduled},
		{"start-only, after start, pending → aired (never live without an end)", ptr(start), nil, RadioPlaylistStatePending, during, RadioEpisodeStatusAired},
		{"start-only, after start, complete → archived", ptr(start), nil, RadioPlaylistStateComplete, after, RadioEpisodeStatusArchived},

		// Windowless (WFMU before PSY-1159, or any provider with no time): never live.
		{"windowless, pending → aired", nil, nil, RadioPlaylistStatePending, during, RadioEpisodeStatusAired},
		{"windowless, complete → archived", nil, nil, RadioPlaylistStateComplete, during, RadioEpisodeStatusArchived},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeEpisodeStatus(tc.starts, tc.ends, tc.playlistState, tc.now)
			if got != tc.want {
				t.Errorf("ComputeEpisodeStatus(%v, %v, %q, %v) = %q, want %q",
					tc.starts, tc.ends, tc.playlistState, tc.now, got, tc.want)
			}
		})
	}
}

// TestComputePlaylistState locks the PSY-1154 post-air completeness transition table:
// a fetch with plays settles to complete (aired) or partial (live); a fetch with no
// playlist on an aired episode burns an attempt and gives up (unavailable) at the cap;
// a non-aired episode never burns an attempt.
func TestComputePlaylistState(t *testing.T) {
	const cap = 3

	cases := []struct {
		name                           string
		isAired, hasPlays, fetchFailed bool
		attempts                       int
		wantState                      string
		wantAttempts                   int
	}{
		// Success paths — plays returned, no failure.
		{"aired + plays → complete", true, true, false, 0, RadioPlaylistStateComplete, 0},
		{"aired + plays → complete (attempts untouched)", true, true, false, 2, RadioPlaylistStateComplete, 2},
		{"live + plays → partial (growing, no attempt)", false, true, false, 0, RadioPlaylistStatePartial, 0},

		// Empty-success / fetch-failure on a non-aired episode — expected, never counts.
		{"live + no plays → pending (no attempt)", false, false, false, 0, RadioPlaylistStatePending, 0},
		{"live + fetch failed → pending (no attempt)", false, false, true, 1, RadioPlaylistStatePending, 1},

		// Failed post-air attempts — increment, then give up at the cap.
		{"aired + no plays, first attempt → pending, attempts=1", true, false, false, 0, RadioPlaylistStatePending, 1},
		{"aired + fetch failed, first attempt → pending, attempts=1", true, false, true, 0, RadioPlaylistStatePending, 1},
		{"aired + no plays, below cap → pending, attempts=2", true, false, false, 1, RadioPlaylistStatePending, 2},
		{"aired + no plays, reaches cap → unavailable", true, false, false, 2, RadioPlaylistStateUnavailable, 3},

		// Defensive: a fetch error reported alongside plays still counts as a failure.
		{"aired + plays BUT fetch failed → failed attempt", true, true, true, 0, RadioPlaylistStatePending, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, attempts := ComputePlaylistState(tc.isAired, tc.hasPlays, tc.fetchFailed, tc.attempts, cap)
			if state != tc.wantState || attempts != tc.wantAttempts {
				t.Errorf("ComputePlaylistState(aired=%v, plays=%v, failed=%v, attempts=%d, cap=%d) = (%q, %d), want (%q, %d)",
					tc.isAired, tc.hasPlays, tc.fetchFailed, tc.attempts, cap, state, attempts, tc.wantState, tc.wantAttempts)
			}
		})
	}
}

// TestShouldBackfillPlaylist locks the single backfill-eligibility predicate shared by
// importEpisode's re-fetch branch and the sweep's candidate query: only an aired,
// still-incomplete episode with attempts left is eligible.
func TestShouldBackfillPlaylist(t *testing.T) {
	const cap = 3
	start := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	during := time.Date(2026, 6, 16, 10, 30, 0, 0, time.UTC)
	before := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	after := time.Date(2026, 6, 16, 17, 0, 0, 0, time.UTC)
	ptr := func(tm time.Time) *time.Time { return &tm }

	cases := []struct {
		name         string
		starts, ends *time.Time
		state        string
		attempts     int
		now          time.Time
		want         bool
	}{
		{"aired + pending + attempts left → eligible", ptr(start), ptr(end), RadioPlaylistStatePending, 0, after, true},
		{"aired + partial + attempts left → eligible", ptr(start), ptr(end), RadioPlaylistStatePartial, 1, after, true},
		{"windowless + pending → eligible (counts as aired)", nil, nil, RadioPlaylistStatePending, 0, during, true},
		{"live → NOT eligible (playlist not final)", ptr(start), ptr(end), RadioPlaylistStatePending, 0, during, false},
		{"scheduled → NOT eligible (not aired)", ptr(start), ptr(end), RadioPlaylistStatePending, 0, before, false},
		{"complete → NOT eligible", ptr(start), ptr(end), RadioPlaylistStateComplete, 0, after, false},
		{"unavailable → NOT eligible", ptr(start), ptr(end), RadioPlaylistStateUnavailable, 3, after, false},
		{"attempts at cap → NOT eligible", ptr(start), ptr(end), RadioPlaylistStatePending, cap, after, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldBackfillPlaylist(tc.starts, tc.ends, tc.state, tc.attempts, cap, tc.now)
			if got != tc.want {
				t.Errorf("ShouldBackfillPlaylist(%v, %v, %q, %d, %d, %v) = %v, want %v",
					tc.starts, tc.ends, tc.state, tc.attempts, cap, tc.now, got, tc.want)
			}
		})
	}
}
