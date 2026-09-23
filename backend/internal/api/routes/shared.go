package routes

import (
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/respond"
	"psychic-homily-backend/internal/services"
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

// rateLimitUnlessValidatedAPIToken is a per-IP limiter with ONE hatch: a
// validated API token, so batch imports by the ph CLI are not throttled. It is
// SkipRateLimitForAdmin with the admin-JWT hatch withheld, which is the nil JWT
// service: an admin session is metered here even though it is exempt on tag
// creation. TestAPITokenBypassThroughRouter pins both halves of that asymmetry.
func rateLimitUnlessValidatedAPIToken(validateAPIToken func(string) bool, name string, requestLimit int, windowLength time.Duration) func(http.Handler) http.Handler {
	return middleware.SkipRateLimitForAdmin(nil, validateAPIToken, ipRateLimiter(name, requestLimit, windowLength))
}

// Values of the `limiter` attribute on the 429 lines of the limiters this
// package builds.
const (
	limiterAuth               = "auth"
	limiterPasskey            = "passkey"
	limiterVerificationResend = "verification_resend"
	limiterPasswordConfirm    = "password_confirm"
	limiterOAuthLinkToken     = "oauth_link_token"
	limiterTagCreate          = "tag_create"
	limiterTagVote            = "tag_vote"
	limiterShowReport         = "show_report"
	limiterEntityReport       = "entity_report"
	limiterShowCreate         = "show_create"
	limiterAIProcess          = "ai_process"
)

// ipRateLimiter is a per-IP (middleware.KeyByClientIP) limiter whose 429s log
// as the limiter named name. Each call returns a limiter with its own counter.
func ipRateLimiter(name string, requestLimit int, window time.Duration) func(http.Handler) http.Handler {
	return httprate.Limit(
		requestLimit,
		window,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler(name, window)),
	)
}

// rateLimitHandler builds the 429 handler for the ipRateLimiter named name. The
// log line is middleware's ratelimit_rejected line with the limiter's real
// window; the response says Retry-After: 60 whatever the window.
func rateLimitHandler(name string, window time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		middleware.LogClientIPRateLimitRejection(r, name, window)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		respond.SafeWrite(r.Context(), w, []byte(`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in 60 seconds."}`))
	}
}
