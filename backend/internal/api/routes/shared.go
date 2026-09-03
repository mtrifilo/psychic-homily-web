package routes

import (
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/respond"
	"psychic-homily-backend/internal/services"
	adminsvc "psychic-homily-backend/internal/services/admin"
)

// RouteContext holds the shared dependencies passed to every route setup function.
// Each function uses only what it needs from the struct.
type RouteContext struct {
	Router    *chi.Mux                   // The chi mux (for Chi-level middleware groups and raw HTTP routes)
	API       huma.API                   // The public Huma API wrapper
	Protected *huma.Group                // Protected (auth-required) Huma API group
	Admin     *huma.Group                // Admin-only Huma API group (auth + IsAdmin enforced upstream)
	SC        *services.ServiceContainer // All instantiated services
	Cfg       *config.Config             // Application configuration

	// ValidateAPIToken reports whether a bearer string is a live API token.
	// Built once in SetupRoutes so every rate-limiter bypass in this package
	// asks the same question of the same service.
	ValidateAPIToken func(string) bool
}

// APITokenValidator adapts APITokenService.ValidateToken (hash, DB lookup,
// revoked/expired/inactive and admin-scope checks) to the boolean predicate the
// rate-limiter bypasses take. A nil service yields a nil predicate, which every
// bypass reads as "no usable token" and therefore meters the request.
func APITokenValidator(svc *adminsvc.APITokenService) func(string) bool {
	if svc == nil {
		return nil
	}
	return func(token string) bool {
		_, _, err := svc.ValidateToken(token)
		return err == nil
	}
}

// rateLimitUnlessAPIToken wraps httprate.Limit but skips rate limiting for
// requests that carry a VALIDATED API token, so batch imports by the ph CLI are
// not throttled. Validation runs only for a phk_-prefixed bearer, so ordinary
// browser traffic reaches the limiter without a database round trip.
//
// The phk_ prefix alone does not exempt anything: a request the authenticating
// middleware resolves to a cookie session is metered as that session no matter
// what its Authorization header claims.
func rateLimitUnlessAPIToken(validateAPIToken func(string) bool, requestLimit int, windowLength time.Duration) func(http.Handler) http.Handler {
	limiter := httprate.Limit(
		requestLimit,
		windowLength,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if middleware.ValidatedAPIToken(validateAPIToken, r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
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
	respond.SafeWrite(r.Context(), w, []byte(`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in 60 seconds."}`))
}
