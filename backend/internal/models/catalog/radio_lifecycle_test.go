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
