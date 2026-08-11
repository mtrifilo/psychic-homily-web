package routes

import (
	"github.com/danielgtaylor/huma/v2"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
)

// setupGraphRoutes registers the catalog-wide graph surfaces.
//
//   - GET /graph/overview — the nightly precomputed "Map of the Scene".
//   - GET /graph/starting-points — the connectivity-ranked artists the zero
//     state suggests when there is no map to draw.
//
// Public and unauthenticated: the map is the same for every visitor by
// construction (it is a single nightly artifact, not a per-user projection), and
// making it anonymous is what lets it be cached hard at every layer.
//
// The two are SEPARATE RESOURCES on purpose. The suggestion list is what the
// zero state falls back to when the overview is unavailable, so folding it into
// the map's payload would make it missing in exactly the states it exists for.
//
// This is the first CATALOG-WIDE graph route; every other graph endpoint is
// entity-scoped and lives with its entity (scenes, venues, festivals,
// collections). A whole-catalog map belongs to none of them, so it gets its own
// path prefix rather than being hung off an arbitrary entity's tree.
func setupGraphRoutes(rc RouteContext) {
	handler := catalogh.NewGraphOverviewHandler(rc.SC.GraphOverview)

	huma.Get(rc.API, "/graph/overview", handler.GetGraphOverviewHandler)
	huma.Get(rc.API, "/graph/starting-points", handler.GetGraphStartingPointsHandler)
}
