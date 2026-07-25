package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupRadioRoutes configures radio entity endpoints (stations, shows, episodes, plays).
func setupRadioRoutes(rc RouteContext) {
	radioHandler := catalogh.NewRadioHandler(rc.SC.Radio, rc.SC.Artist, rc.SC.Release, rc.SC.AuditLog)

	// Public radio station endpoints
	huma.Get(rc.API, "/radio-stations", radioHandler.ListRadioStationsHandler)
	huma.Get(rc.API, "/radio-stations/{slug}", radioHandler.GetRadioStationHandler)
	huma.Get(rc.API, "/radio-stations/{slug}/episodes", radioHandler.GetRadioStationEpisodesHandler)
	huma.Get(rc.API, "/radio-stations/{slug}/now-playing", radioHandler.GetRadioStationNowPlayingHandler)
	huma.Get(rc.API, "/radio-stations/{slug}/top-artists", radioHandler.GetRadioStationTopArtistsHandler)
	huma.Get(rc.API, "/radio-stations/{slug}/top-labels", radioHandler.GetRadioStationTopLabelsHandler)
	huma.Get(rc.API, "/radio-stations/{slug}/graph", radioHandler.GetRadioStationGraphHandler)

	// Public radio show endpoints
	huma.Get(rc.API, "/radio-shows", radioHandler.ListRadioShowsHandler)
	huma.Get(rc.API, "/radio-shows/{slug}", radioHandler.GetRadioShowHandler)
	huma.Get(rc.API, "/radio-shows/{slug}/episodes", radioHandler.GetRadioShowEpisodesHandler)
	huma.Get(rc.API, "/radio-shows/{slug}/episodes/{date}", radioHandler.GetRadioEpisodeByDateHandler)
	huma.Get(rc.API, "/radio-shows/{slug}/top-artists", radioHandler.GetRadioShowTopArtistsHandler)
	huma.Get(rc.API, "/radio-shows/{slug}/top-labels", radioHandler.GetRadioShowTopLabelsHandler)

	// Public "as heard on" endpoints (nested under existing entities)
	huma.Get(rc.API, "/artists/{slug}/radio-plays", radioHandler.GetArtistRadioPlaysHandler)
	huma.Get(rc.API, "/releases/{slug}/radio-plays", radioHandler.GetReleaseRadioPlaysHandler)

	// Public radio aggregation endpoints
	huma.Get(rc.API, "/radio/episodes/recent", radioHandler.GetRecentRadioEpisodesHandler)
	huma.Get(rc.API, "/radio/guide", radioHandler.GetRadioGuideHandler)
	huma.Get(rc.API, "/radio/new-releases", radioHandler.GetRadioNewReleaseRadarHandler)
	huma.Get(rc.API, "/radio/stats", radioHandler.GetRadioStatsHandler)

	// Admin radio station endpoints (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/radio-stations", radioHandler.AdminCreateRadioStationHandler)
	huma.Put(rc.Admin, "/admin/radio-stations/{id}", radioHandler.AdminUpdateRadioStationHandler)
	huma.Delete(rc.Admin, "/admin/radio-stations/{id}", radioHandler.AdminDeleteRadioStationHandler)
	huma.Post(rc.Admin, "/admin/radio-stations/{id}/shows", radioHandler.AdminCreateRadioShowHandler)
	// PSY-1135: one station-scoped trigger (discover|fetch) replaces the old
	// /fetch + /discover endpoints; runs async through RunStationSync.
	huma.Post(rc.Admin, "/admin/radio-stations/{id}/sync", radioHandler.AdminTriggerStationSyncHandler)

	// Admin radio show endpoints
	huma.Put(rc.Admin, "/admin/radio-shows/{id}", radioHandler.AdminUpdateRadioShowHandler)
	huma.Delete(rc.Admin, "/admin/radio-shows/{id}", radioHandler.AdminDeleteRadioShowHandler)
	// PSY-1135: show-scoped historic backfill replaces the old synchronous /import
	// and the /import-job create+start; runs async through RunStationSync.
	huma.Post(rc.Admin, "/admin/radio-shows/{id}/backfill", radioHandler.AdminTriggerShowBackfillHandler)

	// Admin sync-run observability (PSY-1135): poll + cancel, mapped onto
	// radio_sync_runs (replaces the import-job get/cancel/list endpoints).
	huma.Get(rc.Admin, "/admin/radio/sync-runs/{id}", radioHandler.AdminGetSyncRunHandler)
	huma.Post(rc.Admin, "/admin/radio/sync-runs/{id}/cancel", radioHandler.AdminCancelSyncRunHandler)

	// Admin observability feeds (PSY-1129/P5): recent sync-run history (global +
	// per-station) and the station-health rollup for the admin dashboard.
	huma.Get(rc.Admin, "/admin/radio/sync-runs", radioHandler.AdminListSyncRunsHandler)
	huma.Get(rc.Admin, "/admin/radio-stations/{id}/sync-runs", radioHandler.AdminListStationSyncRunsHandler)
	huma.Get(rc.Admin, "/admin/radio/station-health", radioHandler.AdminListStationHealthHandler)
	huma.Get(rc.Admin, "/admin/radio-stations/{id}/health", radioHandler.AdminGetStationHealthHandler)

	// Admin unmatched play management endpoints
	huma.Get(rc.Admin, "/admin/radio/unmatched", radioHandler.AdminGetUnmatchedPlaysHandler)
	huma.Post(rc.Admin, "/admin/radio/plays/{id}/link", radioHandler.AdminLinkPlayHandler)
	huma.Post(rc.Admin, "/admin/radio/plays/bulk-link", radioHandler.AdminBulkLinkPlaysHandler)
	huma.Post(rc.Admin, "/admin/radio/rematch", radioHandler.AdminReMatchPlaysHandler)
}
