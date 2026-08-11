package shared

import (
	"sort"
	"strings"
	"time"

	"psychic-homily-backend/internal/utils"
)

// Venue-local partitioning for a show's upcoming/past split, in SQL.
//
// A show stops being an upcoming listing when its calendar day ends WHERE IT
// HAPPENS, not when the reader's clock rolls over. The moment two surfaces draw
// that boundary differently, the same show reads "upcoming" on one page and
// "past" on another — the bug class PSY-1676/PSY-1695 exist to close. New
// partitioning surfaces should build through this file.
//
// It lives in services/shared rather than catalog or engagement because it
// already has callers in both. The Go-side twin of these rules is
// utils.EventLocation and the frontend's is frontend/lib/utils/showTiming.ts;
// the three must agree on the fallback chain, which is why venueLocalZoneSQL is
// built from utils.StateTimezones rather than restating it.
//
// MIGRATED so far: artist show lists, venue show lists, the artist graph card's
// next-show, the saved-shows list, and the main /shows feed
// (catalog.ShowService.GetUpcomingShows and its GetShowCities picker counts).
//
// SCOPE OF THE TWO LISTS BELOW: show LIST surfaces — the ones that decide which
// rows a reader is shown. Aggregate COUNT surfaces are NOT enumerated, and
// several of them draw their own boundary: tag-page enrichment counts
// (catalog/tag_service.go), the /venues list's upcoming_show_count
// (catalog/venue.go), /scenes' upcoming_count and this_week_count
// (catalog/scene.go), catalog/venue_rail.go, graph_overview.go, charts_rank.go
// and sitemap.go. Do not read a surface's absence here as a claim that it
// already agrees with this file.
//
// NOT migrated — do not read the paragraph above as a description of the whole
// repo, because these still draw their own boundary:
//   - catalog/tag_intersection.go — the tag-page entity counts, start-of-today
//     in UTC. The reason it gave (no request timezone to work from) no longer
//     applies now that the boundary needs no timezone at all. PSY-1760 owns it,
//     deferred for SCOPE rather than risk (that ticket carries the measured
//     cost of the same shape under GetShowCities).
//   - engagement/venue_calendar.go upcomingShowsForVenue — the venue ICS feed,
//     start-of-today in the QUERIED venue's zone, so it can disagree with the
//     venue page for a show booked at two venues.
//   - catalog/artist_graph_helpers.go batchArtistUpcomingShowCounts and
//     batchArtistNextShows — instant-based (event_date > NOW()), which is why a
//     graph node can show no upcoming dot for a show that started earlier today
//     while the card still calls it next.
//
// DELIBERATELY instant-bounded, and NOT candidates for this file — listed so an
// audit does not mistake them for oversights:
//   - services/explore/explore.go — `event_date >= now()` UTC. Already
//     viewer-independent, and /explore is SSR-prefetched with no viewer
//     context, so nothing forces the change. It does mean /explore's list can
//     drop a show that started earlier today while the /shows/cities picker
//     above it still counts that show; documented at its own call site.
//   - catalog/charts_service.go mostAnticipatedHorizon and the scene-week
//     counts — `time.Now().UTC().Truncate(24h)`. A public chart has no
//     requester and ranks over a multi-week horizon, so a one-day boundary
//     nudge does not change what it is measuring.
//
// UNKNOWN ZONES FAIL SOFT (PSY-1761). `AT TIME ZONE` RAISES on a name Postgres
// does not carry rather than degrading, and PSY-1678 made that a whole-site
// outage rather than a one-page one: the main /shows feed and its city picker
// build through here, so ONE poisoned venue row would have taken down /shows,
// the homepage list, /explore's picker and the account-settings city picker at
// once. VenueTZJoin now projects NULL for a zone this server's catalog does not
// carry, which routes that row into the same state-map arm a NULL already
// takes. A bad row costs one venue's shows a possibly-wrong date; it no longer
// costs everybody the page.
//
// Four layers now stand between a bad value and a reader, and the first three
// are what keep the fourth from having to fire:
//  1. WRITE BOUNDARY (PSY-1707) — every path that writes venues.timezone
//     validates against pg_timezone_names and stores NULL rather than a name
//     this server does not carry.
//  2. BACKFILL (migration 20260802043206) — normalized every pre-existing row,
//     applied on stage and production.
//  3. SWEEP (PSY-1695, VenueTimezoneSweep) — re-validates stored zones against
//     the live catalog and NULLs the casualties, so an operator sees a named
//     venue rather than a silently mis-dated one.
//     ENABLE_VENUE_TIMEZONE_SWEEP was confirmed set on both stage and
//     production on 2026-08-08. It is no longer what stands between a drifted
//     zone and an outage — layer 4 is — but it is still what turns a silently
//     wrong date into a logged one, so set it when standing up a new
//     environment rather than treating it as optional.
//  4. READ GUARD (this file, PSY-1761) — the membership test in VenueTZJoin.
//     Unconditional, with no flag and no worker to remember: the table it reads
//     is created and seeded by the migration itself.
//
// WHAT THE GUARD DOES AND DOES NOT CLOSE, stated precisely because the two
// halves have different residual risks:
//
//   - A name that was NEVER in the catalog — the out-of-band SQL write, the
//     write gate skipped by a transient database error — is closed completely
//     and immediately. It cannot be in the snapshot, so it can never be
//     trusted. This is the path with no other defence: the write gate cannot
//     see it and the sweep only reaches it a cycle later.
//   - A name that HAS LEFT the catalog since it was written — the Postgres
//     upgrade, the tzdata refresh, the restore onto a differently-packaged
//     image — is closed only once the snapshot catches up, because until then
//     the snapshot still carries the name and the guard still trusts it. That
//     window is bounded by RefreshTimezoneNamesSnapshot's cadence (boot, plus
//     each sweep cycle), and boot is the one that matters: the events that
//     change the catalog are the events that restart this process.
//
// So a stale snapshot degrades toward TODAY'S behaviour rather than past it,
// and in the other direction it fails safe — a snapshot that has not yet
// GAINED a genuinely new zone merely sends that venue to the state map for one
// refresh interval.

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
// Go map, never from a request. Pinned by TestStateTimezones_AreSafeToInterpolate.
//
// GATED ON COUNTRY, which the Go twin does not do and should: the state map is
// US-only, and its abbreviations collide badly with the rest of the world.
// Western Australia is "WA", Indonesia is "ID", Israel is "IL", India is "IN" --
// an ungated CASE would put a Fremantle venue on America/Los_Angeles and be off
// by 16 hours. Venues outside the US resolve to UTC instead, which is wrong by
// at most half a day rather than confidently wrong by most of one. The real fix
// is populating venues.timezone for them; this is the floor, not the goal.
//
// A NULL country is treated as US because that is what the existing data means:
// the column was added late and the domestic backfill left it unset.
var venueLocalStateCaseSQL = buildVenueLocalStateCaseSQL()

