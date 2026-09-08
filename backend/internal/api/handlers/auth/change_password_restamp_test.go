package auth

import (
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// Changing a password verifies the current one, which is the same factor a
// password sign-in proves. The session goes on with a token stamped for it, so
// a user who just proved the password is not then told to prove it again.

func changePasswordInput() *ChangePasswordRequest {
	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "old-password-123"
	input.Body.NewPassword = "new-password-456"
	return input
}

func TestChangePasswordHandler_RestampsTheSession(t *testing.T) {
	var stamped *authm.User
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(uint, string, string) error { return nil },
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				stamped = u
				return "restamped-token", nil
			},
		}
	})

	user := &authm.User{ID: 1}
	// A session too old to satisfy the gate: the change is what refreshes it,
	// not a second sign-in.
	ctx := testhelpers.CtxWithSessionAuthTime(user, time.Now().Add(-2*time.Hour))

	resp, err := h.ChangePasswordHandler(ctx, changePasswordInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatalf("expected success=true, got message=%q", resp.Body.Message)
	}
	if stamped == nil {
		t.Fatal("expected a token stamped with the factor just proven")
	}
	if stamped.ID != user.ID {
		t.Errorf("restamped for user %d, want %d", stamped.ID, user.ID)
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("cookie name = %q, want %q", resp.SetCookie.Name, config.AuthCookieName)
	}
	if resp.SetCookie.Value != "restamped-token" {
		t.Errorf("cookie value = %q, want the restamped token", resp.SetCookie.Value)
	}
}

// The restamped session is what the credential mints accept, which is the whole
// point of restamping: the two are driven end to end here rather than asserted
// against each other's descriptions.
func TestChangePasswordHandler_RestampSatisfiesTheMintGate(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(uint, string, string) error { return nil },
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(*authm.User) (string, error) { return "restamped-token", nil },
			RenewSessionTokenFn: func(*authm.User, time.Time) (string, error) {
				return "cli-token", nil
			},
		}
	})

	user := &authm.User{ID: 1, IsAdmin: true}
	staleCtx := testhelpers.CtxWithSessionAuthTime(user, time.Now().Add(-2*time.Hour))

	if _, err := h.GenerateCLITokenHandler(staleCtx, &struct{}{}); err == nil {
		t.Fatal("the stale session must be refused before the password change")
	}

	if _, err := h.ChangePasswordHandler(staleCtx, changePasswordInput()); err != nil {
		t.Fatalf("changing the password: %v", err)
	}

	// The session the caller now holds: the handler stamped it at the moment
	// the factor verified, which is what the next request presents.
	freshCtx := testhelpers.CtxWithSessionAuthTime(user, time.Now())
	if _, err := h.GenerateCLITokenHandler(freshCtx, &struct{}{}); err != nil {
		t.Fatalf("the restamped session must mint: %v", err)
	}
}

// The password is already changed by the time the stamp is minted, so a mint
// failure cannot be reported as a failed change. The caller keeps the session
// it arrived with, which the gates go on refusing.
func TestChangePasswordHandler_RestampFailureDoesNotFailTheChange(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(uint, string, string) error { return nil },
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(*authm.User) (string, error) {
				return "", errors.New("jwt outage")
			},
		}
	})

	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now().Add(-2*time.Hour))
	resp, err := h.ChangePasswordHandler(ctx, changePasswordInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("the password changed, so the response must say so")
	}
	if resp.SetCookie.Value != "" {
		t.Error("a failed stamp must not set a cookie")
	}
}

// A failed change proves nothing, so it stamps nothing.
func TestChangePasswordHandler_FailedChangeDoesNotRestamp(t *testing.T) {
	var stamped bool
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(uint, string, string) error {
				return autherrors.ErrInvalidCredentials(nil)
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(*authm.User) (string, error) {
				stamped = true
				return "restamped-token", nil
			},
		}
	})

	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now().Add(-2*time.Hour))
	resp, err := h.ChangePasswordHandler(ctx, changePasswordInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Fatal("expected the change to fail")
	}
	if stamped {
		t.Error("a rejected password must not refresh the session's authentication time")
	}
	if resp.SetCookie.Value != "" {
		t.Error("a rejected password must not set a cookie")
	}

	// And the mint stays refused, which is what the stamp would have changed.
	if err := shared.RequireRecentSessionAuth(ctx, "generate_cli_token"); err == nil {
		t.Error("the session must still be too old to mint")
	}
}
