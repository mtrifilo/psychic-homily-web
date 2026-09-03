package shared

import (
	"time"

	"psychic-homily-backend/internal/utils"
)

// IsShowTimezoneResolved reports whether a show can be given a zone the site
// actually KNOWS, rather than the America/Phoenix default utils.EventLocation
// surrenders to.
//
// It is the Go twin of the frontend's isShowTimezoneResolved
// (frontend/lib/utils/formatters.ts) and answers the same question on the same
// two inputs: a venue timezone the runtime can load, or a state the US map
// carries. The two implementations are kept in step by hand, exactly as
// utils.StateTimezones and its TypeScript copy are.
//
// ASK IT BEFORE NAMING AN HOUR. A guessed zone is wrong by hours for a room
// outside the US, so a wall clock built on it is a fabricated fact; a calendar
// DATE built on the same guess is still the best answer available and is
// published unmarked by every surface that lists shows.
//
// ANSWERS ABOUT NAMEABILITY, NOT ACCURACY. A state the map carries is resolved
// even where the state spans two zones, so an El Paso room reads on
// America/Chicago and prints an hour that is one out. That approximation is the
// one the writers made when the rows were stored, so it is the one the readers
// have to keep.
//
// A non-US state is never inferred from: "England" is not in the map, and
// GetTimezoneForState answers America/Phoenix for it, which is the whole defect
// this predicate exists to catch.
func IsShowTimezoneResolved(timezone *string, stateFallback string) bool {
	if timezone != nil && *timezone != "" {
		if _, err := time.LoadLocation(*timezone); err == nil {
			return true
		}
		// A malformed IANA string is not an answer; fall through to the state
		// map, which is where utils.EventLocation goes next.
	}
	return utils.HasTimezoneForState(stateFallback)
}

// PublishedZone is the zone name a payload may carry alongside a show's
// instant: the location's own name when the zone is known, nil when the
// resolution surrendered to the fallback.
//
// nil rather than the empty string, because a client's gate is a presence check
// and "" would pass a naive one while a missing key cannot. An absent zone is
// the signal to withhold the clock; the date still renders, on whatever the
// client's own fallback resolves to.
func PublishedZone(loc *time.Location, resolved bool) *string {
	if !resolved || loc == nil {
		return nil
	}
	name := loc.String()
	return &name
}

// EventZone resolves both halves at once for a caller holding a venue's own
// columns: the location its date and time are read on, and the zone name the
// payload may publish.
//
// One call rather than two, so the location and the nullable name cannot be
// derived from different inputs, and so time.LoadLocation runs once per row
// rather than twice.
func EventZone(timezone *string, stateFallback string) (*time.Location, *string) {
	loc := utils.EventLocation(timezone, stateFallback)
	return loc, PublishedZone(loc, IsShowTimezoneResolved(timezone, stateFallback))
}
