package catalog

import (
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// The scene roster's per-band upcoming figure and the show behind it.
//
// One query for a whole page of the roster, never one per band: GetActiveArtists
// is the read behind every scene detail page and the /atlas preview, so a
// per-row lookup here would be ten round-trips on all of them.

// rosterUpcoming is one band's upcoming activity: how many shows it has ahead
// of it and the soonest of them. Count and show are produced by a single query
// over a single boundary, so Count > 0 and Next != nil are the same fact.
type rosterUpcoming struct {
	Count int
	Next  contracts.SceneArtistNextShow
}

// rosterUpcomingVenueCols is the display venue this row names, projected from
// shared.PrimaryVenueLateralSQL — the SAME pick rule shared.VenueTZJoin uses for
// the zone. The row's date and the room printed beside it therefore always come
// from one venue, however many rooms a show is booked into.
const rosterUpcomingVenueCols = `COALESCE(iv.name, '') AS venue_name, COALESCE(iv.slug, '') AS venue_slug`

// batchRosterUpcoming returns artist_id → upcoming activity for the given page
// of roster artists. Bands with nothing booked are ABSENT from the map, which
// is the zero the caller renders.
//
// The upcoming boundary is shared.VenueLocalDateCondition, so a show stays in
// the count until midnight in its OWN venue's zone. That is what makes tonight's
// show countable all evening, and it is the same rule the scene's show lists,
// the artist and venue pages, and the /shows feed already partition on. An
// instant-based `event_date > now()` would drop a show that started an hour ago
// and would drop a date-only listing for today outright.
//
// The count is a window function over the SAME filtered rows the next-show pick
// runs on, not a second aggregate: two expressions over one row set cannot
// disagree about how many shows a band has, and the caller's payload asserts
// that they never do.
//
// Errors are returned rather than swallowed. The line this feeds prints a
// COUNT, and a count silently rendered as zero states something false about the
// band; the sibling graph helpers degrade to an empty map because they drive a
// decorative dot.
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

	// show_artists is keyed (show_id, artist_id) and the venue lateral yields at
	// most one row, so the inner query emits exactly one row per (band, upcoming
	// show) and COUNT(*) OVER is the band's distinct upcoming show count.
	//
	// The date is projected by shared.VenueLocalDateSQL rather than re-derived in
	// Go, so the calendar date the row prints is produced by the same expression
	// the WHERE clause tested. to_char pins the rendering to YYYY-MM-DD instead
	// of leaving it to the session's DateStyle.
	//
	// Soonest is ordered by the START INSTANT, not by the venue-local date: two
	// shows sharing a local date are still one before the other. shows.id breaks
	// the remaining tie so the pick is stable across calls.
	//
	// Placeholder order: artist ids, then status. The upcoming condition binds
	// nothing — it evaluates "now" per row against that row's own venue zone.
	var rows []upcomingRow
	if err := s.db.Raw(`
		SELECT DISTINCT ON (u.artist_id)
		       u.artist_id, u.upcoming_count, u.show_id, u.show_slug,
		       u.event_date, u.venue_name, u.venue_slug
		FROM (
			SELECT show_artists.artist_id,
			       shows.id AS show_id,
			       shows.event_date AS starts_at,
			       COALESCE(shows.slug, '') AS show_slug,
			       to_char(`+shared.VenueLocalDateSQL+`, 'YYYY-MM-DD') AS event_date,
			       pv.venue_name,
			       pv.venue_slug,
			       COUNT(*) OVER (PARTITION BY show_artists.artist_id) AS upcoming_count
			FROM show_artists
			JOIN shows ON shows.id = show_artists.show_id
			`+shared.VenueTZJoin+`
			LEFT JOIN LATERAL `+shared.PrimaryVenueLateralSQL(rosterUpcomingVenueCols, "shows.id")+` pv ON true
			WHERE show_artists.artist_id IN ?
			  AND shows.status = ?
			  AND shows.is_cancelled = false
			  AND `+shared.VenueLocalDateCondition("upcoming")+`
		) u
		ORDER BY u.artist_id, u.starts_at ASC, u.show_id ASC
	`, artistIDs, catalogm.ShowStatusApproved).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get scene roster upcoming shows: %w", err)
	}

	for _, r := range rows {
		out[r.ArtistID] = rosterUpcoming{
			Count: r.Upcoming,
			Next: contracts.SceneArtistNextShow{
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
// A band absent from the lookup keeps the zero value: count 0 and no next show,
// which is the state the roster line renders as a bare name.
func applyRosterUpcoming(page []*contracts.SceneArtistResponse, byArtist map[uint]rosterUpcoming) {
	for _, artist := range page {
		u, ok := byArtist[artist.ID]
		if !ok {
			continue
		}
		next := u.Next
		artist.UpcomingShowCount = u.Count
		artist.NextShow = &next
	}
}
