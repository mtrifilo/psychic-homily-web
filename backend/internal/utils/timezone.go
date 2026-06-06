package utils

import (
	"strings"
	"time"
)

// StateTimezones maps US state abbreviations to IANA timezone names.
var StateTimezones = map[string]string{
	"AZ": "America/Phoenix",
	"CA": "America/Los_Angeles",
	"NV": "America/Los_Angeles",
	"CO": "America/Denver",
	"NM": "America/Denver",
	"IL": "America/Chicago",
	"TX": "America/Chicago",
	"NY": "America/New_York",
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
