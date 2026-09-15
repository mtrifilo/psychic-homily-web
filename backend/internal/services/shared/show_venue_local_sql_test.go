package shared

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/utils"
)

// The state CASE is interpolated into SQL, not bound, so its contents are the
// one thing standing between utils.StateTimezones and a syntax error (or worse)
// in every listing query. These are cheap guards on that.

func TestVenueLocalStateCaseSQL_CoversEveryState(t *testing.T) {
	sql := buildVenueLocalStateCaseSQL()
	for state, zone := range utils.StateTimezones {
		want := "WHEN '" + state + "' THEN '" + zone + "'"
		if !strings.Contains(sql, want) {
			t.Errorf("state CASE is missing %q", want)
		}
	}
}

func TestVenueLocalStateCaseSQL_DefaultsToGetTimezoneForState(t *testing.T) {
	sql := buildVenueLocalStateCaseSQL()
	// An unmatched state (including the NULL a venue-less show produces) must
	// land on exactly what the Go helper would return, or the SQL and Go paths
	// disagree for the same show.
	want := " ELSE '" + utils.GetTimezoneForState("") + "' END)"
	if !strings.Contains(sql, want) {
		t.Errorf("state CASE does not default to %q:\n%s", want, sql)
	}
	// ...and the whole thing is wrapped in the country gate, whose own ELSE is
	// UTC so a non-US venue never touches the US state map.
	if !strings.HasSuffix(sql, " ELSE 'UTC' END") {
		t.Errorf("state CASE is not country-gated:\n%s", sql)
	}
	// The ALIASED column, not `venue_tz.country`: the lateral projects under
	// `venue_tz_*` names so nothing it exposes can collide with a `shows`
	// column. A rename on one side and not the other compiles fine and fails at
	// query time, so both sides are pinned here.
	if !strings.Contains(sql, "venue_tz.venue_tz_country") {
		t.Errorf("state CASE does not consult the aliased country column:\n%s", sql)
	}
	if !strings.Contains(VenueTZJoin, "AS venue_tz_country") {
		t.Errorf("lateral does not project the aliased country column:\n%s", VenueTZJoin)
	}
	if !strings.Contains(VenueTZJoin, "AS venue_tz_state") {
		t.Errorf("lateral does not project the aliased state column:\n%s", VenueTZJoin)
	}
	if !strings.Contains(VenueTZJoin, "AS venue_tz_timezone") {
		t.Errorf("lateral does not project the aliased timezone column:\n%s", VenueTZJoin)
	}
}

// Go map iteration is randomized, so an unsorted build would emit a different
// string per process — different query text, different plan cache entries, and
// test output that changes between runs for no reason.
func TestVenueLocalStateCaseSQL_IsStableAcrossBuilds(t *testing.T) {
	first := buildVenueLocalStateCaseSQL()
	for i := 0; i < 20; i++ {
		if got := buildVenueLocalStateCaseSQL(); got != first {
			t.Fatalf("state CASE is not deterministic:\n%s\nvs\n%s", first, got)
		}
	}
}

// Nothing in StateTimezones may carry a quote, backslash, or anything else that
// could terminate the literal it is pasted into. This is what makes the
// interpolation safe by construction rather than by inspection.
var safeSQLLiteral = regexp.MustCompile(`^[A-Za-z0-9/_+-]+$`)

func TestStateTimezones_AreSafeToInterpolate(t *testing.T) {
	for state, zone := range utils.StateTimezones {
		if !safeSQLLiteral.MatchString(state) {
			t.Errorf("state %q is not a safe SQL literal", state)
		}
		if !safeSQLLiteral.MatchString(zone) {
			t.Errorf("zone %q for state %q is not a safe SQL literal", zone, state)
		}
	}
}

