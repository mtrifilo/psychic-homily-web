package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/logger"
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
