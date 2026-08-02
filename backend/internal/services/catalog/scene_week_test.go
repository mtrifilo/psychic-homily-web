package catalog

import (
	"fmt"
	"testing"
	"time"
)

// The anchors below were verified against Go's own time.ISOWeek rather than
// derived by hand, because ISO-8601 week numbering is counterintuitive in
// exactly the places a bug would hide:
//
//   - ISO week 1 of 2026 starts on 2025-12-29 — in the PREVIOUS calendar year.
//   - 2027-01-01 belongs to ISO week 2026-W53, not to 2027.
//   - 2026 has 53 ISO weeks; 2025 and 2027 have 52.
func TestISOWeekStart(t *testing.T) {
	tests := []struct {
		name  string
		year  int
		week  int
		want  string
		isoIn string // the ISO key the start date must round-trip to
	}{
		{"mid-year week", 2026, 31, "2026-07-27", "2026-W31"},
		{"week 1 starts in the previous calendar year", 2026, 1, "2025-12-29", "2026-W01"},
		{"final week of a 53-week year", 2026, 53, "2026-12-28", "2026-W53"},
		{"final week of a 52-week year", 2025, 52, "2025-12-22", "2025-W52"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ISOWeekStart(tc.year, tc.week, time.UTC)
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("ISOWeekStart(%d, %d) = %s, want %s",
					tc.year, tc.week, got.Format("2006-01-02"), tc.want)
			}
			if got.Weekday() != time.Monday {
				t.Errorf("ISOWeekStart(%d, %d) landed on %s, want Monday",
					tc.year, tc.week, got.Weekday())
			}
			if key := ISOWeekKey(got); key != tc.isoIn {
				t.Errorf("ISOWeekKey(ISOWeekStart(%d, %d)) = %s, want %s",
					tc.year, tc.week, key, tc.isoIn)
			}
		})
	}
}

func TestISOWeekKey_UsesISOYearNotCalendarYear(t *testing.T) {
	// 2027-01-01 is in ISO week 2026-W53. Formatting with Year() instead of
	// the year from ISOWeek() would produce "2027-W53", a week that does not
	// exist — and would break prev/next navigation across the year boundary.
	newYearsDay2027 := time.Date(2027, time.January, 1, 12, 0, 0, 0, time.UTC)
	if got := ISOWeekKey(newYearsDay2027); got != "2026-W53" {
		t.Errorf("ISOWeekKey(2027-01-01) = %s, want 2026-W53", got)
	}
}

func TestParseISOWeekKey(t *testing.T) {
	valid := []struct {
		key  string
		year int
		week int
	}{
		{"2026-W31", 2026, 31},
		{"2026-W01", 2026, 1},
		{"2026-W53", 2026, 53}, // 2026 genuinely has 53 weeks
		{"2026-w31", 2026, 31}, // case-insensitive
		{" 2026-W31 ", 2026, 31},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.key, func(t *testing.T) {
			y, w, err := ParseISOWeekKey(tc.key, time.UTC)
			if err != nil {
				t.Fatalf("ParseISOWeekKey(%q) unexpected error: %v", tc.key, err)
			}
			if y != tc.year || w != tc.week {
				t.Errorf("ParseISOWeekKey(%q) = (%d, %d), want (%d, %d)", tc.key, y, w, tc.year, tc.week)
			}
		})
	}

	invalid := []struct {
		name string
		key  string
	}{
		// The load-bearing case: 2025 has only 52 ISO weeks, so W53 must be
		// rejected rather than silently resolving into 2026.
		{"53rd week of a 52-week year", "2025-W53"},
		{"week zero", "2026-W00"},
		{"week 54", "2026-W54"},
		{"no week part", "2026"},
		{"empty", ""},
		{"non-numeric week", "2026-Wxx"},
		{"non-numeric year", "abcd-W31"},
		{"wrong separator", "2026/31"},
		{"year below range", "1969-W01"},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if _, _, err := ParseISOWeekKey(tc.key, time.UTC); err == nil {
				t.Errorf("ParseISOWeekKey(%q) succeeded, want error", tc.key)
			}
		})
	}
}

