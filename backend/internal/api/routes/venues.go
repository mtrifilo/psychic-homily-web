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

	// Admin venue endpoints (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/venues", venueHandler.AdminCreateVenueHandler)
	huma.Put(rc.Admin, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)

	// Protected venue endpoint: admin can delete any venue, non-admin can
	// delete venues they submitted. Stays on rc.Protected with handler-side
	// ownership check.
	// conditional admin — see PSY-423 audit
	huma.Delete(rc.Protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
}
