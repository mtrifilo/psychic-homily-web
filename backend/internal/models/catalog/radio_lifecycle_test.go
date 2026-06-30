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

// TestNormalizeScheduledPlaylistState pins the PSY-1285 invariant: a not-yet-aired
// (scheduled) episode is never 'unavailable' and carries no burned backfill attempts;
// every other phase (live/aired/windowless) is left untouched.
func TestNormalizeScheduledPlaylistState(t *testing.T) {
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
		wantState    string
		wantAttempts int
	}{
		// Scheduled (now < starts): the invariant resets a stranded terminal state.
		{"scheduled + unavailable → reset", ptr(start), ptr(end), RadioPlaylistStateUnavailable, 5, before, RadioPlaylistStatePending, 0},
		{"scheduled + pending w/ burned attempts → clear attempts", ptr(start), ptr(end), RadioPlaylistStatePending, 2, before, RadioPlaylistStatePending, 0},
		{"scheduled + pending + 0 attempts → no-op", ptr(start), ptr(end), RadioPlaylistStatePending, 0, before, RadioPlaylistStatePending, 0},
		// Non-scheduled phases are left exactly as-is.
		{"aired + unavailable → untouched (PSY-1287, not this invariant)", ptr(start), ptr(end), RadioPlaylistStateUnavailable, 5, after, RadioPlaylistStateUnavailable, 5},
		{"live + pending → untouched", ptr(start), ptr(end), RadioPlaylistStatePending, 0, during, RadioPlaylistStatePending, 0},
		{"windowless + unavailable → untouched (windowless is 'aired', never scheduled)", nil, nil, RadioPlaylistStateUnavailable, 5, during, RadioPlaylistStateUnavailable, 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotState, gotAttempts := NormalizeScheduledPlaylistState(tc.starts, tc.ends, tc.state, tc.attempts, tc.now)
			if gotState != tc.wantState || gotAttempts != tc.wantAttempts {
				t.Errorf("NormalizeScheduledPlaylistState(%v,%v,%q,%d,%v) = (%q,%d), want (%q,%d)",
					tc.starts, tc.ends, tc.state, tc.attempts, tc.now, gotState, gotAttempts, tc.wantState, tc.wantAttempts)
			}
		})
	}
}

