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
	// Named new bands (PSY-1781). A data sub-resource of the scene page, not a
	// reader-facing path — no HEAD sibling, unlike the week/day families below,
	// because frontend/proxy.ts never existence-checks it.
	huma.Get(rc.API, "/scenes/{slug}/new-artists", sceneHandler.GetSceneNewArtistsHandler)
	// Two routes with SEPARATE request types — huma treats every declared path
	// param as required, so sharing one type would 422 the bare form before the
	// handler runs. The bare form resolves the scene's CURRENT week server-side
	// (in the scene's own timezone); the keyed form is the stable permalink.
	huma.Get(rc.API, "/scenes/{slug}/week", sceneHandler.GetSceneCurrentWeekHandler)
	huma.Get(rc.API, "/scenes/{slug}/week/{week}", sceneHandler.GetSceneWeekHandler)
	// HEAD on the same handlers. frontend/proxy.ts existence-checks these paths
	// with HEAD before the page streams — without a HEAD route the router
	// answers 405, the proxy fails open, and every bad week key becomes a
	// soft-404 (404 body committed at HTTP 200). Go discards the response body
	// for HEAD, so the handler needs no special casing.
	huma.Head(rc.API, "/scenes/{slug}/week", sceneHandler.GetSceneCurrentWeekHandler)
	huma.Head(rc.API, "/scenes/{slug}/week/{week}", sceneHandler.GetSceneWeekHandler)
	// The day family, same split and same reason as the week family above. The
	// bare form is what the reader-facing /scenes/{slug}/tonight page asks for:
	// "tonight" depends on the SCENE's timezone and on its 6am night boundary
	// (a night is named by the date it began on, so before 06:00 the current
	// night is still the previous date), so only the server can resolve it. A
	// day is NOT derivable from the week payload — tonight can fall in the
	// previous ISO week, which would cost a second request the client cannot
	// know it needs.
	huma.Get(rc.API, "/scenes/{slug}/day", sceneHandler.GetSceneCurrentDayHandler)
	huma.Get(rc.API, "/scenes/{slug}/day/{date}", sceneHandler.GetSceneDayHandler)
	huma.Head(rc.API, "/scenes/{slug}/day", sceneHandler.GetSceneCurrentDayHandler)
	huma.Head(rc.API, "/scenes/{slug}/day/{date}", sceneHandler.GetSceneDayHandler)
}