// The fail-soft guard (PSY-1761). These pin the three properties that make an
// unresolvable venues.timezone degrade instead of raising; each one is a silent
// failure if it regresses, which is why they are asserted on the SQL text
// rather than left to the integration suite alone.
func TestVenueTZJoin_ValidatesTheStoredZoneBeforeProjectingIt(t *testing.T) {
	// Without the membership test, AT TIME ZONE raises on an unknown name and
	// takes the whole /shows feed down with it.
	if !strings.Contains(VenueTZJoin, "IN (SELECT name_lower FROM timezone_names_snapshot)") {
		t.Errorf("the lateral no longer validates the stored zone:\n%s", VenueTZJoin)
	}
	// Case-insensitively, matching AT TIME ZONE and the drift sweep. A stricter
	// guard mis-dates rows the sweep calls healthy, with nothing logged.
	if !strings.Contains(VenueTZJoin, "lower("+venueTZStoredZone+")") {
		t.Errorf("the guard is not case-insensitive:\n%s", VenueTZJoin)
	}
	// The guard belongs in the LATERAL, not beside the COALESCE it feeds:
	// venueLocalZoneSQL is dereferenced two to three times per query and
	// Postgres plans a separate SubPlan for each occurrence of an identical
	// uncorrelated subquery. One occurrence here is one SubPlan per query.
	if strings.Contains(venueLocalZoneSQL, "timezone_names_snapshot") {
		t.Errorf("the guard leaked into the per-occurrence zone expression:\n%s", venueLocalZoneSQL)
	}
	if got := strings.Count(VenueTZJoin, "timezone_names_snapshot"); got != 1 {
		t.Errorf("expected exactly one membership test in the lateral, got %d:\n%s", got, VenueTZJoin)
	}
}

// The guard and catalog.SweepVenueTimezones' drift predicate must strip the
// same whitespace. They agree because both build from this const, and this
// pins that the guard still does — a guard that trimmed more than the detector
// would mis-date rows the detector calls healthy, with nothing logged.
func TestVenueTZJoin_TrimsThroughTheSharedWhitespaceSet(t *testing.T) {
	if !strings.Contains(venueTZStoredZone, VenueTimezoneWhitespaceSQL) {
		t.Errorf("the stored-zone expression bypasses the shared whitespace set:\n%s", venueTZStoredZone)
	}
	if got := strings.Count(venueTZValidatedZoneSQL, venueTZStoredZone); got != 2 {
		t.Errorf("the validated and projected values are not the same expression, got %d uses:\n%s",
			got, venueTZValidatedZoneSQL)
	}
}

// The date and today fragments must resolve the zone identically, or a show
// could be compared against a boundary computed in a different timezone than
// its own event date.
func TestVenueLocalDateAndToday_UseTheSameZoneExpression(t *testing.T) {
	if !strings.Contains(VenueLocalDateSQL, venueLocalZoneSQL) {
		t.Error("VenueLocalDateSQL does not use the shared zone expression")
	}
	if !strings.Contains(VenueLocalTodaySQL, venueLocalZoneSQL) {
		t.Error("VenueLocalTodaySQL does not use the shared zone expression")
	}
}

func TestVenueLocalDateCondition(t *testing.T) {
	if got := VenueLocalDateCondition("all"); got != "" {
		t.Errorf(`"all" must not filter, got %q`, got)
	}
	if got := VenueLocalDateCondition("past"); !strings.Contains(got, " < ") {
		t.Errorf(`"past" must select dates before today, got %q`, got)
	}
	// Both halves must be present: the coarse bound is what keeps the query
	// sargable, the exact one is what makes it correct.
	if got := VenueLocalDateCondition("past"); !strings.Contains(got, pastCoarseBound) {
		t.Errorf(`"past" lost its sargable bound, got %q`, got)
	}
	if got := VenueLocalDateCondition("upcoming"); !strings.Contains(got, upcomingCoarseBound) {
		t.Errorf(`"upcoming" lost its sargable bound, got %q`, got)
	}
	// An unknown filter must behave like the handlers' own default rather than
	// silently returning everything.
	for _, filter := range []string{"upcoming", "", "nonsense"} {
		if got := VenueLocalDateCondition(filter); !strings.Contains(got, " >= ") {
			t.Errorf("filter %q must default to upcoming, got %q", filter, got)
		}
	}
}

