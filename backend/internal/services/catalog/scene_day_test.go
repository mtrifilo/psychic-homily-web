package catalog

import (
	"testing"
	"time"
)

func TestParseCalendarDateKey(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	t.Run("valid keys round-trip through the date", func(t *testing.T) {
		got, err := ParseCalendarDateKey("2026-07-31")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.String() != "2026-07-31" {
			t.Errorf("got %s, want 2026-07-31", got)
		}
		if s := got.start(phoenix); s.Format("2006-01-02 15:04:05") != "2026-07-31 00:00:00" {
			t.Errorf("start = %s, want 2026-07-31 00:00:00", s.Format("2006-01-02 15:04:05"))
		}
	})

	t.Run("leap day of a leap year is real", func(t *testing.T) {
		if _, err := ParseCalendarDateKey("2028-02-29"); err != nil {
			t.Errorf("2028-02-29 rejected, but 2028 is a leap year: %v", err)
		}
	})

	// A date is a DATE. Parsing it in the scene's zone would make a real day
	// unparseable wherever its local midnight is skipped by a DST jump — one
	// day a year, a permanent 404 on a legitimate permalink.
	t.Run("a date whose local midnight does not exist is still real", func(t *testing.T) {
		havana, err := time.LoadLocation("America/Havana")
		if err != nil {
			t.Skipf("America/Havana unavailable: %v", err)
		}
		// Havana springs forward AT midnight; 2026-03-08 has no 00:00.
		got, err := ParseCalendarDateKey("2026-03-08")
		if err != nil {
			t.Fatalf("2026-03-08 rejected: %v", err)
		}
		if got.String() != "2026-03-08" {
			t.Errorf("got %s, want 2026-03-08", got)
		}
		// The window must still hold the day: 01:00 is the first local time
		// that exists on it, and noon is squarely inside.
		start, end := got.start(havana), got.addDays(1).start(havana)
		for _, hour := range []int{1, 12, 23} {
			at := time.Date(2026, time.March, 8, hour, 0, 0, 0, havana)
			if at.Before(start) || !at.Before(end) {
				t.Errorf("2026-03-08 %02d:00 falls outside its own window [%s, %s)", hour, start, end)
			}
		}
	})

	t.Run("rejects a year outside the representable range", func(t *testing.T) {
		for _, key := range []string{"1969-12-31", "0001-01-01"} {
			if _, err := ParseCalendarDateKey(key); err == nil {
				t.Errorf("ParseCalendarDateKey(%q) accepted an out-of-range year", key)
			}
		}
	})

	// The SERVABLE window is narrower and belongs to GetSceneDay, which has the
	// scene's clock. Parsing stays purely structural so the two concerns cannot
	// be confused — but the constant must still be the one the edge copies.
	t.Run("parsing does not apply the servable window", func(t *testing.T) {
		if _, err := ParseCalendarDateKey("1999-01-01"); err != nil {
			t.Errorf("1999-01-01 is structurally valid; rejecting it here hides the window check: %v", err)
		}
		if sceneFirstTrackedYear != 2015 {
			t.Errorf("sceneFirstTrackedYear = %d; frontend/proxy.ts and sceneDay.ts hardcode 2015 and must be updated in lockstep",
				sceneFirstTrackedYear)
		}
	})

	// Go's parser NORMALIZES an out-of-range date rather than rejecting it —
	// "2026-02-30" becomes March 2nd — so without the round-trip check every
	// impossible date would silently become a second URL for a real day's
	// content. These are the cases that check the round-trip, not the parser.
	invalid := []string{
		"2026-02-30",           // February never has 30 days
		"2026-13-01",           // month out of range
		"2026-07-32",           // day out of range
		"2027-02-29",           // 2027 is not a leap year
		"2026-7-31",            // unpadded month
		"2026-07-31T00:00:00Z", // an instant, not a calendar date
		"2026-W31",             // an ISO week key, not a date
		"tonight",
		"",
	}
	for _, key := range invalid {
		t.Run("rejects "+key, func(t *testing.T) {
			if _, err := ParseCalendarDateKey(key); err == nil {
				t.Errorf("ParseCalendarDateKey(%q) accepted an invalid key", key)
			}
		})
	}
}

