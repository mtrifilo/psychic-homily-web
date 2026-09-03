package shared

import (
	"time"

	"psychic-homily-backend/internal/utils"
)

// EventLocationResolved resolves the location a show's date and time are read
// on, and reports whether that location is one the site actually KNOWS rather
// than the America/Phoenix default utils.EventLocation surrenders to.
//
// The location it returns is utils.EventLocation's, branch for branch. The
// second value is the Go twin of the frontend's isShowTimezoneResolved
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
// GetTimezoneForState answers America/Phoenix for it, which is the defect this
// exists to catch.
//
// Both halves come from ONE call so a caller cannot read a date on one input
// and decide publishability on another, and so time.LoadLocation runs at most
// once per row: the explicit zone is loaded here rather than being loaded to
// build the location and probed again to answer the question.
func EventLocationResolved(timezone *string, stateFallback string) (*time.Location, bool) {
	if timezone != nil && *timezone != "" {
		if loc, err := time.LoadLocation(*timezone); err == nil {
			return loc, true
		}
		// A malformed IANA string is not an answer; fall through to the state
		// map, which is where utils.EventLocation goes next.
	}
	return utils.EventLocation(nil, stateFallback), utils.HasTimezoneForState(stateFallback)
}

// EventZone is the payload form: the location a date is read on, and the zone
// NAME a payload may publish beside it.
//
// The name is nil exactly when EventLocationResolved says no. nil rather than
// the empty string, because a client's gate is a presence check and "" would
// pass a naive one while a missing key cannot. An absent zone is the signal to
// withhold the clock; the date still renders, on whatever the client's own
// fallback resolves to.
func EventZone(timezone *string, stateFallback string) (*time.Location, *string) {
	loc, resolved := EventLocationResolved(timezone, stateFallback)
	if !resolved {
		return loc, nil
	}
	name := loc.String()
	return loc, &name
}
