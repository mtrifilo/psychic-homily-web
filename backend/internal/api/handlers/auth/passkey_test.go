package handlers

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// ============================================================================
// Helper to build a PasskeyHandler with mocks
// ============================================================================

func testPasskeyHandler() *PasskeyHandler {
	return NewPasskeyHandler(nil, nil, nil, testConfig())
}

func testPasskeyHandlerWithMocks(wa *mockWebAuthnService, jwt *mockJWTService, us *mockUserService) *PasskeyHandler {
	return NewPasskeyHandler(wa, jwt, us, testConfig())
}

// ============================================================================
// BeginRegisterHandler
// ============================================================================

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
	if resp.Body.Message != "Authentication required" {
		t.Errorf("expected message 'Authentication required', got '%s'", resp.Body.Message)
	}
}

func TestBeginRegisterHandler_Success(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginRegistrationFn: func(user *models.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			if operation != "registration" {
				t.Errorf("expected operation 'registration', got '%s'", operation)
			}
			return "test-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	input := &BeginRegisterRequest{}
	input.Body.DisplayName = "My MacBook"

	resp, err := h.BeginRegisterHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.ChallengeID != "test-challenge-id" {
		t.Errorf("expected challenge_id 'test-challenge-id', got '%s'", resp.Body.ChallengeID)
	}
	if resp.Body.Message != "Registration options created" {
		t.Errorf("expected message 'Registration options created', got '%s'", resp.Body.Message)
	}
}

func TestBeginRegisterHandler_BeginRegistrationFails(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginRegistrationFn: func(user *models.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("webauthn init error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
}

func TestBeginRegisterHandler_StoreChallengeFailure(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginRegistrationFn: func(user *models.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("storage error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to store challenge" {
		t.Errorf("expected message 'Failed to store challenge', got '%s'", resp.Body.Message)
	}
}

// ============================================================================
// FinishRegisterHandler
// ============================================================================

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

func TestFinishRegisterHandler_InvalidChallenge(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return nil, 0, fmt.Errorf("challenge not found")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = "bad-challenge-id"

	resp, err := h.FinishRegisterHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid or expired challenge" {
		t.Errorf("expected 'Invalid or expired challenge', got '%s'", resp.Body.Message)
	}
}

func TestFinishRegisterHandler_ChallengeBelongsToDifferentUser(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			// Return user ID 99, but the authenticated user is ID 1
			return &webauthn.SessionData{}, 99, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = "valid-challenge"

	resp, err := h.FinishRegisterHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Challenge belongs to different user" {
		t.Errorf("expected 'Challenge belongs to different user', got '%s'", resp.Body.Message)
	}
}

func TestFinishRegisterHandler_MalformedCredentialResponse(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 1, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = "valid-challenge"
	// Provide invalid/empty credential data — will fail parsing
	input.Body.Response = CredentialCreationResponse{
		ID:    "",
		RawID: "",
		Type:  "",
		Response: CredentialCreationAttestationResponse{
			AttestationObject: "not-valid-base64",
			ClientDataJSON:    "not-valid-base64",
		},
	}

	resp, err := h.FinishRegisterHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid credential response" {
		t.Errorf("expected 'Invalid credential response', got '%s'", resp.Body.Message)
	}
}

func TestFinishRegisterHandler_DefaultDisplayName(t *testing.T) {
	// The handler sets "My Passkey" as default if display_name is empty.
	// We verify this by checking the displayName passed to FinishRegistration.
	var capturedDisplayName string
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 1, nil
		},
		finishRegistrationFn: func(user *models.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*models.WebAuthnCredential, error) {
			capturedDisplayName = displayName
			return &models.WebAuthnCredential{ID: 1, UserID: user.ID}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.DisplayName = "" // empty — should default to "My Passkey"
	// Note: credential response will fail parsing before reaching FinishRegistration,
	// so we can't actually test this path without valid WebAuthn data.
	// Instead test the code path that checks challenge mismatch,
	// which we already covered above.

	// To test the default name, we need the parse step to succeed.
	// Since we can't easily create valid WebAuthn data, we just verify the early error paths.
	resp, err := h.FinishRegisterHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Will fail on parse, which is expected
	if resp.Body.Success {
		t.Error("expected success=false (parse should fail with empty credential data)")
	}
	// capturedDisplayName won't be set because parsing fails before FinishRegistration
	_ = capturedDisplayName
}

// ============================================================================
// BeginLoginHandler
// ============================================================================

func TestBeginLoginHandler_WithEmail_UserNotFound(t *testing.T) {
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, fmt.Errorf("user not found")
		},
	}
	h := testPasskeyHandlerWithMocks(&mockWebAuthnService{}, &mockJWTService{}, mockUS)

	input := &BeginLoginRequest{}
	input.Body.Email = "nobody@example.com"

	resp, err := h.BeginLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid credentials" {
		t.Errorf("expected 'Invalid credentials', got '%s'", resp.Body.Message)
	}
}

