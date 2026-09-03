package routes

import (
	"net/http"
	"strings"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/testenv"
)

// PSY-1362/1373/1814: rate limiting for public read endpoints (graph-card, artist/
// show/venue/label/scene reads, etc.), mounted globally in cmd/server/main.go.
// Two properties keep a single global mount safe:
//   - it keys by AUTH STATE (middleware.RateLimitPublicReadsByAuthState):
//     anonymous → per-IP (APIRequestsPerMinute); authenticated session JWT →
//     per-USER (PublicReadUserRequestsPerMinute, PSY-1373); a validated phk_
//     API token → exempt (PSY-1814, so ingest search does not share the
//     anonymous per-IP bucket with logged-out visitors). Per-user keying means
//     it never collides shared-IP logged-in users, so it needs no per-route
//     wiring; and
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
// KeyByIP/proxy PREREQUISITE — RESOLVED by PSY-1608, and the concern was real.
// This comment used to warn that r.RemoteAddr might not carry the client IP
// behind a proxy. It does not: measured on production, RemoteAddr is the Railway
// edge node's address, so buckets were per-edge-node and shared by every client
// routed through one. Three nodes each held an independent counter and a single
// client could never exhaust any of them — so the limiter could neither stop an
// abuser nor avoid locking out everyone sharing that node.
//
// All limiters now key on middleware.KeyByClientIP, which reads the address the
// trusted proxy appended to X-Forwarded-For (counting from the RIGHT, so a
// caller cannot spoof it) and falls back to RemoteAddr. Re-verify with a burst
// if the proxy topology changes — RATE_LIMIT_TRUSTED_PROXY_HOPS encodes how
// many proxies front this service, and the limiter logs `ratelimit_proxy_trust`
// once per process reporting what it actually observed.
const EnablePublicReadRateLimitsEnvVar = "ENABLE_PUBLIC_READ_RATE_LIMITS"

// IsPublicReadRateLimitEnabled reports whether the public-read limiter is active.
// Reuses the "==1" flag convention (testenv.IsFlagEnabled) but NOT the
// environment gate — this is a rollout switch, honored in every environment.
func IsPublicReadRateLimitEnabled(getenv func(string) string) bool {
	return testenv.IsFlagEnabled(EnablePublicReadRateLimitsEnvVar, getenv)
}

// infraPathsExemptFromRateLimit are exact request paths a global anonymous
// limiter must NEVER throttle. Both health endpoints are polled anonymously and
// often from a single IP by load balancers / uptime probes; a 429 there would
// flap the service unhealthy and cause an outage — the opposite of what
// abuse-protection should do. /health/ready is the worse of the two to throttle:
// it is the endpoint alerting watches, so a 429 pages a human about a service
// that is fine.
//
// Matching is EXACT, not by prefix — adding a health endpoint requires adding it
// here too, or it silently lands on the anonymous per-IP budget.
var infraPathsExemptFromRateLimit = []string{"/health", "/health/ready"}

// personalFeedRouteTemplates are the token-in-URL feed routes whose holder is
// exempt from the public-read limiter (PSY-1430 iCal + PSY-1505 Atom). Google
// Calendar / Apple Calendar / RSS readers poll from shared cloud IPs; putting
// them on the anonymous per-IP public-read bucket (PSY-1418 / PSY-1362) would
// unfairly 429 feed fetchers that share an egress IP with scrapers. Auth is the
// URL token itself (hashed lookup), not a session JWT, so they never land in
// the authenticated per-user bucket either.
//
// The templates come from the registrations in calendar.go, so this list and
// the router agree by construction. /calendar/token CRUD is NOT one of them: it
// is a three-segment path under /calendar/ whose second segment is "token", and
// no calendar token validates under that name.
var personalFeedRouteTemplates = []string{
	savedShowsFeedRoute,
	followsActivityFeedRoute,
	legacyCalendarFeedRoute,
}

// feedRouteTokenPlaceholder is the segment a personal-feed route template puts
// where the plaintext token goes.
const feedRouteTokenPlaceholder = "{token}"

// personalFeedTokenFromPath returns the plaintext token a request path carries
// in a personal-feed route's {token} segment, or "" when the path is not one of
// those routes. Matching the FULL route shape (not a bare /feeds/ prefix) is
// what keeps an unrelated path under /feeds/ from reaching the token lookup at
// all.
func personalFeedTokenFromPath(path string) string {
	for _, template := range personalFeedRouteTemplates {
		prefix, suffix, found := strings.Cut(template, feedRouteTokenPlaceholder)
		if !found || len(path) <= len(prefix)+len(suffix) {
			continue
		}
		if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
			continue
		}
		token := path[len(prefix) : len(path)-len(suffix)]
		if strings.Contains(token, "/") {
			continue
		}
		return token
	}
	return ""
}

