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
	huma.Get(rc.API, "/charts/most-anticipated", chartsHandler.GetMostAnticipatedShowsHandler)
	huma.Get(rc.API, "/charts/most-active-artists", chartsHandler.GetMostActiveArtistsHandler)
	huma.Get(rc.API, "/charts/busiest-venues", chartsHandler.GetBusiestVenuesHandler)
	huma.Get(rc.API, "/charts/openers-to-watch", chartsHandler.GetOpenersToWatchHandler)
	huma.Get(rc.API, "/charts/on-the-radio", chartsHandler.GetOnTheRadioArtistsHandler)
	huma.Get(rc.API, "/charts/popular-artists", chartsHandler.GetPopularArtistsHandler)
	huma.Get(rc.API, "/charts/active-venues", chartsHandler.GetActiveVenuesHandler)
	huma.Get(rc.API, "/charts/hot-releases", chartsHandler.GetHotReleasesHandler)
	huma.Get(rc.API, "/charts/overview", chartsHandler.GetChartsOverviewHandler)
}
