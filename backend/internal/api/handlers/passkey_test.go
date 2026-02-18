package handlers

import (
	"context"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
)

func testPasskeyHandler() *PasskeyHandler {
	return NewPasskeyHandler(nil, nil, nil, testConfig())
}

// --- NewPasskeyHandler ---

func TestNewPasskeyHandler(t *testing.T) {
	h := testPasskeyHandler()
	if h == nil {
		t.Fatal("expected non-nil PasskeyHandler")
	}
}

// --- BeginRegisterHandler ---

func TestBeginRegisterHandler_NoAuth(t *testing.T) {
	h := testPasskeyHandler()
	input := &BeginRegisterRequest{}

	resp, err := h.BeginRegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
}

// --- FinishRegisterHandler ---

func TestFinishRegisterHandler_NoAuth(t *testing.T) {
	h := testPasskeyHandler()
	input := &FinishRegisterRequest{}

	resp, err := h.FinishRegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
}

// --- ListCredentialsHandler ---

func TestListCredentialsHandler_NoAuth(t *testing.T) {
	h := testPasskeyHandler()

	resp, err := h.ListCredentialsHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
}

// --- DeleteCredentialHandler ---

func TestDeleteCredentialHandler_NoAuth(t *testing.T) {
	h := testPasskeyHandler()
	input := &DeleteCredentialRequest{CredentialID: 1}

	resp, err := h.DeleteCredentialHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
}

// --- BeginSignupHandler ---

func TestBeginSignupHandler_EmptyEmail(t *testing.T) {
	h := testPasskeyHandler()
	input := &BeginSignupRequest{}

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
}

func TestBeginSignupHandler_MissingTermsAcceptance(t *testing.T) {
	h := testPasskeyHandler()
	input := &BeginSignupRequest{}
	input.Body.Email = "test@example.com"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
}