package routes

import (
	"net/http"
	"regexp"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/testenv"
)

// PSY-1482: dedicated rate limit for authenticated one-row mutations
// (save/unsave show+release, follow/unfollow entity+scene, venue confirm, and
// PSY-1991's single entity-request file+withdraw), mounted globally in
// cmd/server/main.go. Public-read limiting (PSY-1362/1373) explicitly exempts
// writes, so these mutations had no ceiling on rc.Protected (JWT only).
//
// A single global mount keeps the SHARED per-user budget honest: one limiter
// instance meters every in-scope path, so save and follow cannot be spammed
// independently. It keys per USER (not IP), so shared-IP logged-in users never
// collide.

// EnableEngagementMutationRateLimitsEnvVar is an OPT-IN flag gating BOTH this
// limiter and EntityRequestBatchRateLimiter: each is a pass-through noop unless
// this is set to "1". Opt-in (not a default-on
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

// engagementMutationPathPatterns match the in-scope endpoints on their concrete
// request paths (path params already substituted):
//   - /saved-shows/{show_id}        (save/unsave show)
//   - /saved-releases/{release_id}  (save/unsave release)
//   - /{entity_type}/{entity_id}/follow AND /scenes/{slug}/follow (follow/unfollow)
//   - /venues/{venue_id}/confirm    (PSY-1542 venue confirm-current)
//   - /entity-requests              (PSY-1991 file one creation request)
//   - /entity-requests/{id}/withdraw (PSY-1991 retract one)
//
// Both follow shapes are three-segment paths ending in /follow, so one pattern
// covers them. Read-shaped helpers are deliberately NOT matched: /follows/batch
// (POST body of ids) does not end in /follow, and the save-count batch paths are
// on the public-read allowlist — both stay off the mutation budget per policy.
// /entity-requests/batch is not matched either: it carries up to 200 submissions
// per request and has its own budget (entity_request_batch_rate_limit.go).
//
// Venue confirm joins the SHARED budget rather than getting its own: it is the
// same class of one-tap authenticated toggle, and a separate budget would let a
// farmer spend a full allowance on confirmations while still spending a full
// allowance on follows. Confirmation counts are freshness evidence, so cheap
// mass-confirming is exactly the abuse this ceiling exists to bound.
//
// Filing and withdrawing a single entity request join it for the same reason.
// Both frontend surfaces file through the batch route and no caller in this
// repo posts to the single route, so the loop a contributor can drive today is
// one batch file plus one withdraw, bounded by the tighter of the two budgets.
// Sharing one counter with withdraw is what keeps a future caller of the single
// route from getting a fresh allowance for each half of that loop.
var engagementMutationPathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^/saved-shows/[^/]+$`),
	regexp.MustCompile(`^/saved-releases/[^/]+$`),
	regexp.MustCompile(`^/[^/]+/[^/]+/follow$`),
	regexp.MustCompile(`^/venues/[^/]+/confirm$`),
	regexp.MustCompile(`^/entity-requests/[^/]+/withdraw$`),
}

// engagementMutationExactPaths are the in-scope paths that carry no parameter.
// They are matched by comparison rather than by a regexp that would spell the
// same literal, and are checked first so the common case costs no match at all.
var engagementMutationExactPaths = map[string]bool{
	"/entity-requests": true,
}

// isEngagementMutationRequest reports whether a request is an in-scope mutation
// (POST/DELETE on one of the paths above). GETs (e.g. /saved-shows list,
// /saved-shows/{id}/check, follower counts) are reads and never matched.
func isEngagementMutationRequest(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		return false
	}
	// The ESCAPED path is the string chi routes on. Matching the decoded one
	// would let /entity-requests/a%2Fb/withdraw route while reading as four
	// segments here, so a route the server serves would not be metered.
	path := r.URL.EscapedPath()
	if engagementMutationExactPaths[path] {
		return true
	}
	for _, re := range engagementMutationPathPatterns {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// EngagementMutationRateLimiter returns the chi middleware that throttles
// authenticated one-row mutations against a shared per-user budget
// (middleware.EngagementMutationBurstPerMinute + EngagementMutationSustainedPerHour,
// both must pass). Admin JWTs and validated API tokens bypass. Non-mutation
// requests pass straight through. Returns a pass-through noop unless the opt-in
// flag is set. Mounted once, globally, before route registration.
func EngagementMutationRateLimiter(jwtService *auth.JWTService, validateAPIToken func(string) bool, getenv func(string) string) func(http.Handler) http.Handler {
	if !IsEngagementMutationRateLimitEnabled(getenv) {
		return noopRateLimiter()
	}
	// Admin JWTs and validated API tokens bypass via SkipRateLimitForAdmin (the
	// same helper the tag limiters use); everyone else flows through the shared
	// per-user burst+sustained budget.
	limiter := middleware.SkipRateLimitForAdmin(
		jwtService,
		validateAPIToken,
		middleware.RateLimitMutationsByUser(
			jwtService,
			middleware.RateLimitEngagementMutationBurst(),
			middleware.RateLimitEngagementMutationSustained(),
		),
	)
	return limitWhen(limiter, isEngagementMutationRequest)
}
