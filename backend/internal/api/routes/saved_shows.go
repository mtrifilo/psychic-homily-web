package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
)

// setupSavedShowRoutes configures saved show endpoints (user's personal "My List")
// All endpoints require authentication via protected group
func setupSavedShowRoutes(rc RouteContext) {
	savedShowHandler := engagementh.NewSavedShowHandler(rc.SC.SavedShow)

	// Protected saved show endpoints
	huma.Post(rc.Protected, "/saved-shows/{show_id}", savedShowHandler.SaveShowHandler)
	huma.Delete(rc.Protected, "/saved-shows/{show_id}", savedShowHandler.UnsaveShowHandler)
	huma.Get(rc.Protected, "/saved-shows", savedShowHandler.GetSavedShowsHandler)
	huma.Get(rc.Protected, "/saved-shows/{show_id}/check", savedShowHandler.CheckSavedHandler)
	huma.Post(rc.Protected, "/saved-shows/check-batch", savedShowHandler.CheckBatchSavedHandler)
}
