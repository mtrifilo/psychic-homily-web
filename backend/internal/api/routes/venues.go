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

	// Admin-only venue endpoints (PSY-423: route-gated by HumaAdminMiddleware).
	// Non-admins must use the suggest-edit / pending-edit flow.
	huma.Post(rc.Admin, "/admin/venues", venueHandler.AdminCreateVenueHandler)
	huma.Put(rc.Admin, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)

	// Conditional-admin: any authenticated user can delete venues they
	// submitted; admin can delete any. Stays on Protected with handler-side
	// ownership-or-admin check.
	huma.Delete(rc.Protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
}
