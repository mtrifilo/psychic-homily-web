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

// airedWindow is a fixed bounded air window plus the instants around it, shared by
// the playlist-eligibility tests below.
var (
	epStart    = time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	epEnd      = time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	epBefore   = time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	epDuring   = time.Date(2026, 6, 16, 10, 30, 0, 0, time.UTC)
	epAfter    = time.Date(2026, 6, 16, 17, 0, 0, 0, time.UTC)
	epDeadline = epEnd.Add(RadioPlaylistGiveUpAfter)
)

func tp(tm time.Time) *time.Time { return &tm }

// windowedFacts is a bounded-window episode with the given state/attempts, no plays
// and no post-air attempt recorded yet. withPlays adds a play count to it.
func windowedFacts(state string, attempts int) PlaylistFetchFacts {
	return PlaylistFetchFacts{
		StartsAt: tp(epStart), EndsAt: tp(epEnd), AirDate: time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
		PlaylistState: state, Attempts: attempts,
	}
}

func withPlays(f PlaylistFetchFacts, playCount int) PlaylistFetchFacts {
	f.PlayCount = playCount
	return f
}

// TestPlanPlaylistFetch locks the single eligibility predicate (PSY-1562) across all
// three time phases: scheduled never fetches, live refreshes while incomplete, and
// aired backfills subject to the cooldown and the give-up deadline.
func TestPlanPlaylistFetch(t *testing.T) {
	cases := []struct {
		name  string
		facts PlaylistFetchFacts
		now   time.Time
		want  PlaylistFetchPlan
	}{
		// Scheduled: the playlist legitimately does not exist yet.
		{"scheduled → nothing", windowedFacts(RadioPlaylistStatePending, 0), epBefore, PlaylistFetchPlan{}},
		{"scheduled + a stale unavailable label → still nothing, still not terminal",
			windowedFacts(RadioPlaylistStateUnavailable, 5), epBefore, PlaylistFetchPlan{}},

		// Live: refresh while incomplete, no cooldown, no ceiling.
		{"live + pending → live refresh", windowedFacts(RadioPlaylistStatePending, 0), epDuring,
			PlaylistFetchPlan{Fetch: true, Live: true}},
		{"live + partial → live refresh", windowedFacts(RadioPlaylistStatePartial, 0), epDuring,
			PlaylistFetchPlan{Fetch: true, Live: true}},
		{"at starts_at → live", windowedFacts(RadioPlaylistStatePending, 0), epStart,
			PlaylistFetchPlan{Fetch: true, Live: true}},
		{"at ends_at → still live", windowedFacts(RadioPlaylistStatePending, 0), epEnd,
			PlaylistFetchPlan{Fetch: true, Live: true}},
		{"live + complete → nothing", windowedFacts(RadioPlaylistStateComplete, 0), epDuring, PlaylistFetchPlan{}},

		// Aired: the post-air backfill.
		{"one ns past ends_at → backfill", windowedFacts(RadioPlaylistStatePending, 0), epEnd.Add(time.Nanosecond),
			PlaylistFetchPlan{Fetch: true}},
		{"aired + pending → backfill", windowedFacts(RadioPlaylistStatePending, 0), epAfter,
			PlaylistFetchPlan{Fetch: true}},
		{"aired + partial → backfill (the final post-air fetch)", windowedFacts(RadioPlaylistStatePartial, 0), epAfter,
			PlaylistFetchPlan{Fetch: true}},
		{"aired + complete → nothing, and never terminal", windowedFacts(RadioPlaylistStateComplete, 0), epAfter,
			PlaylistFetchPlan{}},
		// The inversion PSY-1562 turns on: the label does NOT gate post-air eligibility,
		// so a stale give-up cannot strand an episode and needs no normalizer to clear it.
		{"aired + unavailable INSIDE the deadline → still eligible", windowedFacts(RadioPlaylistStateUnavailable, 5), epAfter,
			PlaylistFetchPlan{Fetch: true}},
		{"aired + past the deadline → terminal", windowedFacts(RadioPlaylistStatePending, 0), epDeadline,
			PlaylistFetchPlan{Exhausted: true}},
		{"aired + attempt ceiling reached → terminal", windowedFacts(RadioPlaylistStatePending, RadioBackfillMaxAttempts), epAfter,
			PlaylistFetchPlan{Exhausted: true}},

		// Windowless: aired the moment it exists; the deadline comes from air_date
		// widened by the timezone air_date does not record.
		{"windowless + pending → backfill",
			PlaylistFetchFacts{AirDate: epBefore.Truncate(24 * time.Hour), PlaylistState: RadioPlaylistStatePending},
			epAfter, PlaylistFetchPlan{Fetch: true}},
		{"windowless with no air date at all → fails closed as terminal",
			PlaylistFetchFacts{PlaylistState: RadioPlaylistStatePending}, epAfter,
			PlaylistFetchPlan{Exhausted: true}},

		// Cooldown: wants a fetch, just not yet — distinct from terminal.
		{"aired but attempted 1h ago → cooling down, NOT terminal",
			PlaylistFetchFacts{StartsAt: tp(epStart), EndsAt: tp(epEnd), PlaylistState: RadioPlaylistStatePending,
				LastBackfillAttemptAt: tp(epAfter.Add(-time.Hour))},
			epAfter, PlaylistFetchPlan{}},
		{"aired and the cooldown has elapsed → backfill",
			PlaylistFetchFacts{StartsAt: tp(epStart), EndsAt: tp(epEnd), PlaylistState: RadioPlaylistStatePending,
				LastBackfillAttemptAt: tp(epAfter.Add(-RadioPlaylistRetryCooldown))},
			epAfter, PlaylistFetchPlan{Fetch: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlanPlaylistFetch(tc.facts, tc.now); got != tc.want {
				t.Errorf("PlanPlaylistFetch(%+v, %v) = %+v, want %+v", tc.facts, tc.now, got, tc.want)
			}
		})
	}
}

