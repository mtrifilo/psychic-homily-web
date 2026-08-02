package shared

import (
	"sort"
	"strings"

	"psychic-homily-backend/internal/utils"
)

// Venue-local partitioning for a show's upcoming/past split, in SQL.
//
// A show stops being an upcoming listing when its calendar day ends WHERE IT
// HAPPENS, not when the reader's clock rolls over. Anything that partitions
// shows into upcoming/past must build through this file, because the moment two
// surfaces draw the boundary differently the same show reads "upcoming" on one
// page and "past" on another, which is the bug class PSY-1676/PSY-1695 exist to
// close.
//
// This lives in services/shared rather than in catalog or engagement because it
// already has callers in both (artist and venue show lists; the saved-shows
// list) and had drifted into two byte-identical copies. The Go-side twin of
// these rules is utils.EventLocation, and the frontend's is
// frontend/lib/utils/showTiming.ts — the three must agree on the fallback
// chain, which is why venueLocalZoneSQL below is built from utils.StateTimezones
// rather than restating it.

// PrimaryVenueLateralSQL renders the repo's primary-venue pick as a LATERAL
// subquery: at most one deterministic venue per show, lowest venue_id first.
// `cols` selects from venues aliased iv; `showIDExpr` anchors the pick (s.id,
// shows.id, ub.entity_id, ...). Both must be compile-time literals, NEVER
// runtime input — they are interpolated, not bound.
//
// Every venue-ATTRIBUTING query must build through this so the pick rule cannot
// drift between surfaces.
func PrimaryVenueLateralSQL(cols, showIDExpr string) string {
	return `(
			SELECT ` + cols + `
			FROM show_venues sv
			JOIN venues iv ON iv.id = sv.venue_id
			WHERE sv.show_id = ` + showIDExpr + `
			ORDER BY sv.venue_id ASC
			LIMIT 1
		)`
}

// venueLocalStateCaseSQL is utils.StateTimezones rendered as a SQL CASE over the
// primary venue's state, so a venue whose `timezone` was never geocoded is
// judged on the same zone every OTHER surface already renders it in
// (utils.EventLocation's second arm). Without it a Phoenix venue with a NULL
// timezone would be partitioned in UTC and sit 7 hours away from how its own
// show page, ICS feed, and reminder email date the very same show.
//
// Built from the Go map rather than restating it: a hand-copied CASE is exactly
// the drift utils.StateTimezones' own doc comment warns about. Keys are sorted
// so the rendered string is stable across process starts (Go map iteration is
// randomized), which keeps query plans and test output deterministic.
//
// Interpolation is safe by construction: every fragment comes from a hardcoded
// Go map, never from a request. The ELSE mirrors GetTimezoneForState's
// America/Phoenix default, which also covers a show with no venue row at all
// (state reads NULL, so no WHEN matches). That US-centric default is a KNOWN
// LIMIT shared with the frontend, not an endorsement: a non-US venue needs its
// `timezone` column populated to be judged on its own calendar.
var venueLocalStateCaseSQL = buildVenueLocalStateCaseSQL()

func buildVenueLocalStateCaseSQL() string {
	states := make([]string, 0, len(utils.StateTimezones))
	for state := range utils.StateTimezones {
		states = append(states, state)
	}
	sort.Strings(states)

	var b strings.Builder
	b.WriteString("CASE upper(btrim(venue_tz.state))")
	for _, state := range states {
		b.WriteString(" WHEN '")
		b.WriteString(state)
		b.WriteString("' THEN '")
		b.WriteString(utils.StateTimezones[state])
		b.WriteString("'")
	}
	b.WriteString(" ELSE '")
	b.WriteString(utils.GetTimezoneForState(""))
	b.WriteString("' END")
	return b.String()
}

