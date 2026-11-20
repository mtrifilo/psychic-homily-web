package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
)

// ShowEventDateBoundMonths is how far ahead CreateShow accepts an event_date
// without the explicit override. The ph CLI's dry run applies the same number
// (cli/src/lib/dateBound.ts); the two are kept equal by hand.
const ShowEventDateBoundMonths = 18

// ShowEventDateBound is the latest event_date CreateShow accepts at `now`
// without the override: now plus ShowEventDateBoundMonths calendar months, in
// UTC. At an 18 month horizon a venue's zone cannot move the answer by more
// than a day, so the comparison is between instants.
func ShowEventDateBound(now time.Time) time.Time {
	return now.UTC().AddDate(0, ShowEventDateBoundMonths, 0)
}

// checkShowEventDateBound refuses an event_date after ShowEventDateBound(now)
// unless override is set. The bound itself is accepted.
func checkShowEventDateBound(eventDate, now time.Time, override bool) error {
	if override {
		return nil
	}
	bound := ShowEventDateBound(now)
	if eventDate.After(bound) {
		return apperrors.ErrShowEventDateBeyondBound(eventDate, bound, ShowEventDateBoundMonths)
	}
	return nil
}
