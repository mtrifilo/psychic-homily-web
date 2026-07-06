package middleware

import (
	"net/http"
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

// SkipRateLimitForAuthenticated wraps a rate-limit middleware so ANY
// authenticated request bypasses the per-IP limiter — a valid JWT (admin or
// not) or a trusted phk_ API token. Only anonymous/unauthenticated traffic
// hits the underlying limiter.
//
// PSY-1362: public unauthenticated read endpoints (graph-card, artist/show/
// venue/label/scene reads, etc.) were unthrottled. Limiting ONLY anonymous
// traffic protects them from scraping/abuse while never throttling logged-in
// users — which sidesteps the shared-IP false-positive risk of a blanket
// per-IP limit (offices, universities, carrier NAT, where many real, logged-in
// users share one egress IP). Broader bypass than SkipRateLimitForAdmin (which
// limits non-admin authenticated users); reuses the same JWT/API-token helpers.
func SkipRateLimitForAuthenticated(jwtService *auth.JWTService, limiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isTrustedAPIToken(r) || isAuthenticatedRequest(jwtService, r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

// isAuthenticatedRequest returns true when the request carries a validly-signed,
// unexpired session token. Uses HasValidSessionToken (signature/claims only, NO
// DB lookup) — unlike isAdminTokenRequest, the anonymous-vs-authenticated
// decision needs no fresh user record, and this middleware runs on every
// request, so a per-request DB query would be wasteful. Any failure
// (missing/invalid/expired token, nil service) returns false.
func isAuthenticatedRequest(jwtService *auth.JWTService, r *http.Request) bool {
	if jwtService == nil {
		return false
	}
	token := extractJWT(r)
	if token == "" {
		return false
	}
	return jwtService.HasValidSessionToken(token)
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
