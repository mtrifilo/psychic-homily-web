package auth

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
)

// A refused address names an account that exists, which the caller can act on.
// The goth callback carries the same refusal to its redirect.
func TestAppleCallbackHandler_RefusedLinkKeepsItsOwnCode(t *testing.T) {
	svc := &mockAppleAuthService{findErr: autherrors.ErrUserExists("apple-refused@example.com")}
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

// A genuine fault still reports as one, so the refusal branch has not
// swallowed the fail-closed arm it sits in front of.
func TestAppleCallbackHandler_BackendFaultStaysServiceUnavailable(t *testing.T) {
	svc := &mockAppleAuthService{findErr: autherrors.ErrServiceUnavailable("database", nil)}
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