func buildVenueLocalStateCaseSQL() string {
	states := make([]string, 0, len(utils.StateTimezones))
	for state := range utils.StateTimezones {
		states = append(states, state)
	}
	sort.Strings(states)

	var b strings.Builder
	b.WriteString("CASE WHEN venue_tz.venue_tz_country IS NULL OR btrim(venue_tz.venue_tz_country) = '' OR ")
	b.WriteString("upper(btrim(venue_tz.venue_tz_country)) IN ('US', 'USA', 'UNITED STATES') THEN (")
	b.WriteString("CASE upper(btrim(venue_tz.venue_tz_state))")
	for _, state := range states {
		b.WriteString(" WHEN '")
		b.WriteString(state)
		b.WriteString("' THEN '")
		b.WriteString(utils.StateTimezones[state])
		b.WriteString("'")
	}
	// Mirrors GetTimezoneForState's default, and also covers a show with no
	// venue row at all (state reads NULL, so no WHEN matches).
	b.WriteString(" ELSE '")
	b.WriteString(utils.GetTimezoneForState(""))
	b.WriteString("' END) ELSE 'UTC' END")
	return b.String()
}

// VenueTimezoneWhitespaceSQL is the character set every reader of a stored
// venues.timezone strips before doing anything with it.
//
// Exported because the READ GUARD in this file and the DRIFT DETECTOR in
// catalog.SweepVenueTimezones have to agree on it exactly. A detector that
// trimmed less than the guard would call a venue healthy while the guard was
// quietly sending its shows to the state map — a wrong date with nothing logged
// anywhere, which is the one failure this whole design is trying not to create.
// One definition means they cannot drift.
const VenueTimezoneWhitespaceSQL = `E' \t\n\r\f\v'`

// venueTZStoredZone is the stored zone as every expression below sees it:
// trimmed, with blank read as NULL. Named once so the value that gets VALIDATED
// and the value that gets PROJECTED are the same string by construction rather
// than by two edits staying in sync.
const venueTZStoredZone = `NULLIF(btrim(iv.timezone, ` + VenueTimezoneWhitespaceSQL + `), '')`

