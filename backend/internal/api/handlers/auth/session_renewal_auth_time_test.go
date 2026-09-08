package auth

import (
	"context"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
)

// The two endpoints that mint a session from a session rather than from a
// factor: POST /auth/refresh and the admin CLI-token mint. Both must hand the
// authentication time of the presented session to the minting service, so that
// a session too old to satisfy a re-authentication gate cannot renew its way
// out of the refusal.

// ctxWithSessionAuthTime builds what the JWT middlewares put in front of these
// handlers: a principal and, when the credential carries one, the time a
// factor last completed for it.
func ctxWithSessionAuthTime(user *authm.User, authAt time.Time) context.Context {
	ctx := testhelpers.CtxWithUser(user)
	if authAt.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, middleware.SessionAuthTimeContextKey, authAt)
}

func TestRefreshTokenHandler_PassesTheSessionsAuthTimeThrough(t *testing.T) {
	authAt := time.Now().Add(-90 * time.Minute).UTC()

	var got time.Time
	var called bool
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: userID}, nil
			},
			RefreshUserTokenFn: func(user *authm.User, a time.Time) (string, error) {
				called, got = true, a
				return "renewed-token", nil
			},
		}
	})

	resp, err := h.RefreshTokenHandler(ctxWithSessionAuthTime(&authm.User{ID: 1}, authAt), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the renewal to reach the auth service")
	}
	if !got.Equal(authAt) {
		t.Errorf("renewal got auth time %v, want the session's %v", got, authAt)
	}
	if resp.Body.Token != "renewed-token" {
		t.Errorf("expected token=renewed-token, got %s", resp.Body.Token)
	}
}

// A session carrying no authentication time renews without acquiring one: the
// handler passes the zero value on rather than substituting the current time.
func TestRefreshTokenHandler_LegacySessionRenewsWithNoAuthTime(t *testing.T) {
	var got time.Time
	var called bool
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: userID}, nil
			},
			RefreshUserTokenFn: func(user *authm.User, a time.Time) (string, error) {
				called, got = true, a
				return "renewed-token", nil
			},
		}
	})

	if _, err := h.RefreshTokenHandler(testhelpers.CtxWithUser(&authm.User{ID: 1}), &struct{}{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the renewal to reach the auth service")
	}
	if !got.IsZero() {
		t.Errorf("renewal got auth time %v, want the zero value", got)
	}
}

func TestGenerateCLITokenHandler_PassesTheSessionsAuthTimeThrough(t *testing.T) {
	authAt := time.Now().Add(-4 * time.Hour).UTC()

	var got time.Time
	var stamped bool
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			RenewSessionTokenFn: func(u *authm.User, a time.Time) (string, error) {
				got = a
				return "cli-token", nil
			},
			CreateTokenFn: func(u *authm.User) (string, error) {
				stamped = true
				return "factor-token", nil
			},
		}
	})

	ctx := ctxWithSessionAuthTime(&authm.User{ID: 1, IsAdmin: true}, authAt)
	resp, err := h.GenerateCLITokenHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stamped {
		t.Error("minting a session from a session must not stamp a new authentication time")
	}
	if !got.Equal(authAt) {
		t.Errorf("CLI mint got auth time %v, want the session's %v", got, authAt)
	}
	if resp.Body.Token != "cli-token" {
		t.Errorf("expected token=cli-token, got %s", resp.Body.Token)
	}
}
