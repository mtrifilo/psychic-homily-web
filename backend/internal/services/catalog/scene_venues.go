package catalog

import (
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// trackedVenuePredicate returns a WHERE fragment (on the given venues alias)
// selecting the rooms a scene TRACKS — the scene's venues, verified — plus its
// positional bind args. The alias must name a VENUES table or row; the fragment
// is self-parenthesised, so it is safe to AND or OR it into a larger predicate.
//
// It exists so "a room this scene tracks" has ONE definition. Three queries ask
// it — the day page's footer, the detail page's leaderboard, and the
// verifiedVenueCount that gates scene existence — and they project, order and
// count differently, so they cannot share a whole query. What they must never
// disagree on is WHICH rooms count: excluding (say) permanently-closed rooms
// from one query alone would leave two pages naming different rooms for the same
// city, or a scene whose venue_count and venue LIST disagree.
//
// BINDING: args go in SQL TEXT order, not "predicate first". The rule
// venuePredicate states for its own callers — splice it first so its args lead —
// does not survive being wrapped: sceneVenueLeaderboard has placeholders in a
// derived table that precede this fragment, and binds those first. A new caller
// must count placeholders as they appear in the string, and a slip is silent
// (both branches bind strings, so a swap returns zero rows rather than erroring).
func trackedVenuePredicate(scope sceneScope, alias string) (string, []any) {
	vp, args := scope.venuePredicate(alias)
	return "((" + vp + ") AND " + alias + ".verified = true)", args
}

// sceneVenueLeaderboard lists the scene's tracked rooms with each one's count of
// approved shows still to come, busiest first.
//
// The count is bounded at the NIGHT in progress in each show's own venue zone
// (shared.VenueLocalNightDateCondition), not at the caller's instant. A room
// whose only booking is TONIGHT reading 0, ranked below a room with one show in
// November, is precisely the failure a leaderboard cannot have, and an instant
// bound produces it from the moment the first set starts.
//
// SceneStats.UpcomingShowCount is drawn on the same boundary, so the two agree
// about which nights they are counting. They still differ in WHAT they count:
// see the contract on SceneVenueSummary.UpcomingShowCount.
//
// The zone that dates a row is the show's PRIMARY venue's, not v2's, because
// the boundary is the repo's shared one. For a bill split across two rooms the
// second room's tally is drawn on the first room's clock, which matters only
// where a scope spans a timezone line.
//
// Name breaks a count tie and id breaks a name tie, so the order is total: the
// rooms with nothing booked (most of them, in a sparse scene) come back in a
// stable alphabetical block rather than shuffling between requests. Id is not
// belt-and-braces — venue names are unique only per city
// (idx_venues_name_city_unique), and a metro scope spans several, so one scene
// really can hold two rooms of the same name.
func (s *SceneService) sceneVenueLeaderboard(scope sceneScope) ([]contracts.SceneVenueSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	// The rooms being ranked, and the rooms whose shows are counted, are the
	// same set — the aggregate is scoped INSIDE the derived table rather than
	// filtered after it. Postgres cannot push the outer join's qual into a
	// grouped subquery, so without this the scene page would aggregate every
	// upcoming show in the catalog to rank a dozen rooms.
	inner, innerArgs := trackedVenuePredicate(scope, "v2")
	outer, outerArgs := trackedVenuePredicate(scope, "v")

	// Placeholders bind in SQL TEXT order: the derived table's, then the outer
	// WHERE's. The scope args appear TWICE because the predicate does. The night
	// condition binds nothing: Postgres evaluates it per row against that row's
	// own venue zone.
	//
	// The derived table's shows must be UNALIASED: shared.VenueTZJoin's lateral
	// correlates on `shows.id`, and the aliased form fails at query time on a
	// live scene page rather than at build.
	args := append([]any{catalogm.ShowStatusApproved}, innerArgs...)
	args = append(args, outerArgs...)

	// LEFT JOIN + COALESCE, not an inner join: a room whose shows are all in the
	// past has to survive as a 0, not vanish. Same derived-table shape as
	// GetActiveVenues, which by contrast DROPS its zero rows — it ranks a global
	// chart, where a room with nothing on has nothing to say.
	var venues []contracts.SceneVenueSummary
	if err := s.db.Raw(`
		SELECT v.id AS id,
		       v.name AS name,
		       COALESCE(v.slug, '') AS slug,
		       COALESCE(v.website, '') AS website,
		       COALESCE(v.city, '') AS city,
		       COALESCE(v.state, '') AS state,
		       COALESCE(show_counts.cnt, 0) AS upcoming_show_count
		FROM venues v
		LEFT JOIN (
			SELECT sv.venue_id, COUNT(DISTINCT shows.id) AS cnt
			FROM show_venues sv
			JOIN shows ON shows.id = sv.show_id
			JOIN venues v2 ON v2.id = sv.venue_id
			`+shared.VenueTZJoin+`
			WHERE shows.status = ?
			  AND `+shared.VenueLocalNightDateCondition+`
			  AND `+inner+`
			GROUP BY sv.venue_id
		) show_counts ON show_counts.venue_id = v.id
		WHERE `+outer+`
		ORDER BY upcoming_show_count DESC, v.name ASC, v.id ASC
	`, args...).Scan(&venues).Error; err != nil {
		return nil, fmt.Errorf("failed to list scene venues: %w", err)
	}
	if venues == nil {
		venues = []contracts.SceneVenueSummary{}
	}
	return venues, nil
}