// venueTZValidatedZoneSQL projects the primary venue's stored zone ONLY when
// this server's catalog still carries it, and NULL otherwise. That NULL is the
// whole fail-soft mechanism: venueLocalZoneSQL's COALESCE already routes a NULL
// into the US state map, so an unknown name lands on the same arm an ungeocoded
// venue does instead of raising (PSY-1761).
//
// WHY THE MEMBERSHIP TEST LIVES HERE, in the lateral's projection, rather than
// beside the COALESCE it feeds. venueLocalZoneSQL is dereferenced two to three
// times per query (the date condition names it twice, a year filter adds a
// third), and Postgres does NOT deduplicate identical uncorrelated subqueries —
// each occurrence plans its own SubPlan with its own hash build. Hoisting the
// test into the projection makes it ONE SubPlan per query however many times the
// zone is read downstream. Measured across all sixteen statements the migrated
// surfaces emit: one `SubPlan 1`, never a second.
//
// WHY A SNAPSHOT TABLE AND NOT pg_timezone_names. The catalog view is a
// set-returning function that reopens the tzdata files per scan. Measured on
// postgres:18 against this repo's own queries: one scan is 3.5 ms on an idle
// box and 43-63 ms under load, and the uncorrelated-subquery form of this guard
// cost 8-11x on the two hottest statements (GetUpcomingShows' count 4.6 -> 52.1 ms,
// GetShowCities 1.2 -> 9.1 ms) even though every scan correctly planned at
// loops=1. The snapshot answers the same question from 487 ordinary rows.
//
// THE loops=1 CLAIM IS THE ONE TO RE-VERIFY if this is ever touched, because
// this file has been wrong about it before. An earlier revision validated the
// zone with a JOIN inside this same lateral; as a LATERAL-CORRELATED RELATION
// the planner pushed it into the inner side of the nested loop over shows and
// re-scanned the catalog PER SHOW — `loops=17228`, 8.1s for one COUNT. A
// SubPlan is a different animal (it is an expression, and an uncorrelated one
// keeps its hash table across rescans of the node holding it), which is why
// this shape survives where that one did not. Do NOT reintroduce a relation
// here, and do not trust a single good plan: verify loops=1 on every consumer.
//
// The comparison is case-INSENSITIVE, matching how `AT TIME ZONE` resolves a
// name and matching VenueTimezoneSweep's drift predicate. A guard stricter on
// CASE than the sweep would silently send rows the sweep calls healthy to the
// state map.
//
// It is NARROWER than `AT TIME ZONE` in one deliberate respect. `AT TIME ZONE`
// resolves more than pg_timezone_names: measured on postgres:18, it also
// accepts pg_timezone_abbrevs entries ('EST') and bare POSIX TZ specs
// ('EST5EDT', 'FOO+3'), none of which appear in the catalog this guard tests
// against. All of them are rejected here and land on the state map.
//
// That is policy, not oversight, and it matches the WRITE side rather than
// diverging from it: shared.NormalizeIANATimezone already refuses to store any
// of them, for the reason spelled out at its own definition — a fixed-offset
// spec carries no DST rule, so it would freeze a venue on standard time and
// mis-date half its shows. A read guard laxer than the write gate would accept
// values the gate exists to keep out, and VenueTimezoneSweep would NULL them
// on its next pass anyway. The state map is the same answer, sooner.
var venueTZValidatedZoneSQL = `CASE WHEN lower(` + venueTZStoredZone + `) ` +
	`IN (SELECT name_lower FROM timezone_names_snapshot) ` +
	`THEN ` + venueTZStoredZone + ` END AS venue_tz_timezone`

// VenueTZJoin resolves each show's venue-local zone inputs. Requires the query
// to have already joined `shows` — the lateral correlates on shows.id.
//
// ONE lateral, and nothing else. Everything downstream of it is a pure scalar
// expression, so the whole zone resolution costs a single index hop on
// show_venues_pkey per candidate row plus one hashed membership probe.
//
// venue_tz_timezone is the VALIDATED zone, not the raw column — see
// venueTZValidatedZoneSQL. A caller that wants the stored value as written must
// read venues.timezone itself; this projection deliberately cannot hand a
// reader a name that would raise.
//
// Nothing here covers the GO-side readers (utils.EventLocation, the ICS feeds,
// the Discord notifier), which resolve through Go's catalog rather than
// Postgres'. The two disagree in both directions: "localtime" and "Factory" pass
// the write gate and fail time.LoadLocation. Those readers have their own
// fallback and are not made safe by anything here.
//
// The projected columns are ALIASED rather than carried under their source
// names, and that prefix is load-bearing rather than decorative. `venues` and
// `shows` both have a `state`, so a bare `venue_tz.state` beside `shows.state`
// left two traps for every caller that queries `shows` directly: an unqualified
// `state` in a WHERE/GROUP BY is ambiguous and Postgres raises, and a bare
// `SELECT *` widens to include the lateral's `state`, which GORM then scans
// over the show's own. The earlier migrated surfaces all happened to select an
// id column and never hit either. Aliasing removes both structurally: no alias
// here collides with a `shows` column, and none matches a field name on any
// model, so a caller cannot reintroduce the collision by forgetting to qualify.
var VenueTZJoin = `LEFT JOIN LATERAL ` +
	PrimaryVenueLateralSQL(
		venueTZValidatedZoneSQL+", iv.state AS venue_tz_state, iv.country AS venue_tz_country",
		"shows.id",
	) + ` venue_tz ON true`

