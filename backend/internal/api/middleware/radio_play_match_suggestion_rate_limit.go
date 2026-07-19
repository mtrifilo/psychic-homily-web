package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"psychic-homily-backend/internal/services/auth"
)

// RadioPlayMatchSuggestionRequestsPerHour is the per-user rate limit for
// community POST /radio/plays/{id}/match-suggestions (PSY-1494). Conservative
// default: enough for genuine playlist matching, tight enough to keep the
// admin review queue from being flooded.
const RadioPlayMatchSuggestionRequestsPerHour = 20

// radioPlayMatchSuggestionUserIDKey is the context key for the per-user limiter.
type radioPlayMatchSuggestionUserIDKey struct{}

// RadioPlayMatchSuggestionUserKeyFunc keys the match-suggestion limiter by the
// authenticated user id stashed by RateLimitRadioPlayMatchSuggestionsByUser.
// Exported so routes can pass it to httprate.WithKeyFuncs.
func RadioPlayMatchSuggestionUserKeyFunc(r *http.Request) (string, error) {
	uid, ok := r.Context().Value(radioPlayMatchSuggestionUserIDKey{}).(uint)
	if !ok {
		return "", fmt.Errorf(
			"RadioPlayMatchSuggestionUserKeyFunc: no authenticated user id in context; " +
				"must be mounted via RateLimitRadioPlayMatchSuggestionsByUser")
	}
	return "user:" + strconv.FormatUint(uint64(uid), 10), nil
}

// RateLimitRadioPlayMatchSuggestionsByUser meters authenticated community
// match-suggestion POSTs against a per-user hourly budget. Unauthenticated
// requests pass through (JWT middleware 401s them anyway).
func RateLimitRadioPlayMatchSuggestionsByUser(
	jwtService *auth.JWTService,
	limiter func(http.Handler) http.Handler,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid, ok := sessionUserID(jwtService, r); ok {
				ctx := context.WithValue(r.Context(), radioPlayMatchSuggestionUserIDKey{}, uid)
				limited.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
