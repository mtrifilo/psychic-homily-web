package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupChartsRoutes configures public top charts endpoints.
// All endpoints are public — no authentication required.
func setupChartsRoutes(rc RouteContext) {
	chartsHandler := catalogh.NewChartsHandler(rc.SC.Charts)

	huma.Get(rc.API, "/charts/trending-shows", chartsHandler.GetTrendingShowsHandler)
	huma.Get(rc.API, "/charts/popular-artists", chartsHandler.GetPopularArtistsHandler)
	huma.Get(rc.API, "/charts/active-venues", chartsHandler.GetActiveVenuesHandler)
	huma.Get(rc.API, "/charts/hot-releases", chartsHandler.GetHotReleasesHandler)
	huma.Get(rc.API, "/charts/overview", chartsHandler.GetChartsOverviewHandler)
}
