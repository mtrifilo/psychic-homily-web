package catalog

// Venue-local partitioning for a show's upcoming/past split.
//
// A show stops being an upcoming listing when its calendar day ends WHERE IT
// HAPPENS, not when the reader's clock rolls over. Partitioning on a
// caller-supplied zone made the same show upcoming for a reader in Hawaii and
// past for one in Berlin at the same instant, and made both disagree with the
// show page's own state (which derives it venue-locally in the frontend's
// lib/utils/showTiming).
//
// The SQL below is the catalog-side twin of savedShowVenueTZJoin in
// internal/services/engagement/saved_show.go, deliberately kept identical in
// shape so a saved show, an artist page row, and a venue page row all answer
// the question the same way. The two live apart because the joins they hang
// off differ (bookmarks vs show_artists/show_venues); if a third caller
// appears, promote one copy rather than adding a fourth.

// showVenueTZJoin resolves each show's venue-local IANA zone for the
// upcoming/past partition. The first venue (lowest venue_id) decides the zone,
// mirroring the "first venue" convention the calendar and reminder renderers
// use, so a show booked across two venues still has exactly ONE venue-local
// date and cannot land in "upcoming" on one surface and "past" on another.
//
// The pg_timezone_names lookup + COALESCE('UTC') follows the radio precedent
// (stationLocalToday): a NULL, blank, or malformed stored zone degrades to UTC
// instead of erroring the whole query. Shows with no venue at all keep the
// LEFT LATERAL row NULL, so the conditions below COALESCE once more.
//
// Requires the query to have already joined `shows` — the lateral correlates
// on shows.id.
const showVenueTZJoin = `LEFT JOIN LATERAL (
	SELECT COALESCE(
		(SELECT name FROM pg_timezone_names
		 WHERE lower(name) = lower(btrim(v.timezone, E' \t\n\r'))),
		'UTC') AS name
	FROM show_venues sv
	JOIN venues v ON v.id = sv.venue_id
	WHERE sv.show_id = shows.id
	ORDER BY sv.venue_id
	LIMIT 1
) venue_tz ON true`

// showVenueLocalDateSQL is the show's calendar date in its venue's local zone.
// event_date is TIMESTAMPTZ (migration 000028), so a single AT TIME ZONE shifts
// the instant onto the venue's wall clock before the ::date cast, matching how
// the calendar and reminder services render it with time.Time.In(venueZone).
const showVenueLocalDateSQL = `(shows.event_date AT TIME ZONE COALESCE(venue_tz.name, 'UTC'))::date`

// showVenueLocalTodaySQL is "today" on the venue's local calendar. A show
// graduates from upcoming to past when this date passes its venue-local event
// date, i.e. at venue-local midnight, not at the event's start instant. A show
// already in progress is therefore still an upcoming listing, which is what the
// listing surfaces want and emphatically NOT what a ticket offer wants.
const showVenueLocalTodaySQL = `(now() AT TIME ZONE COALESCE(venue_tz.name, 'UTC'))::date`

// showVenueLocalDateCondition returns the WHERE fragment selecting timeFilter's
// side of venue-local today, or "" for "all" (no date filter, and therefore no
// need to join showVenueTZJoin at all).
//
// The fragment carries no bind parameters: "now" is evaluated by Postgres per
// row against that row's own venue zone, which is the whole point — a single
// Go-side boundary instant cannot express "midnight, in each show's own zone".
//
// Unknown filters fall through to "upcoming", matching the handlers' own
// default for an omitted time_filter.
func showVenueLocalDateCondition(timeFilter string) string {
	switch timeFilter {
	case "past":
		return showVenueLocalDateSQL + " < " + showVenueLocalTodaySQL
	case "all":
		return ""
	default: // "upcoming"
		return showVenueLocalDateSQL + " >= " + showVenueLocalTodaySQL
	}
}