// validatedFeedToken is the one spelling of "this request holds a usable
// personal-feed token": the exemption resolves the question here rather than
// deriving it from the URL shape. The callback is the DB lookup
// (CalendarService.ValidateCalendarToken, adapted by
// middleware.CalendarTokenValidator).
//
// The phcal_ prefix is a cheap pre-filter, not the credential: a path without it
// never reaches the database, and a path with it is exempt only if the lookup
// resolves. A nil callback answers false, so an exemption wired without a
// calendar service meters every feed request rather than exempting them all.
func validatedFeedToken(validate func(string) bool, path string) bool {
	if validate == nil {
		return false
	}
	token := personalFeedTokenFromPath(path)
	if !strings.HasPrefix(token, engagement.CalendarTokenPrefix) {
		return false
	}
	return validate(token)
}

// PublicReadRateLimiter returns the chi middleware that throttles public-READ
// traffic (GET/HEAD): anonymous requests to middleware.APIRequestsPerMinute (100)
// per IP, and authenticated requests to middleware.PublicReadUserRequestsPerMinute
// (300) per USER (PSY-1373 — a finite per-user cap instead of a full bypass, so a
// throwaway signup can't scrape unmetered while shared-IP logged-in users stay
// un-collided), further backstopped by a coarse per-IP ceiling on authenticated
// traffic (middleware.PublicReadAuthenticatedIPCeilingPerMinute, 1000/min, PSY-1378)
// so one IP running many scripted accounts is bounded in aggregate. A request
// whose bearer passes validateAPIToken (APITokenService.ValidateToken — not the
// phk_ prefix alone) is exempt so ingest search does not starve visitors on the
// same IP (PSY-1814). Infra paths are exempt, and so is a personal-feed request
// whose URL token validates (PSY-2017), not the path shape alone. Returns a
// pass-through noop unless the opt-in flag is set. Mounted once, globally,
// before route registration.
func PublicReadRateLimiter(jwtService *auth.JWTService, validateAPIToken, validateFeedToken func(string) bool, getenv func(string) string) func(http.Handler) http.Handler {
	if !IsPublicReadRateLimitEnabled(getenv) {
		return noopRateLimiter()
	}
	limiter := middleware.RateLimitPublicReadsByAuthState(
		jwtService,
		validateAPIToken,
		middleware.RateLimitAPIEndpoints(), // anonymous → per-IP
		middleware.RateLimitPublicReadUserEndpoints(),          // authenticated → per-user
		middleware.RateLimitPublicReadAuthenticatedIPCeiling(), // authenticated → coarse per-IP ceiling (PSY-1378)
	)
	limiter = skipRateLimitForPaths(limiter, infraPathsExemptFromRateLimit...)
	limiter = skipRateLimitForValidatedFeedToken(limiter, validateFeedToken)
	return limitReadMethodsOnly(limiter)
}

// SaveCountsBatchPath is the batch save-count endpoint. It is declared once and
// used by BOTH the route registration and the read-via-POST allowlist below, so
// the two cannot silently drift apart — a rename that reached only one of them
// would quietly leave the endpoint unmetered.
const SaveCountsBatchPath = "/shows/saves/batch"

// ReleaseSaveCountsBatchPath is the saved-release twin of
// SaveCountsBatchPath. Both are read-shaped POSTs because the ID set lives in
// the request body.
const ReleaseSaveCountsBatchPath = "/releases/saves/batch"

// FollowsBatchPath is the batch follow-count endpoint. Like the save-count
// batch paths, it is declared once and used by BOTH the route registration and
// the read-via-POST allowlist below.
const FollowsBatchPath = "/follows/batch"

// readViaPostPaths are POST endpoints that are semantically READS: the request
// body only carries a batch of entity IDs, and the response is public data.
// They must share the public-read budget — otherwise an anonymous caller gets
// an unmetered batch aggregate query simply because the endpoint takes a body.
var readViaPostPaths = []string{SaveCountsBatchPath, ReleaseSaveCountsBatchPath, FollowsBatchPath}

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

// skipRateLimitForValidatedFeedToken wraps a limiter so a personal-feed request
// bypasses it only when the token in its URL resolves to a live calendar token.
// A junk token is metered as whatever the request otherwise is, anonymous
// included, so naming the URL shape buys nothing.
//
// The lookup runs BEFORE the limiter, which means a forged phcal_ path buys one
// indexed read on a request the limiter may be about to reject. That order is
// the same trade RateLimitPublicReadsByAuthState already takes for phk_ tokens,
// and for the same reason: validating after the limiter would make a live feed
// token increment the bucket it is meant to skip.
func skipRateLimitForValidatedFeedToken(limiter func(http.Handler) http.Handler, validateFeedToken func(string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if validatedFeedToken(validateFeedToken, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}
