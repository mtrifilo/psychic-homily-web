package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupVenueRoutes(rc RouteContext) {
	venueHandler := catalogh.NewVenueHandler(rc.SC.Venue, rc.SC.Discord, rc.SC.AuditLog, rc.SC.Revision)

	// Public venue endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/venues", venueHandler.ListVenuesHandler)
	huma.Get(rc.API, "/venues/cities", venueHandler.GetVenueCitiesHandler)
	huma.Get(rc.API, "/venues/search", venueHandler.SearchVenuesHandler)
	huma.Get(rc.API, "/venues/{venue_id}", venueHandler.GetVenueHandler)
	huma.Get(rc.API, "/venues/{venue_id}/shows", venueHandler.GetVenueShowsHandler)
	huma.Get(rc.API, "/venues/{venue_id}/genres", venueHandler.GetVenueGenresHandler)
	huma.Get(rc.API, "/venues/{venue_id}/bill-network", venueHandler.GetVenueBillNetworkHandler)

	// Protected venue endpoints - require authentication
	huma.Post(rc.Protected, "/admin/venues", venueHandler.AdminCreateVenueHandler)
	huma.Put(rc.Protected, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)
	huma.Delete(rc.Protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
}