// TestEmptyEpisodeIsNotRefetchedOnTheNextCycle is the PSY-1558 acceptance criterion
// PSY-1562 exists to meet, driven through the real settle→plan round trip rather than
// asserted on the cooldown constant: a post-air fetch that finds nothing must leave
// the episode ineligible for the sweep that follows an hour later.
func TestEmptyEpisodeIsNotRefetchedOnTheNextCycle(t *testing.T) {
	const sweepInterval = time.Hour // RADIO_BACKFILL_INTERVAL_HOURS default

	facts := windowedFacts(RadioPlaylistStatePending, 0)
	attemptAt := epAfter

	state, attempts := SettlePlaylistStateAfterFetch(facts, false, attemptAt)
	if state != RadioPlaylistStatePending || attempts != 1 {
		t.Fatalf("after one empty post-air fetch = (%q, %d), want (pending, 1)", state, attempts)
	}

	// The row as the sweep would next read it.
	facts.PlaylistState = state
	facts.Attempts = attempts
	facts.LastBackfillAttemptAt = &attemptAt

	if plan := PlanPlaylistFetch(facts, attemptAt.Add(sweepInterval)); plan.Fetch {
		t.Errorf("the sweep one interval later re-fetched an episode just found empty: %+v", plan)
	}
	// ...and it is not terminal either — it is merely waiting.
	if plan := PlanPlaylistFetch(facts, attemptAt.Add(sweepInterval)); plan.Exhausted {
		t.Errorf("an episode inside its give-up deadline must not be terminal: %+v", plan)
	}
	if plan := PlanPlaylistFetch(facts, attemptAt.Add(RadioPlaylistRetryCooldown)); !plan.Fetch {
		t.Errorf("once the cooldown elapses the episode must be eligible again: %+v", plan)
	}
}

// TestLatePublishedTracklistIsStillCaught guards the bound that makes the retry policy
// worth having: NTS routinely publishes a tracklist 0-3 days after air and rarely 4
// (measured 2026-07-29 on PSY-1556). An episode must still be fetched on day 4 —
// including a WINDOWLESS one, whose air instant is only known to the day.
func TestLatePublishedTracklistIsStillCaught(t *testing.T) {
	airDate := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	// Worst case for a windowless row: air_date parses at UTC midnight but the episode
	// really aired late on that local day in the westernmost zone, 36h later.
	windowless := PlaylistFetchFacts{AirDate: airDate, PlaylistState: RadioPlaylistStateUnavailable, Attempts: 5}
	trueAir := airDate.Add(RadioAirDateZoneSlack)
	for _, lag := range []time.Duration{0, 24 * time.Hour, 3 * 24 * time.Hour, 4 * 24 * time.Hour} {
		if plan := PlanPlaylistFetch(windowless, trueAir.Add(lag)); !plan.Fetch {
			t.Errorf("windowless episode at a %s publish lag is not eligible: %+v", lag, plan)
		}
	}

	windowed := windowedFacts(RadioPlaylistStateUnavailable, 5)
	if plan := PlanPlaylistFetch(windowed, epEnd.Add(4*24*time.Hour)); !plan.Fetch {
		t.Errorf("windowed episode at a 4-day publish lag is not eligible: %+v", plan)
	}
}

