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
	huma.Get(rc.API, "/radio/new-releases", radioHandler.GetRadioNewReleaseRadarHandler)
	huma.Get(rc.API, "/radio/stats", radioHandler.GetRadioStatsHandler)

	// Admin radio station endpoints (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/radio-stations", radioHandler.AdminCreateRadioStationHandler)
	huma.Put(rc.Admin, "/admin/radio-stations/{id}", radioHandler.AdminUpdateRadioStationHandler)
	huma.Delete(rc.Admin, "/admin/radio-stations/{id}", radioHandler.AdminDeleteRadioStationHandler)
	huma.Post(rc.Admin, "/admin/radio-stations/{id}/shows", radioHandler.AdminCreateRadioShowHandler)
	huma.Post(rc.Admin, "/admin/radio-stations/{id}/fetch", radioHandler.AdminTriggerFetchHandler)
	huma.Post(rc.Admin, "/admin/radio-stations/{id}/discover", radioHandler.AdminDiscoverShowsHandler)

	// Admin radio show endpoints
	huma.Put(rc.Admin, "/admin/radio-shows/{id}", radioHandler.AdminUpdateRadioShowHandler)
	huma.Delete(rc.Admin, "/admin/radio-shows/{id}", radioHandler.AdminDeleteRadioShowHandler)
	huma.Post(rc.Admin, "/admin/radio-shows/{id}/import", radioHandler.AdminImportShowEpisodesHandler)

	// Admin import job endpoints
	huma.Post(rc.Admin, "/admin/radio-shows/{id}/import-job", radioHandler.AdminCreateImportJobHandler)
	huma.Get(rc.Admin, "/admin/radio/import-jobs/{id}", radioHandler.AdminGetImportJobHandler)
	huma.Post(rc.Admin, "/admin/radio/import-jobs/{id}/cancel", radioHandler.AdminCancelImportJobHandler)
	huma.Get(rc.Admin, "/admin/radio-shows/{id}/import-jobs", radioHandler.AdminListImportJobsHandler)

	// Admin unmatched play management endpoints
	huma.Get(rc.Admin, "/admin/radio/unmatched", radioHandler.AdminGetUnmatchedPlaysHandler)
	huma.Post(rc.Admin, "/admin/radio/plays/{id}/link", radioHandler.AdminLinkPlayHandler)
	huma.Post(rc.Admin, "/admin/radio/plays/bulk-link", radioHandler.AdminBulkLinkPlaysHandler)
}
