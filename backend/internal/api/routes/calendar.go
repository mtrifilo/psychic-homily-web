package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
)

// setupCalendarRoutes configures calendar feed and token management endpoints
func setupCalendarRoutes(rc RouteContext) {
	calendarHandler := engagementh.NewCalendarHandler(rc.SC.Calendar, rc.Cfg)

	// Public Chi route for ICS feed (token-authenticated, not JWT)
	rc.Router.Get("/calendar/{token}", calendarHandler.GetCalendarFeedHandler)

	// Protected Huma routes for token CRUD
	huma.Post(rc.Protected, "/calendar/token", calendarHandler.CreateCalendarTokenHandler)
	huma.Get(rc.Protected, "/calendar/token", calendarHandler.GetCalendarTokenStatusHandler)
	huma.Delete(rc.Protected, "/calendar/token", calendarHandler.DeleteCalendarTokenHandler)
}
