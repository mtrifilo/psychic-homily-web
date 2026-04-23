package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/logger"
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
			if isAdminTokenRequest(jwtService, r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
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
	w.Write([]byte(`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in 60 seconds."}`))
}
