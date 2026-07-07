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
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitPasskeyEndpoints creates a rate limiter for passkey/WebAuthn endpoints
// 20 requests per minute per IP - slightly more lenient for multi-step flows
func RateLimitPasskeyEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		PasskeyRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitAPIEndpoints creates a general rate limiter for API endpoints
// 100 requests per minute per IP - basic abuse protection
func RateLimitAPIEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		APIRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitTagCreateEndpoints creates a rate limiter for tag creation endpoints
// 20 requests per hour per IP - prevents tag spam on entities
func RateLimitTagCreateEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		TagCreateRequestsPerHour,
		time.Hour,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(RateLimitExceededHandler),
	)
}

// RateLimitTagVoteEndpoints creates a rate limiter for tag voting endpoints
// 30 requests per minute per IP - prevents rapid vote manipulation
func RateLimitTagVoteEndpoints() func(http.Handler) http.Handler {
	return httprate.Limit(
		TagVoteRequestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
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
func SkipRateLimitForAdmin(jwtService *auth.JWTService, limiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// API tokens (phk_ prefix) and JWT admins both bypass: API tokens are
			// admin-only and trusted, so they shouldn't be throttled during bulk
			// imports — matching routes.rateLimitUnlessAPIToken (show creation).
			// Without the API-token branch the ph CLI (which authenticates with a
			// phk_ token, not a JWT) gets throttled on bulk tagging despite PSY-345.
			if isTrustedAPIToken(r) || isAdminTokenRequest(jwtService, r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

// isTrustedAPIToken reports whether the request carries an API token (phk_
// prefix). API tokens are admin-only and trusted by construction (see
// internal/services/admin/api_token.go), so — like routes.rateLimitUnlessAPIToken used
// for show creation — they bypass the per-IP limiter. This intentionally trusts
// the prefix rather than re-validating the token (the JWT validator rejects
// phk_ tokens anyway); it covers the ph CLI doing bulk imports (PSY-345).
func isTrustedAPIToken(r *http.Request) bool {
	return strings.HasPrefix(extractJWT(r), APITokenPrefix)
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

// RateLimitPublicReadsByAuthState routes each request to the right limiter:
// authenticated (a cryptographically-verified session JWT) → a per-USER bucket
// (userLimiter, higher cap); anonymous → a per-IP bucket (anonLimiter).
//
// PSY-1373: this replaces the old full bypass for authenticated users. A full
// bypass meant one throwaway signup defeated the anti-scraping limit entirely
// (session tokens mint without a human gate). A finite per-user cap keeps
// shared-IP logged-in users un-collided (each keyed by their own id, so an office
// doesn't share one bucket — PSY-1362's requirement) while still metering a
// scraper account. DB-free: the id comes from the verified token
// (auth.JWTService.SessionUserID), no per-request DB query.
//
// SECURITY: like PSY-1362 this does NOT honor the phk_ API-token prefix (trusted
// without validation) — a forged prefix must not grant the higher authenticated
// cap on these no-downstream-auth public reads. Only an unforgeable JWT with a
// user_id claim routes to the per-user limiter; everything else is anonymous.
//
// RESIDUAL (adversarial-review, tracked as a follow-up): the per-user cap bounds a
// SINGLE account. It does not bound aggregate throughput from one IP running many
// scripted accounts — there is no per-IP ceiling on authenticated traffic (adding
// one would re-introduce the shared-IP collisions per-user keying exists to avoid).
// Scripted multi-account scraping is partially mitigated by the per-IP signup
// limiter on /auth/register (10/min, wired inline in routes/auth.go) and by each
// account being a bannable identity; full mitigation (per-IP ceiling / signup
// friction / bot detection) is a deeper effort beyond this ticket's "a single
// account can't scrape unmetered" AC — tracked in PSY-1378.
func RateLimitPublicReadsByAuthState(jwtService *auth.JWTService, anonLimiter, userLimiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		anon := anonLimiter(next)
		user := userLimiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid, ok := sessionUserID(jwtService, r); ok {
				ctx := context.WithValue(r.Context(), rateLimitUserIDKey{}, uid)
				user.ServeHTTP(w, r.WithContext(ctx))
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

// extractJWT reads the JWT from either the Authorization header or the
// auth_token cookie, matching the logic in JWTMiddleware. Returns empty
// string when no token is present.
func extractJWT(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
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