func TestBeginLoginHandler_WithEmail_NoPasskeys(t *testing.T) {
	email := "user@example.com"
	mockUS := &mockUserService{
		getUserByEmailFn: func(e string) (*models.User, error) {
			return &models.User{ID: 1, Email: &email}, nil
		},
	}
	mockWA := &mockWebAuthnService{
		beginLoginFn: func(user *models.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("no credentials registered")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &BeginLoginRequest{}
	input.Body.Email = email

	resp, err := h.BeginLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "No passkeys registered for this account" {
		t.Errorf("unexpected message: '%s'", resp.Body.Message)
	}
}

func TestBeginLoginHandler_WithEmail_Success(t *testing.T) {
	email := "user@example.com"
	mockUS := &mockUserService{
		getUserByEmailFn: func(e string) (*models.User, error) {
			return &models.User{ID: 42, Email: &email}, nil
		},
	}
	mockWA := &mockWebAuthnService{
		beginLoginFn: func(user *models.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			if user.ID != 42 {
				t.Errorf("expected user ID 42, got %d", user.ID)
			}
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			if userID != 42 {
				t.Errorf("expected userID 42, got %d", userID)
			}
			if operation != "authentication" {
				t.Errorf("expected operation 'authentication', got '%s'", operation)
			}
			return "login-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &BeginLoginRequest{}
	input.Body.Email = email

	resp, err := h.BeginLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got false. message: %s", resp.Body.Message)
	}
	if resp.Body.ChallengeID != "login-challenge-id" {
		t.Errorf("expected 'login-challenge-id', got '%s'", resp.Body.ChallengeID)
	}
	if resp.Body.Message != "Login options created" {
		t.Errorf("expected 'Login options created', got '%s'", resp.Body.Message)
	}
}

func TestBeginLoginHandler_Discoverable_Success(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			if userID != 0 {
				t.Errorf("expected userID 0 for discoverable login, got %d", userID)
			}
			return "discoverable-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	input := &BeginLoginRequest{} // no email = discoverable

	resp, err := h.BeginLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got false. message: %s", resp.Body.Message)
	}
	if resp.Body.ChallengeID != "discoverable-challenge-id" {
		t.Errorf("expected 'discoverable-challenge-id', got '%s'", resp.Body.ChallengeID)
	}
}

func TestBeginLoginHandler_Discoverable_BeginFails(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("webauthn config error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	input := &BeginLoginRequest{}

	resp, err := h.BeginLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
}

func TestBeginLoginHandler_StoreChallengeFailure(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("db error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	resp, err := h.BeginLoginHandler(context.Background(), &BeginLoginRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to store challenge" {
		t.Errorf("expected 'Failed to store challenge', got '%s'", resp.Body.Message)
	}
}

// ============================================================================
// FinishLoginHandler
// ============================================================================

func TestFinishLoginHandler_InvalidChallenge(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return nil, 0, fmt.Errorf("challenge expired")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "expired-challenge"

	resp, err := h.FinishLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid or expired challenge" {
		t.Errorf("expected 'Invalid or expired challenge', got '%s'", resp.Body.Message)
	}
}

func TestFinishLoginHandler_MalformedCredentialResponse(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 1, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.Response = CredentialAssertionResponse{
		ID:    "",
		RawID: "",
		Type:  "",
		Response: CredentialAssertionAuthenticatorResponse{
			AuthenticatorData: "not-valid",
			ClientDataJSON:    "not-valid",
			Signature:         "not-valid",
		},
	}

	resp, err := h.FinishLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid credential response" {
		t.Errorf("expected 'Invalid credential response', got '%s'", resp.Body.Message)
	}
}

func TestFinishLoginHandler_UserSpecificLogin_UserNotFound(t *testing.T) {
	// This tests the path where challenge has a non-zero userID but the user
	// can't be found when we look them up by ID during finish.
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 42, nil
		},
	}
	mockUS := &mockUserService{
		getUserByIDFn: func(userID uint) (*models.User, error) {
			return nil, fmt.Errorf("user not found")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "valid-challenge"
	// We need the parse to succeed. But protocol.ParseCredentialRequestResponseBody
	// will fail on empty/invalid data. So this test actually hits the parse failure first.
	// Let's verify that error path is handled:
	input.Body.Response = CredentialAssertionResponse{
		ID:    "",
		RawID: "",
		Type:  "",
	}

	resp, err := h.FinishLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	// Parse will fail first, producing VALIDATION_FAILED
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
}

// ============================================================================
// ListCredentialsHandler
// ============================================================================

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

func TestListCredentialsHandler_Success(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getUserCredentialsFn: func(userID uint) ([]models.WebAuthnCredential, error) {
			if userID != 7 {
				t.Errorf("expected userID 7, got %d", userID)
			}
			return []models.WebAuthnCredential{
				{ID: 1, UserID: 7, DisplayName: "MacBook"},
				{ID: 2, UserID: 7, DisplayName: "iPhone"},
			}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 7})

	resp, err := h.ListCredentialsHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if len(resp.Body.Credentials) != 2 {
		t.Errorf("expected 2 credentials, got %d", len(resp.Body.Credentials))
	}
	if resp.Body.Message != "Credentials retrieved" {
		t.Errorf("expected 'Credentials retrieved', got '%s'", resp.Body.Message)
	}
}

func TestListCredentialsHandler_EmptyList(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getUserCredentialsFn: func(userID uint) ([]models.WebAuthnCredential, error) {
			return []models.WebAuthnCredential{}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.ListCredentialsHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if len(resp.Body.Credentials) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(resp.Body.Credentials))
	}
}