// The year filter's two halves, and the one case where widening it would be a
// silent data leak rather than an empty page.
func TestVenueLocalYearCondition(t *testing.T) {
	// Non-positive means "all years", which is the ONLY input allowed to
	// produce an empty fragment.
	for _, year := range []int{0, -1, -2026} {
		got, args := VenueLocalYearCondition(year)
		if got != "" || args != nil {
			t.Errorf("year %d must not filter, got %q %v", year, got, args)
		}
	}

	sql, args := VenueLocalYearCondition(2019)
	if !strings.Contains(sql, VenueLocalYearSQL) {
		t.Errorf("year filter lost its venue-local bucket expression, got %q", sql)
	}
	if !strings.Contains(sql, "shows.event_date >= ?") || !strings.Contains(sql, "shows.event_date < ?") {
		t.Errorf("year filter lost its sargable bounds, got %q", sql)
	}
	// Three binds, and the year itself is the last of them: an interpolated
	// year would be the one injection hole in this file.
	if len(args) != 3 {
		t.Fatalf("year filter must bind 3 arguments, got %v", args)
	}
	if args[2] != 2019 {
		t.Errorf("year filter must BIND the year, got args %v and sql %q", args, sql)
	}
	if strings.Contains(sql, "2019") {
		t.Errorf("year filter interpolated the year into %q", sql)
	}

	// A year past the representable range keeps the exact predicate and drops
	// only the bounds. Returning "" here would answer "every year" to a caller
	// who asked for one.
	sql, args = VenueLocalYearCondition(maxCoarseBoundedYear + 1)
	if sql == "" {
		t.Fatal("an out-of-range year must still filter, not widen to all years")
	}
	// The bucket expression itself mentions shows.event_date, so look for the
	// bound comparisons rather than the column name.
	if strings.Contains(sql, "shows.event_date >= ?") || strings.Contains(sql, "shows.event_date < ?") {
		t.Errorf("an out-of-range year must drop its unrepresentable bounds, got %q", sql)
	}
	if len(args) != 1 || args[0] != maxCoarseBoundedYear+1 {
		t.Errorf("out-of-range year must still bind the year, got %v", args)
	}
}

// The bucket expression and the partitioning date must resolve the zone the same
// way. If they drifted, a show could be listed under one year and partitioned by
// another zone's calendar.
func TestVenueLocalYearSQL_BuildsOnTheSharedDateExpression(t *testing.T) {
	if !strings.Contains(VenueLocalYearSQL, VenueLocalDateSQL) {
		t.Errorf("VenueLocalYearSQL does not derive from VenueLocalDateSQL: %q", VenueLocalYearSQL)
	}
}

// The month filter's two halves. Same shape as the year filter above, and the
// same one case where widening it would be a silent data leak rather than an
// empty page.
func TestVenueLocalMonthCondition(t *testing.T) {
	// A pair that does not name a month is the ONLY input allowed to produce an
	// empty fragment, and the handler is what refuses those as client errors.
	for _, tc := range []struct{ year, month int }{
		{0, 11}, {-1, 11}, {2026, 0}, {2026, 13}, {2026, -3},
	} {
		got, args := VenueLocalMonthCondition(tc.year, tc.month)
		if got != "" || args != nil {
			t.Errorf("(%d, %d) must not filter, got %q %v", tc.year, tc.month, got, args)
		}
	}

	sql, args := VenueLocalMonthCondition(2026, 11)
	if !strings.Contains(sql, VenueLocalYearSQL) || !strings.Contains(sql, VenueLocalMonthSQL) {
		t.Errorf("month filter must pin BOTH year and month, got %q", sql)
	}
	if !strings.Contains(sql, "shows.event_date >= ?") || !strings.Contains(sql, "shows.event_date < ?") {
		t.Errorf("month filter lost its sargable bounds, got %q", sql)
	}
	// Four binds, the year and month last: interpolating either would be the one
	// injection hole in this file.
	if len(args) != 4 {
		t.Fatalf("month filter must bind 4 arguments, got %v", args)
	}
	if args[2] != 2026 || args[3] != 11 {
		t.Errorf("month filter must BIND year and month, got args %v", args)
	}
	if strings.Contains(sql, "2026") || strings.Contains(sql, "= 11") {
		t.Errorf("month filter interpolated its period into %q", sql)
	}

	// The coarse bounds must straddle the whole month, or a row at either edge
	// is dropped by a hint that is supposed to carry no correctness weight.
	lower, ok := args[0].(time.Time)
	if !ok {
		t.Fatalf("lower bound must be a time.Time, got %T", args[0])
	}
	upper, ok := args[1].(time.Time)
	if !ok {
		t.Fatalf("upper bound must be a time.Time, got %T", args[1])
	}
	monthStart := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
	if !lower.Before(monthStart.Add(-24 * time.Hour)) {
		t.Errorf("lower bound %v does not clear the widest inhabited offset before %v", lower, monthStart)
	}
	if !upper.After(monthEnd.Add(24 * time.Hour)) {
		t.Errorf("upper bound %v does not clear the widest inhabited offset after %v", upper, monthEnd)
	}

	// A year past the representable range keeps the exact predicate and drops
	// only the bounds, for the reason the year filter states.
	sql, args = VenueLocalMonthCondition(maxCoarseBoundedYear+1, 3)
	if sql == "" {
		t.Fatal("an out-of-range year must still filter, not widen to every month")
	}
	if strings.Contains(sql, "shows.event_date >= ?") || strings.Contains(sql, "shows.event_date < ?") {
		t.Errorf("an out-of-range month window must drop its unrepresentable bounds, got %q", sql)
	}
	if len(args) != 2 || args[0] != maxCoarseBoundedYear+1 || args[1] != 3 {
		t.Errorf("out-of-range month window must still bind its period, got %v", args)
	}
}