func TestParseISOWeekKey_RoundTripsEveryWeekOfSeveralYears(t *testing.T) {
	// Property check: for every ISO week that actually exists across a span
	// covering both 52- and 53-week years, formatting a week's Monday must
	// reproduce the key that generated it.
	for year := 2024; year <= 2028; year++ {
		// Dec 28 is always in the final ISO week of its year.
		_, weeksInYear := time.Date(year, time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
		for week := 1; week <= weeksInYear; week++ {
			key := ISOWeekKey(ISOWeekStart(year, week, time.UTC))
			gotY, gotW, err := ParseISOWeekKey(key, time.UTC)
			if err != nil {
				t.Fatalf("year %d week %d: key %q failed to parse: %v", year, week, key, err)
			}
			if gotY != year || gotW != week {
				t.Errorf("round trip for %d-W%02d produced (%d, %d) via key %q", year, week, gotY, gotW, key)
			}
		}
		// The week after the last one must not exist in this year. Built by
		// hand rather than via ISOWeekStart, which would silently normalize an
		// out-of-range week into the next year instead of producing the
		// invalid key this asserts against.
		if weeksInYear < 53 {
			overflow := fmt.Sprintf("%04d-W%02d", year, weeksInYear+1)
			if _, _, err := ParseISOWeekKey(overflow, time.UTC); err == nil {
				t.Errorf("year %d: %s should not exist but parsed", year, overflow)
			}
		}
	}
}

func TestISOWeekStart_RespectsLocation(t *testing.T) {
	// The week must begin at local midnight, not UTC midnight. Chicago is
	// UTC-5/-6, so the same ISO week starts at a different absolute instant
	// there than in UTC — that offset is exactly what keeps a 21:00 Sunday
	// Chicago show (02:00 Monday UTC) inside the correct week.
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	utcStart := ISOWeekStart(2026, 31, time.UTC)
	chiStart := ISOWeekStart(2026, 31, chicago)

	if utcStart.Format("2006-01-02") != chiStart.Format("2006-01-02") {
		t.Errorf("week should start on the same calendar date in both zones: utc=%s chicago=%s",
			utcStart.Format("2006-01-02"), chiStart.Format("2006-01-02"))
	}
	if chiStart.Hour() != 0 || chiStart.Minute() != 0 {
		t.Errorf("Chicago week start = %s, want local midnight", chiStart.Format(time.RFC3339))
	}
	if !chiStart.After(utcStart) {
		t.Errorf("Chicago midnight (%s) should be a later instant than UTC midnight (%s)",
			chiStart.Format(time.RFC3339), utcStart.Format(time.RFC3339))
	}

	// A 21:00 Sunday show in Chicago is 02:00 Monday UTC. It must fall inside
	// the week that ENDS that Sunday, not the one that begins Monday.
	sundayNight := time.Date(2026, time.August, 2, 21, 0, 0, 0, chicago)
	weekStart := ISOWeekStart(2026, 31, chicago)
	weekEnd := weekStart.AddDate(0, 0, 7)
	if !sundayNight.Before(weekEnd) || sundayNight.Before(weekStart) {
		t.Errorf("Sunday 21:00 Chicago show (%s) fell outside week 2026-W31 [%s, %s)",
			sundayNight.Format(time.RFC3339), weekStart.Format(time.RFC3339), weekEnd.Format(time.RFC3339))
	}
	if sundayNight.UTC().Format("2006-01-02") != "2026-08-03" {
		t.Fatalf("precondition failed: expected the show to be 2026-08-03 in UTC, got %s",
			sundayNight.UTC().Format("2006-01-02"))
	}
}

// weekHasEnded is what tells a client whether it may cache a week hard, so the
// cases below are the ones where a naive UTC date comparison — the thing this
// exists to keep OFF the client — reaches a different answer.
func TestWeekHasEnded(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// 2026-W31 runs Mon 2026-07-27 through Sun 2026-08-02, Chicago time.
	start := ISOWeekStart(2026, 31, chicago)
	end := start.AddDate(0, 0, 7)

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{
			name: "mid-week is not over",
			now:  time.Date(2026, time.July, 30, 12, 0, 0, 0, chicago),
			want: false,
		},
		{
			// The boundary a client gets wrong. It is already Monday in UTC
			// while Chicago is still in Sunday night — the week's busiest
			// hours, when its show list can very much still change.
			name: "Sunday 23:00 local, already Monday in UTC, is not over",
			now:  time.Date(2026, time.August, 2, 23, 0, 0, 0, chicago),
			want: false,
		},
		{
			name: "the opening instant of the following week is over",
			now:  end,
			want: true,
		},
		{
			name: "a week months behind is over",
			now:  time.Date(2026, time.December, 1, 0, 0, 0, 0, chicago),
			want: true,
		},
		{
			// A future week is neither current nor past, and freezing it is the
			// failure this flag exists to prevent: every page links to "next
			// week", so it gets fetched days before it goes live.
			name: "a future week is not over",
			now:  time.Date(2026, time.July, 1, 0, 0, 0, 0, chicago),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekHasEnded(tt.now, end); got != tt.want {
				t.Errorf("weekHasEnded(%s, %s) = %v, want %v",
					tt.now.Format(time.RFC3339), end.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

// The week spanning the US spring-forward transition is 167 hours long, not
// 168. Adding a fixed duration instead of walking calendar days would end it an
// hour late and report it as still running after the following Monday began.
func TestWeekHasEnded_AcrossDSTTransition(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	// DST starts Sun 2026-03-08, so 2026-W10 (Mon 2026-03-02 .. Sun 2026-03-08)
	// is the week that contains the transition.
	start := ISOWeekStart(2026, 10, chicago)
	end := start.AddDate(0, 0, 7)

	if got := end.Sub(start); got != 167*time.Hour {
		t.Fatalf("precondition failed: week 2026-W10 spans %s, want 167h (spring forward)", got)
	}
	if !weekHasEnded(end, end) {
		t.Errorf("week should be over at local midnight on the following Monday (%s)",
			end.Format(time.RFC3339))
	}
	if weekHasEnded(end.Add(-time.Second), end) {
		t.Errorf("week should still be running one second before %s", end.Format(time.RFC3339))
	}
}

// 2026-W53 exists and runs into 2027. This is a comparison of instants, so the
// ISO-year rollover that trips key arithmetic cannot reach it — pinned here
// because "does 2026-W53 sort before 2027-W01" is exactly the shortcut a future
// change might reach for.
func TestWeekHasEnded_AcrossISOYearRollover(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}

	start := ISOWeekStart(2026, 53, chicago)
	if got := ISOWeekKey(start); got != "2026-W53" {
		t.Fatalf("precondition failed: ISOWeekStart(2026, 53) is %s", got)
	}
	end := start.AddDate(0, 0, 7)

	// New Year's Day 2027 falls inside 2026-W53, so the week is still running.
	newYearsDay := time.Date(2027, time.January, 1, 12, 0, 0, 0, chicago)
	if weekHasEnded(newYearsDay, end) {
		t.Errorf("2026-W53 should still be running on %s", newYearsDay.Format(time.RFC3339))
	}
	// 2027-W01 opens on 2027-01-04; by then W53 is over.
	if !weekHasEnded(ISOWeekStart(2027, 1, chicago), end) {
		t.Error("2026-W53 should be over once 2027-W01 has opened")
	}
}

// The window a list count is measured over must be the window the week page
// serves, so this pins BOTH ends rather than trusting the helper to have picked
// the right Monday on its own.
func TestSceneCalendarWeekWindow(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	tests := []struct {
		name  string
		now   time.Time
		start time.Time
	}{
		{
			name:  "midweek",
			now:   time.Date(2026, 7, 30, 14, 0, 0, 0, chicago),
			start: time.Date(2026, 7, 27, 0, 0, 0, 0, chicago),
		},
		{
			name:  "the Monday itself is its own week's first instant",
			now:   time.Date(2026, 7, 27, 0, 0, 0, 0, chicago),
			start: time.Date(2026, 7, 27, 0, 0, 0, 0, chicago),
		},
		{
			name:  "the closing Sunday still belongs to the week it ends",
			now:   time.Date(2026, 8, 2, 23, 59, 59, 0, chicago),
			start: time.Date(2026, 7, 27, 0, 0, 0, 0, chicago),
		},
		{
			// ISO week 1 of 2026 opens in the previous CALENDAR year, which is
			// the case a naive "year + week number" implementation gets wrong.
			name:  "an ISO week opening in the previous calendar year",
			now:   time.Date(2025, 12, 31, 12, 0, 0, 0, chicago),
			start: time.Date(2025, 12, 29, 0, 0, 0, 0, chicago),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end := sceneCalendarWeekWindow(tc.now, chicago)
			if !start.Equal(tc.start) {
				t.Errorf("start = %s, want %s", start, tc.start)
			}
			if want := tc.start.AddDate(0, 0, 7); !end.Equal(want) {
				t.Errorf("end = %s, want %s", end, want)
			}
			if start.After(tc.now) || !end.After(tc.now) {
				t.Errorf("window [%s, %s) does not contain now=%s", start, end, tc.now)
			}
		})
	}
}

// A week is seven CALENDAR days, not 168 hours: the spring-forward week is 167
// hours long and must still end at local midnight, or a scene in that zone would
// count an hour of the following Monday as part of the week it links to.
func TestSceneCalendarWeekWindow_AcrossDSTTransition(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// US DST began 2026-03-08, inside the week opening 2026-03-02.
	start, end := sceneCalendarWeekWindow(time.Date(2026, 3, 4, 12, 0, 0, 0, chicago), chicago)

	wantEnd := time.Date(2026, 3, 9, 0, 0, 0, 0, chicago)
	if !end.Equal(wantEnd) {
		t.Errorf("end = %s, want local midnight %s", end, wantEnd)
	}
	if got := end.Sub(start); got != 167*time.Hour {
		t.Errorf("spring-forward week spans %s, want 167h", got)
	}
}

// A nil location is a caller bug, not a reason to panic partway through a list:
// an unresolvable scene timezone must still yield a usable window.
func TestSceneCalendarWeekWindow_NilLocationFallsBackToUTC(t *testing.T) {
	now := time.Date(2026, 7, 30, 14, 0, 0, 0, time.UTC)
	start, end := sceneCalendarWeekWindow(now, nil)

	if want := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Errorf("start = %s, want %s", start, want)
	}
	if want := start.AddDate(0, 0, 7); !end.Equal(want) {
		t.Errorf("end = %s, want %s", end, want)
	}
}