// venueLocalZoneSQL is the resolved zone for the primary venue, mirroring
// utils.EventLocation's precedence: the stored venue timezone, then the US state
// map, which itself defaults to America/Phoenix.
//
// It reads venue_tz_timezone raw because the lateral already did the work:
// venueTZValidatedZoneSQL trims it, maps blank to NULL, and maps a name this
// server's catalog does not carry to NULL as well. So there are exactly two
// cases here, and both are the same case — a usable zone, or the fallback.
//
// The fallback arm is the US state map rather than a bare 'UTC', deliberately.
// Eight of production's 237 venues have no geocoded zone, and every OTHER
// surface dates their shows through utils.EventLocation's state arm — sending
// only this path to UTC would put a Phoenix venue's listing 7 hours away from
// its own show page, which is the class of disagreement PSY-1676 exists to
// remove. It costs nothing in the hot path: a CASE over two already-fetched
// columns is a scalar expression, not a relation scan.
var venueLocalZoneSQL = `COALESCE(venue_tz.venue_tz_timezone, ` + venueLocalStateCaseSQL + `)`

// VenueLocalDateSQL is the show's calendar date in its venue's local zone.
// event_date is TIMESTAMPTZ (migration 000028), so a single AT TIME ZONE shifts
// the instant onto the venue's wall clock before the ::date cast, matching how
// the calendar and reminder services render it with time.Time.In(venueZone).
var VenueLocalDateSQL = `(shows.event_date AT TIME ZONE ` + venueLocalZoneSQL + `)::date`

// VenueLocalYearSQL is the calendar YEAR of the show's venue-local date, as an
// int. It is the only year convention listing surfaces may use: the bill-network
// window resolver (catalog/venue_bill_network.go) buckets by UTC year instead,
// and those two already disagree for a late-night show near a New Year boundary.
// Do not introduce a third.
//
// Like everything else derived from venueLocalZoneSQL it is a pure scalar
// expression over VenueTZJoin's already-fetched columns, so it adds no relation
// to the lateral path. See VenueTZJoin's note on the 8.1s regression that
// joining one cost.
var VenueLocalYearSQL = `EXTRACT(YEAR FROM ` + VenueLocalDateSQL + `)::int`

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

// yearCoarseMargin widens the sargable UTC bounds below far enough that no
// venue-local instant of the requested year can fall outside them. The inhabited
// UTC offset range is -12:00 to +14:00, so one day would already cover it; two
// matches the margin the upcoming/past coarse bounds use, for the same reason
// (historical offsets have been stranger than the present ones, and the extra
// day costs nothing in selectivity).
const yearCoarseMargin = 48 * time.Hour

// maxCoarseBoundedYear is the largest year whose coarse bounds are worth
// building. Above it the Go time.Time bounds stop round-tripping cleanly through
// the driver, and since the bounds are a planner hint rather than a correctness
// input, the honest move is to go without them. See VenueLocalYearCondition.
const maxCoarseBoundedYear = 9999

// VenueLocalYearCondition returns the WHERE fragment and bind arguments
// narrowing a show list to a single VENUE-LOCAL calendar year, or ("", nil) when
// year is zero or negative, which every caller reads as "all years".
//
// Requires the query to have joined VenueTZJoin: the exact half dereferences
// venue_tz. Callers that would otherwise skip the join for timeFilter "all" must
// add it back when a year is requested.
//
// The year is BOUND, not interpolated. The coarse UTC bounds carry no
// correctness weight (the exact venue-local equality decides membership); they
// exist only so the planner can start an index scan on idx_shows_event_date at
// the boundary instead of walking the venue's whole history, exactly like
// upcomingCoarseBound and pastCoarseBound above. An absurd year therefore
// returns an empty page rather than a wrong one: it drops the unrepresentable
// bounds and lets the exact equality match nothing, which is the answer the
// caller asked for. Widening it to "" instead would silently return EVERY year.
func VenueLocalYearCondition(year int) (string, []any) {
	if year <= 0 {
		return "", nil
	}

	exact := VenueLocalYearSQL + " = ?"
	if year > maxCoarseBoundedYear {
		return exact, []any{year}
	}

	lower := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Add(-yearCoarseMargin)
	upper := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC).Add(yearCoarseMargin)

	return "shows.event_date >= ? AND shows.event_date < ? AND " + exact,
		[]any{lower, upper, year}
}
