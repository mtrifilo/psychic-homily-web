package catalog

import (
	"time"

	apperrors "psychic-homily-backend/internal/errors"
)

// ShowEventDateBoundMonths is how far ahead a new show's event_date may be
// without the explicit override. Two other places state the same number and
// are kept equal to this one by TestShowEventDateBound_CopiesAgree: the ph
// CLI's dry run (cli/src/lib/dateBound.ts) and the doc tag on the create
// request's allow_far_future_date field (handlers/catalog/show.go).
const ShowEventDateBoundMonths = 18

// showEventDateBound is the latest event_date accepted at `now` without the
// override: now plus ShowEventDateBoundMonths calendar months, in UTC. At an
// 18 month horizon a venue's zone cannot move the answer by more than a day,
// so the comparison is between instants.
func showEventDateBound(now time.Time) time.Time {
	return monthsAfter(now, ShowEventDateBoundMonths)
}

// monthsAfter is `now` plus `months` calendar months in UTC, with Go's AddDate
// overflow (Aug 31 plus 18 months is Mar 2 or 3). The far-future bound and the
// purge tool's far-future report both measure with it.
func monthsAfter(now time.Time, months int) time.Time {
	return now.UTC().AddDate(0, months, 0)
}

// CheckShowEventDateBound refuses an event_date after showEventDateBound(now)
// unless override is set. The bound itself is accepted. CreateShow and the
// discovery importer both call it; the importer never overrides.
func CheckShowEventDateBound(eventDate, now time.Time, override bool) error {
	if override {
		return nil
	}
	bound := showEventDateBound(now)
	if eventDate.After(bound) {
		return apperrors.ErrShowEventDateBeyondBound(eventDate, bound, ShowEventDateBoundMonths)
	}
	return nil
}
