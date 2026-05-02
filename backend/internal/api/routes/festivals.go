package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupFestivalRoutes(rc RouteContext) {
	festivalHandler := catalogh.NewFestivalHandler(rc.SC.Festival, rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)

	// Public festival endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/festivals", festivalHandler.ListFestivalsHandler)
	huma.Get(rc.API, "/festivals/search", festivalHandler.SearchFestivalsHandler)
	huma.Get(rc.API, "/festivals/{festival_id}", festivalHandler.GetFestivalHandler)
	huma.Get(rc.API, "/festivals/{festival_id}/artists", festivalHandler.GetFestivalArtistsHandler)
	huma.Get(rc.API, "/festivals/{festival_id}/venues", festivalHandler.GetFestivalVenuesHandler)
	huma.Get(rc.API, "/artists/{artist_id}/festivals", festivalHandler.GetArtistFestivalsHandler)

	// Festival intelligence endpoints (public, computed from existing data)
	intelHandler := catalogh.NewFestivalIntelligenceHandler(rc.SC.FestivalIntelligence, rc.SC.Festival, rc.SC.Artist)
	huma.Get(rc.API, "/festivals/{festival_id}/similar", intelHandler.GetSimilarFestivalsHandler)
	huma.Get(rc.API, "/festivals/{festival_a_id}/overlap/{festival_b_id}", intelHandler.GetFestivalOverlapHandler)
	huma.Get(rc.API, "/festivals/{festival_id}/breakouts", intelHandler.GetFestivalBreakoutsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/festival-trajectory", intelHandler.GetArtistFestivalTrajectoryHandler)
	huma.Get(rc.API, "/festivals/series/{series_slug}/compare", intelHandler.GetSeriesComparisonHandler)

	// Admin-only festival endpoints (PSY-423: route-gated by HumaAdminMiddleware).
	huma.Post(rc.Admin, "/festivals", festivalHandler.CreateFestivalHandler)
	huma.Put(rc.Admin, "/festivals/{festival_id}", festivalHandler.UpdateFestivalHandler)
	huma.Delete(rc.Admin, "/festivals/{festival_id}", festivalHandler.DeleteFestivalHandler)
	huma.Post(rc.Admin, "/festivals/{festival_id}/artists", festivalHandler.AddFestivalArtistHandler)
	huma.Put(rc.Admin, "/festivals/{festival_id}/artists/{artist_id}", festivalHandler.UpdateFestivalArtistHandler)
	huma.Delete(rc.Admin, "/festivals/{festival_id}/artists/{artist_id}", festivalHandler.RemoveFestivalArtistHandler)
	huma.Post(rc.Admin, "/festivals/{festival_id}/venues", festivalHandler.AddFestivalVenueHandler)
	huma.Delete(rc.Admin, "/festivals/{festival_id}/venues/{venue_id}", festivalHandler.RemoveFestivalVenueHandler)
}
