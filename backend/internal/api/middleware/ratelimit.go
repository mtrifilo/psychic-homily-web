package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/respond"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Rate limit configurations for different endpoint types
const (
	// AuthRequestsPerMinute is the rate limit for auth endpoints (login, register, magic-link)
	// Strict limit to prevent brute force and credential stuffing
	AuthRequestsPerMinute = 10

	// PasskeyRequestsPerMinute is the rate limit for passkey/WebAuthn endpoints
	// Slightly higher due to multi-step nature of WebAuthn flows
	PasskeyRequestsPerMinute = 20

	// APIRequestsPerMinute is the rate limit for general API endpoints
	// Provides basic protection against abuse
	APIRequestsPerMinute = 100

	// ShowCreateRequestsPerHour is the rate limit for show creation
	// Prevents flooding the admin approval queue
	ShowCreateRequestsPerHour = 10

	// AIProcessRequestsPerMinute is the rate limit for AI show processing
	// Calls external Anthropic API — expensive operation
	AIProcessRequestsPerMinute = 5

	// ReportRequestsPerMinute is the rate limit for show report submissions
	// Prevents spamming admins with reports
	ReportRequestsPerMinute = 5

	// TagCreateRequestsPerHour is the rate limit for tag creation (adding tags to entities).
	// Prevents spamming entities with tags.
	TagCreateRequestsPerHour = 20

	// TagVoteRequestsPerMinute is the rate limit for tag voting.
	// Prevents rapid vote manipulation.
	TagVoteRequestsPerMinute = 30

	// PublicReadUserRequestsPerMinute is the per-USER rate limit for authenticated
	// public reads (PSY-1373). Higher than the anonymous per-IP limit
	// (APIRequestsPerMinute) — a logged-in user power-browsing the graph shouldn't
	// hit it — but finite, so one throwaway signup can't scrape unmetered. Keyed
	// by user id, so shared-IP logged-in users each get their own bucket.
	PublicReadUserRequestsPerMinute = 300

	// PublicReadAuthenticatedIPCeilingPerMinute is a COARSE per-IP backstop applied
	// to authenticated public reads IN ADDITION to the per-user cap above (PSY-1378).
	// The per-user cap bounds a SINGLE account; it does nothing against one origin
	// running many scripted accounts (N accounts = N × 300/min from one IP, since
	// account creation is only loosely throttled at 10/min). This ceiling bounds
	// that aggregate: an authenticated request must pass BOTH the per-user cap and
	// this per-IP ceiling (stricter wins).
	//
	// Deliberately HIGH relative to the per-user cap (≈3× 300/min) so it is a
	// scraper backstop, not a shared-IP throttle: a real human almost never sustains
	// even 300/min, so a legitimate small office of logged-in users stays well under
	// 1000/min aggregate (PSY-1362/1373's shared-IP requirement). Bucketed
	// separately from the anonymous per-IP limiter (APIRequestsPerMinute), so
	// authenticated and anonymous traffic from one IP never share a counter. Tune
	// upward if legitimate campus/office NAT is observed hitting it on stage.
	PublicReadAuthenticatedIPCeilingPerMinute = 1000
)