// TestGiveUpIsTerminalAndUnresettable pins the property PSY-1558 proved the old design
// lacked. The deadline is a function of frozen air time, so once it passes there is no
// combination of state and attempt count — the two fields every old normalizer reset —
// that reopens the episode.
func TestGiveUpIsTerminalAndUnresettable(t *testing.T) {
	for _, state := range []string{RadioPlaylistStatePending, RadioPlaylistStatePartial, RadioPlaylistStateUnavailable} {
		for _, attempts := range []int{0, 1, RadioBackfillMaxAttempts} {
			facts := windowedFacts(state, attempts)
			facts.LastBackfillAttemptAt = nil // the most permissive memo there is
			plan := PlanPlaylistFetch(facts, epDeadline)
			if plan.Fetch || !plan.Exhausted {
				t.Errorf("state=%q attempts=%d past the deadline = %+v, want terminal", state, attempts, plan)
			}
		}
	}
}

// TestAttemptCeilingCannotFireBeforeTheDeadline is the structural guard against
// PSY-1558's actual defect: two bounds that contradict each other. The ceiling is
// defense-in-depth only, so it must always outlast the longest possible deadline —
// a hand-edited smaller value would silently swallow the late-publish window.
func TestAttemptCeilingCannotFireBeforeTheDeadline(t *testing.T) {
	longest := RadioPlaylistGiveUpAfter + RadioAirDateZoneSlack
	slots := int(longest / RadioPlaylistRetryCooldown)
	if RadioBackfillMaxAttempts <= slots {
		t.Errorf("RadioBackfillMaxAttempts = %d but the longest deadline holds %d cooldown slots — "+
			"the ceiling would terminate an episode before its deadline",
			RadioBackfillMaxAttempts, slots)
	}
	if RadioPlaylistGiveUpAfter < 4*24*time.Hour {
		t.Errorf("RadioPlaylistGiveUpAfter = %s, under the measured 4-day NTS publish lag", RadioPlaylistGiveUpAfter)
	}
}

// TestSettlePlaylistStateAfterFetch locks the post-fetch transition table: a fetch with
// plays settles to complete (aired) or partial (live); an aired fetch with no playlist
// burns an attempt and records the give-up once no further attempt is possible; a
// non-aired fetch never burns an attempt and never erases a live 'partial' (PSY-1370).
func TestSettlePlaylistStateAfterFetch(t *testing.T) {
	cases := []struct {
		name         string
		facts        PlaylistFetchFacts
		hasPlays     bool
		now          time.Time
		wantState    string
		wantAttempts int
	}{
		{"aired + plays → complete", windowedFacts(RadioPlaylistStatePending, 0), true, epAfter, RadioPlaylistStateComplete, 0},
		{"aired + plays leaves attempts untouched", windowedFacts(RadioPlaylistStatePending, 2), true, epAfter, RadioPlaylistStateComplete, 2},
		{"live + plays → partial", windowedFacts(RadioPlaylistStatePending, 0), true, epDuring, RadioPlaylistStatePartial, 0},
		{"live + no plays → pending, no attempt burned", windowedFacts(RadioPlaylistStatePending, 1), false, epDuring, RadioPlaylistStatePending, 1},
		{"live + no plays holds an existing partial (PSY-1370)", windowedFacts(RadioPlaylistStatePartial, 0), false, epDuring, RadioPlaylistStatePartial, 0},
		{"scheduled + no plays → pending, no attempt burned", windowedFacts(RadioPlaylistStatePending, 0), false, epBefore, RadioPlaylistStatePending, 0},
		{"aired + no plays → pending, one attempt burned", windowedFacts(RadioPlaylistStatePending, 0), false, epAfter, RadioPlaylistStatePending, 1},
		{"aired + no plays below the ceiling stays eligible", windowedFacts(RadioPlaylistStatePending, 3), false, epAfter, RadioPlaylistStatePending, 4},
		{"aired + no plays reaching the ceiling → unavailable",
			windowedFacts(RadioPlaylistStatePending, RadioBackfillMaxAttempts-1), false, epAfter, RadioPlaylistStateUnavailable, RadioBackfillMaxAttempts},
		// The give-up label is written on the LAST attempt the deadline allows, rather
		// than leaving a row that reads 'pending' forever while never being fetched again.
		{"aired + no plays with under one cooldown left → unavailable",
			windowedFacts(RadioPlaylistStatePending, 0), false, epDeadline.Add(-time.Minute), RadioPlaylistStateUnavailable, 1},
		// An episode that already has tracks never reads 'pending' or 'unavailable' —
		// those would contradict what it is showing. Same answer RederivePlaylistState
		// gives, so the two writers cannot disagree about a row.
		{"aired + empty re-fetch of an episode that has plays → partial",
			withPlays(windowedFacts(RadioPlaylistStatePartial, 0), 5), false, epAfter, RadioPlaylistStatePartial, 1},
		{"aired + empty re-fetch past the deadline with plays → still partial, never unavailable",
			withPlays(windowedFacts(RadioPlaylistStatePartial, 0), 5), false, epDeadline.Add(-time.Minute), RadioPlaylistStatePartial, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, attempts := SettlePlaylistStateAfterFetch(tc.facts, tc.hasPlays, tc.now)
			if state != tc.wantState || attempts != tc.wantAttempts {
				t.Errorf("SettlePlaylistStateAfterFetch(%+v, plays=%v, %v) = (%q, %d), want (%q, %d)",
					tc.facts, tc.hasPlays, tc.now, state, attempts, tc.wantState, tc.wantAttempts)
			}
		})
	}
}

