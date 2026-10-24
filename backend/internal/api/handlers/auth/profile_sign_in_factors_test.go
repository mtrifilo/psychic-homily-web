package auth

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func profileHandlerWithFactors(user *authm.User, factors func(*authm.User) (contracts.SignInFactors, error)) *AuthHandler {
	return authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(uint) (*authm.User, error) {
				return user, nil
			},
			GetSignInFactorsFn: factors,
		}
	})
}

func TestGetProfileHandler_CarriesTheSignInFactors(t *testing.T) {
	hash := "$2a$10$secret-hash-that-must-never-leave"
	user := &authm.User{ID: 7, Email: strPtr("factors@example.com"), PasswordHash: &hash}
	var askedFor *authm.User
	h := profileHandlerWithFactors(user, func(u *authm.User) (contracts.SignInFactors, error) {
		askedFor = u
		return contracts.SignInFactors{HasPassword: true, PasskeyCount: 2}, nil
	})

	resp, err := h.GetProfileHandler(testhelpers.CtxWithUser(&authm.User{ID: 7}), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if askedFor != user {
		t.Error("the factors must be read for the profile's own user row")
	}
	if resp.Body.HasPassword == nil || !*resp.Body.HasPassword {
		t.Errorf("has_password = %v, want true", resp.Body.HasPassword)
	}
	if resp.Body.PasskeyCount == nil || *resp.Body.PasskeyCount != 2 {
		t.Errorf("passkey_count = %v, want 2", resp.Body.PasskeyCount)
	}

	wire, body := wireBody(t, resp)
	if body["has_password"] != true || body["passkey_count"] != float64(2) {
		t.Errorf("wire factors = %v / %v, want true / 2", body["has_password"], body["passkey_count"])
	}
	if leaked := leakedTerm(wire, hash, "password_hash", "passkey_credentials"); leaked != "" {
		t.Errorf("the profile body carries %q: %s", leaked, wire)
	}
}

// A provider-only account answers false and 0 explicitly: the frontend offers
// a passkey only on those two values, never on their absence.
func TestGetProfileHandler_ProviderOnlyAccountAnswersFalseAndZero(t *testing.T) {
	user := &authm.User{ID: 8, Email: strPtr("google-only@example.com")}
	h := profileHandlerWithFactors(user, func(*authm.User) (contracts.SignInFactors, error) {
		return contracts.SignInFactors{}, nil
	})

	resp, err := h.GetProfileHandler(testhelpers.CtxWithUser(&authm.User{ID: 8}), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, body := wireBody(t, resp)
	if v, ok := body["has_password"]; !ok || v != false {
		t.Errorf("has_password = %v (present %v), want an explicit false", v, ok)
	}
	if v, ok := body["passkey_count"]; !ok || v != float64(0) {
		t.Errorf("passkey_count = %v (present %v), want an explicit 0", v, ok)
	}
}

func TestGetProfileHandler_FactorsFailureOmitsThemAndKeepsTheProfile(t *testing.T) {
	user := &authm.User{ID: 9, Email: strPtr("blip@example.com")}
	h := profileHandlerWithFactors(user, func(*authm.User) (contracts.SignInFactors, error) {
		return contracts.SignInFactors{}, fmt.Errorf("count failed")
	})

	resp, err := h.GetProfileHandler(testhelpers.CtxWithUser(&authm.User{ID: 9}), &struct{}{})
	if err != nil {
		t.Fatalf("a factors failure must not fail the profile: %v", err)
	}
	if !resp.Body.Success || resp.Body.User == nil || resp.Body.User.ID != 9 {
		t.Fatalf("expected the profile to answer as usual, got success=%v user=%v", resp.Body.Success, resp.Body.User)
	}
	_, body := wireBody(t, resp)
	if _, ok := body["has_password"]; ok {
		t.Error("has_password must be omitted when it could not be read")
	}
	if _, ok := body["passkey_count"]; ok {
		t.Error("passkey_count must be omitted when it could not be read")
	}
}

func TestGetProfileHandler_ProfileFailureNeverReadsFactors(t *testing.T) {
	called := false
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(uint) (*authm.User, error) {
				return nil, fmt.Errorf("db error")
			},
			GetSignInFactorsFn: func(*authm.User) (contracts.SignInFactors, error) {
				called = true
				return contracts.SignInFactors{}, nil
			},
		}
	})

	_, _ = h.GetProfileHandler(testhelpers.CtxWithUser(&authm.User{ID: 1}), &struct{}{})
	if called {
		t.Error("the factors must not be read for a profile that failed")
	}
}

// wireBody is the profile body as it goes on the wire, raw and decoded.
func wireBody(t *testing.T, resp *UserProfileResponse) (string, map[string]any) {
	t.Helper()
	wire, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(wire, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return string(wire), body
}

// leakedTerm returns the first of terms that body contains, or "".
func leakedTerm(body string, terms ...string) string {
	for _, term := range terms {
		if strings.Contains(body, term) {
			return term
		}
	}
	return ""
}