// RateLimitAuthEndpoints creates a strict rate limiter for authentication endpoints
// 10 requests per minute per IP - helps prevent:
// - Brute force attacks
// - Credential stuffing
// - Email bombing via magic links
// - Spam account creation
func RateLimitAuthEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		AuthRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(KeyByClientIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitPasskeyEndpoints creates a rate limiter for passkey/WebAuthn endpoints
// 20 requests per minute per IP - slightly more lenient for multi-step flows
func RateLimitPasskeyEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		PasskeyRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(KeyByClientIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitAPIEndpoints creates a general rate limiter for API endpoints
// 100 requests per minute per IP - basic abuse protection
func RateLimitAPIEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		APIRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(KeyByClientIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitTagCreateEndpoints creates a rate limiter for tag creation endpoints
// 20 requests per hour per IP - prevents tag spam on entities
func RateLimitTagCreateEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		TagCreateRequestsPerHour,
		time.Hour,
		httprate.WithKeyFuncs(KeyByClientIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitTagVoteEndpoints creates a rate limiter for tag voting endpoints
// 30 requests per minute per IP - prevents rapid vote manipulation
func RateLimitTagVoteEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		TagVoteRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(KeyByClientIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// SkipRateLimitForAdmin wraps a rate-limit middleware so authenticated admins
// bypass the per-IP limiter. Non-admin requests — including unauthenticated
// traffic — still hit the underlying limiter.
//
// PSY-345: admins doing bulk contributor work (e.g. tagging sessions) hit the
// 20/hour tag-create and 30/minute tag-vote limits against their own IP. This
// lets us keep the abuse-prevention limits tight for anonymous/IP-level
// traffic while not blocking the people running the site.
func SkipRateLimitForAdmin(jwtService *auth.JWTService, validateAPIToken func(string) bool, limiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Two hatches, both earned by a credential the caller actually
			// holds: an API token that passes validateAPIToken, and a JWT whose
			// user is an admin. The bare phk_ prefix is not one of them, so a
			// request the authenticator resolves to a cookie session is metered
			// as that session whatever its Authorization header claims.
			//
			// Both checks are database lookups, and validateAPIToken runs
			// BEFORE the limiter so a live token never increments the bucket.
			// A phk_-prefixed bearer therefore costs one api_tokens lookup even
			// when it names nothing, which is the same trade
			// RateLimitPublicReadsByAuthState documents for public reads.
			if ValidatedAPIToken(validateAPIToken, r) || isAdminTokenRequest(jwtService, r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

// APITokenValidator adapts an API token service to the predicate the limiter
// bypasses take: true only for a token APITokenService.ValidateToken resolves
// to a live row (hash lookup, plus its revoked, expired, inactive and scope
// checks). A nil service yields a nil predicate, which ValidatedAPIToken reads
// as "no usable token".
//
// Pure: it holds no per-process state, so callers may build it wherever they
// wire a limiter.
func APITokenValidator(svc contracts.APITokenServiceInterface) func(string) bool {
	if svc == nil {
		return nil
	}
	return func(token string) bool {
		_, _, err := svc.ValidateToken(token)
		return err == nil
	}
}

// isAdminTokenRequest returns true when the request carries a valid JWT whose
// user has IsAdmin=true. Any failure (missing token, invalid token, non-admin
// user) returns false — the caller applies the rate limit in those cases.
func isAdminTokenRequest(jwtService *auth.JWTService, r *http.Request) bool {
	if jwtService == nil {
		return false
	}
	token := extractJWT(r)
	if token == "" {
		return false
	}
	user, err := jwtService.ValidateToken(token)
	if err != nil || user == nil {
		return false
	}
	return user.IsAdmin
}

// rateLimitUserIDKey is the context key under which
// RateLimitPublicReadsByAuthState stashes the authenticated user id so the
// per-user limiter's key func can read it without re-parsing the token.
type rateLimitUserIDKey struct{}

// rateLimitUserKeyFunc keys the authenticated per-user limiter by the user id
// RateLimitPublicReadsByAuthState stashes in context. If the value is absent it
// FAILS LOUD (httprate turns a key-func error into a 428) rather than silently
// keying every request as "user:0" — so mounting RateLimitPublicReadUserEndpoints
// standalone (without the router that sets the id) is a detectable misuse, not a
// single shared 300/min bucket for the whole site (adversarial-review MEDIUM).
func rateLimitUserKeyFunc(r *http.Request) (string, error) {
	uid, ok := r.Context().Value(rateLimitUserIDKey{}).(uint)
	if !ok {
		return "", fmt.Errorf(
			"rateLimitUserKeyFunc: no authenticated user id in context; " +
				"RateLimitPublicReadUserEndpoints must be mounted via RateLimitPublicReadsByAuthState")
	}
	return "user:" + strconv.FormatUint(uint64(uid), 10), nil
}

// RateLimitPublicReadUserEndpoints is the per-USER limiter for authenticated
// public reads: PublicReadUserRequestsPerMinute per user id (NOT per IP), so
// shared-IP logged-in users each get their own bucket. Pair with
// RateLimitPublicReadsByAuthState, which supplies the user id via context.
func RateLimitPublicReadUserEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		PublicReadUserRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(rateLimitUserKeyFunc),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitPublicReadAuthenticatedIPCeiling is the COARSE per-IP backstop for
// authenticated public reads (PSY-1378): PublicReadAuthenticatedIPCeilingPerMinute
// per IP, keyed by r.RemoteAddr. It is chained on the authenticated path of
// RateLimitPublicReadsByAuthState INSIDE the per-user limiter (see the ORDER note
// there) so that one IP running many scripted accounts is bounded in aggregate —
// the per-user cap alone only meters a single account. Its own httprate store means
// it never shares a counter with the anonymous per-IP limiter (RateLimitAPIEndpoints).
func RateLimitPublicReadAuthenticatedIPCeiling() func(http.Handler) http.Handler {
	return httprate.Limit(
		PublicReadAuthenticatedIPCeilingPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(KeyByClientIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitPublicReadsByAuthState routes each request to the right limiter:
// authenticated (a cryptographically-verified session JWT) → a per-USER bucket
// (userLimiter, higher cap) additionally guarded by a coarse per-IP ceiling
// (ipCeilingLimiter, PSY-1378); a validated phk_ API token (PSY-1814) → no
// public-read limiter at all; anonymous → a per-IP bucket (anonLimiter).
//
// PSY-1373: this replaces the old full bypass for authenticated users. A full
// bypass meant one throwaway signup defeated the anti-scraping limit entirely
// (session tokens mint without a human gate). A finite per-user cap keeps
// shared-IP logged-in users un-collided (each keyed by their own id, so an office
// doesn't share one bucket — PSY-1362's requirement) while still metering a
// scraper account. Session-JWT path is DB-free: the id comes from the verified
// token (auth.JWTService.SessionUserID), no per-request DB query.
//
// SECURITY: only APITokenService.ValidateToken (hashes, DB lookup,
// revoked/expired/inactive checks), injected as validateAPIToken, exempts a
// request from the anonymous bucket. A forged phk_ prefix grants neither a
// higher cap nor a bypass on these public reads, which have no downstream
// authentication to catch it. The callback is invoked only when the bearer has the phk_ prefix, so
// visitor GETs with no Authorization never hit the database. Admin session JWTs
// still route to the per-user 300/min bucket, not this bypass.
//
// Failed validation falls through to anonLimiter, so the DB lookup runs BEFORE
// the per-IP cap. That order is load-bearing: validating after the limiter
// would increment the anonymous bucket for real ingest tokens (PSY-1814 AC).
// Forged-prefix floods can therefore generate hashed lookups beyond 100/min;
// a per-IP pre-cap on validation would re-starve ingest on a shared IP.
//
// AGGREGATE PER-IP BOUND (PSY-1378): the per-user cap bounds a SINGLE account. On
// its own it does not bound aggregate throughput from one IP running many scripted
// accounts (N accounts = N × the per-user cap). ipCeilingLimiter closes that: it is
// a coarse per-IP backstop chained on the authenticated path so an authenticated
// read must pass BOTH the per-user cap and the ceiling (stricter wins). It is
// bucketed separately from anonLimiter, so authenticated and anonymous traffic from
// one IP never share a counter, and it sits far above any legit single-office
// aggregate. Signup remains additionally throttled per-IP at /auth/register (10/min).
//
// ORDER — per-user OUTER, ceiling INNER: httprate increments a limiter's counter
// whenever the request clears that limiter's own limit, regardless of whether a
// downstream limiter then rejects it. So the ceiling must sit INSIDE the per-user
// cap: a single account spamming past its own per-user cap is 429'd by the OUTER
// per-user limiter and never reaches (nor increments) the shared per-IP ceiling —
// otherwise one misbehaving account's rejected retries (~17/s → 1000/min) could
// deplete the ceiling and collaterally 429 every OTHER authenticated user on the
// same IP, re-introducing the shared-IP false-positives per-user keying exists to
// avoid. With this order the ceiling counts only requests that cleared the per-user
// cap (real delivered throughput), while the multi-account aggregate bound still
// holds (e.g. 4 accounts × 300/min = 1200 > 1000 trips the ceiling).
func RateLimitPublicReadsByAuthState(jwtService *auth.JWTService, validateAPIToken func(string) bool, anonLimiter, userLimiter, ipCeilingLimiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		anon := anonLimiter(next)
		// Authenticated path: per-user cap OUTER, per-IP ceiling INNER (see the
		// ORDER note above). Request must clear both; a single account's own-cap
		// rejections stop at the per-user limiter and never touch the ceiling.
		user := userLimiter(ipCeilingLimiter(next))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid, ok := sessionUserID(jwtService, r); ok {
				ctx := context.WithValue(r.Context(), rateLimitUserIDKey{}, uid)
				user.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if ValidatedAPIToken(validateAPIToken, r) {
				next.ServeHTTP(w, r)
				return
			}
			anon.ServeHTTP(w, r)
		})
	}
}

// sessionUserID returns the user id from a validly-signed session token (DB-free),
// or ok=false for anonymous/invalid/expired/forged requests (incl. phk_ tokens,
// which carry no session JWT). Any failure (nil service, no token) returns false.
func sessionUserID(jwtService *auth.JWTService, r *http.Request) (uint, bool) {
	if jwtService == nil {
		return 0, false
	}
	token := extractJWT(r)
	if token == "" {
		return 0, false
	}
	return jwtService.SessionUserID(token)
}

// ValidatedAPIToken is the one spelling of "the caller holds a usable API
// token": every limiter bypass resolves the question here rather than deriving
// it from the prefix. The callback is the DB lookup
// (APITokenService.ValidateToken, adapted by APITokenValidator), invoked only
// when the credential carries the phk_ prefix, so requests with no API token
// never reach the database through this path.
//
// A nil callback answers false, so a limiter wired without a token service
// meters every request rather than exempting them all.
func ValidatedAPIToken(validate func(string) bool, r *http.Request) bool {
	if validate == nil {
		return false
	}
	token := extractJWT(r)
	if !strings.HasPrefix(token, APITokenPrefix) {
		return false
	}
	return validate(token)
}

// extractJWT reads the credential from either the Authorization header or the
// auth_token cookie, in the same order and with the same header parsing
// (bearerTokenFromHeader) the authenticating middleware uses, so the limiter
// and the authenticator always see the same credential. Returns empty string
// when no token is present.
func extractJWT(r *http.Request) string {
	if token := bearerTokenFromHeader(r.Header.Get("Authorization")); token != "" {
		return token
	}
	if cookie, err := r.Cookie("auth_token"); err == nil {
		return cookie.Value
	}
	return ""
}

// RateLimitExceededHandler handles rate limit exceeded responses
func RateLimitExceededHandler(w http.ResponseWriter, r *http.Request) {
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

	// Return 429 Too Many Requests with JSON response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)
	respond.SafeWrite(r.Context(), w, []byte(`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in 60 seconds."}`))
}