// TestRederivePlaylistState pins the write-time invariant enforcement that replaced the
// three read-time normalizers and reopenLivePlaylistState: after the air window changes,
// a verdict settled at the old phase is re-derived at the new one.
func TestRederivePlaylistState(t *testing.T) {
	cases := []struct {
		name         string
		facts        PlaylistFetchFacts
		now          time.Time
		wantState    string
		wantAttempts int
	}{
		// PSY-1285: a corrected schedule reveals the episode has not aired yet, so no
		// give-up and no burned attempt can stand.
		{"scheduled + unavailable → pending, 0", windowedFacts(RadioPlaylistStateUnavailable, 5), epBefore, RadioPlaylistStatePending, 0},
		{"scheduled + burned attempts → pending, 0", windowedFacts(RadioPlaylistStatePending, 2), epBefore, RadioPlaylistStatePending, 0},
		{"scheduled carrying real plays is left alone", withPlays(windowedFacts(RadioPlaylistStatePartial, 1), 4), epBefore, RadioPlaylistStatePartial, 1},

		// The reopenLivePlaylistState cases, now handled by the same function: an
		// end-less row that read as 'aired' mid-broadcast is corrected once its end
		// bound arrives from the airing feed.
		{"live + complete with plays → partial", withPlays(windowedFacts(RadioPlaylistStateComplete, 0), 6), epDuring, RadioPlaylistStatePartial, 0},
		{"live + complete without plays → pending, 0", windowedFacts(RadioPlaylistStateComplete, 2), epDuring, RadioPlaylistStatePending, 0},
		{"live + unavailable → pending, 0", windowedFacts(RadioPlaylistStateUnavailable, 5), epDuring, RadioPlaylistStatePending, 0},
		{"live + pending → unchanged", windowedFacts(RadioPlaylistStatePending, 0), epDuring, RadioPlaylistStatePending, 0},

		// PSY-1287: a windowless false give-up gets a real window; inside the deadline
		// it reopens, past it the label stays terminal and truthful.
		{"aired inside the deadline + unavailable → pending, attempts KEPT", windowedFacts(RadioPlaylistStateUnavailable, 5), epAfter, RadioPlaylistStatePending, 5},
		{"aired past the deadline stays unavailable, attempts KEPT", windowedFacts(RadioPlaylistStatePending, 2), epDeadline, RadioPlaylistStateUnavailable, 2},
		{"aired + complete is final", withPlays(windowedFacts(RadioPlaylistStateComplete, 0), 9), epAfter, RadioPlaylistStateComplete, 0},
		{"aired with plays but no post-air playlist yet → partial", withPlays(windowedFacts(RadioPlaylistStatePending, 1), 5), epAfter, RadioPlaylistStatePartial, 1},
		// This function runs on EVERY re-list, so an aired episode's attempt counter must
		// survive it — a reset here would be PSY-1558's defect reintroduced on a path that
		// runs every cycle.
		{"aired re-derive never clears an earned attempt counter",
			windowedFacts(RadioPlaylistStatePending, RadioBackfillMaxAttempts-1), epAfter,
			RadioPlaylistStatePending, RadioBackfillMaxAttempts - 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, attempts := RederivePlaylistState(tc.facts, tc.now)
			if state != tc.wantState || attempts != tc.wantAttempts {
				t.Errorf("RederivePlaylistState(%+v, %v) = (%q, %d), want (%q, %d)",
					tc.facts, tc.now, state, attempts, tc.wantState, tc.wantAttempts)
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
