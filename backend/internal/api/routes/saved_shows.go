package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

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
}
