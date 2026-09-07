package auth

import (
	"context"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
)

// A refused address names an account that exists, so the handler answers with
// that refusal's own code and copy rather than as a fault.
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
	// UserMessage() is what the handler emits. ToExternalMessage is a separate
	// table that happens to agree for this code, so asserting against it would
	// point a maintainer at the wrong function.
	if want := autherrors.ErrUserExists("apple-refused@example.com").UserMessage(); resp.Body.Message != want {
		t.Errorf("message = %q, want %q", resp.Body.Message, want)
	}
	if resp.Body.Token != "" {
		t.Error("a refused link must not issue a token")
	}
}

// A genuine fault still reports as SERVICE_UNAVAILABLE, so the refusal branch
// has not swallowed the generic arm it sits in front of.
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
