package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupChartsRoutes configures the top charts endpoints.
// All endpoints are public except /charts/me, the authed personal stats strip.
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
	huma.Get(rc.API, "/charts/new-releases", chartsHandler.GetNewReleasesHandler)
	huma.Get(rc.API, "/charts/rank", chartsHandler.GetChartEntityRankHandler)
	huma.Get(rc.API, "/charts/summary", chartsHandler.GetChartsSummaryHandler)
	huma.Get(rc.API, "/charts/freshly-added", chartsHandler.GetFreshlyAddedHandler)
	huma.Get(rc.API, "/charts/scenes", chartsHandler.GetChartScenesHandler)
	huma.Get(rc.API, "/charts/overview", chartsHandler.GetChartsOverviewHandler)

	// Personal stats strip: the user's own aggregates, so it requires auth
	// (anonymous → 401; the frontend simply doesn't render the strip).
	huma.Get(rc.Protected, "/charts/me", chartsHandler.GetPersonalChartsStatsHandler)
}
