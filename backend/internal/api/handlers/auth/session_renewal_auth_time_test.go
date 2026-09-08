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

// The stale session, and the same session after POST /auth/refresh renewed it.
// The renewal is what the gate exists to see through: it moves the issue time
// with no factor behind it, so it must buy no access to the mint.
func TestGenerateCLITokenHandler_StaleAndRefreshedSessionsRefused(t *testing.T) {
	user := &authm.User{ID: 1, IsAdmin: true, IsActive: true}
	stale := time.Now().Add(-2 * time.Hour)

	for name, authAt := range map[string]time.Time{
		"stale session":                 stale,
		"stale session, then refreshed": testhelpers.SessionAuthTimeAfterRenewal(t, user, stale.Truncate(time.Second)),
	} {
		t.Run(name, func(t *testing.T) {
			var called bool
			h := authHandler(func(ah *AuthHandler) {
				ah.jwtService = &testhelpers.MockJWTService{
					RenewSessionTokenFn: func(u *authm.User, a time.Time) (string, error) {
						called = true
						return "cli-token", nil
					},
				}
			})

			if _, err := h.GenerateCLITokenHandler(testhelpers.CtxWithSessionAuthTime(user, authAt), &struct{}{}); err == nil {
				t.Fatal("expected the mint to be refused")
			}
			if called {
				t.Error("a refused request must not reach the JWT service")
			}
		})
	}
}
