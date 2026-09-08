package catalog

import (
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// rosterUpcoming is one band's upcoming activity in a scene: how many shows it
// has ahead of it there and the soonest of them. Both come from one query over
// one filtered row set, so Count > 0 and Next != nil are the same fact.
type rosterUpcoming struct {
	Count int
	Next  *contracts.SceneArtistNextShow
}

// rosterUpcomingVenueCols is the venue a row names. It is projected from
// shared.PrimaryVenueLateralSQL, the same pick rule shared.VenueTZJoin uses for
// the zone, so the room a row names is always the room whose zone dated it.
const rosterUpcomingVenueCols = `COALESCE(iv.name, '') AS venue_name, COALESCE(iv.slug, '') AS venue_slug`

// EnrichRosterUpcoming stamps a page of roster rows with their upcoming figures
// for this scene, in ONE query. A band with nothing booked in the scene keeps
// the zero value: count 0 and no next show.
//
// It is a separate call rather than part of GetActiveArtists because most
// consumers of that roster never render these fields. The /atlas hover preview
// and the mobile scene list both read the same endpoint and draw names only;
// folding the query in would charge them for a figure they do not print.
//
// SCOPE IS THE SCENE, not the band's whole calendar. Every other number on the
// page is metro-scoped, so a band's figure here counts its shows at rooms this
// scene tracks and nothing else, and Next is the soonest of those. The rule is
// the scope's own venuePredicate rather than a second metro spelling.
//
// The upcoming boundary is shared.VenueLocalDateCondition: a show leaves the
// count at midnight in its OWN venue's zone, so a date-only listing for tonight
// counts all evening. The scene graph's per-artist figures on the same page
// (batchArtistUpcomingShowCounts, batchArtistNextShows) are bounded on the start
// INSTANT instead, so for a show already under way the two disagree: this count
// still holds it and the graph node's dot does not.
//
// Errors are returned rather than degraded to an empty map, unlike those graph
// helpers: a count silently reported as zero states something false about a
// band, where an unlit dot states nothing.
func (s *SceneService) EnrichRosterUpcoming(city, state string, page []*contracts.SceneArtistResponse) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(page) == 0 {
		return nil
	}

	scope, err := s.scopeFor(city, state)
	if err != nil {
		return err
	}

	ids := make([]uint, len(page))
	for i, artist := range page {
		ids[i] = artist.ID
	}

	byArtist, err := s.batchRosterUpcoming(scope, ids)
	if err != nil {
		return err
	}

	for _, artist := range page {
		if u, ok := byArtist[artist.ID]; ok {
			artist.UpcomingShowCount = u.Count
			artist.NextShow = u.Next
		}
	}
	return nil
}

// batchRosterUpcoming returns artist_id -> upcoming activity in `scope`, for the
// given page of roster artists. Bands with nothing booked in the scene are
// absent from the map.
func (s *SceneService) batchRosterUpcoming(scope sceneScope, artistIDs []uint) (map[uint]rosterUpcoming, error) {
	out := make(map[uint]rosterUpcoming, len(artistIDs))
	if len(artistIDs) == 0 {
		return out, nil
	}

	type upcomingRow struct {
		ArtistID  uint   `gorm:"column:artist_id"`
		Upcoming  int    `gorm:"column:upcoming_count"`
		ShowID    uint   `gorm:"column:show_id"`
		ShowSlug  string `gorm:"column:show_slug"`
		EventDate string `gorm:"column:event_date"`
		VenueName string `gorm:"column:venue_name"`
		VenueSlug string `gorm:"column:venue_slug"`
	}

	vp, vargs := scope.venuePredicate("sv_venue")

	// The scene test is an EXISTS rather than a join, so a show booked into two
	// rooms still contributes exactly ONE row. With show_artists keyed
	// (show_id, artist_id), the inner query emits one row per (band, upcoming
	// in-scene show), which is what makes COUNT(*) OVER the band's distinct
	// in-scene upcoming show count.
	//
	// A show qualifies on ANY room in scope while the room it NAMES is the
	// primary pick, so a show split between an in-scene and an out-of-scene room
	// can name the out-of-scene one. Naming a different room from the one that
	// dated it would be the worse defect, and the pick rule is the repo's.
	//
	// The venue lateral sits OUTSIDE the subquery, on the one surviving row per
	// band, rather than on every upcoming show the count walks.
	//
	// The date is rendered by shared.VenueLocalDateSQL, the same expression the
	// WHERE clause tested, so a projected date cannot contradict the filter that
	// selected it. to_char pins the format rather than leaving it to DateStyle.
	//
	// Soonest is ordered by the START INSTANT, not the venue-local date: two
	// shows sharing a local date are still one before the other. shows.id breaks
	// the remaining tie, so the pick is stable across calls.
	//
	// Placeholder order: artist ids, status, then the scope's venue args. The
	// upcoming condition binds nothing.
	args := append([]any{artistIDs, catalogm.ShowStatusApproved}, vargs...)
	var rows []upcomingRow
	if err := s.db.Raw(`
		SELECT u.artist_id, u.upcoming_count, u.show_id, u.show_slug, u.event_date,
		       pv.venue_name, pv.venue_slug
		FROM (
			SELECT DISTINCT ON (show_artists.artist_id)
			       show_artists.artist_id,
			       shows.id AS show_id,
			       COALESCE(shows.slug, '') AS show_slug,
			       to_char(`+shared.VenueLocalDateSQL+`, 'YYYY-MM-DD') AS event_date,
			       COUNT(*) OVER (PARTITION BY show_artists.artist_id) AS upcoming_count
			FROM show_artists
			JOIN shows ON shows.id = show_artists.show_id
			`+shared.VenueTZJoin+`
			WHERE show_artists.artist_id IN ?
			  AND shows.status = ?
			  AND shows.is_cancelled = false
			  AND EXISTS (
				SELECT 1
				FROM show_venues sv_scope
				JOIN venues sv_venue ON sv_venue.id = sv_scope.venue_id
				WHERE sv_scope.show_id = shows.id AND `+vp+`
			  )
			  AND `+shared.VenueLocalDateCondition("upcoming")+`
			ORDER BY show_artists.artist_id, shows.event_date ASC, shows.id ASC
		) u
		LEFT JOIN LATERAL `+shared.PrimaryVenueLateralSQL(rosterUpcomingVenueCols, "u.show_id")+` pv ON true
	`, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get scene roster upcoming shows: %w", err)
	}

	for _, r := range rows {
		out[r.ArtistID] = rosterUpcoming{
			Count: r.Upcoming,
			Next: &contracts.SceneArtistNextShow{
				ID:        r.ShowID,
				Slug:      r.ShowSlug,
				EventDate: r.EventDate,
				VenueName: r.VenueName,
				VenueSlug: r.VenueSlug,
			},
		}
	}
	return out, nil
}
