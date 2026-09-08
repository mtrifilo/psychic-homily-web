package auth

import (
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

// A handler that mints a session from a session rather than from a factor hands
// the presented session's authentication time to the minting service, so that a
// session too old to satisfy a re-authentication gate cannot renew its way out
// of the refusal.

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

	resp, err := h.RefreshTokenHandler(testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, authAt), &struct{}{})
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

	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Time{})
	if _, err := h.RefreshTokenHandler(ctx, &struct{}{}); err != nil {
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
	// Inside the re-authentication window, because the mint refuses anything
	// older. The point stands within it: the CLI token carries the session's
	// own authentication time rather than a fresh stamp, so it ages out of the
	// window when that session would have.
	authAt := time.Now().Add(-1 * time.Minute).UTC()

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

	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1, IsAdmin: true}, authAt)
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

// A credential that establishes no authentication time reaches no mint: an API
// token, and a session issued before the auth_at claim existed, both present
// the zero value and are refused here. The renewal service keeps its own
// handling of a zero authentication time; nothing routes one to it from here.
func TestGenerateCLITokenHandler_SessionWithoutAuthTimeRefused(t *testing.T) {
	var called bool
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			RenewSessionTokenFn: func(u *authm.User, a time.Time) (string, error) {
				called = true
				return "cli-token", nil
			},
		}
	})

	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1, IsAdmin: true}, time.Time{})
	if _, err := h.GenerateCLITokenHandler(ctx, &struct{}{}); err == nil {
		t.Fatal("a session establishing no authentication time must be refused")
	}
	if called {
		t.Error("a refused request must not reach the JWT service")
	}
}

// A session whose factor completed two hours ago is refused. POST /auth/refresh
// hands such a session a token carrying that same authentication time, so it
// arrives here as this exact value and is refused the same way; that the
// renewal carries it rather than moving it is established by
// TestRequireRecentSessionAuth_RenewalBuysNoFreshness in handlers/shared, which
// runs the renewal against the JWT service.
func TestGenerateCLITokenHandler_StaleSessionRefused(t *testing.T) {
	user := &authm.User{ID: 1, IsAdmin: true, IsActive: true}

	var called bool
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			RenewSessionTokenFn: func(u *authm.User, a time.Time) (string, error) {
				called = true
				return "cli-token", nil
			},
		}
	})

	ctx := testhelpers.CtxWithSessionAuthTime(user, time.Now().Add(-2*time.Hour))
	if _, err := h.GenerateCLITokenHandler(ctx, &struct{}{}); err == nil {
		t.Fatal("expected the mint to be refused")
	}
	if called {
		t.Error("a refused request must not reach the JWT service")
	}
}
