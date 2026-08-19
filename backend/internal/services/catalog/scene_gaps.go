package catalog

import (
	"fmt"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Scene completeness gaps (PSY-1845).
//
// The scene front page publishes its own unfinished edges as content — "11
// Chicago bands have no Bandcamp link → help finish Chicago" — so the ask is a
// specific, countable task instead of a generic invitation to contribute. This
// file computes those numbers.
//
// Both counts reuse the scene's EXISTING scoping rules rather than respelling
// them: artistPredicate for "based here" (so the listen-link gap is a subset of
// the same roster SceneStats.ArtistCount reports) and venuePredicate for "the
// scene's rooms" (so "played here" means what every other show-side scene
// surface means). A second, independently-written spelling of either would
// drift, and a gap line that disagrees with the roster count directly above it
// is worse than no gap line.

// noListenLinkSQL builds the "has no listen link" predicate for the artists
// alias: every listen-link column blank. Column names are internal literals,
// never caller input, so the concatenation carries no injection surface.
//
// The four columns are NOT chosen here — they are the project's existing
// definition of a music-platform link, shared with the streaming-discovery
// worklist (models/catalog.StreamingDiscoveryStatus) and with the migration
// that backfilled it (20260524213942). The other Social columns
// (instagram/facebook/twitter/website) are deliberately excluded: a band's
// Instagram is not somewhere you can listen to them, and counting it as one
// would report the gap closed while the reader still has nothing to play.
// Keep in lockstep with that worklist — if the set changes, both move together
// or the admin queue and the public gap line start describing different bands.
//
// The test is NULLIF(TRIM(col), empty-string) IS NULL rather than a bare IS
// NULL, because a link column can hold an empty string as well as NULL — the
// update paths route through utils.NilIfEmpty, but rows predating that (and any
// direct write) can carry a blank. A bare IS NULL reads a blank as "has a link"
// and would silently UNDERSTATE the gap, which is the one direction this number
// must not be wrong in: it would hide work from the very readers being asked to
// do it.
//
// (Spelled in prose rather than as literal SQL because gofmt's doc-comment
// reformatter rewrites a pair of straight single quotes into a typographic
// quote — see pattern_gofmt_doc_comment_quotes.)
func noListenLinkSQL(alias string) string {
	listenLinkColumns := []string{"spotify", "bandcamp", "youtube", "soundcloud"}

	pred := ""
	for i, col := range listenLinkColumns {
		if i > 0 {
			pred += " AND "
		}
		pred += "NULLIF(TRIM(" + alias + "." + col + "), '') IS NULL"
	}
	return pred
}

// GetSceneGaps returns the scene's completeness gap counts.
//
// Existence gate: the same verified-venue threshold GetSceneDetail applies, so
// a parseable slug that is not (yet) a scene 404s here exactly as it does
// there. Deliberately STRICTER than the sibling /new-artists route, which is
// permissive because its module hides itself client-side. A gap payload has no
// such fallback — answering 200 with two zeros for an arbitrary city slug would
// publish "this place is complete" about a place that does not exist.
func (s *SceneService) GetSceneGaps(city, state string) (*contracts.SceneGapsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)

	venueCount, err := s.verifiedVenueCount(scope)
	if err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	ap, aargs := s.artistPredicate(scope, "a")
	vp, vargs := scope.venuePredicate("v")

	// Gap 1 — bands based here with nothing to listen to. Same roster scope as
	// SceneStats.ArtistCount, so this count is always a subset of that one and
	// the page can honestly say "N of your M bands".
	var missingListenLink int64
	if err := s.db.Raw(`
		SELECT COUNT(*)
		FROM artists a
		WHERE `+ap+`
		  AND `+noListenLinkSQL("a"),
		aargs...,
	).Scan(&missingListenLink).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists missing a listen link: %w", err)
	}

	// Gap 2 — bands that have played the scene's rooms but carry no home
	// location, so artistPredicate can never claim them however local they are.
	//
	// "No location" is city AND state both blank, matching the project's
	// existing artists_missing_location definition (services/admin
	// data_quality.go). A band with a PARTIAL location (city but no state) is
	// also unclaimable by the roster, but it is a different, smaller gap with a
	// different fix, and folding it in here would quietly redefine a published
	// number. It is intentionally not counted.
	//
	// Approved shows only, all dates: this is a backlog to work through, not a
	// pulse. An old bill is exactly as fixable as tonight's, and windowing it
	// would make the number shrink on its own without anyone doing the work.
	// DISTINCT because a band that played the scene ten times is ONE gap.
	//
	// Venue scope is venuePredicate (every room in the scene's scope), NOT
	// trackedVenuePredicate (verified rooms only). That matches the show-side
	// figures in GetSceneDetail rather than the venue leaderboard: a band that
	// played an unverified room in the metro still played this scene, and its
	// missing location is still this scene's to fix.
	countArgs := append(append([]any{}, vargs...), catalogm.ShowStatusApproved)
	var missingLocation int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id)
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		JOIN artists a ON a.id = sa.artist_id
		WHERE `+vp+`
		  AND s.status = ?
		  AND NULLIF(TRIM(a.city), '') IS NULL
		  AND NULLIF(TRIM(a.state), '') IS NULL
	`, countArgs...).Scan(&missingLocation).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists on bills missing a location: %w", err)
	}

	return &contracts.SceneGapsResponse{
		City:                          city,
		State:                         state,
		Slug:                          buildSceneSlug(city, state),
		ArtistsMissingListenLink:      int(missingListenLink),
		ArtistsOnBillsMissingLocation: int(missingLocation),
	}, nil
}
