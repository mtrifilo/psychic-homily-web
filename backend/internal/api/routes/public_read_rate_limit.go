package routes

import (
	"net/http"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/testenv"
)

// PSY-1362/1373: rate limiting for public read endpoints (graph-card, artist/
// show/venue/label/scene reads, etc.), mounted globally in cmd/server/main.go.
// Two properties keep a single global mount safe:
//   - it keys by AUTH STATE (middleware.RateLimitPublicReadsByAuthState):
//     anonymous → per-IP (APIRequestsPerMinute); authenticated → per-USER
//     (PublicReadUserRequestsPerMinute, PSY-1373). Per-user keying means it never
//     collides shared-IP logged-in users, so it needs no per-route wiring; and
//   - it only limits READ methods (GET/HEAD) — writes keep their own dedicated
//     limiters (auth, tag, report, show-create), so a shared read budget can't
//     429 an unrelated anonymous write (e.g. a login after heavy browsing).

// EnablePublicReadRateLimitsEnvVar is an OPT-IN flag: the limiter is a
// pass-through noop unless this is set to "1". Opt-in (not a default-on
// kill-switch) is what makes the ticket's "observe 429 on stage before prod"
// rollout real: ship inert everywhere, enable on stage, watch 429 rates, then
// enable in prod. It also keeps CI/E2E runs (whose workers share one IP)
// unthrottled without any harness change, and de-risks the KeyByIP/proxy
// assumption below — prod is only ever enabled after stage confirms real client
// IPs reach the limiter. Matches the codebase's ENABLE_* rollout convention
// (e.g. the artist-enrichment sweeps).
//
// KeyByIP/proxy PREREQUISITE: the underlying limiter keys on r.RemoteAddr via
// httprate.KeyByIP (like every existing limiter here). If the backend sits behind
// a proxy/LB that does not preserve the client IP in RemoteAddr, all anonymous
// traffic collapses onto one bucket. Verify real client IPs reach the limiter on
// stage (429 rates behave per-abuser, not site-wide) before enabling in prod; if
// not, front it with a trusted-proxy RealIP step first.
const EnablePublicReadRateLimitsEnvVar = "ENABLE_PUBLIC_READ_RATE_LIMITS"

// IsPublicReadRateLimitEnabled reports whether the public-read limiter is active.
// Reuses the "==1" flag convention (testenv.IsFlagEnabled) but NOT the
// environment gate — this is a rollout switch, honored in every environment.
func IsPublicReadRateLimitEnabled(getenv func(string) string) bool {
	return testenv.IsFlagEnabled(EnablePublicReadRateLimitsEnvVar, getenv)
}

// infraPathsExemptFromRateLimit are exact request paths a global anonymous
// limiter must NEVER throttle. /health is polled anonymously and often from a
// single IP by load balancers / uptime probes; a 429 there would flap the
// service unhealthy and cause an outage — the opposite of what abuse-protection
// should do.
var infraPathsExemptFromRateLimit = []string{"/health"}

// PublicReadRateLimiter returns the chi middleware that throttles public-READ
// traffic (GET/HEAD): anonymous requests to middleware.APIRequestsPerMinute (100)
// per IP, and authenticated requests to middleware.PublicReadUserRequestsPerMinute
// (300) per USER (PSY-1373 — a finite per-user cap instead of a full bypass, so a
// throwaway signup can't scrape unmetered while shared-IP logged-in users stay
// un-collided), further backstopped by a coarse per-IP ceiling on authenticated
// traffic (middleware.PublicReadAuthenticatedIPCeilingPerMinute, 1000/min, PSY-1378)
// so one IP running many scripted accounts is bounded in aggregate. Infra paths
// above are exempt. Returns a pass-through noop unless the opt-in flag is set.
// Mounted once, globally, before route registration.
func PublicReadRateLimiter(jwtService *auth.JWTService, getenv func(string) string) func(http.Handler) http.Handler {
	if !IsPublicReadRateLimitEnabled(getenv) {
		return noopRateLimiter()
	}
	limiter := middleware.RateLimitPublicReadsByAuthState(
		jwtService,
		middleware.RateLimitAPIEndpoints(),                       // anonymous → per-IP
		middleware.RateLimitPublicReadUserEndpoints(),            // authenticated → per-user
		middleware.RateLimitPublicReadAuthenticatedIPCeiling(),   // authenticated → coarse per-IP ceiling (PSY-1378)
	)
	limiter = skipRateLimitForPaths(limiter, infraPathsExemptFromRateLimit...)
	return limitReadMethodsOnly(limiter)
}

// readViaPostPaths are POST endpoints that are semantically READS: the request
// body only carries a batch of entity IDs, and the response is public data.
// They must share the public-read budget — otherwise an anonymous caller gets
// an unmetered batch aggregate query simply because the endpoint takes a body.
var readViaPostPaths = []string{"/shows/saves/batch"}

// limitReadMethodsOnly applies the limiter to safe read methods (GET/HEAD) plus
// the read-via-POST batch endpoints above. Genuine writes (POST/PUT/PATCH/
// DELETE) pass through — they keep their own dedicated limiters, so a shared
// read budget must not throttle an unrelated anonymous write. (OPTIONS never
// reaches here: the CORS middleware short-circuits preflight upstream.)
func limitReadMethodsOnly(limiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	readPosts := make(map[string]bool, len(readViaPostPaths))
	for _, p := range readViaPostPaths {
		readPosts[p] = true
	}
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isRead := r.Method == http.MethodGet || r.Method == http.MethodHead
			isReadViaPost := r.Method == http.MethodPost && readPosts[r.URL.Path]
			if isRead || isReadViaPost {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
