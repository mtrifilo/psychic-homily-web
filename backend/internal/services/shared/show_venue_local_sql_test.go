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