// The servable window is what keeps a public endpoint from being a load
// generator: outside it every answer is an empty day, and an empty day is the
// most expensive response this surface has.
func TestDateIsServable(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}
	now := time.Date(2026, time.July, 31, 21, 0, 0, 0, phoenix)

	tests := []struct {
		date calendarDate
		want bool
	}{
		{calendarDate{2026, time.July, 31}, true},                    // today
		{calendarDate{sceneFirstTrackedYear, time.January, 1}, true}, // the floor
		{calendarDate{sceneFirstTrackedYear - 1, time.December, 31}, false},
		{calendarDate{2027, time.December, 31}, true}, // the ceiling, now + 1
		{calendarDate{2028, time.January, 1}, false},
		{calendarDate{1970, time.January, 1}, false}, // structurally valid, not servable
		{calendarDate{9999, time.December, 31}, false},
	}
	for _, tc := range tests {
		if got := dateIsServable(tc.date, now); got != tc.want {
			t.Errorf("dateIsServable(%s, now=%s) = %v, want %v",
				tc.date, now.Format("2006-01-02"), got, tc.want)
		}
	}
}

// Adjacent days must MEET — end(D) exactly equals start(D+1). An overlap lists
// a show on two dates, each of which its own permalink disagrees with; a gap
// hides it from both. Instant arithmetic gets this wrong in any zone whose DST
// transition lands at or near local midnight.
func TestCalendarDate_AdjacentWindowsMeetExactly(t *testing.T) {
	zones := []string{
		"America/Phoenix",           // no DST at all
		"America/Chicago",           // transition at 02:00
		"America/Havana",            // spring-forward AT midnight: 00:00 does not exist
		"America/Santiago",          // southern hemisphere, transition at midnight
		"Asia/Beirut",               // transition at midnight, both directions
		"Pacific/Apia",              // 2011-12-30 skipped entirely (date-line move)
		"America/Argentina/Cordoba", // a historical jump wider than one hour
		"Pacific/Kiritimati",        // 1994-12-31 skipped entirely
		"Australia/Lord_Howe",       // half-hour DST offset
		"Asia/Kathmandu",            // +05:45, a non-hour base offset
	}
	for _, name := range zones {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Skipf("%s unavailable: %v", name, err)
		}
		t.Run(name, func(t *testing.T) {
			// Every day from 1970 to 2036, so the historical skipped-day cases
			// above are actually crossed rather than merely named.
			date := calendarDate{1970, time.January, 1}
			for i := 0; i < 24_106; i++ {
				next := date.addDays(1)
				start, end := date.start(loc), next.start(loc)

				// Never inverted. Equal IS allowed and IS correct: a date-line
				// move can skip a whole date, and zero elapsed time on a date
				// that did not happen is the truth, not a bug.
				if end.Before(start) {
					t.Fatalf("%s: inverted window ([%s, %s))", date, start, end)
				}
				// Adjacent windows meet exactly — no overlap (one show on two
				// dates) and no gap (a show on neither).
				if !next.start(loc).Equal(end) {
					t.Fatalf("%s -> %s: windows do not meet", date, next)
				}
				// The property the whole feature rests on: an instant that
				// RENDERS as this date in this zone falls in this date's
				// window. Local noon exists in every real zone on every date
				// that happened at all, and it is what the show query's
				// `event_date.In(loc).Format` is effectively compared against.
				//
				// Note this is asserted on the INSTANT, not on how `start`
				// itself renders — at a midnight jump the boundary is a local
				// time that never existed, and Go displays it in the pre-jump
				// offset. The moment is right; its rendering is a gap artifact.
				if start.Before(end) {
					noon := time.Date(date.year, date.month, date.day, 12, 0, 0, 0, loc)
					if noon.Before(start) || !noon.Before(end) {
						t.Fatalf("%s: local noon (%s) falls outside its own window [%s, %s)",
							date, noon, start, end)
					}
				}
				date = next
			}
		})
	}
}

