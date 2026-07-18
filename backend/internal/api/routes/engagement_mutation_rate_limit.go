package routes

import (
	"net/http"
	"regexp"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/testenv"
)

// PSY-1482: dedicated rate limit for authenticated engagement-toggle mutations
// (save/unsave show+release, follow/unfollow entity+scene), mounted globally in
// cmd/server/main.go. Public-read limiting (PSY-1362/1373) explicitly exempts
// writes, so these mutations had no ceiling on rc.Protected (JWT only).
//
// A single global mount keeps the SHARED per-user budget honest: one limiter
// instance meters every in-scope path, so save and follow cannot be spammed
// independently. It keys per USER (not IP), so shared-IP logged-in users never
// collide.

// EnableEngagementMutationRateLimitsEnvVar is an OPT-IN flag: the limiter is a
// pass-through noop unless this is set to "1". Opt-in (not a default-on
// kill-switch) is what makes the policy's "observe 429 on stage before prod"
// rollout real: ship inert everywhere, enable on stage, watch 429 rates, then
// enable in prod. It also keeps CI/E2E runs unthrottled without any harness
// change. Matches ENABLE_PUBLIC_READ_RATE_LIMITS.
const EnableEngagementMutationRateLimitsEnvVar = "ENABLE_ENGAGEMENT_MUTATION_RATE_LIMITS"

// IsEngagementMutationRateLimitEnabled reports whether the engagement-mutation
// limiter is active. Reuses the "==1" flag convention (testenv.IsFlagEnabled)
// but not the environment gate — this is a rollout switch, honored in every
// environment.
func IsEngagementMutationRateLimitEnabled(getenv func(string) string) bool {
	return testenv.IsFlagEnabled(EnableEngagementMutationRateLimitsEnvVar, getenv)
}

// engagementMutationPathPatterns match the in-scope engagement-toggle endpoints
// on their concrete request paths (path params already substituted):
//   - /saved-shows/{show_id}        (save/unsave show)
//   - /saved-releases/{release_id}  (save/unsave release)
//   - /{entity_type}/{entity_id}/follow AND /scenes/{slug}/follow (follow/unfollow)
//
// Both follow shapes are three-segment paths ending in /follow, so one pattern
// covers them. Read-shaped helpers are deliberately NOT matched: /follows/batch
// (POST body of ids) does not end in /follow, and the save-count batch paths are
// on the public-read allowlist — both stay off the mutation budget per policy.
var engagementMutationPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/saved-shows/[^/]+$`),
	regexp.MustCompile(`^/saved-releases/[^/]+$`),
	regexp.MustCompile(`^/[^/]+/[^/]+/follow$`),
}

// isEngagementMutationRequest reports whether a request is an in-scope
// engagement toggle mutation (POST/DELETE on one of the paths above). GETs
// (e.g. /saved-shows list, /saved-shows/{id}/check, follower counts) are reads
// and never matched.
func isEngagementMutationRequest(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		return false
	}
	for _, re := range engagementMutationPathPatterns {
		if re.MatchString(r.URL.Path) {
			return true
		}
	}
	return false
}

// EngagementMutationRateLimiter returns the chi middleware that throttles
// authenticated engagement-toggle mutations against a shared per-user budget
// (middleware.EngagementMutationBurstPerMinute + EngagementMutationSustainedPerHour,
// both must pass). Admin JWTs and trusted phk_ tokens bypass. Non-mutation
// requests pass straight through. Returns a pass-through noop unless the opt-in
// flag is set. Mounted once, globally, before route registration.
func EngagementMutationRateLimiter(jwtService *auth.JWTService, getenv func(string) string) func(http.Handler) http.Handler {
	if !IsEngagementMutationRateLimitEnabled(getenv) {
		return noopRateLimiter()
	}
	limiter := middleware.RateLimitEngagementMutationsByUser(
		jwtService,
		middleware.RateLimitEngagementMutationBurst(),
		middleware.RateLimitEngagementMutationSustained(),
	)
	return limitEngagementMutationsOnly(limiter)
}

// limitEngagementMutationsOnly applies the limiter only to in-scope engagement
// mutations; every other request (reads, unrelated writes) passes through so a
// shared engagement budget never 429s an unrelated endpoint.
func limitEngagementMutationsOnly(limiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isEngagementMutationRequest(r) {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
