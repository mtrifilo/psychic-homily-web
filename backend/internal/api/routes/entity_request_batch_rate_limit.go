package routes

import (
	"net/http"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
)

// PSY-1991: dedicated rate limit for POST /entity-requests/batch, mounted
// globally in cmd/server/main.go beside the shared engagement-mutation limiter.
//
// The batch route has its OWN per-user budget rather than joining the shared
// one. One batch request carries up to the endpoint's item cap (200) while every
// mutation on the shared budget carries exactly one row, so the two cannot be
// priced the same; and metering the paste flow out of the shared budget would
// let a contributor's paste session 429 their own follows and saves.
//
// It is metered per REQUEST, not per item. A per-item price would refuse a
// paste's later chunks mid-flight, which is the failure the batch route exists
// to remove. What bounds rows is the endpoint's per-request item cap and its
// per-item dedup, not this limiter.
//
// Flag posture is the engagement limiter's: this is inert unless
// ENABLE_ENGAGEMENT_MUTATION_RATE_LIMITS is set, so both entity-request budgets
// arrive in an environment together.

// EntityRequestBatchPath is the batch entity-request endpoint, declared once and
// read by both this limiter and the route registration, so a rename cannot leave
// the endpoint unmetered.
const EntityRequestBatchPath = "/entity-requests/batch"

// isEntityRequestBatchRequest reports whether a request is a batch file (POST on
// the batch path). Nothing else on the path shape is a write.
func isEntityRequestBatchRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == EntityRequestBatchPath
}

// EntityRequestBatchRateLimiter returns the chi middleware that throttles batch
// entity-request filing against a per-user budget
// (middleware.EntityRequestBatchBurstPerMinute + EntityRequestBatchSustainedPerHour,
// both must pass). Admin JWTs and validated API tokens bypass. Every other
// request passes straight through. Returns a pass-through noop unless
// EnableEngagementMutationRateLimitsEnvVar is set. Mounted once, globally,
// before route registration.
func EntityRequestBatchRateLimiter(jwtService *auth.JWTService, validateAPIToken func(string) bool, getenv func(string) string) func(http.Handler) http.Handler {
	if !IsEngagementMutationRateLimitEnabled(getenv) {
		return noopRateLimiter()
	}
	limiter := middleware.SkipRateLimitForAdmin(
		jwtService,
		validateAPIToken,
		middleware.RateLimitMutationsByUser(
			jwtService,
			middleware.RateLimitEntityRequestBatchBurst(),
			middleware.RateLimitEntityRequestBatchSustained(),
		),
	)
	return limitWhen(limiter, isEntityRequestBatchRequest)
}
