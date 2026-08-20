package catalog

import (
	"time"

	"psychic-homily-backend/internal/utils"
)

// Date-only handling for the show create/update API.
//
// THE PROBLEM THIS SOLVES, and why it can only be solved HERE. shows.event_date
// is a TIMESTAMPTZ with no companion precision flag, so a submission carrying
// only a calendar date has to be widened into an instant on the way in. A client
// with no time to send naturally serializes "2026-08-15" as "2026-08-15T00:00:00Z",
// and stored verbatim that is the previous evening in every US zone — the show
// then renders and sorts a day early and drops out of every upcoming bound from
// 17:00 the day before (Phoenix). That is the PSY-1780/PSY-1849/PSY-1861 wound.
//
// It cannot be repaired downstream. Once the row is written, "date-only midnight"
// and "a real 20:00 EDT show" are the SAME instant — 2026-08-15T00:00:00Z is
// both — so no read-side rule can separate them, and a heuristic that assumed
// 00:00:00Z meant date-only would re-date genuine 8pm Eastern and 7pm Central
// shows, the two most common start times in the catalog's densest scenes.
// PSY-1861's survey established that; the read-side repair of already-stored
// rows is deliberately NOT attempted anywhere in this codebase.
//
// The write boundary is the last place the distinction still exists, because the
// decoded time.Time retains the OFFSET the client wrote, which the database
// column does not. "2026-08-15T00:00:00Z" arrives as offset 0 at wall-clock
// 00:00:00; "2026-08-14T20:00:00-04:00" is the same instant but arrives as
// offset -4h at wall-clock 20:00:00. So a bare date is identifiable here and
// nowhere after here, which is why this is a normalization and not a validation:
// rejecting would break the ingest paths that legitimately have no time to send,
// and there is no later checkpoint to defer the decision to.

// isDateOnlySubmission reports whether an event_date arrived as a bare calendar
// date rather than a real instant.
//
// The test is the SUBMITTED representation, never the equivalent UTC instant:
// zero UTC offset AND midnight on the client's own wall clock. Both halves are
// required and each excludes a case the other would catch wrongly.
//
//   - Requiring midnight excludes every ordinary timed submission.
//   - Requiring a ZERO OFFSET is what protects real shows. A client that sends
//     "2026-08-14T20:00:00-04:00" states a genuine 8pm Eastern door time whose
//     instant happens to be UTC midnight; it reads as 20:00 at offset -4h here,
//     fails the midnight test, and is passed through untouched. An instant-based
//     test would have re-anchored it and moved the show a day.
//   - Conversely "2026-08-15T00:00:00-07:00" is an explicit venue-local midnight
//     (a genuine late set). Nonzero offset, so it is left alone. A client that
//     means midnight can always say so unambiguously by naming its offset.
//
// The residual false positive is a client that deliberately means exactly
// 00:00:00 UTC and writes it with a zero offset. For a US-facing show catalog
// that is 17:00-20:00 local depending on zone, i.e. a time the client should be
// expressing in venue-local terms anyway, and the anchored result is still the
// correct calendar EVENING. Wrong by hours in a case that should not occur;
// the alternative was wrong by a day in a case that occurs constantly.
func isDateOnlySubmission(submitted time.Time) bool {
	_, offsetSeconds := submitted.Zone()
	if offsetSeconds != 0 {
		return false
	}
	return submitted.Hour() == 0 &&
		submitted.Minute() == 0 &&
		submitted.Second() == 0 &&
		submitted.Nanosecond() == 0
}

// anchorDateOnlyEventDate returns the instant to store for a submitted
// event_date, plus whether it was re-anchored.
//
// A bare date is anchored at utils.DateOnlyEventHour in the show's assumed zone,
// matching handlers/community.parseShowEventDate and
// services/pipeline.parseEventDate. Anything else is returned unchanged.
//
// The returned bool exists so callers can LOG the rewrite. A silent instant
// rewrite is the kind of thing that reads as data corruption when someone later
// diffs what they sent against what was stored, and the log line is the only
// place that connects the two.
func anchorDateOnlyEventDate(submitted time.Time, state string) (time.Time, bool) {
	if !isDateOnlySubmission(submitted) {
		return submitted, false
	}
	// Year/month/day are read off the submitted value directly. At a zero offset
	// its wall-clock date IS the calendar date the client meant, so there is no
	// conversion to get wrong here.
	loc := utils.EventLocation(nil, state)
	return time.Date(
		submitted.Year(), submitted.Month(), submitted.Day(),
		utils.DateOnlyEventHour, 0, 0, 0, loc,
	), true
}

// showEventDateState picks the state whose zone a date-only event_date is
// anchored in.
//
// The VENUE's state wins over the show body's. utils.EventLocation answers "what
// zone is this room in", and the show-level city/state is a denormalized
// convenience that can be blank or stale while the venue row is the thing that
// actually has a location. This also matches services/pipeline.parseEventDate,
// which anchors on the scraped venue's configured state.
//
// The first venue is used when several are billed. Multi-venue shows are rare
// and their rooms are in one metro in practice, so any of them resolves to the
// same zone; caller order is the only stable pick available at this layer.
//
// An empty result is fine and is NOT an error: utils.EventLocation defaults it
// through the state map to America/Phoenix, the same documented fallback every
// other date-only writer takes.
func showEventDateState(venueStates []string, fallbacks ...string) string {
	for _, s := range venueStates {
		if s != "" {
			return s
		}
	}
	for _, s := range fallbacks {
		if s != "" {
			return s
		}
	}
	return ""
}
