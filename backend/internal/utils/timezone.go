package utils

import (
	"strings"
	"time"
)

// StateTimezones maps US state (and DC) abbreviations to IANA timezone names.
//
// This is the full 50-state + DC map. It MUST stay in sync with the writer-side
// maps the show data was actually stored under — cli/src/lib/timezone.ts and
// frontend/lib/utils/timeUtils.ts (PSY-1009) — because the venue-timezone
// backfill (PSY-987) derives the zone a date-only show was *written* under from
// this map: a short map (defaulting most states to Phoenix) would make a
// correctly-stored explicit-time show in an unmapped state read as a false 20:00
// Phoenix default and be wrongly re-anchored. States that span two zones use
// their predominant zone (the same approximation the writers made).
//
// The three maps are identical as of PSY-987, but the sync is NOT enforced by
// CI (they're Go vs TypeScript): TestGetTimezoneForState_FullMapCoverage only
// guards this Go map's own coverage. If you edit any of the three, edit all
// three — drift here silently re-opens the re-anchor corruption class.
var StateTimezones = map[string]string{
	// Pacific
	"CA": "America/Los_Angeles",
	"NV": "America/Los_Angeles",
	"OR": "America/Los_Angeles",
	"WA": "America/Los_Angeles",
	// Mountain
	"AZ": "America/Phoenix", // no DST
	"CO": "America/Denver",
	"NM": "America/Denver",
	"MT": "America/Denver",
	"UT": "America/Denver",
	"WY": "America/Denver",
	"ID": "America/Boise",
	// Central
	"IL": "America/Chicago",
	"TX": "America/Chicago",
	"AL": "America/Chicago",
	"AR": "America/Chicago",
	"IA": "America/Chicago",
	"KS": "America/Chicago",
	"LA": "America/Chicago",
	"MN": "America/Chicago",
	"MO": "America/Chicago",
	"MS": "America/Chicago",
	"ND": "America/Chicago",
	"NE": "America/Chicago",
	"OK": "America/Chicago",
	"SD": "America/Chicago",
	"TN": "America/Chicago",
	"WI": "America/Chicago",
	"IN": "America/Indiana/Indianapolis",
	// Eastern
	"NY": "America/New_York",
	"CT": "America/New_York",
	"DC": "America/New_York",
	"DE": "America/New_York",
	"FL": "America/New_York",
	"GA": "America/New_York",
	"KY": "America/New_York",
	"MA": "America/New_York",
	"MD": "America/New_York",
	"ME": "America/New_York",
	"MI": "America/New_York",
	"NC": "America/New_York",
	"NH": "America/New_York",
	"NJ": "America/New_York",
	"OH": "America/New_York",
	"PA": "America/New_York",
	"RI": "America/New_York",
	"SC": "America/New_York",
	"VA": "America/New_York",
	"VT": "America/New_York",
	"WV": "America/New_York",
	// Non-contiguous
	"AK": "America/Anchorage",
	"HI": "Pacific/Honolulu",
}

// GetTimezoneForState returns the IANA timezone for a US state abbreviation.
// Defaults to "America/Phoenix" if the state is not found.
func GetTimezoneForState(state string) string {
	if tz, ok := StateTimezones[strings.ToUpper(state)]; ok {
		return tz
	}
	return "America/Phoenix"
}

// EventLocation resolves the IANA location for rendering an event time in a
// venue's local zone. Precedence: a valid explicit venue timezone, then the US
// state->tz map (GetTimezoneForState, which itself defaults unknown/empty input
// to America/Phoenix), and finally UTC only if a non-empty timezone string
// fails to load. A malformed venue timezone falls through to the state map
// rather than jumping straight to UTC. (PSY-996)
func EventLocation(timezone *string, stateFallback string) *time.Location {
	if timezone != nil && *timezone != "" {
		if loc, err := time.LoadLocation(*timezone); err == nil {
			return loc
		}
		// Malformed/unknown IANA string — fall through to the state map below.
	}
	if loc, err := time.LoadLocation(GetTimezoneForState(stateFallback)); err == nil {
		return loc
	}
	return time.UTC
}
