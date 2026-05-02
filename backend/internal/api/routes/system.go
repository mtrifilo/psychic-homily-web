package routes

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	systemh "psychic-homily-backend/internal/api/handlers/system"
)

// setupSystemRoutes configures system/infrastructure endpoints
func setupSystemRoutes(rc RouteContext) {
	// Health check endpoint
	huma.Get(rc.API, "/health", systemh.HealthHandler)

	// OpenAPI specification endpoint
	api := rc.API
	rc.Router.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.OpenAPI())
	})
}
