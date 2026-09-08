package testhelpers

import (
	"context"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
)

// CtxWithSessionAuthTime builds what the JWT middlewares put in front of a
// handler: a principal and the time an authentication factor last completed
// for the credential the request presented. The zero time is what an API-token
// principal and a session minted before the auth_at claim both present.
func CtxWithSessionAuthTime(user *authm.User, authAt time.Time) context.Context {
	return context.WithValue(CtxWithUser(user), middleware.SessionAuthTimeContextKey, authAt)
}

// SessionAuthTimeAfterRenewal returns the authentication time a session
// carrying authAt still presents after POST /auth/refresh has renewed it.
//
// The renewal and the middleware's read are both run for real against a JWT
// service, so a test asserting that a refreshed session is no fresher than the
// one it replaced asserts it against the code the endpoint runs rather than
// against a restatement of the rule.
func SessionAuthTimeAfterRenewal(t *testing.T, user *authm.User, authAt time.Time) time.Time {
	t.Helper()

	cfg := &config.Config{JWT: config.JWTConfig{
		SecretKey: "test-secret-key-at-least-32-characters-long",
		Expiry:    24,
	}}
	jwtService := authsvc.NewJWTService(nil, cfg, &MockUserService{
		GetUserByIDFn: func(uint) (*authm.User, error) { return user, nil },
	})

	renewed, err := jwtService.RenewSessionToken(user, authAt)
	if err != nil {
		t.Fatalf("renewing the session: %v", err)
	}
	_, renewedAuthAt, err := jwtService.ValidateSession(renewed)
	if err != nil {
		t.Fatalf("reading the renewed session: %v", err)
	}
	return renewedAuthAt
}
