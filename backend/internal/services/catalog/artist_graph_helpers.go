package catalog

// Shared helpers for the artist graph endpoints (scene graph PSY-367, venue
// bill network PSY-365, station graph PSY-1081). Extracted in PSY-1081 when a
// third copy of the upcoming-show-count batch query was about to land.

import (
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// batchArtistUpcomingShowCounts returns a map of artist_id → upcoming
// approved show count (globally, not scoped to the graph's anchor entity), so
// the graph node green-dot indicator stays consistent with the rest of the
// app. Returns an empty map (never nil) so callers can index without a nil
// check. Errors degrade to zero counts — the indicator is decorative, not
// load-bearing (same posture as the original scene/venue helpers).
func batchArtistUpcomingShowCounts(db *gorm.DB, artistIDs []uint) map[uint]int {
	out := make(map[uint]int, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ArtistID  uint
		ShowCount int64
	}
	var rows []row
	db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) AS show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = ? AND shows.event_date > NOW()",
			artistIDs, catalogm.ShowStatusApproved).
		Group("show_artists.artist_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.ArtistID] = int(r.ShowCount)
	}
	return out
}
