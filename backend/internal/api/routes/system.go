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
	// healthcheck; see health.go for why it must not become dependency-aware.
	// HEAD matters here beyond symmetry — the Dockerfile healthcheck probes
	// with `wget --spider`, which sends HEAD.
	huma.Get(rc.API, "/health", systemh.HealthHandler)
	huma.Head(rc.API, "/health", systemh.HealthHandler)

	// Readiness: 503 when a critical dependency is unreachable. This is the
	// endpoint uptime monitoring watches; nothing restarts on its result. Both
	// are exempt from the public-read rate limiter (see
	// infraPathsExemptFromRateLimit) — that exemption is exact-match, so a
	// rename here needs a matching edit there.
	//
	// Declaring 503 puts the endpoint's defining behavior in the OpenAPI
	// document; without it the generated client types describe only 200 and a
	// generic error, and the status code IS the product here.
	withServiceUnavailable := func(o *huma.Operation) {
		o.Errors = append(o.Errors, http.StatusServiceUnavailable)
	}
	huma.Get(rc.API, "/health/ready", systemh.ReadinessHandler, withServiceUnavailable)

	// HEAD is registered on both because an external uptime monitor is the
	// intended caller and HEAD is a common probe default (it is what `curl -I`
	// and many load balancers send). Huma routes one method per registration, so
	// without this a HEAD probe gets 405 — which matches no sane alert rule and
	// leaves the monitor permanently reporting a status the operator never
	// configured for. Go discards the body for HEAD, so the handler is unchanged.
	huma.Head(rc.API, "/health/ready", systemh.ReadinessHandler, withServiceUnavailable)

	// OpenAPI specification endpoint
	api := rc.API
	rc.Router.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond.SafeEncode(r.Context(), w, api.OpenAPI())
	})
}
