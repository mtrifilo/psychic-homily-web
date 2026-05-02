package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
)

// setupFavoriteVenueRoutes configures favorite venue endpoints
// All endpoints require authentication via protected group
func setupFavoriteVenueRoutes(rc RouteContext) {
	favoriteVenueHandler := engagementh.NewFavoriteVenueHandler(rc.SC.FavoriteVenue)

	// Protected favorite venue endpoints
	huma.Post(rc.Protected, "/favorite-venues/{venue_id}", favoriteVenueHandler.FavoriteVenueHandler)
	huma.Delete(rc.Protected, "/favorite-venues/{venue_id}", favoriteVenueHandler.UnfavoriteVenueHandler)
	huma.Get(rc.Protected, "/favorite-venues", favoriteVenueHandler.GetFavoriteVenuesHandler)
	huma.Get(rc.Protected, "/favorite-venues/{venue_id}/check", favoriteVenueHandler.CheckFavoritedHandler)
	huma.Get(rc.Protected, "/favorite-venues/shows", favoriteVenueHandler.GetFavoriteVenueShowsHandler)
}
