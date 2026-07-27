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