// VenueTZJoin resolves each show's venue-local IANA zone. Requires the query to
// have already joined `shows` — the lateral correlates on shows.id.
//
// TWO joins, deliberately, and the split is load-bearing for performance. The
// lateral picks the primary venue's RAW stored zone and state (a cheap index hop
// on show_venues_pkey); VALIDATING that zone against pg_timezone_names is a
// separate plain LEFT JOIN so Postgres materializes the view ONCE per query and
// hash-joins it. Resolving the name inside the lateral instead makes it a
// correlated subquery that re-scans pg_timezone_names' ~490 rows PER SHOW:
// measured at 136ms for an 800-show artist and 3.4s for a 20k-show venue,
// against ~10ms and ~500ms for the form below. Do not fold them back together.
//
// The DISTINCT ON makes fan-out impossible by construction rather than by
// assertion. pg_timezone_names is populated from the host OS tzdata, so
// "lower(name) is unique" is an environment-dependent claim (it holds on the
// Postgres 18 test image); a duplicate would multiply rows and silently corrupt
// every total built on this join.
var VenueTZJoin = `LEFT JOIN LATERAL ` +
	PrimaryVenueLateralSQL("iv.timezone, iv.state", "shows.id") + ` venue_tz ON true
	LEFT JOIN (SELECT DISTINCT ON (lower(name)) name FROM pg_timezone_names) venue_tzn
		ON lower(venue_tzn.name) = lower(btrim(venue_tz.timezone, E' \t\n\r'))`

// venueLocalZoneSQL is the resolved zone for the primary venue, mirroring
// utils.EventLocation's precedence: a valid explicit venue timezone, then the
// US state map, which itself defaults to America/Phoenix.
var venueLocalZoneSQL = `COALESCE(venue_tzn.name, ` + venueLocalStateCaseSQL + `)`

// VenueLocalDateSQL is the show's calendar date in its venue's local zone.
// event_date is TIMESTAMPTZ (migration 000028), so a single AT TIME ZONE shifts
// the instant onto the venue's wall clock before the ::date cast, matching how
// the calendar and reminder services render it with time.Time.In(venueZone).
var VenueLocalDateSQL = `(shows.event_date AT TIME ZONE ` + venueLocalZoneSQL + `)::date`

// VenueLocalTodaySQL is "today" on the venue's local calendar. A show graduates
// from upcoming to past when this date passes its venue-local event date, i.e.
// at venue-local midnight, not at the event's start instant. A show already in
// progress is therefore still an upcoming listing, which is what listing
// surfaces want and emphatically NOT what a ticket offer wants — offers gate on
// the start instant (frontend showTiming.hasShowStarted).
var VenueLocalTodaySQL = `(now() AT TIME ZONE ` + venueLocalZoneSQL + `)::date`

// Coarse bounds on shows.event_date that are LOSSLESS with respect to the exact
// venue-local conditions below, and unlike them are sargable against
// idx_shows_event_date (migration 000001). They exist purely so the planner can
// start an index scan at the boundary instead of walking the venue's whole
// history; the exact condition still decides membership.
//
// Why they cannot drop a row that the exact condition would keep:
//
//   - upcoming is "venue-local date >= venue-local today", and the earliest
//     instant satisfying that is venue-local midnight today. Midnight today is
//     never more than one local day behind now, whatever the zone's offset — so
//     no upcoming show can sit earlier than now() minus a day. The margin is
//     TWO days, not one, because a local day is not always 24 hours: a
//     DST fall-back stretches it to 25, and zones have historically shifted by
//     more. The extra day costs nothing in selectivity and removes the class of
//     bug entirely.
//   - past is "venue-local date < venue-local today", whose latest instant is
//     the moment before venue-local midnight today, and midnight today is always
//     at or before now. So every past show is strictly before now().
//
// Verified against the exact conditions across the full inhabited offset range
// plus NULL, blank and malformed venue zones — see
// TestGetShowsForArtist_ExtremeVenueOffsetSurvivesCoarsePrefilter.
const (
	upcomingCoarseBound = `shows.event_date >= now() - interval '2 days'`
	pastCoarseBound     = `shows.event_date < now()`
)

// VenueLocalDateCondition returns the WHERE fragment selecting timeFilter's side
// of venue-local today, or "" for "all" (no date filter, and therefore no need
// to join VenueTZJoin at all).
//
// The exact half carries no bind parameters: "now" is evaluated by Postgres per
// row against that row's own venue zone, which is the whole point — a single
// Go-side boundary instant cannot express "midnight, in each show's own zone".
//
// Unknown filters fall through to "upcoming", matching the handlers' own default
// for an omitted time_filter.
func VenueLocalDateCondition(timeFilter string) string {
	switch timeFilter {
	case "past":
		return pastCoarseBound + " AND " + VenueLocalDateSQL + " < " + VenueLocalTodaySQL
	case "all":
		return ""
	default: // "upcoming"
		return upcomingCoarseBound + " AND " + VenueLocalDateSQL + " >= " + VenueLocalTodaySQL
	}
}
