package routes

import (
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// RouteContext holds the shared dependencies passed to every route setup function.
// Each function uses only what it needs from the struct.
//
// Group hierarchy (PSY-423):
//   - API:       no auth — public endpoints
//   - Protected: API + JWT middleware — auth-required, any signed-in user
//   - Admin:     API + JWT + Admin middleware — auth-required AND IsAdmin=true
//
// Register endpoints on the narrowest group that fits. Admin-only endpoints
// belong on Admin so the admin gate is visible at registration time and
// handlers don't carry a redundant shared.RequireAdmin(ctx) call. Endpoints
// where the user can act on their own resource OR an admin can act on any
// (e.g. "delete own show vs delete any show") stay on Protected with
// handler-side conditional logic.
type RouteContext struct {
	Router    *chi.Mux                   // The chi mux (for Chi-level middleware groups and raw HTTP routes)
	API       huma.API                   // The public Huma API wrapper
	Protected *huma.Group                // Protected (auth-required) Huma API group
	Admin     *huma.Group                // Admin-only Huma API group (JWT + IsAdmin enforced by middleware)
	SC        *services.ServiceContainer // All instantiated services
	Cfg       *config.Config             // Application configuration
}

// rateLimitUnlessAPIToken wraps httprate.Limit but skips rate limiting for
// requests authenticated with an API token (phk_ prefix). API tokens are
// admin-only and trusted — they shouldn't be throttled during batch imports.
func rateLimitUnlessAPIToken(requestLimit int, windowLength time.Duration) func(http.Handler) http.Handler {
	limiter := httprate.Limit(
		requestLimit,
		windowLength,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer phk_") {
				// API token — bypass rate limit
				next.ServeHTTP(w, r)
				return
			}
			// Normal request — apply rate limit
			limiter(next).ServeHTTP(w, r)
		})
	}
}

// rateLimitHandler handles rate limit exceeded responses with JSON
func rateLimitHandler(w http.ResponseWriter, r *http.Request) {
	// Log the rate limit hit
	log := logger.FromContext(r.Context())
	if log == nil {
		log = logger.Default()
	}
	log.Warn("rate limit exceeded",
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr,
	)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)
	w.Write([]byte(`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in 60 seconds."}`))
}
