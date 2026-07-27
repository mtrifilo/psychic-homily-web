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
	huma.Get(rc.API, "/scenes/{slug}/shows", sceneHandler.GetSceneShowsHandler)
	huma.Get(rc.API, "/scenes/{slug}/genres", sceneHandler.GetSceneGenresHandler)
	huma.Get(rc.API, "/scenes/{slug}/graph", sceneHandler.GetSceneGraphHandler)
	// Two routes with SEPARATE request types — huma treats every declared path
	// param as required, so sharing one type would 422 the bare form before the
	// handler runs. The bare form resolves the scene's CURRENT week server-side
	// (in the scene's own timezone); the keyed form is the stable permalink.
	huma.Get(rc.API, "/scenes/{slug}/week", sceneHandler.GetSceneCurrentWeekHandler)
	huma.Get(rc.API, "/scenes/{slug}/week/{week}", sceneHandler.GetSceneWeekHandler)
}
