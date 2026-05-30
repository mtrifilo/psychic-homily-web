package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

func setupEntityExistenceRoutes(rc RouteContext) {
	entityExistenceHandler := catalogh.NewEntityExistenceHandler(rc.SC.EntityExistence)

	huma.Head(rc.API, "/entities/{entity_type}/{entity_id}/exists", entityExistenceHandler.EntityExistsHandler)
}