// The day filter. It compares against the shared venue-local DATE expression
// rather than against extracts of it, so the window and the date a row prints
// cannot drift.
func TestVenueLocalDayCondition(t *testing.T) {
	for _, tc := range []struct{ year, month, day int }{
		{0, 11, 14}, {2026, 0, 14}, {2026, 11, 0}, {2026, 11, 31}, {2027, 2, 29}, {2026, 13, 1}, {2026, 11, 32},
	} {
		got, args := VenueLocalDayCondition(tc.year, tc.month, tc.day)
		if got != "" || args != nil {
			t.Errorf("(%d, %d, %d) must not filter, got %q %v", tc.year, tc.month, tc.day, got, args)
		}
	}

	sql, args := VenueLocalDayCondition(2026, 11, 14)
	if !strings.Contains(sql, VenueLocalDateSQL+" = ?::date") {
		t.Errorf("day filter must compare the shared venue-local date expression, got %q", sql)
	}
	if !strings.Contains(sql, "shows.event_date >= ?") || !strings.Contains(sql, "shows.event_date < ?") {
		t.Errorf("day filter lost its sargable bounds, got %q", sql)
	}
	if len(args) != 3 {
		t.Fatalf("day filter must bind 3 arguments, got %v", args)
	}
	if args[2] != "2026-11-14" {
		t.Errorf("day filter must BIND an ISO date, got %v", args)
	}
	if strings.Contains(sql, "2026-11-14") {
		t.Errorf("day filter interpolated its date into %q", sql)
	}

	// 29 February in a leap year is a real date and must filter.
	sql, args = VenueLocalDayCondition(2028, 2, 29)
	if sql == "" || len(args) != 3 || args[2] != "2028-02-29" {
		t.Errorf("a leap day must filter, got %q %v", sql, args)
	}
}

