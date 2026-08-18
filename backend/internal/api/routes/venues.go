package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
)

func setupVenueRoutes(rc RouteContext) {
	venueHandler := catalogh.NewVenueHandler(rc.SC.Venue, rc.SC.Discord, rc.SC.AuditLog, rc.SC.Revision)

	// Public venue endpoints - registered on main API without middleware.
	//
	// The static segments are grouped above `/venues/{venue_id}` for reading
	// order, NOT for precedence: chi walks static children before parameterised
	// ones, so `/venues/listing` beats the parameter regardless of registration
	// order. venue_listing_routing_test.go asserts the resolution rather than
	// the ordering, which is the part that would actually break.
	huma.Get(rc.API, "/venues", venueHandler.ListVenuesHandler)
	huma.Get(rc.API, "/venues/cities", venueHandler.GetVenueCitiesHandler)
	huma.Get(rc.API, "/venues/listing", venueHandler.ListVenueListingHandler)
	huma.Get(rc.API, "/venues/search", venueHandler.SearchVenuesHandler)
	huma.Get(rc.API, "/venues/{venue_id}", venueHandler.GetVenueHandler)
	huma.Get(rc.API, "/venues/{venue_id}/shows", venueHandler.GetVenueShowsHandler)
	// Sibling of the list rather than a field on it: the year picker must offer
	// every year regardless of which one the list is filtered to, and the
	// histogram does not change as the reader pages. See the handler's note.
	huma.Get(rc.API, "/venues/{venue_id}/shows/years", venueHandler.GetVenueShowYearsHandler)
	// The same histogram at month resolution, for the pager's range labels
	// rather than the year picker (PSY-1769). A sibling static segment under the
	// same parameterised parent, so it resolves exactly the way `years` does.
	huma.Get(rc.API, "/venues/{venue_id}/shows/months", venueHandler.GetVenueShowMonthsHandler)
	huma.Get(rc.API, "/venues/{venue_id}/genres", venueHandler.GetVenueGenresHandler)
	huma.Get(rc.API, "/venues/{venue_id}/bill-network", venueHandler.GetVenueBillNetworkHandler)

	// PSY-1584: public iCalendar feed of upcoming shows at this room.
	//
	// Registered on the chi router, not Huma, because the response is a raw
	// text/calendar document — same reason the personal /feeds/… ICS route is a
	// chi handler. It therefore does NOT appear in the OpenAPI spec.
	//
	// The parameter MUST stay named `venue_id` to match its Huma siblings above.
	// chi keys its routing tree on path SHAPE, not on parameter name, so a
	// sibling registered as `{slug}` would not create a second route — it would
	// silently rename the parameter for whichever registration lost, and no
	// handler-level test could see it. venue_calendar_routing_test.go walks the
	// built tree to hold that line.
	//
	// This path is intentionally NOT added to the personal-feed rate-limit
	// exemption in public_read_rate_limit.go: unlike /feeds/{token}/…, it is
	// anonymous and unauthenticated, so it belongs on the ordinary public-read
	// budget rather than being handed an unmetered lane.
	venueCalendarHandler := engagementh.NewVenueCalendarHandler(rc.SC.VenueCalendar, rc.Cfg)
	rc.Router.Get("/venues/{venue_id}/calendar.ics", venueCalendarHandler.GetVenueCalendarFeedHandler)
	rc.Router.Head("/venues/{venue_id}/calendar.ics", venueCalendarHandler.GetVenueCalendarFeedHandler)

	// PSY-1542: community freshness. Any authenticated user at any trust tier
	// can confirm a venue's info is current — it is the cheapest contribution
	// on-ramp, so it is deliberately NOT trust-gated. Idempotent: a repeat
	// confirm no-ops and returns the same aggregate. Throttled by the shared
	// engagement-mutation limiter (engagement_mutation_rate_limit.go).
	venueConfirmHandler := catalogh.NewVenueConfirmHandler(rc.SC.Venue)
	huma.Post(rc.Protected, "/venues/{venue_id}/confirm", venueConfirmHandler.ConfirmVenueHandler)

	// Admin venue endpoints (PSY-423: rc.Admin enforces auth + IsAdmin)
	huma.Post(rc.Admin, "/admin/venues", venueHandler.AdminCreateVenueHandler)
	huma.Put(rc.Admin, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)

	// Protected venue endpoint: admin can delete any venue, non-admin can
	// delete venues they submitted. Stays on rc.Protected with handler-side
	// ownership check.
	// conditional admin — see PSY-423 audit
	huma.Delete(rc.Protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
}