// The date walk must be pure calendar arithmetic: no zone may shift it.
func TestCalendarDate_AddDays(t *testing.T) {
	tests := []struct {
		from calendarDate
		n    int
		want string
	}{
		{calendarDate{2026, time.July, 31}, 1, "2026-08-01"},
		{calendarDate{2026, time.August, 1}, -1, "2026-07-31"},
		{calendarDate{2026, time.December, 31}, 1, "2027-01-01"},
		{calendarDate{2027, time.January, 1}, -1, "2026-12-31"},
		{calendarDate{2028, time.February, 28}, 1, "2028-02-29"}, // leap year
		{calendarDate{2027, time.February, 28}, 1, "2027-03-01"}, // not one
	}
	for _, tc := range tests {
		if got := tc.from.addDays(tc.n); got.String() != tc.want {
			t.Errorf("%s.addDays(%d) = %s, want %s", tc.from, tc.n, got, tc.want)
		}
	}
}

// The 6am boundary is the whole reason /tonight cannot be "today's date".
func TestTonightDate_RollsOverAtSixAM(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "evening is its own night",
			now:  time.Date(2026, time.July, 31, 21, 0, 0, 0, phoenix),
			want: "2026-07-31",
		},
		{
			// The case the rule exists for: someone at the venue at 01:00 is
			// standing at a show the site files under the previous date.
			name: "just after midnight is still the previous night",
			now:  time.Date(2026, time.August, 1, 1, 0, 0, 0, phoenix),
			want: "2026-07-31",
		},
		{
			name: "05:59 is the last minute of the previous night",
			now:  time.Date(2026, time.August, 1, 5, 59, 59, 0, phoenix),
			want: "2026-07-31",
		},
		{
			name: "06:00 opens the new night",
			now:  time.Date(2026, time.August, 1, 6, 0, 0, 0, phoenix),
			want: "2026-08-01",
		},
		{
			// A month boundary is where an off-by-one would be least visible.
			name: "rolls back across a month boundary",
			now:  time.Date(2026, time.August, 1, 2, 30, 0, 0, phoenix),
			want: "2026-07-31",
		},
		{
			name: "rolls back across a year boundary",
			now:  time.Date(2027, time.January, 1, 3, 0, 0, 0, phoenix),
			want: "2026-12-31",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tonightDate(tc.now); got.String() != tc.want {
				t.Errorf("tonightDate(%s) = %s, want %s",
					tc.now.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}

// The night must be named correctly in a zone that springs forward AT midnight,
// where the 00:00–01:00 the rollback would otherwise land in does not exist.
func TestTonightDate_AcrossAMidnightDSTJump(t *testing.T) {
	havana, err := time.LoadLocation("America/Havana")
	if err != nil {
		t.Skipf("America/Havana unavailable: %v", err)
	}

	// 01:30 on 2026-03-08, moments after the clocks jumped from 23:59 (Mar 7)
	// straight to 01:00. Still the night of the 7th by the 6am rule.
	now := time.Date(2026, time.March, 8, 1, 30, 0, 0, havana)
	if got := tonightDate(now); got.String() != "2026-03-07" {
		t.Errorf("tonightDate(%s) = %s, want 2026-03-07", now.Format(time.RFC3339), got)
	}

	// And once the night has rolled over, the 8th is nameable — the date whose
	// local midnight does not exist.
	morning := time.Date(2026, time.March, 8, 9, 0, 0, 0, havana)
	if got := tonightDate(morning); got.String() != "2026-03-08" {
		t.Errorf("tonightDate(%s) = %s, want 2026-03-08", morning.Format(time.RFC3339), got)
	}
}

// A day is frozen only when it can no longer gain shows AND is no longer the
// night people are out on. The two conditions come apart between midnight and
// 06:00 — which is exactly when the live night is most likely to be looked up.
func TestDayHasEnded(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}
	day := func(y int, m time.Month, d int) calendarDate {
		return calendarDate{y, m, d}
	}

	tests := []struct {
		name  string
		now   time.Time
		start calendarDate
		want  bool
	}{
		{
			name:  "tonight, mid-evening",
			now:   time.Date(2026, time.July, 31, 21, 0, 0, 0, phoenix),
			start: day(2026, time.July, 31),
			want:  false,
		},
		{
			// The case the second clause exists for. The clock is past midnight,
			// so the date's window has closed — but this is still the night the
			// reader is standing in, and freezing it would serve "Shows Tonight"
			// for a day after it ended.
			name:  "tonight, after midnight but before 6am",
			now:   time.Date(2026, time.August, 1, 1, 0, 0, 0, phoenix),
			start: day(2026, time.July, 31),
			want:  false,
		},
		{
			name:  "tonight, 05:59 — the last minute of the night",
			now:   time.Date(2026, time.August, 1, 5, 59, 59, 0, phoenix),
			start: day(2026, time.July, 31),
			want:  false,
		},
		{
			// 06:00 hands "tonight" to Aug 1, so Jul 31 finally freezes.
			name:  "the night before, once 6am has opened a new one",
			now:   time.Date(2026, time.August, 1, 6, 0, 0, 0, phoenix),
			start: day(2026, time.July, 31),
			want:  true,
		},
		{
			// The archive must still freeze during those small hours, or every
			// crawler walk costs a live query.
			name:  "a genuinely old day, asked for at 01:00",
			now:   time.Date(2026, time.August, 1, 1, 0, 0, 0, phoenix),
			start: day(2026, time.July, 30),
			want:  true,
		},
		{
			// Neither current nor past. This is the one that must never freeze:
			// the "next day →" link is on every page and gets followed before
			// that day goes live.
			name:  "a future day",
			now:   time.Date(2026, time.July, 31, 21, 0, 0, 0, phoenix),
			start: day(2026, time.August, 5),
			want:  false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tonight := tonightDate(tc.now)
			end := tc.start.addDays(1).start(phoenix)
			if got := dayHasEnded(tc.now, end, tc.start, tonight); got != tc.want {
				t.Errorf("dayHasEnded(now=%s, date=%s) = %v, want %v",
					tc.now.Format(time.RFC3339), tc.start, got, tc.want)
			}
		})
	}
}

