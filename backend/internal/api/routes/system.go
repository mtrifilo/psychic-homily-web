package routes

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	systemh "psychic-homily-backend/internal/api/handlers/system"
	"psychic-homily-backend/internal/respond"
)

// setupSystemRoutes configures system/infrastructure endpoints
func setupSystemRoutes(rc RouteContext) {
	// Liveness: always 200 while the process serves. This is Railway's deploy
	// healthcheck, so its failure restarts the service.
	huma.Get(rc.API, "/health", systemh.HealthHandler)

	// Readiness: 503 when a critical dependency is unreachable. This is the
	// endpoint uptime monitoring watches; nothing restarts on its result. Both
	// are exempt from the public-read rate limiter (see
	// infraPathsExemptFromRateLimit) — that exemption is exact-match, so a
	// rename here needs a matching edit there.
	huma.Get(rc.API, "/health/ready", systemh.ReadinessHandler)

	// OpenAPI specification endpoint
	api := rc.API
	rc.Router.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond.SafeEncode(r.Context(), w, api.OpenAPI())
	})
}
