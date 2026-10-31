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

	claims := func(exp any, withExp bool) jwt.MapClaims {
		c := jwt.MapClaims{
			"iss": "https://appleid.apple.com",
			"aud": cfg.Apple.BundleID,
			"sub": "apple-sub-1",
			"iat": time.Now().Unix(),
		}
		if withExp {
			c["exp"] = exp
		}
		return c
	}
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

	control, svc := callback(signAppleClaims(t, key, "known-kid", claims(time.Now().Add(time.Hour).Unix(), true)))
	if !control.Body.Success || !svc.resolved {
		t.Fatalf("the control token with an expiry did not sign in: code=%q message=%q", control.Body.ErrorCode, control.Body.Message)
	}

	for name, c := range map[string]jwt.MapClaims{
		"no exp claim": claims(nil, false),
		"exp is null":  claims(nil, true),
	} {
		t.Run(name, func(t *testing.T) {
			resp, svc := callback(signAppleClaims(t, key, "known-kid", c))
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