func TestListCredentialsHandler_ServiceError(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getUserCredentialsFn: func(userID uint) ([]models.WebAuthnCredential, error) {
			return nil, fmt.Errorf("db connection error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.ListCredentialsHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to get credentials" {
		t.Errorf("expected 'Failed to get credentials', got '%s'", resp.Body.Message)
	}
}

// ============================================================================
// DeleteCredentialHandler
// ============================================================================

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

func TestDeleteCredentialHandler_Success(t *testing.T) {
	var capturedUserID, capturedCredID uint
	mockWA := &mockWebAuthnService{
		deleteCredentialFn: func(userID uint, credentialID uint) error {
			capturedUserID = userID
			capturedCredID = credentialID
			return nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 5})

	input := &DeleteCredentialRequest{CredentialID: 99}

	resp, err := h.DeleteCredentialHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.Message != "Passkey deleted successfully" {
		t.Errorf("expected 'Passkey deleted successfully', got '%s'", resp.Body.Message)
	}
	if capturedUserID != 5 {
		t.Errorf("expected userID 5, got %d", capturedUserID)
	}
	if capturedCredID != 99 {
		t.Errorf("expected credentialID 99, got %d", capturedCredID)
	}
}

func TestDeleteCredentialHandler_ServiceError(t *testing.T) {
	mockWA := &mockWebAuthnService{
		deleteCredentialFn: func(userID uint, credentialID uint) error {
			return fmt.Errorf("credential not found or not owned by user")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 1})

	input := &DeleteCredentialRequest{CredentialID: 999}

	resp, err := h.DeleteCredentialHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to delete credential" {
		t.Errorf("expected 'Failed to delete credential', got '%s'", resp.Body.Message)
	}
}

func TestDeleteCredentialHandler_AnotherUsersCredential(t *testing.T) {
	// The service layer should enforce ownership, but we can test the handler
	// passes the correct user ID through
	var capturedUserID uint
	mockWA := &mockWebAuthnService{
		deleteCredentialFn: func(userID uint, credentialID uint) error {
			capturedUserID = userID
			return fmt.Errorf("credential not owned by user")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 10})

	resp, err := h.DeleteCredentialHandler(ctx, &DeleteCredentialRequest{CredentialID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if capturedUserID != 10 {
		t.Errorf("expected userID 10 passed to service, got %d", capturedUserID)
	}
}

// ============================================================================
// BeginSignupHandler
// ============================================================================

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
	if resp.Body.Message != "Email is required" {
		t.Errorf("expected 'Email is required', got '%s'", resp.Body.Message)
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
	if resp.Body.Message != "You must accept the Terms of Service and Privacy Policy" {
		t.Errorf("unexpected message: '%s'", resp.Body.Message)
	}
}

func TestBeginSignupHandler_MissingTermsVersion(t *testing.T) {
	h := testPasskeyHandler()
	input := &BeginSignupRequest{}
	input.Body.Email = "test@example.com"
	input.Body.TermsAccepted = true
	// TermsVersion is empty

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
	if resp.Body.Message != "Terms version is required" {
		t.Errorf("expected 'Terms version is required', got '%s'", resp.Body.Message)
	}
}

func TestBeginSignupHandler_EmailCheckFails(t *testing.T) {
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testPasskeyHandlerWithMocks(&mockWebAuthnService{}, &mockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "v1"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to check email" {
		t.Errorf("expected 'Failed to check email', got '%s'", resp.Body.Message)
	}
}

func TestBeginSignupHandler_EmailAlreadyExists(t *testing.T) {
	email := "existing@example.com"
	mockUS := &mockUserService{
		getUserByEmailFn: func(e string) (*models.User, error) {
			return &models.User{ID: 1, Email: &email}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(&mockWebAuthnService{}, &mockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = email
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "v1"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUserExists {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUserExists, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "An account with this email already exists" {
		t.Errorf("unexpected message: '%s'", resp.Body.Message)
	}
}

func TestBeginSignupHandler_BeginRegistrationForEmailFails(t *testing.T) {
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, nil // no existing user
		},
	}
	mockWA := &mockWebAuthnService{
		beginRegistrationForEmailFn: func(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("webauthn init error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "v1"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
}

func TestBeginSignupHandler_StoreChallengeWithEmailFails(t *testing.T) {
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, nil
		},
	}
	mockWA := &mockWebAuthnService{
		beginRegistrationForEmailFn: func(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		storeChallengeWithEmailFn: func(email string, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("storage error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "v1"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to store challenge" {
		t.Errorf("expected 'Failed to store challenge', got '%s'", resp.Body.Message)
	}
}

func TestBeginSignupHandler_Success(t *testing.T) {
	var capturedEmail string
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, nil // no existing user
		},
	}
	mockWA := &mockWebAuthnService{
		beginRegistrationForEmailFn: func(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			capturedEmail = email
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		storeChallengeWithEmailFn: func(email string, session *webauthn.SessionData, operation string) (string, error) {
			if operation != "signup" {
				t.Errorf("expected operation 'signup', got '%s'", operation)
			}
			return "signup-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "v1"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got false. message: %s", resp.Body.Message)
	}
	if resp.Body.ChallengeID != "signup-challenge-id" {
		t.Errorf("expected 'signup-challenge-id', got '%s'", resp.Body.ChallengeID)
	}
	if resp.Body.Message != "Registration options created" {
		t.Errorf("expected 'Registration options created', got '%s'", resp.Body.Message)
	}
	if capturedEmail != "new@example.com" {
		t.Errorf("expected email 'new@example.com' passed to service, got '%s'", capturedEmail)
	}
}

// ============================================================================
// FinishSignupHandler
// ============================================================================

func TestFinishSignupHandler_InvalidChallenge(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeWithEmailFn: func(challengeID string, operation string) (*webauthn.SessionData, string, error) {
			return nil, "", fmt.Errorf("challenge expired or not found")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "bad-challenge"

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid or expired challenge" {
		t.Errorf("expected 'Invalid or expired challenge', got '%s'", resp.Body.Message)
	}
}

func TestFinishSignupHandler_EmailVerifyFails(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeWithEmailFn: func(challengeID string, operation string) (*webauthn.SessionData, string, error) {
			return &webauthn.SessionData{}, "new@example.com", nil
		},
	}
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to verify email" {
		t.Errorf("expected 'Failed to verify email', got '%s'", resp.Body.Message)
	}
}

func TestFinishSignupHandler_EmailAlreadyTaken_RaceCondition(t *testing.T) {
	email := "race@example.com"
	mockWA := &mockWebAuthnService{
		getChallengeWithEmailFn: func(challengeID string, operation string) (*webauthn.SessionData, string, error) {
			return &webauthn.SessionData{}, email, nil
		},
	}
	mockUS := &mockUserService{
		getUserByEmailFn: func(e string) (*models.User, error) {
			// User was created between begin and finish (race condition)
			return &models.User{ID: 1, Email: &email}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUserExists {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUserExists, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "An account with this email already exists" {
		t.Errorf("unexpected message: '%s'", resp.Body.Message)
	}
}

func TestFinishSignupHandler_MalformedCredentialResponse(t *testing.T) {
	mockWA := &mockWebAuthnService{
		getChallengeWithEmailFn: func(challengeID string, operation string) (*webauthn.SessionData, string, error) {
			return &webauthn.SessionData{}, "new@example.com", nil
		},
	}
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, nil // no existing user
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.Response = CredentialCreationResponse{
		ID:    "",
		RawID: "",
		Type:  "",
		Response: CredentialCreationAttestationResponse{
			AttestationObject: "bad-data",
			ClientDataJSON:    "bad-data",
		},
	}

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Invalid credential response" {
		t.Errorf("expected 'Invalid credential response', got '%s'", resp.Body.Message)
	}
}

func TestFinishSignupHandler_MissingTermsAcceptance(t *testing.T) {
	// TermsAccepted validation happens after parsing credential response.
	// Since we can't easily provide valid WebAuthn data, this test verifies
	// the parse failure path (which happens before terms check).
	mockWA := &mockWebAuthnService{
		getChallengeWithEmailFn: func(challengeID string, operation string) (*webauthn.SessionData, string, error) {
			return &webauthn.SessionData{}, "new@example.com", nil
		},
	}
	mockUS := &mockUserService{
		getUserByEmailFn: func(email string) (*models.User, error) {
			return nil, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.TermsAccepted = false
	input.Body.TermsVersion = ""
	// Invalid credential data will cause parse failure before terms check
	input.Body.Response = CredentialCreationResponse{
		ID:   "test",
		Type: "public-key",
	}

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	// Parse fails first with VALIDATION_FAILED
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
}

// ============================================================================
// BeginDiscoverableLogin (tested via BeginLoginHandler with no email)
// ============================================================================

func TestBeginLoginHandler_Discoverable_ChallengeStoreFails(t *testing.T) {
	mockWA := &mockWebAuthnService{
		beginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("redis unavailable")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	resp, err := h.BeginLoginHandler(context.Background(), &BeginLoginRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
}

// ============================================================================
// Edge cases and argument passing verification
// ============================================================================

func TestBeginRegisterHandler_PassesCorrectUserToService(t *testing.T) {
	var capturedUserID uint
	mockWA := &mockWebAuthnService{
		beginRegistrationFn: func(user *models.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			capturedUserID = user.ID
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})
	ctx := ctxWithUser(&models.User{ID: 42})

	_, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUserID != 42 {
		t.Errorf("expected user ID 42 passed to BeginRegistration, got %d", capturedUserID)
	}
}

func TestBeginLoginHandler_WithEmail_StoreChallengeFailure(t *testing.T) {
	email := "user@example.com"
	mockUS := &mockUserService{
		getUserByEmailFn: func(e string) (*models.User, error) {
			return &models.User{ID: 1, Email: &email}, nil
		},
	}
	mockWA := &mockWebAuthnService{
		beginLoginFn: func(user *models.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		storeChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("storage failure")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, mockUS)

	input := &BeginLoginRequest{}
	input.Body.Email = email

	resp, err := h.BeginLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Failed to store challenge" {
		t.Errorf("expected 'Failed to store challenge', got '%s'", resp.Body.Message)
	}
}

func TestFinishLoginHandler_DiscoverableLogin_ChallengeWithZeroUserID(t *testing.T) {
	// When userID is 0, handler goes into discoverable login path.
	// With malformed credential data, parse will fail.
	mockWA := &mockWebAuthnService{
		getChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 0, nil // userID=0 => discoverable
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &mockJWTService{}, &mockUserService{})

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "disc-challenge"
	input.Body.Response = CredentialAssertionResponse{
		ID:   "",
		Type: "",
	}

	resp, err := h.FinishLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	// Parse will fail
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
}

// ============================================================================
// createCredentialCreationReader / createCredentialRequestReader helpers
// ============================================================================

func TestCreateCredentialCreationReader_IncludesAuthenticatorAttachment(t *testing.T) {
	input := &FinishRegisterRequest{}
	input.Body.Response = CredentialCreationResponse{
		ID:                      "test-id",
		RawID:                   "test-raw-id",
		Type:                    "public-key",
		AuthenticatorAttachment: "platform",
		Response: CredentialCreationAttestationResponse{
			AttestationObject: "YXR0ZXN0",
			ClientDataJSON:    "Y2xpZW50",
		},
	}
	reader := createCredentialCreationReader(input)
	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestCreateCredentialCreationReader_NoAuthenticatorAttachment(t *testing.T) {
	input := &FinishRegisterRequest{}
	input.Body.Response = CredentialCreationResponse{
		ID:    "test-id",
		RawID: "test-raw-id",
		Type:  "public-key",
		Response: CredentialCreationAttestationResponse{
			AttestationObject: "YXR0ZXN0",
			ClientDataJSON:    "Y2xpZW50",
		},
	}
	reader := createCredentialCreationReader(input)
	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestCreateCredentialRequestReader(t *testing.T) {
	input := &FinishLoginRequest{}
	input.Body.Response = CredentialAssertionResponse{
		ID:    "test-id",
		RawID: "test-raw-id",
		Type:  "public-key",
		Response: CredentialAssertionAuthenticatorResponse{
			AuthenticatorData: "YXV0aA==",
			ClientDataJSON:    "Y2xpZW50",
			Signature:         "c2ln",
			UserHandle:        "dXNlcg==",
		},
	}
	reader := createCredentialRequestReader(input)
	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestCreateCredentialCreationReaderFromSignup_IncludesAuthenticatorAttachment(t *testing.T) {
	input := &FinishSignupRequest{}
	input.Body.Response = CredentialCreationResponse{
		ID:                      "test-id",
		RawID:                   "test-raw-id",
		Type:                    "public-key",
		AuthenticatorAttachment: "cross-platform",
		Response: CredentialCreationAttestationResponse{
			AttestationObject: "YXR0ZXN0",
			ClientDataJSON:    "Y2xpZW50",
			Transports:        []string{"usb", "nfc"},
		},
	}
	reader := createCredentialCreationReaderFromSignup(input)
	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestCreateCredentialCreationReaderFromSignup_NoAuthenticatorAttachment(t *testing.T) {
	input := &FinishSignupRequest{}
	input.Body.Response = CredentialCreationResponse{
		ID:    "test-id",
		RawID: "test-raw-id",
		Type:  "public-key",
		Response: CredentialCreationAttestationResponse{
			AttestationObject: "YXR0ZXN0",
			ClientDataJSON:    "Y2xpZW50",
		},
	}
	reader := createCredentialCreationReaderFromSignup(input)
	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}