// The invariant the frontend's cache windows rest on: a payload may be frozen
// for a day, or headed "Tonight", never both.
func TestDayHasEnded_NeverAgreesWithIsTonight(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	// Walk a full day at 30-minute steps, asking about each of three dates —
	// the 6am seam is where the two used to overlap.
	for step := 0; step < 48; step++ {
		now := time.Date(2026, time.July, 31, 0, 0, 0, 0, phoenix).
			Add(time.Duration(step) * 30 * time.Minute)
		tonight := tonightDate(now)
		for offset := -1; offset <= 1; offset++ {
			date := calendarDate{2026, time.July, 31}.addDays(offset)
			end := date.addDays(1).start(phoenix)
			if date == tonight && dayHasEnded(now, end, date, tonight) {
				t.Fatalf("at %s, %s is BOTH tonight and past — a client would freeze the live night",
					now.Format(time.RFC3339), date)
			}
		}
	}
}

// The viewer's clock must never enter this answer: the same instant asked in
// two zones has to name the same Phoenix night, which is what stops a reader in
// Berlin from being shown tomorrow's listings for a Phoenix room.
func TestTonightDate_AnswersInTheSceneZoneNotTheViewers(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	// 22:00 Friday in Phoenix is already 07:00 SATURDAY in Berlin.
	instant := time.Date(2026, time.July, 31, 22, 0, 0, 0, phoenix)

	if got := tonightDate(instant.In(phoenix)).String(); got != "2026-07-31" {
		t.Errorf("scene-zone answer = %s, want 2026-07-31", got)
	}
	// Proof that passing the VIEWER's zone would give a different night — which
	// is why GetSceneDay converts to the scene's location before calling this.
	if got := tonightDate(instant.In(berlin)).String(); got == "2026-07-31" {
		t.Fatal("Berlin-rendered instant gave the same night; the test no longer proves the zone matters")
	}
}