// TestWindowForDate locks the PSY-1238 schedule→air-window mapping: a WFMU
// episode's frozen [starts_at, ends_at] is built from the matching weekday slot
// in the schedule's timezone, with overnight wrap, DST-correct instants, and a
// nil window when no slot matches (so ComputeEpisodeStatus settles to aired).
func TestWindowForDate(t *testing.T) {
	ny := func() *time.Location {
		loc, err := time.LoadLocation("America/New_York")
		if err != nil {
			t.Fatalf("load America/New_York: %v", err)
		}
		return loc
	}()
	// 2026-06-26 is a Friday (EDT, UTC-4); 2026-01-09 is a Friday (EST, UTC-5).
	sched := func(slots ...RadioScheduleSlot) *RadioSchedule {
		return &RadioSchedule{Timezone: "America/New_York", Slots: slots}
	}

	t.Run("normal daytime slot → same-day window in schedule tz", func(t *testing.T) {
		s := sched(RadioScheduleSlot{DayOfWeek: 5, Start: "15:00", End: "18:00"}) // Fri 3-6pm
		start, end, err := s.WindowForDate("2026-06-26")
		if err != nil || start == nil || end == nil {
			t.Fatalf("got (%v,%v,%v), want a window", start, end, err)
		}
		wantStart := time.Date(2026, 6, 26, 15, 0, 0, 0, ny)
		wantEnd := time.Date(2026, 6, 26, 18, 0, 0, 0, ny)
		if !start.Equal(wantStart) || !end.Equal(wantEnd) {
			t.Errorf("got [%v, %v], want [%v, %v]", start, end, wantStart, wantEnd)
		}
	})

	t.Run("overnight slot (End <= Start) ends next day", func(t *testing.T) {
		s := sched(RadioScheduleSlot{DayOfWeek: 5, Start: "21:00", End: "00:00"}) // Fri 9pm-Mid
		start, end, err := s.WindowForDate("2026-06-26")
		if err != nil || start == nil || end == nil {
			t.Fatalf("got (%v,%v,%v), want a window", start, end, err)
		}
		wantStart := time.Date(2026, 6, 26, 21, 0, 0, 0, ny)
		wantEnd := time.Date(2026, 6, 27, 0, 0, 0, 0, ny) // next day midnight
		if !start.Equal(wantStart) || !end.Equal(wantEnd) {
			t.Errorf("got [%v, %v], want [%v, %v]", start, end, wantStart, wantEnd)
		}
		if !end.After(*start) {
			t.Errorf("overnight end %v must be after start %v", end, start)
		}
	})

	t.Run("overnight slot ending in the spring-forward gap stays ordered (fails safe)", func(t *testing.T) {
		// 2026-03-08 is the US spring-forward day; 02:00–02:59 doesn't exist. A
		// Sat 23:30→02:30 slot wraps into that gap. We don't assert the exact
		// normalized instant (Go's choice), only that a window is produced and
		// end stays after start — the window can close early but never inverts.
		s := sched(RadioScheduleSlot{DayOfWeek: 6, Start: "23:30", End: "02:30"}) // Sat 11:30pm–2:30am
		start, end, err := s.WindowForDate("2026-03-07")                          // Saturday
		if err != nil || start == nil || end == nil {
			t.Fatalf("got (%v,%v,%v), want a window", start, end, err)
		}
		if !end.After(*start) {
			t.Errorf("DST-gap end %v must still be after start %v", end, start)
		}
	})

	t.Run("DST-aware: same wall-clock slot, different UTC offset in winter vs summer", func(t *testing.T) {
		s := sched(RadioScheduleSlot{DayOfWeek: 5, Start: "15:00", End: "18:00"})
		summer, _, err := s.WindowForDate("2026-06-26") // EDT (UTC-4)
		if err != nil || summer == nil {
			t.Fatalf("summer: got (%v,%v), want a window", summer, err)
		}
		winter, _, err := s.WindowForDate("2026-01-09") // EST (UTC-5)
		if err != nil || winter == nil {
			t.Fatalf("winter: got (%v,%v), want a window", winter, err)
		}
		// 15:00 local is 19:00Z in EDT but 20:00Z in EST — a fixed offset would
		// collapse them; an IANA zone keeps them an hour apart.
		if summer.UTC().Hour() != 19 {
			t.Errorf("summer 15:00 EDT should be 19:00Z, got %d:00Z", summer.UTC().Hour())
		}
		if winter.UTC().Hour() != 20 {
			t.Errorf("winter 15:00 EST should be 20:00Z, got %d:00Z", winter.UTC().Hour())
		}
	})

	t.Run("post-midnight slot resolves on its corrected calendar day (PSY-1283)", func(t *testing.T) {
		// A WFMU 3-6am show sits in the PREVIOUS day's grid column (broadcast-day grid,
		// 6am→6am) but airs the next calendar day. After PSY-1283 the slot is stored with
		// the real airing weekday (Sunday), so an episode airing Sunday 2026-06-28 resolves
		// to a Sun 03:00–06:00 window — the F4 "Freeform Jazz Dance" case.
		s := sched(RadioScheduleSlot{DayOfWeek: 0, Start: "03:00", End: "06:00"}) // Sunday 3-6am
		start, end, err := s.WindowForDate("2026-06-28")                          // a Sunday
		if err != nil || start == nil || end == nil {
			t.Fatalf("got (%v,%v,%v), want a Sunday window", start, end, err)
		}
		wantStart := time.Date(2026, 6, 28, 3, 0, 0, 0, ny)
		wantEnd := time.Date(2026, 6, 28, 6, 0, 0, 0, ny)
		if !start.Equal(wantStart) || !end.Equal(wantEnd) {
			t.Errorf("got [%v, %v], want [%v, %v]", start, end, wantStart, wantEnd)
		}
		// The pre-fix day (Saturday=6) leaves the Sunday episode WINDOWLESS — Impact #1 of
		// the off-by-one (nil air-window → ComputeEpisodeStatus settles to aired, never live).
		buggy := sched(RadioScheduleSlot{DayOfWeek: 6, Start: "03:00", End: "06:00"})
		if bs, be, _ := buggy.WindowForDate("2026-06-28"); bs != nil || be != nil {
			t.Errorf("pre-fix Saturday slot must yield nil window for a Sunday air_date, got [%v, %v]", bs, be)
		}
	})

	t.Run("no slot for the weekday → nil window (off-schedule airing)", func(t *testing.T) {
		s := sched(RadioScheduleSlot{DayOfWeek: 1, Start: "06:00", End: "10:00"}) // Mon only
		start, end, err := s.WindowForDate("2026-06-26")                          // Friday
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if start != nil || end != nil {
			t.Errorf("want nil window for an unscheduled weekday, got [%v, %v]", start, end)
		}
	})

	t.Run("multiple slots same weekday → earliest-start wins, independent of array order", func(t *testing.T) {
		// Later slot listed FIRST: a stable pick must still choose the earliest
		// start (09:00), not the array-order head — so a re-ordered scrape can't
		// flip a frozen window.
		s := sched(
			RadioScheduleSlot{DayOfWeek: 5, Start: "20:00", End: "21:00"},
			RadioScheduleSlot{DayOfWeek: 5, Start: "09:00", End: "10:00"},
		)
		start, _, err := s.WindowForDate("2026-06-26")
		if err != nil || start == nil {
			t.Fatalf("got (%v,%v), want a window", start, err)
		}
		if start.Hour() != 9 {
			t.Errorf("earliest same-day slot should win (09:00), got %d:00", start.Hour())
		}
	})

	t.Run("empty schedule → nil window", func(t *testing.T) {
		start, end, err := sched().WindowForDate("2026-06-26")
		if err != nil || start != nil || end != nil {
			t.Errorf("empty schedule: got (%v,%v,%v), want all nil", start, end, err)
		}
	})

	t.Run("invalid air_date → error", func(t *testing.T) {
		s := sched(RadioScheduleSlot{DayOfWeek: 5, Start: "15:00", End: "18:00"})
		if _, _, err := s.WindowForDate("not-a-date"); err == nil {
			t.Error("want an error for a malformed air_date")
		}
	})

	t.Run("invalid timezone → error", func(t *testing.T) {
		s := &RadioSchedule{Timezone: "Bogus/Zone", Slots: []RadioScheduleSlot{{DayOfWeek: 5, Start: "15:00", End: "18:00"}}}
		if _, _, err := s.WindowForDate("2026-06-26"); err == nil {
			t.Error("want an error for an unloadable timezone")
		}
	})
}
