package catalog

import (
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// rosterUpcoming is one band's upcoming activity: how many shows it has ahead
// of it and the soonest of them. Both come from one query over one filtered row
// set, so Count > 0 and Next != nil are the same fact.
type rosterUpcoming struct {
	Count int
	Next  *contracts.SceneArtistNextShow
}

// rosterUpcomingVenueCols is the display venue a row names. It is projected
// from shared.PrimaryVenueLateralSQL, the same pick rule shared.VenueTZJoin
// uses for the zone, so the room printed on a row is always the room whose zone
// dated it.
const rosterUpcomingVenueCols = `COALESCE(iv.name, '') AS venue_name, COALESCE(iv.slug, '') AS venue_slug`

// batchRosterUpcoming returns artist_id → upcoming activity for a page of
// roster artists, in ONE query. Bands with nothing booked are absent from the
// map, which is the zero the caller renders.
//
// The boundary is shared.VenueLocalDateCondition: a show leaves the count at
// midnight in its OWN venue's zone, so a date-only listing for tonight counts
// all evening. The scene graph's own per-artist figures on the same page
// (batchArtistUpcomingShowCounts, batchArtistNextShows) are bounded on the
// start INSTANT instead, so for a show already under way the two disagree: this
// count still holds it and the graph node's dot does not.
//
// Errors are returned rather than degraded to an empty map, unlike those graph
// helpers: a count silently rendered as zero states something false about a
// band, where an unlit dot states nothing.
func (s *SceneService) batchRosterUpcoming(artistIDs []uint) (map[uint]rosterUpcoming, error) {
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

	// show_artists is keyed (show_id, artist_id), so the inner query emits
	// exactly one row per (band, upcoming show) and COUNT(*) OVER is the band's
	// distinct upcoming show count.
	//
	// The venue lateral sits OUTSIDE the subquery, on the one surviving row per
	// band, rather than on every upcoming show the count walks.
	//
	// The date is rendered by shared.VenueLocalDateSQL, the same expression the
	// WHERE clause tested, so a printed date cannot contradict the filter that
	// selected it. to_char pins the format rather than leaving it to DateStyle.
	//
	// Soonest is ordered by the START INSTANT, not the venue-local date: two
	// shows sharing a local date are still one before the other. shows.id breaks
	// the remaining tie, so the pick is stable across calls.
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
			  AND `+shared.VenueLocalDateCondition("upcoming")+`
			ORDER BY show_artists.artist_id, shows.event_date ASC, shows.id ASC
		) u
		LEFT JOIN LATERAL `+shared.PrimaryVenueLateralSQL(rosterUpcomingVenueCols, "u.show_id")+` pv ON true
	`, artistIDs, catalogm.ShowStatusApproved).Scan(&rows).Error; err != nil {
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

// applyRosterUpcoming stamps a page of roster rows with their upcoming figures.
// A band absent from the lookup keeps the zero value, which is the state the
// roster line renders as a bare name.
func applyRosterUpcoming(page []*contracts.SceneArtistResponse, byArtist map[uint]rosterUpcoming) {
	for _, artist := range page {
		if u, ok := byArtist[artist.ID]; ok {
			artist.UpcomingShowCount = u.Count
			artist.NextShow = u.Next
		}
	}
}
