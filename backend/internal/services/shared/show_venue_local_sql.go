package shared

import (
	"fmt"
	"sort"
	"strconv"
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
// next-show, the saved-shows list, the main /shows feed
// (catalog.ShowService.GetUpcomingShows and its GetShowCities picker counts),
// and the whole tag page — its entity counts (catalog/tag_intersection.go) and
// the per-entity upcoming_show_count its cards print (PSY-1760).
//
// COUNT surfaces also build through here, and are named separately so the scope
// paragraph below stays true of the list surfaces it is about:
//   - the scene page's headline upcoming_show_count (catalog/scene.go
//     GetSceneDetail), its rooms leaderboard (catalog/scene_venues.go) and the
//     roster's per-band upcoming count and next-show pick
//     (catalog/scene_roster_upcoming.go), on the NIGHT bound
//     (VenueLocalNightDateCondition), which is the rule the tonight bucket those
//     three sit beside is drawn on.
//   - the scenes DIRECTORY's upcoming_count (catalog/scene.go ListScenes), on
//     that same NIGHT bound, because a card links to the page printing the
//     headline above. The two draw one boundary over different room sets: the
//     directory counts verified rooms only. Its this_week_count slices the same
//     night set through VenueLocalNightWindowCondition, so the pair is nested
//     at every hour.
//   - the /venues directory's per-room upcoming_show_count and its next_show
//     pick (catalog/venue.go GetVenuesWithShowCounts), on the NIGHT bound, and
//     its last_show pick on that bound's exact complement
//     (VenueLocalNightPastDateCondition). It draws the SAME boundary as the
//     scene page's rooms leaderboard over a narrower set of shows: that
//     directory also excludes cancelled nights (UncancelledShowPredicateSQL)
//     and the leaderboard does not, so one room's two counts differ by the
//     cancelled shows in the window.
//
// SCOPE OF THE TWO LISTS BELOW: show LIST surfaces — the ones that decide which
// rows a reader is shown. Aggregate COUNT surfaces are NOT enumerated, and
// several of them draw their own boundary: catalog/venue_rail.go,
// graph_overview.go, charts_rank.go and sitemap.go. Do not read a surface's
// absence here as a claim that it already agrees with this file.
//
// NOT migrated — do not read the paragraph above as a description of the whole
// repo, because these still draw their own boundary:
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

// VenueLocalMonthSQL is the calendar MONTH (1-12) of the show's venue-local
// date, as an int.
//
// Never a bucket key on its own: "March" is not a period, and grouping by this
// alone would fold every March in a venue's history into one row. Every consumer
// groups by VenueLocalYearSQL alongside it, which is also why the two are
// defined next to each other rather than at their call sites.
//
// Same shape and the same cost as VenueLocalYearSQL — a scalar expression over
// VenueTZJoin's already-fetched columns, adding no relation to the lateral path.
var VenueLocalMonthSQL = `EXTRACT(MONTH FROM ` + VenueLocalDateSQL + `)::int`

// VenueLocalTodaySQL is "today" on the venue's local calendar. A show graduates
// from upcoming to past when this date passes its venue-local event date, i.e.
// at venue-local midnight, not at the event's start instant. A show already in
// progress is therefore still an upcoming listing, which is what listing
// surfaces want and emphatically NOT what a ticket offer wants — offers gate on
// the start instant (frontend showTiming.hasShowStarted).
var VenueLocalTodaySQL = `(` + venueLocalNowSQL + `)::date`

// venueLocalNowSQL is this instant on the primary venue's own wall clock, as a
// timestamp WITHOUT time zone.
//
// Every boundary below is derived from it rather than respelling it. The two
// that partition upcoming from past (VenueLocalTodaySQL and
// venueLocalNightStartDateSQL) differ only in the rule applied to this clock,
// and an edit to the zone chain that reached one spelling and not the other
// would reopen exactly the disagreement this file exists to close.
var venueLocalNowSQL = `now() AT TIME ZONE ` + venueLocalZoneSQL

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
//
// "upcoming" here means FROM MIDNIGHT. A surface printed beside a listing of
// tonight wants VenueLocalNightDateCondition instead, which holds the night in
// progress until NightStartHour. The two are the same answer for eighteen hours
// a day, so a surface that takes the wrong one looks correct in most of its
// screenshots; pick on what the number sits next to, not on what it returns
// when you look at it.
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

// NightStartHour is the local hour at which a new night begins.
//
// Before it, the night in progress is still the PREVIOUS calendar date: a night
// is named by the date it BEGAN on, so at 01:00 on Saturday the night people are
// out on is Friday's. The scene page's counts are bounded here rather than at
// midnight, so they and the tonight listing they sit beside name the same night:
// the headline figure, the rooms leaderboard and the per-band roster count. The
// inventory at the top of this file says which surface takes which bound.
// catalog/scene_day.go's tonightDate is the Go statement of the same rule and
// reads this constant rather than restating it.
//
// It unifies the HOUR, not the clock. The tonight bucket applies it to the
// scene's zone, which is the modal stored zone of the scene's verified rooms;
// the conditions here apply it per row to the show's primary venue's. The two
// coincide wherever those resolve alike, and can disagree for one night edge
// where they do not: a metro spanning a timezone line (several US states hold
// two), or a room whose stored zone differs from its neighbours'.
const NightStartHour = 6

// nightStartDateSQL renders the night-start rule over an expression that already
// yields a venue-LOCAL wall clock: the local date, or the one before it when
// that clock has not yet reached NightStartHour.
//
// Shifting the wall clock back NightStartHour hours and truncating states the
// rule in one expression. localNowExpr must be a timestamp WITHOUT time zone,
// the offset already resolved, so the subtraction is pure wall-clock arithmetic
// and no DST transition can carry the result onto a neighbouring date. That is
// the same wall clock catalog's tonightDate compares with Hour().
//
// Taken as a parameter rather than hardcoding `now()` so the rule can be
// evaluated by Postgres against a stated clock in a test; production has one
// caller and it passes now().
func nightStartDateSQL(localNowExpr string) string {
	return `((` + localNowExpr + `) - interval '` + strconv.Itoa(NightStartHour) + ` hours')::date`
}

// venueLocalNightStartDateSQL is the night-start date on the primary venue's
// own clock.
var venueLocalNightStartDateSQL = nightStartDateSQL(venueLocalNowSQL)

// nightUpcomingCoarseBound is the sargable UTC bound that is lossless with
// respect to the exact night condition below, for the same reason and in the
// same way as upcomingCoarseBound.
//
// The earliest instant that can satisfy the night condition is local midnight
// on the night-start date, which before NightStartHour is YESTERDAY. That is at
// most NightStartHour plus one local day behind now, and a local day can run 25
// hours across a fall-back transition, so 31 hours is the widest real gap.
//
// THREE days is therefore slack rather than arithmetic necessity: two would
// cover it. It keeps the same one-extra-day cushion upcomingCoarseBound takes
// over its own 25-hour worst case, and an extra day of already-past shows costs
// nothing in selectivity against an open-ended upper side.
//
// The guard on it is arithmetic over the same constants the fragment is
// rendered from. It is weaker than upcomingCoarseBound's, which is pinned
// against real Postgres across the inhabited offset range: an edit to the EXACT
// condition rather than to these constants would not fail anything here.
const nightCoarseMarginDays = 3

var nightUpcomingCoarseBound = `shows.event_date >= now() - interval '` +
	strconv.Itoa(nightCoarseMarginDays) + ` days'`

// VenueLocalNightDateCondition returns the WHERE fragment selecting the shows a
// scene surface counts as still to come: those whose venue-local date is on or
// after the night in progress.
//
// It differs from VenueLocalDateCondition("upcoming") only between midnight and
// NightStartHour, where it still holds the night that is under way. A surface
// that prints a DATE should think before taking it: between those hours this
// keeps a row whose date reads as yesterday's.
//
// Carries no bind parameters, like its midnight twin: "now" is evaluated by
// Postgres per row against that row's own venue zone.
var VenueLocalNightDateCondition = nightUpcomingCoarseBound + " AND " +
	VenueLocalDateSQL + " >= " + venueLocalNightStartDateSQL

// VenueLocalNightPastDateCondition selects the shows a night-bounded surface
// has left behind: those whose venue-local date is before the night in
// progress.
//
// It is the EXACT complement of VenueLocalNightDateCondition over approved
// shows, because both edges are the same venueLocalNightStartDateSQL. A surface
// that pairs a night-bounded "next" with VenueLocalDateCondition("past")
// instead would count a show on the previous local date as both, every night
// between midnight and NightStartHour.
//
// pastCoarseBound is lossless here for the same reason it is on the midnight
// twin: the latest instant this condition can keep is the moment before local
// midnight on the night-start date, and that date's midnight is never after
// now.
//
// Carries no bind parameters, like the conditions above it.
var VenueLocalNightPastDateCondition = pastCoarseBound + " AND " +
	VenueLocalDateSQL + " < " + venueLocalNightStartDateSQL

// VenueLocalNightWindowCondition returns the WHERE fragment selecting the shows
// on the night in progress and the nights-1 nights that follow it, each row
// judged on its own venue's clock.
//
// Both edges are drawn from the SAME night-start date, so the set is a subset of
// VenueLocalNightDateCondition's by construction rather than by argument: a
// surface printing this beside that one cannot lead with a window count larger
// than the total it slices, whatever hour it is read at. It is a window of whole
// venue-local NIGHTS rather than a span from the request instant, so its edges
// move once a night and two readers minutes apart see one number.
//
// nights is rendered by strconv.Itoa, which emits only digits and a sign, so
// the interpolation is safe for any int a caller can pass. Fewer than one night
// renders an unsatisfiable window and counts nothing rather than erroring.
// Carries no bind parameters, like the conditions above it.
func VenueLocalNightWindowCondition(nights int) string {
	return VenueLocalNightDateCondition + " AND " + VenueLocalDateSQL + " < (" +
		venueLocalNightStartDateSQL + " + " + strconv.Itoa(nights) + ")"
}

// periodCoarseMargin widens the sargable UTC bounds below far enough that no
// venue-local instant of the requested period can fall outside them. The
// inhabited UTC offset range is -12:00 to +14:00, so one day would already cover
// it; two matches the margin the upcoming/past coarse bounds use, for the same
// reason (historical offsets have been stranger than the present ones, and the
// extra day costs nothing in selectivity).
//
// ONE margin for the year, month and day conditions rather than one each: they
// are the same calendar period question at three resolutions, and a margin that
// reached one of them and not the others would leave the narrower windows
// dropping rows the wider one keeps.
const periodCoarseMargin = 48 * time.Hour

// maxCoarseBoundedYear is the largest year whose coarse bounds are worth
// building. Above it the Go time.Time bounds stop round-tripping cleanly through
// the driver, and since the bounds are a planner hint rather than a correctness
// input, the honest move is to go without them. See
// coarseBoundedPeriodCondition.
const maxCoarseBoundedYear = 9999

// coarseBoundedPeriodCondition assembles one calendar-period fragment: the
// exact venue-local equality that decides membership, prefixed where possible by
// sargable UTC bounds on shows.event_date.
//
// ONE assembly for the year, month and day conditions below. The coarse bounds
// carry no correctness weight, so the rule for dropping them and the rule for
// widening them are both invariants of the family rather than of any one
// resolution, and a copy per resolution is a copy that can be corrected in two
// places out of three.
//
// start and end are the period's UTC endpoints, half-open. An end past the
// representable range drops the bounds and keeps the exact fragment, which
// matches nothing rather than everything: widening to "" would answer "every
// period" to a caller who asked for one.
func coarseBoundedPeriodCondition(start, end time.Time, exact string, exactArgs ...any) (string, []any) {
	if start.Year() > maxCoarseBoundedYear || end.Year() > maxCoarseBoundedYear+1 {
		return exact, exactArgs
	}

	args := make([]any, 0, len(exactArgs)+2)
	args = append(args, start.Add(-periodCoarseMargin), end.Add(periodCoarseMargin))
	args = append(args, exactArgs...)

	return "shows.event_date >= ? AND shows.event_date < ? AND " + exact, args
}

// VenueLocalYearCondition returns the WHERE fragment and bind arguments
// narrowing a show list to a single VENUE-LOCAL calendar year, or ("", nil) when
// year is zero or negative, which every caller reads as "all years".
//
// Requires the query to have joined VenueTZJoin: the exact half dereferences
// venue_tz. Callers that would otherwise skip the join for timeFilter "all" must
// add it back when a year is requested.
//
// The year is BOUND, not interpolated. The coarse UTC bounds exist only so the
// planner can start an index scan on idx_shows_event_date at the boundary
// instead of walking the venue's whole history, exactly like upcomingCoarseBound
// and pastCoarseBound above; coarseBoundedPeriodCondition owns what happens when
// they are unrepresentable.
func VenueLocalYearCondition(year int) (string, []any) {
	if year <= 0 {
		return "", nil
	}

	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	return coarseBoundedPeriodCondition(start, start.AddDate(1, 0, 0), VenueLocalYearSQL+" = ?", year)
}

// VenueLocalMonthCondition returns the WHERE fragment and bind arguments
// narrowing a show list to a single VENUE-LOCAL calendar month, or ("", nil)
// when the pair does not name a month, which every caller reads as "no month
// narrowing".
//
// Year AND month, never month alone: VenueLocalMonthSQL's own note says why, and
// this is the helper that makes the pair the only way to ask. Both are BOUND.
func VenueLocalMonthCondition(year, month int) (string, []any) {
	if year <= 0 || month < 1 || month > 12 {
		return "", nil
	}

	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	return coarseBoundedPeriodCondition(start, start.AddDate(0, 1, 0),
		VenueLocalYearSQL+" = ? AND "+VenueLocalMonthSQL+" = ?", year, month)
}

// VenueLocalDayCondition returns the WHERE fragment and bind arguments narrowing
// a show list to a single VENUE-LOCAL calendar date, or ("", nil) when the
// triple does not name a real date, which every caller reads as "no day
// narrowing".
//
// The date is compared against VenueLocalDateSQL itself rather than against a
// third EXTRACT, so the day window and the date tile a row prints are derived
// from one expression. The bound argument is an ISO date string cast to `date`
// by Postgres: a timestamptz bound would be re-read through the SESSION's zone,
// which is the caller-anchored boundary this file exists to keep out of listing
// surfaces.
//
// time.Date normalises 31 February into 3 March rather than failing, so the
// round-trip below is what separates a real date from a typo. A caller that owes
// its user an error for one refuses it before asking: the empty fragment here is
// indistinguishable from "no window requested".
func VenueLocalDayCondition(year, month, day int) (string, []any) {
	start, ok := venueLocalDayStart(year, month, day)
	if !ok {
		return "", nil
	}

	isoDate := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	return coarseBoundedPeriodCondition(start, start.AddDate(0, 0, 1),
		VenueLocalDateSQL+" = ?::date", isoDate)
}

// VenueLocalDayRangeCondition returns the WHERE fragment and bind arguments
// narrowing a show list to a RUN of consecutive venue-local calendar dates
// beginning on the given date, or ("", nil) when the triple does not name a real
// date or the run is shorter than two days.
//
// A run of ONE is refused rather than served: VenueLocalDayCondition already is
// that window, and an equality is the narrower predicate. Callers pick between
// the two on the span they were handed, which is what keeps one day at exactly
// one SQL spelling.
//
// Both edges are venue-local DATES compared against VenueLocalDateSQL, the same
// expression the single-day window and the date tile a row prints derive from,
// so a run and the days inside it cannot disagree about which rows they hold.
// Half-open at the end, so consecutive runs tile without overlapping.
//
// The run's LENGTH is not bounded here. How long a run a caller may ask for is a
// contract question, answered by ShowCalendarWindow.Validate and by the request
// schema; this builds whatever run it is handed.
func VenueLocalDayRangeCondition(year, month, day, days int) (string, []any) {
	if days < 2 {
		return "", nil
	}
	start, ok := venueLocalDayStart(year, month, day)
	if !ok {
		return "", nil
	}
	end := start.AddDate(0, 0, days)

	return coarseBoundedPeriodCondition(start, end,
		VenueLocalDateSQL+" >= ?::date AND "+VenueLocalDateSQL+" < ?::date",
		start.Format("2006-01-02"), end.Format("2006-01-02"))
}

// venueLocalDayStart is the UTC midnight of a calendar date, and whether the
// triple names a real one.
//
// The single real-date gate the day and run conditions share. time.Date
// normalises 31 February into 3 March rather than failing, so the round-trip is
// the check, and having it in one place is what keeps the two windows agreeing
// about which dates exist.
func venueLocalDayStart(year, month, day int) (time.Time, bool) {
	if year <= 0 || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	start := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if start.Year() != year || int(start.Month()) != month || start.Day() != day {
		return time.Time{}, false
	}
	return start, true
}

// VenueLocalWindowCondition is the ONE fragment a calendar window names,
// narrowest resolution first: a run of days, then a single day, then a month.
//
// The ladder lives here rather than at each call site because which resolution
// wins is a property of the window vocabulary, not of any one reader, and the
// three builders it calls signal "not my resolution" by answering with an empty
// fragment. A caller reading that convention for itself re-derives, every time,
// why a run of one must not take the range builder.
//
// An empty answer means the window narrows NOTHING, which every caller has to
// tell apart from a window it refused: in SQL the two are the same query.
func VenueLocalWindowCondition(year, month, day, days int) (string, []any) {
	if condition, args := VenueLocalDayRangeCondition(year, month, day, days); condition != "" {
		return condition, args
	}
	if condition, args := VenueLocalDayCondition(year, month, day); condition != "" {
		return condition, args
	}
	return VenueLocalMonthCondition(year, month)
}
