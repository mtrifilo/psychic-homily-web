package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	authsvc "psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// appleSignInRecorder validates identity tokens with the real Apple service
// and records whether the handler went on to resolve an account or mint a
// session.
type appleSignInRecorder struct {
	contracts.AppleAuthServiceInterface
	resolved bool
	minted   bool
}

func (r *appleSignInRecorder) FindOrCreateAppleUser(*contracts.AppleIdentityTokenClaims, string, string, *contracts.OAuthSignupConsent) (*authm.User, error) {
	r.resolved = true
	return &authm.User{ID: 1}, nil
}

func (r *appleSignInRecorder) GenerateToken(*authm.User) (string, error) {
	r.minted = true
	return "apple-session", nil
}

// An Apple identity token that is correctly signed for this app but carries no
// expiry is refused with TOKEN_INVALID before any account is found, created or
// signed in. The same token with an expiry signs in, so the expiry is the only
// thing the refusal turns on.
func TestAppleCallbackHandler_TokenWithoutExpiryIsInvalid(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	stubAppleKeyEndpoint(t, appleKeySet(key, "known-kid"))

	cfg := testConfig()
	cfg.Apple.BundleID = "com.psychichomily.ios"

	callback := func(token string) (*AppleCallbackResponse, *appleSignInRecorder) {
		t.Helper()
		svc := &appleSignInRecorder{AppleAuthServiceInterface: authsvc.NewAppleAuthService(nil, cfg, nil)}
		h := NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, cfg)
		input := &AppleCallbackRequest{}
		input.Body.IdentityToken = token
		resp, err := h.AppleCallbackHandler(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return resp, svc
	}

	control, svc := callback(signedAppleToken(t, key, cfg.Apple.BundleID, "known-kid", time.Now().Add(time.Hour)))
	if !control.Body.Success || !svc.resolved || !svc.minted || control.Body.Token == "" {
		t.Fatalf("the control token with an expiry did not sign in: code=%q message=%q", control.Body.ErrorCode, control.Body.Message)
	}

	for name, withoutExpiry := range map[string]func(jwt.MapClaims){
		"no exp claim": func(c jwt.MapClaims) { delete(c, "exp") },
		"exp is null":  func(c jwt.MapClaims) { c["exp"] = nil },
	} {
		t.Run(name, func(t *testing.T) {
			claims := appleClaimsFor(cfg.Apple.BundleID, "apple-sub-1", "", time.Now().Add(time.Hour))
			withoutExpiry(claims)
			resp, svc := callback(signAppleClaims(t, key, "known-kid", claims))
			if resp.Body.Success {
				t.Fatal("a token without an expiry signed in")
			}
			if resp.Body.ErrorCode != autherrors.CodeTokenInvalid {
				t.Errorf("error_code = %q, want %q", resp.Body.ErrorCode, autherrors.CodeTokenInvalid)
			}
			if resp.Body.Message != "Invalid Apple identity token" {
				t.Errorf("message = %q", resp.Body.Message)
			}
			if svc.resolved {
				t.Error("the refused token reached FindOrCreateAppleUser")
			}
			if svc.minted || resp.Body.Token != "" || resp.SetCookie.Value != "" {
				t.Error("the refused token was given a session")
			}
		})
	}
}
