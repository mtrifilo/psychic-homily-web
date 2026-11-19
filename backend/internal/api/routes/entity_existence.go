package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupEntityExistenceRoutes registers the frontend proxy's existence probes.
// Each is HEAD-only: the status is the whole answer, so there is no body worth
// a GET.
func setupEntityExistenceRoutes(rc RouteContext) {
	entityExistenceHandler := catalogh.NewEntityExistenceHandler(rc.SC.EntityExistence)

	huma.Head(rc.API, "/entities/{entity_type}/{entity_id}/exists", entityExistenceHandler.EntityExistsHandler)

	// Families the generic route above does not answer get paths of their own
	// rather than new {entity_type} values. A frontend probing an API that does
	// not carry a route yet gets the router's text/plain 404, which it reads as
	// no answer; a new {entity_type} value on a running API that predates it
	// would get the API's own 404, which it reads as "missing".
	//
	// `{slug}` is spelled as on every other /radio-stations/{slug} route, so the
	// router reports one parameter name at that position. `radio-shows` rather
	// than `shows` keeps these off the show-visibility inventory's `/shows/{`
	// shape: a radio show is not a show.
	huma.Head(rc.API, "/radio-stations/{slug}/exists", entityExistenceHandler.RadioStationExistsHandler)
	huma.Head(rc.API, "/radio-stations/{slug}/radio-shows/{show_slug}/exists", entityExistenceHandler.RadioShowExistsHandler)
	huma.Head(rc.API, "/radio-stations/{slug}/radio-shows/{show_slug}/episodes/{date}/exists", entityExistenceHandler.RadioEpisodeExistsHandler)
	// On the main API with no auth middleware: the answer is the one a caller
	// without a session gets.
	huma.Head(rc.API, "/collections/{slug}/exists", entityExistenceHandler.CollectionExistsHandler)
}
