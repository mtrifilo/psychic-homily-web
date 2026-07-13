package routes

import (
	"github.com/danielgtaylor/huma/v2"

	engagementh "psychic-homily-backend/internal/api/handlers/engagement"
	"psychic-homily-backend/internal/api/middleware"
)

func setupSavedReleaseRoutes(rc RouteContext) {
	handler := engagementh.NewSavedReleaseHandler(rc.SC.SavedRelease)

	// Register the static batch path before /releases/{release_id}.
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Post(optionalAuthGroup, ReleaseSaveCountsBatchPath, handler.BatchReleaseSaveCountsHandler)
	huma.Get(optionalAuthGroup, "/releases/{release_id}/saves", handler.GetReleaseSaveCountHandler)

	huma.Post(rc.Protected, "/saved-releases/{release_id}", handler.SaveReleaseHandler)
	huma.Delete(rc.Protected, "/saved-releases/{release_id}", handler.UnsaveReleaseHandler)
	huma.Get(rc.Protected, "/saved-releases", handler.GetSavedReleasesHandler)
}
