package shared

import (
	"regexp"
	"strings"
	"testing"

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
	if !strings.Contains(VenueTZJoin, "lower(NULLIF(btrim(iv.timezone") {
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

// The trim applied to the value being VALIDATED and the trim applied to the
// value being PROJECTED must strip the same characters. Trimming one and not
// the other would validate a string that is not the one AT TIME ZONE receives.
func TestVenueTZJoin_ValidatesAndProjectsTheSameTrimmedValue(t *testing.T) {
	if got := strings.Count(venueTZValidatedZoneSQL, "btrim(iv.timezone, "+venueTZWhitespace+")"); got != 2 {
		t.Errorf("the validated and projected values are not trimmed identically:\n%s", venueTZValidatedZoneSQL)
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
