package shared

import (
	"strconv"
	"testing"
	"time"

	"psychic-homily-backend/internal/testutil"
)

// The night-start rule, evaluated by POSTGRES against a stated clock.
//
// The surfaces that consume it are bounded on `now()`, so their integration
// tests can only exercise the hours the suite happens to run in: between 06:00
// and midnight the night bound and a plain midnight bound return the same date,
// and a broken shift would pass for eighteen hours a day. This is the pin that
// does not depend on when it runs.
//
// The expectations are written out rather than re-derived, so this compares the
// SQL against a table a reader can check by eye instead of against a second
// copy of the same arithmetic.
func TestNightStartDateSQL_NamesTheNightInProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cases := []struct {
		name     string
		localNow string
		want     string
	}{
		{"an ordinary evening", "2026-09-07 21:30:00", "2026-09-07"},
		{"the moment after midnight", "2026-09-08 00:00:00", "2026-09-07"},
		{"deep in the small hours", "2026-09-08 03:15:00", "2026-09-07"},
		{"the last second of the night", "2026-09-08 05:59:59", "2026-09-07"},
		{"the moment the night turns over", "2026-09-08 06:00:00", "2026-09-08"},
		{"mid-morning", "2026-09-08 10:00:00", "2026-09-08"},
		// A month boundary, where "subtract a day" is the step most likely to be
		// written as arithmetic on the day number alone.
		{"the small hours of the first", "2026-10-01 02:00:00", "2026-09-30"},
		{"the small hours of new year's day", "2027-01-01 01:00:00", "2026-12-31"},
		// The clock this expression reads is a wall clock with the offset already
		// resolved, so a spring-forward date is an ordinary date to it: 07:00
		// exists on 2026-03-08 in America/Denver even though 02:30 does not.
		{"the morning of a spring-forward date", "2026-03-08 07:00:00", "2026-03-08"},
		{"the small hours of a spring-forward date", "2026-03-08 01:30:00", "2026-03-07"},
	}

	query := "SELECT " + nightStartDateSQL("?::timestamp")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got time.Time
			if err := td.DB.Raw(query, tc.localNow).Scan(&got).Error; err != nil {
				t.Fatalf("evaluating the night-start rule: %v", err)
			}
			if formatted := got.Format("2006-01-02"); formatted != tc.want {
				t.Errorf("local clock %s: night-start date = %s, want %s", tc.localNow, formatted, tc.want)
			}
		})
	}
}

// The seven-night window's EDGES, evaluated by POSTGRES against a stated clock,
// for the same reason the night-start rule above is pinned here: every surface
// that consumes it reads now(), so an integration test can only exercise the
// hours it happens to run in.
//
// It evaluates the expressions VenueLocalNightWindowCondition renders, over a
// stated local clock and a stated venue-local show date, so an edit to the edge
// operator or the window length fails in the package that owns the rule rather
// than only in a database-backed catalog test.
func TestVenueLocalNightWindow_HoldsTheNightInProgressAndSixMore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	const nights = 7
	nightStart := nightStartDateSQL("?::timestamp")
	// Placeholders bind in SQL TEXT order: show date, clock, show date, clock.
	query := "SELECT ?::date >= " + nightStart +
		" AND ?::date < (" + nightStart + " + " + strconv.Itoa(nights) + ")"

	cases := []struct {
		name      string
		localNow  string
		showDate  string
		wantInSet bool
	}{
		{"tonight, read in the evening", "2026-09-07 21:30:00", "2026-09-07", true},
		{"the previous night, read in the evening", "2026-09-07 21:30:00", "2026-09-06", false},
		{"the sixth night after, read in the evening", "2026-09-07 21:30:00", "2026-09-13", true},
		{"the seventh night after, read in the evening", "2026-09-07 21:30:00", "2026-09-14", false},
		// After midnight the night in progress is still the PREVIOUS date, and
		// the whole window moves back with it rather than only its lower edge.
		{"the night in progress, read after midnight", "2026-09-08 03:15:00", "2026-09-07", true},
		{"the coming night, read after midnight", "2026-09-08 03:15:00", "2026-09-08", true},
		{"the night before, read after midnight", "2026-09-08 03:15:00", "2026-09-06", false},
		{"the seventh night after, read after midnight", "2026-09-08 03:15:00", "2026-09-14", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got bool
			if err := td.DB.Raw(query, tc.showDate, tc.localNow, tc.showDate, tc.localNow).
				Scan(&got).Error; err != nil {
				t.Fatalf("evaluating the night window: %v", err)
			}
			if got != tc.wantInSet {
				t.Errorf("clock %s, show on %s: in window = %v, want %v",
					tc.localNow, tc.showDate, got, tc.wantInSet)
			}
		})
	}
}

// The coarse UTC bound is a planner hint, so it must never be the thing that
// decides membership: every instant the exact condition keeps has to be inside
// it.
//
// It reasons about nightCoarseMarginDays, the constant the SQL fragment is
// RENDERED from, so the number under test and the number shipped are the same
// number and there is no literal to keep in step.
func TestNightUpcomingCoarseBound_CoversTheEarliestQualifyingInstant(t *testing.T) {
	// The earliest instant the exact condition keeps is local midnight on the
	// night-start date. Read just before NightStartHour that date is yesterday,
	// putting the instant NightStartHour plus one local day back, and a local day
	// runs 25 hours across a fall-back transition.
	worstCaseLag := NightStartHour*time.Hour + 25*time.Hour
	margin := nightCoarseMarginDays * 24 * time.Hour
	if worstCaseLag >= margin {
		t.Errorf("the coarse bound (%v) does not cover the earliest qualifying instant (%v behind now)",
			margin, worstCaseLag)
	}
}
