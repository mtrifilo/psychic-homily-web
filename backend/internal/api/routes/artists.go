package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupArtistRoutes(rc RouteContext) {
	artistHandler := catalogh.NewArtistHandler(rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision, rc.Cfg)

	// Public artist endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/artists", artistHandler.ListArtistsHandler)
	huma.Get(rc.API, "/artists/cities", artistHandler.GetArtistCitiesHandler)
	huma.Get(rc.API, "/artists/listing", artistHandler.ListArtistListingHandler)
	huma.Get(rc.API, "/artists/search", artistHandler.SearchArtistsHandler)
	huma.Get(rc.API, "/artists/{artist_id}", artistHandler.GetArtistHandler)
	huma.Get(rc.API, "/artists/{artist_id}/shows", artistHandler.GetArtistShowsHandler)
	// Sibling of the list rather than a field on it: the year picker must offer
	// every year regardless of which one the list is filtered to, and the
	// histogram does not change as the reader pages. See the handler's note.
	huma.Get(rc.API, "/artists/{artist_id}/shows/years", artistHandler.GetArtistShowYearsHandler)
	// The same histogram at month resolution, for the pager's range labels rather
	// than the year picker (PSY-1842, the artist half of PSY-1769). A sibling
	// static segment under the same parameterised parent, so it resolves exactly
	// the way `years` does — chi walks static children before parameterised ones.
	huma.Get(rc.API, "/artists/{artist_id}/shows/months", artistHandler.GetArtistShowMonthsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/labels", artistHandler.GetArtistLabelsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/aliases", artistHandler.GetArtistAliasesHandler)

	// Protected artist endpoints (any authenticated user)
	huma.Delete(rc.Protected, "/artists/{artist_id}", artistHandler.DeleteArtistHandler)

	// Admin artist endpoints (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/artists", artistHandler.AdminCreateArtistHandler)
	huma.Patch(rc.Admin, "/admin/artists/{artist_id}", artistHandler.AdminUpdateArtistHandler)
	huma.Post(rc.Admin, "/admin/artists/{artist_id}/aliases", artistHandler.AddArtistAliasHandler)
	huma.Delete(rc.Admin, "/admin/artists/{artist_id}/aliases/{alias_id}", artistHandler.DeleteArtistAliasHandler)
	huma.Post(rc.Admin, "/admin/artists/merge", artistHandler.MergeArtistsHandler)
}
