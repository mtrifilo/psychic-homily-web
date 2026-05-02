package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupSceneRoutes configures scene (city aggregation) endpoints.
// All endpoints are public — no authentication required.
func setupSceneRoutes(rc RouteContext) {
	sceneHandler := catalogh.NewSceneHandler(rc.SC.Scene)

	huma.Get(rc.API, "/scenes", sceneHandler.ListScenesHandler)
	huma.Get(rc.API, "/scenes/{slug}", sceneHandler.GetSceneDetailHandler)
	huma.Get(rc.API, "/scenes/{slug}/artists", sceneHandler.GetSceneActiveArtistsHandler)
	huma.Get(rc.API, "/scenes/{slug}/genres", sceneHandler.GetSceneGenresHandler)
	huma.Get(rc.API, "/scenes/{slug}/graph", sceneHandler.GetSceneGraphHandler)
}