// The multi-day run. Half-open on the same venue-local date expression the
// single-day window uses, and silent for a span of one so the two windows cannot
// both claim the same day.
func TestVenueLocalDayRangeCondition(t *testing.T) {
	for _, tc := range []struct{ year, month, day, days int }{
		{2026, 11, 14, 1}, {2026, 11, 14, 0}, {2026, 11, 14, -3},
		{0, 11, 14, 3}, {2026, 0, 14, 3}, {2026, 11, 0, 3},
		{2027, 2, 29, 3}, {2026, 13, 1, 3}, {2026, 11, 32, 3},
	} {
		got, args := VenueLocalDayRangeCondition(tc.year, tc.month, tc.day, tc.days)
		if got != "" || args != nil {
			t.Errorf("(%d, %d, %d, %d) must not filter, got %q %v",
				tc.year, tc.month, tc.day, tc.days, got, args)
		}
	}

	sql, args := VenueLocalDayRangeCondition(2026, 11, 14, 3)
	if !strings.Contains(sql, VenueLocalDateSQL+" >= ?::date") ||
		!strings.Contains(sql, VenueLocalDateSQL+" < ?::date") {
		t.Errorf("run filter must bound the shared venue-local date expression, got %q", sql)
	}
	if !strings.Contains(sql, "shows.event_date >= ?") || !strings.Contains(sql, "shows.event_date < ?") {
		t.Errorf("run filter lost its sargable bounds, got %q", sql)
	}
	if len(args) != 4 {
		t.Fatalf("run filter must bind 4 arguments, got %v", args)
	}
	// Half-open: a three-day run from the 14th ends BEFORE the 17th, so the 16th
	// is the last date in it.
	if args[2] != "2026-11-14" || args[3] != "2026-11-17" {
		t.Errorf("run filter must bind its half-open ISO edges, got %v", args[2:])
	}
	if strings.Contains(sql, "2026-11-14") || strings.Contains(sql, "2026-11-17") {
		t.Errorf("run filter interpolated its dates into %q", sql)
	}

	// A run crossing a month, a year and a leap day is arithmetic on the anchor
	// rather than on the month's length.
	sql, args = VenueLocalDayRangeCondition(2027, 12, 30, 7)
	if sql == "" || len(args) != 4 || args[2] != "2027-12-30" || args[3] != "2028-01-06" {
		t.Errorf("a run must cross a year boundary, got %q %v", sql, args)
	}
	_, args = VenueLocalDayRangeCondition(2028, 2, 27, 3)
	if len(args) != 4 || args[3] != "2028-03-01" {
		t.Errorf("a run must count the leap day, got %v", args)
	}

	// The same margin the other three period filters share.
	_, dayArgs := VenueLocalDayCondition(2026, 1, 1)
	_, runArgs := VenueLocalDayRangeCondition(2026, 1, 1, 2)
	if !dayArgs[0].(time.Time).Equal(runArgs[0].(time.Time)) {
		t.Errorf("a run must share the day window's lower coarse bound: %v vs %v",
			dayArgs[0], runArgs[0])
	}
}

// The ladder: narrowest resolution wins, and a window that names no period
// narrows nothing. Pinned as an ORDERING, because the alternative is every
// caller re-deriving why a run of one must not take the range builder.
func TestVenueLocalWindowCondition(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		year, month, day, days int
		want                   string
	}{
		{"a run", 2026, 11, 14, 3, "run"},
		{"a run of one is the day", 2026, 11, 14, 1, "day"},
		{"a day", 2026, 11, 14, 0, "day"},
		{"a month", 2026, 11, 0, 0, "month"},
		{"an impossible day falls back to its month", 2027, 2, 31, 0, "month"},
		{"no window", 0, 0, 0, 0, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := VenueLocalWindowCondition(tc.year, tc.month, tc.day, tc.days)

			var want string
			switch tc.want {
			case "run":
				want, _ = VenueLocalDayRangeCondition(tc.year, tc.month, tc.day, tc.days)
			case "day":
				want, _ = VenueLocalDayCondition(tc.year, tc.month, tc.day)
			case "month":
				want, _ = VenueLocalMonthCondition(tc.year, tc.month)
			}
			if got != want {
				t.Errorf("window (%d, %d, %d, %d) took the wrong resolution:\n got %q\nwant %q",
					tc.year, tc.month, tc.day, tc.days, got, want)
			}
		})
	}
}

// The three period filters must share one margin. A margin that reached the year
// window and not the narrower ones would leave a month or day window dropping
// rows the year window keeps, which is the class the shared constant closes.
func TestPeriodConditionsShareOneCoarseMargin(t *testing.T) {
	_, yearArgs := VenueLocalYearCondition(2026)
	_, monthArgs := VenueLocalMonthCondition(2026, 1)
	_, dayArgs := VenueLocalDayCondition(2026, 1, 1)

	yearLower := yearArgs[0].(time.Time)
	monthLower := monthArgs[0].(time.Time)
	dayLower := dayArgs[0].(time.Time)

	if !yearLower.Equal(monthLower) || !yearLower.Equal(dayLower) {
		t.Errorf("January windows must share one lower bound: year %v, month %v, day %v",
			yearLower, monthLower, dayLower)
	}
}
