package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupArtistRoutes(rc RouteContext) {
	artistHandler := catalogh.NewArtistHandler(rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)

	// Public artist endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/artists", artistHandler.ListArtistsHandler)
	huma.Get(rc.API, "/artists/cities", artistHandler.GetArtistCitiesHandler)
	huma.Get(rc.API, "/artists/search", artistHandler.SearchArtistsHandler)
	huma.Get(rc.API, "/artists/{artist_id}", artistHandler.GetArtistHandler)
	huma.Get(rc.API, "/artists/{artist_id}/shows", artistHandler.GetArtistShowsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/labels", artistHandler.GetArtistLabelsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/aliases", artistHandler.GetArtistAliasesHandler)

	// Protected artist endpoints
	huma.Delete(rc.Protected, "/artists/{artist_id}", artistHandler.DeleteArtistHandler)
	huma.Post(rc.Protected, "/admin/artists", artistHandler.AdminCreateArtistHandler)
	huma.Patch(rc.Protected, "/admin/artists/{artist_id}", artistHandler.AdminUpdateArtistHandler)
	huma.Post(rc.Protected, "/admin/artists/{artist_id}/aliases", artistHandler.AddArtistAliasHandler)
	huma.Delete(rc.Protected, "/admin/artists/{artist_id}/aliases/{alias_id}", artistHandler.DeleteArtistAliasHandler)
	huma.Post(rc.Protected, "/admin/artists/merge", artistHandler.MergeArtistsHandler)
}
