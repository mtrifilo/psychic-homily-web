package routes

import (
	"net/http"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/testenv"
)

// PSY-1362: rate limiting for public, unauthenticated read endpoints (graph-card,
// artist/show/venue/label/scene reads, etc.), mounted globally in
// cmd/server/main.go. The limiter bypasses any authenticated request (see
// middleware.SkipRateLimitForAuthenticated), so only anonymous traffic is
// throttled — which is why it can safely be a global middleware rather than a
// per-route mount.

// DisablePublicReadRateLimitsEnvVar is a PRODUCTION kill-switch: set to "1" to
// replace the public-read limiter with a pass-through noop. Unlike
// DisableAuthRateLimitsEnvVar this is deliberately NOT testenv-gated (no
// ENVIRONMENT allow-list, no boot refusal) — it must be usable in prod/staging
// to roll the limiter back if 429 rates spike after a deploy, and to disable it
// for CI/E2E runs whose workers share one IP. It only ever loosens limits, so
// enabling it anywhere is safe.
const DisablePublicReadRateLimitsEnvVar = "DISABLE_PUBLIC_READ_RATE_LIMITS"

// IsPublicReadRateLimitDisabled reports whether the public-read limiter should
// be a noop. Reuses the "==1" flag convention (testenv.IsFlagEnabled) but NOT
// the environment gate — see the const doc.
func IsPublicReadRateLimitDisabled(getenv func(string) string) bool {
	return testenv.IsFlagEnabled(DisablePublicReadRateLimitsEnvVar, getenv)
}

// infraPathsExemptFromRateLimit are exact request paths a global anonymous
// limiter must NEVER throttle. /health is polled anonymously and often from a
// single IP by load balancers / uptime probes; a 429 there would flap the
// service unhealthy and cause an outage — the opposite of what abuse-protection
// should do.
var infraPathsExemptFromRateLimit = []string{"/health"}

// PublicReadRateLimiter returns the chi middleware that throttles anonymous
// public-read traffic to middleware.APIRequestsPerMinute (100) per IP, bypassing
// any authenticated request AND the infra paths above. Returns a pass-through
// noop when the kill-switch is set. Mounted once, globally, before route
// registration.
func PublicReadRateLimiter(jwtService *auth.JWTService, getenv func(string) string) func(http.Handler) http.Handler {
	if IsPublicReadRateLimitDisabled(getenv) {
		return noopRateLimiter()
	}
	limiter := middleware.SkipRateLimitForAuthenticated(jwtService, middleware.RateLimitAPIEndpoints())
	return skipRateLimitForPaths(limiter, infraPathsExemptFromRateLimit...)
}

// skipRateLimitForPaths wraps a limiter so exact-match paths bypass it entirely.
func skipRateLimitForPaths(limiter func(http.Handler) http.Handler, paths ...string) func(http.Handler) http.Handler {
	exempt := make(map[string]bool, len(paths))
	for _, p := range paths {
		exempt[p] = true
	}
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if exempt[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}
