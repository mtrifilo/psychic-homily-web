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

// DateOnlyEventHour is the venue-local hour a show carrying a DATE but no time
// of day is anchored at.
//
// The alternative is storing bare UTC midnight, and that is the bug this
// constant exists to keep closed. shows.event_date is a TIMESTAMPTZ with no
// companion precision flag, so once a date-only value is written as 00:00:00Z
// nothing downstream can recover the fact that its time was never known. In
// every US zone that instant is the PREVIOUS evening local, so the show sorts
// and renders under the wrong calendar day, and any "still upcoming" bound drops
// it from the moment UTC midnight passes — 17:00 the day before in Phoenix.
// PSY-1780, PSY-1849 and PSY-1861 are three separate bites of that one wound.
//
// 20:00 is not a guess at any particular door time; it is a convention chosen so
// that the anchored instant lands unambiguously on the intended calendar EVENING
// in every US zone, which is the only property readers depend on.
//
// Exported and referenced by every writer rather than restated as a literal:
// discovery imports (services/pipeline.parseEventDate) and community request
// fulfillment (handlers/community.parseShowEventDate) must agree, or a reader
// inspecting a row cannot tell which writer produced it. The frontend show form
// (frontend/features/shows/components/show-form-utils.DEFAULT_EVENT_TIME, which
// a blank time field falls back to) and the ph CLI are writers too and are NOT
// compile-checked against this — keep them in sync by hand, exactly as
// StateTimezones warns above.
//
// NOT enforced on the show create/update API, and deliberately so. Those
// endpoints take event_date as an already-parsed instant, and an instant cannot
// carry the fact that its time was never known: the frontend form serializes
// 20:00 venue-local to a plain UTC string, so a genuine 8pm show at an Eastern
// venue in summer arrives as exactly 00:00:00Z and is indistinguishable from a
// bare date. Closing that path needs an explicit precision signal in the
// contract (a YYYY-MM-DD variant, the way parseShowEventDate already accepts
// one), not a rule inferred from the instant. See PSY-1861.
const DateOnlyEventHour = 20

// GetTimezoneForState returns the IANA timezone for a US state abbreviation.
// Defaults to "America/Phoenix" if the state is not found.
func GetTimezoneForState(state string) string {
	if tz, ok := StateTimezones[strings.ToUpper(state)]; ok {
		return tz
	}
	return "America/Phoenix"
}

// HasTimezoneForState reports whether StateTimezones carries this state, i.e.
// whether GetTimezoneForState answers from the map rather than from its
// America/Phoenix default.
//
// The distinction the default erases: "AZ" and "England" both come back as
// America/Phoenix, and only the first of them is an answer. A caller about to
// print a WALL CLOCK needs to tell them apart, because the second is wrong by
// hours; a caller printing a calendar DATE does not, because the fallback day is
// still the best available one.
//
// Case folding matches GetTimezoneForState exactly, and neither trims, so the
// two always agree on which inputs the map holds.
func HasTimezoneForState(state string) bool {
	_, ok := StateTimezones[strings.ToUpper(state)]
	return ok
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
