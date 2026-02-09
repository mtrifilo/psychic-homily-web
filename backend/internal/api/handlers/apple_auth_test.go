package handlers

import (
	"context"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
)

func testAppleAuthHandler() *AppleAuthHandler {
	return NewAppleAuthHandler(nil, nil, testConfig())
}

// --- NewAppleAuthHandler ---

func TestNewAppleAuthHandler(t *testing.T) {
	h := testAppleAuthHandler()
	if h == nil {
		t.Fatal("expected non-nil AppleAuthHandler")
	}
}

// --- AppleCallbackHandler ---

func TestAppleCallbackHandler_EmptyToken(t *testing.T) {
	h := testAppleAuthHandler()
	input := &AppleCallbackRequest{}
	// IdentityToken is empty string (zero value)

	resp, err := h.AppleCallbackHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Identity token is required" {
		t.Errorf("expected message 'Identity token is required', got %q", resp.Body.Message)
	}
}
