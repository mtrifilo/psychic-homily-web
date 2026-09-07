package auth

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// erroringAppleAuthService returns a fixed error from FindOrCreateAppleUser so
// the handler's discrimination between a refusal and a fault can be driven
// without a database.
type erroringAppleAuthService struct {
	err error
}

func (m *erroringAppleAuthService) ValidateIdentityToken(string) (*contracts.AppleIdentityTokenClaims, error) {
	return &contracts.AppleIdentityTokenClaims{
		Email:            "apple-refused@example.com",
		RegisteredClaims: jwt.RegisteredClaims{Subject: "apple-sub-refused"},
	}, nil
}

func (m *erroringAppleAuthService) FindOrCreateAppleUser(*contracts.AppleIdentityTokenClaims, string, string) (*authm.User, error) {
	return nil, m.err
}

func (m *erroringAppleAuthService) GenerateToken(*authm.User) (string, error) {
	return "", nil
}

// TestAppleCallbackHandler_RefusedLinkKeepsItsOwnCode: the address belongs to
// an account that exists, which is a decision the caller can act on, not the
// backend being unavailable. The goth callback carries the same refusal to its
// redirect; this is the iOS-facing half of that pair.
func TestAppleCallbackHandler_RefusedLinkKeepsItsOwnCode(t *testing.T) {
	svc := &erroringAppleAuthService{err: autherrors.ErrUserExists("apple-refused@example.com")}
	h := NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, testConfig())

	input := &AppleCallbackRequest{}
	input.Body.IdentityToken = "any-token"

	resp, err := h.AppleCallbackHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false for a refused link")
	}
	if resp.Body.ErrorCode != autherrors.CodeUserExists {
		t.Errorf("error_code = %q, want %q", resp.Body.ErrorCode, autherrors.CodeUserExists)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeUserExists) {
		t.Errorf("message = %q, want %q", resp.Body.Message, autherrors.ToExternalMessage(autherrors.CodeUserExists))
	}
	if resp.Body.Token != "" {
		t.Error("a refused link must not issue a token")
	}
}

// A genuine backend fault still reports as one, so the new branch has not
// swallowed the fail-closed arm it sits in front of.
func TestAppleCallbackHandler_BackendFaultStaysServiceUnavailable(t *testing.T) {
	svc := &erroringAppleAuthService{err: autherrors.ErrServiceUnavailable("database", nil)}
	h := NewAppleAuthHandler(svc, &testhelpers.MockDiscordService{}, testConfig())

	input := &AppleCallbackRequest{}
	input.Body.IdentityToken = "any-token"

	resp, err := h.AppleCallbackHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("error_code = %q, want %q", resp.Body.ErrorCode, autherrors.CodeServiceUnavailable)
	}
	if resp.Body.Message != "Failed to process Apple sign-in" {
		t.Errorf("message = %q, want the generic failure copy", resp.Body.Message)
	}
}
