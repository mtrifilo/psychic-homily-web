package catalog

import (
	"log/slog"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Retraction reconcile (PSY-1286): WFMU DJs sometimes create a playlist page
// (accidentally, or pre-created for a broadcast) that WFMU later deletes. Our
// scraper faithfully imports it while it exists, and nothing ever noticed the
// upstream deletion — the row lived on as an "aired, pending, 0-track" phantom
// (hidden from the Latest-playlists feed by airedEpisodeVisibleSQL, but still
// polluting the ungated per-show archive and the DB). Root-cause evidence on
// the ticket: three such rows in June 2026, each a real WFMU playlist id that
// now 404s and is gone from the show's archive page.
//
// The reconcile piggybacks on data the scheduled fetch already holds: for a
// provider whose listing is exhaustive (ExhaustiveEpisodeLister — the fetch
// returns every episode the provider currently publishes in the window), a
// stored episode inside the window that is absent from the fetch result has
// been retracted upstream. No new HTTP.

// retractedEpisodeGuardStates are the playlist states a retraction delete may
// touch. 'pending' (never fetched) covers a fresh stray; 'unavailable' covers
// one whose playlist fetch 404'd to exhaustion before this reconcile saw it.
// 'partial'/'complete' rows carry plays and are excluded belt-and-suspenders
// by the play guards too.
var retractedEpisodeGuardStates = []string{
	catalogm.RadioPlaylistStatePending,
	catalogm.RadioPlaylistStateUnavailable,
}

// retractedEpisodeRow is the RETURNING projection of a reconcile delete,
// logged so an upstream retraction is always traceable.
type retractedEpisodeRow struct {
	ID         uint
	ExternalID string
	AirDate    string
}

// reconcileRetractedEpisodes deletes this show's placeholder rows that the
// provider no longer publishes, and returns how many it deleted. upstream is
// the fetch result for [since, now]; the caller must only pass a SUCCESSFUL
// fetch's result.
//
// Deletion is bounded three ways, each load-bearing:
//
//   - Capability: only providers asserting ExhaustiveEpisodeLister authorize
//     absence-means-retracted. For everyone else this is a no-op.
//   - Date window: [day(since)+1, day(now)UTC-1). The lower bound skips the
//     `since` boundary day — the archive parser drops rows whose UTC-midnight
//     air date is before a mid-day `since`, so that day's stored rows can be
//     absent from the result without being retracted. The upper bound stays a
//     full day behind UTC-today because the provider's local "today" (e.g.
//     WFMU's ET wfmuTodayCap) can trail the UTC date; same-day churn is never
//     reconciled — a stray created today is caught tomorrow.
//   - Row guards: only trackless placeholders (play_count = 0, no radio_plays
//     rows, playlist_state pending/unavailable) are deletable. A real episode
//     carries plays or a complete/partial playlist state and is untouchable
//     here regardless of what the listing says.
//
// An EMPTY upstream result skips the reconcile entirely: a parser broken by a
// page-layout change returns nothing, and "we parsed nothing" must not read as
// "everything was retracted". A PARTIAL parse could still drop a genuine
// trackless row, which is why deletion is recoverable by construction — the
// next healthy fetch re-imports the episode from the same listing (create-on-
// import), so the worst case is a placeholder row flickering, not data loss.
func (s *RadioService) reconcileRetractedEpisodes(showID uint, provider RadioPlaylistProvider, upstream []RadioEpisodeImport, since, now time.Time) int {
	lister, ok := provider.(ExhaustiveEpisodeLister)
	if !ok || !lister.EpisodeListingIsExhaustive() {
		return 0
	}
	if len(upstream) == 0 {
		return 0
	}

	lower := since.UTC().Truncate(24 * time.Hour).AddDate(0, 0, 1).Format("2006-01-02")
	upper := now.UTC().Truncate(24 * time.Hour).AddDate(0, 0, -1).Format("2006-01-02")
	if lower >= upper {
		return 0
	}

	upstreamIDs := make([]string, 0, len(upstream))
	for _, ep := range upstream {
		if ep.ExternalID != "" {
			upstreamIDs = append(upstreamIDs, ep.ExternalID)
		}
	}
	if len(upstreamIDs) == 0 {
		return 0
	}

	var deleted []retractedEpisodeRow
	err := s.db.Raw(`
		DELETE FROM radio_episodes e
		WHERE e.show_id = ?
		  AND e.air_date >= ? AND e.air_date < ?
		  AND e.external_id IS NOT NULL
		  AND e.external_id NOT IN (?)
		  AND e.play_count = 0
		  AND e.playlist_state IN (?)
		  AND NOT EXISTS (SELECT 1 FROM radio_plays p WHERE p.episode_id = e.id)
		RETURNING e.id, e.external_id, e.air_date`,
		showID, lower, upper, upstreamIDs, retractedEpisodeGuardStates,
	).Scan(&deleted).Error
	if err != nil {
		// Log-and-continue: the reconcile is a hygiene pass riding a fetch
		// cycle; a failed delete must not fail the show's import.
		slog.Default().Error("radio retraction reconcile: delete failed",
			"show_id", showID, "error", err)
		return 0
	}

	for _, row := range deleted {
		slog.Default().Info("radio retraction reconcile: deleted upstream-retracted episode",
			"show_id", showID, "episode_id", row.ID,
			"external_id", row.ExternalID, "air_date", row.AirDate)
	}
	return len(deleted)
}
