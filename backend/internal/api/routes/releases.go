package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupReleaseRoutes(rc RouteContext) {
	releaseHandler := catalogh.NewReleaseHandler(rc.SC.Release, rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)

	// Public release endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/releases", releaseHandler.ListReleasesHandler)
	huma.Get(rc.API, "/releases/search", releaseHandler.SearchReleasesHandler)
	huma.Get(rc.API, "/releases/{release_id}", releaseHandler.GetReleaseHandler)
	huma.Get(rc.API, "/artists/{artist_id}/releases", releaseHandler.GetArtistReleasesHandler)

	// Protected release endpoints (admin-only checks inside handlers)
	huma.Post(rc.Protected, "/releases", releaseHandler.CreateReleaseHandler)
	huma.Put(rc.Protected, "/releases/{release_id}", releaseHandler.UpdateReleaseHandler)
	huma.Delete(rc.Protected, "/releases/{release_id}", releaseHandler.DeleteReleaseHandler)
	huma.Post(rc.Protected, "/releases/{release_id}/links", releaseHandler.AddExternalLinkHandler)
	huma.Delete(rc.Protected, "/releases/{release_id}/links/{link_id}", releaseHandler.RemoveExternalLinkHandler)
}
