package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
)

// setupCalendarRoutes configures personal feed and token management endpoints.
// Public feed paths are token-authenticated (not JWT). Canonical paths:
//   - /feeds/{token}/saved-shows.ics  (PSY-1430 iCal)
//   - /feeds/{token}/follows.atom     (PSY-1505 Atom activity)
//
// /calendar/{token} is a backward-compatible iCal alias.
// Both /feeds/… shapes inherit the PSY-1418 personal-feed rate-limit exemption
// via the /feeds/ prefix (see public_read_rate_limit.go).
func setupCalendarRoutes(rc RouteContext) {
	calendarHandler := engagementh.NewCalendarHandler(rc.SC.Calendar, rc.Cfg)

	rc.Router.Get("/feeds/{token}/saved-shows.ics", calendarHandler.GetCalendarFeedHandler)
	rc.Router.Get("/feeds/{token}/follows.atom", calendarHandler.GetFollowsActivityFeedHandler)
	rc.Router.Get("/calendar/{token}", calendarHandler.GetCalendarFeedHandler)

	// Protected Huma routes for token CRUD
	huma.Post(rc.Protected, "/calendar/token", calendarHandler.CreateCalendarTokenHandler)
	huma.Get(rc.Protected, "/calendar/token", calendarHandler.GetCalendarTokenStatusHandler)
	huma.Delete(rc.Protected, "/calendar/token", calendarHandler.DeleteCalendarTokenHandler)
}
