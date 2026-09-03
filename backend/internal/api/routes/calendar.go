package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
)

// personalFeedRoutes are the route templates whose {token} segment IS the
// credential: the caller authenticates by holding the URL, with no session and
// no Authorization header. Declared once and read by BOTH the registrations
// below and the public-read limiter's exemption (public_read_rate_limit.go), so
// a feed path spelled one way in the router and another in the exemption cannot
// pass review.
const (
	savedShowsFeedRoute      = "/feeds/{token}/saved-shows.ics" // PSY-1430 iCal
	followsActivityFeedRoute = "/feeds/{token}/follows.atom"    // PSY-1505 Atom activity
	legacyCalendarFeedRoute  = "/calendar/{token}"              // backward-compatible iCal alias
)

// setupCalendarRoutes configures personal feed and token management endpoints.
// Public feed paths are token-authenticated (not JWT); the token management CRUD
// beneath /calendar/token is session-authenticated and ordinary.
func setupCalendarRoutes(rc RouteContext) {
	calendarHandler := engagementh.NewCalendarHandler(rc.SC.Calendar, rc.Cfg)

	rc.Router.Get(savedShowsFeedRoute, calendarHandler.GetCalendarFeedHandler)
	rc.Router.Get(followsActivityFeedRoute, calendarHandler.GetFollowsActivityFeedHandler)
	rc.Router.Get(legacyCalendarFeedRoute, calendarHandler.GetCalendarFeedHandler)

	// Protected Huma routes for token CRUD
	huma.Post(rc.Protected, "/calendar/token", calendarHandler.CreateCalendarTokenHandler)
	huma.Get(rc.Protected, "/calendar/token", calendarHandler.GetCalendarTokenStatusHandler)
	huma.Delete(rc.Protected, "/calendar/token", calendarHandler.DeleteCalendarTokenHandler)
}
