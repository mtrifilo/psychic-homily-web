package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

// savedShowsCalendarDownloadRoute is the session-authenticated one-shot calendar
// file of the caller's upcoming saved shows. chi tries the static segment before
// the {show_id} parameter, so GET reaches this route; it carries no POST or
// DELETE, so those still reach the save and unsave operations with
// show_id "calendar.ics", exactly as before it existed.
const savedShowsCalendarDownloadRoute = "/saved-shows/calendar.ics"

// setupSavedShowRoutes configures saved show endpoints.
//
// A user's saved list is private (the protected endpoints below). A show's save
// COUNT is public: it is an aggregate that never identifies who saved, and it
// doubles as the buzz signal rendered on show rows for logged-out visitors.
func setupSavedShowRoutes(rc RouteContext) {
	savedShowHandler := engagementh.NewSavedShowHandler(rc.SC.SavedShow)

	// Public endpoints with optional auth (counts always available; the caller's
	// own is_saved state included when authenticated).
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}/saves", savedShowHandler.GetSaveCountHandler)
	// SaveCountsBatchPath, not a literal: the read-via-POST rate-limit allowlist
	// keys off the same constant.
	huma.Post(optionalAuthGroup, SaveCountsBatchPath, savedShowHandler.BatchSaveCountsHandler)

	// Protected saved show endpoints (the user's own private list)
	huma.Post(rc.Protected, "/saved-shows/{show_id}", savedShowHandler.SaveShowHandler)
	huma.Delete(rc.Protected, "/saved-shows/{show_id}", savedShowHandler.UnsaveShowHandler)
	huma.Get(rc.Protected, "/saved-shows", savedShowHandler.GetSavedShowsHandler)
	// The web client reads is_saved from the save-count endpoints above; this
	// single-show check is still consumed by the iOS app
	// (ios/PsychicHomily/Networking/APIEndpoints.swift). Do not remove it as
	// "dead code" without updating that client.
	huma.Get(rc.Protected, "/saved-shows/{show_id}/check", savedShowHandler.CheckSavedHandler)

	// A raw chi route because the response is a text/calendar file, not JSON.
	// JWTMiddleware accepts a session only; an API token is refused. The session
	// config lets it clear a session cookie the token checks reject.
	calendarHandler := engagementh.NewCalendarHandler(rc.SC.Calendar, rc.SC.User, rc.Cfg)
	rc.Router.With(middleware.JWTMiddleware(rc.SC.JWT, rc.Cfg.Session)).
		Get(savedShowsCalendarDownloadRoute, calendarHandler.GetSavedShowsCalendarDownloadHandler)
}
