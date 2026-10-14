package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Helper to build a PasskeyHandler with mocks
// ============================================================================

func testPasskeyHandler() *PasskeyHandler {
	return NewPasskeyHandler(nil, nil, nil, nil, testConfig())
}

// malformedCredentialJSON is a credential body the WebAuthn parser rejects.
var malformedCredentialJSON = json.RawMessage(`{"id":"","type":"","response":{"clientDataJSON":"not-valid"}}`)

// testPasskeyHandlerWithMocks builds a handler whose email service is present
// but unconfigured (MockEmailService.IsConfigured defaults to false), so the
// signup verification send short-circuits without touching the mock.
func testPasskeyHandlerWithMocks(wa *testhelpers.MockWebAuthnService, jwt *testhelpers.MockJWTService, us *testhelpers.MockUserService) *PasskeyHandler {
	return NewPasskeyHandler(wa, jwt, us, &testhelpers.MockEmailService{}, testConfig())
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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationFn: func(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			if operation != "registration" {
				t.Errorf("expected operation 'registration', got '%s'", operation)
			}
			return "test-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now())

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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationFn: func(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("webauthn init error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now())

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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationFn: func(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("storage error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now())

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeRegistrationChallengeFn: func(string, uint) (*webauthn.SessionData, error) {
			return nil, autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", fmt.Errorf("challenge not found"))
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now())

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
	var consumedFor uint
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeRegistrationChallengeFn: func(_ string, userID uint) (*webauthn.SessionData, error) {
			consumedFor = userID
			return nil, autherrors.NewAuthError(autherrors.CodeUnauthorized, "Challenge belongs to different user", nil)
		},
		FinishRegistrationFn: func(*authm.User, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string) (*authm.WebAuthnCredential, error) {
			t.Fatal("a refused challenge must not reach registration")
			return nil, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now())

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
	if consumedFor != 1 {
		t.Errorf("challenge consumed for user %d, want the signed-in user 1", consumedFor)
	}
}

func TestFinishRegisterHandler_MalformedCredentialResponse(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeRegistrationChallengeFn: func(string, uint) (*webauthn.SessionData, error) {
			return &webauthn.SessionData{}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Now())

	input := &FinishRegisterRequest{}
	input.Body.ChallengeID = "valid-challenge"
	// Provide invalid/empty credential data — will fail parsing
	input.Body.Response = malformedCredentialJSON

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

// ============================================================================
// BeginLoginHandler
// ============================================================================

func TestBeginLoginHandler_WithEmail_UserNotFound(t *testing.T) {
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, fmt.Errorf("user not found")
		},
	}
	h := testPasskeyHandlerWithMocks(&testhelpers.MockWebAuthnService{}, &testhelpers.MockJWTService{}, mockUS)

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
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &email}, nil
		},
	}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginLoginFn: func(user *authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("no credentials registered")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

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
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(e string) (*authm.User, error) {
			return &authm.User{ID: 42, Email: &email}, nil
		},
	}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginLoginFn: func(user *authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			if user.ID != 42 {
				t.Errorf("expected user ID 42, got %d", user.ID)
			}
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			if userID != 42 {
				t.Errorf("expected userID 42, got %d", userID)
			}
			if operation != "authentication" {
				t.Errorf("expected operation 'authentication', got '%s'", operation)
			}
			return "login-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			if userID != 0 {
				t.Errorf("expected userID 0 for discoverable login, got %d", userID)
			}
			return "discoverable-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("webauthn config error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("db error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return nil, 0, autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", fmt.Errorf("challenge expired"))
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 1, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.Response = malformedCredentialJSON

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

// The service decides an email-specific sign-in for the challenge's account,
// a missing account included; the handler loads the account only for a
// sign-in the service accepted.
func TestFinishLoginHandler_UserSpecificLogin_UserNotFound(t *testing.T) {
	_, device := softRegistration(t)
	body := softAssertion(t, device)
	var decidedFor uint
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 42, nil
		},
		FinishLoginFn: func(user *authm.User, _ *webauthn.SessionData, _ *protocol.ParsedCredentialAssertionData) (*authm.WebAuthnCredential, error) {
			decidedFor = user.ID
			return nil, autherrors.NewAuthError(autherrors.CodeInvalidCredentials, "User not found", fmt.Errorf("record not found"))
		},
	}
	mockUS := &testhelpers.MockUserService{
		GetUserByIDFn: func(userID uint) (*authm.User, error) {
			t.Error("a refused sign-in must not load the account")
			return nil, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.Response = body

	resp, err := h.FinishLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "User not found" {
		t.Errorf("expected the refusal's own message, got %q", resp.Body.Message)
	}
	if decidedFor != 42 {
		t.Errorf("sign-in decided for user %d, want the challenge's user 42", decidedFor)
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
	mockWA := &testhelpers.MockWebAuthnService{
		GetUserCredentialsFn: func(userID uint) ([]authm.WebAuthnCredential, error) {
			if userID != 7 {
				t.Errorf("expected userID 7, got %d", userID)
			}
			return []authm.WebAuthnCredential{
				{ID: 1, UserID: 7, DisplayName: "MacBook"},
				{ID: 2, UserID: 7, DisplayName: "iPhone"},
			}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})

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
	mockWA := &testhelpers.MockWebAuthnService{
		GetUserCredentialsFn: func(userID uint) ([]authm.WebAuthnCredential, error) {
			return []authm.WebAuthnCredential{}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

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
	mockWA := &testhelpers.MockWebAuthnService{
		GetUserCredentialsFn: func(userID uint) ([]authm.WebAuthnCredential, error) {
			return nil, fmt.Errorf("db connection error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

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
	mockWA := &testhelpers.MockWebAuthnService{
		DeleteCredentialFn: func(userID uint, credentialID uint) error {
			capturedUserID = userID
			capturedCredID = credentialID
			return nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 5})

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
	mockWA := &testhelpers.MockWebAuthnService{
		DeleteCredentialFn: func(userID uint, credentialID uint) error {
			return fmt.Errorf("credential not found or not owned by user")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

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
	mockWA := &testhelpers.MockWebAuthnService{
		DeleteCredentialFn: func(userID uint, credentialID uint) error {
			capturedUserID = userID
			return fmt.Errorf("credential not owned by user")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 10})

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
	want := autherrors.ErrTermsAcceptanceRequired("")
	if resp.Body.ErrorCode != want.Code {
		t.Errorf("expected error_code=%s, got %s", want.Code, resp.Body.ErrorCode)
	}
	if resp.Body.Message != want.UserMessage() {
		t.Errorf("expected %q, got %q", want.UserMessage(), resp.Body.Message)
	}
}

// TestBeginSignupHandler_MissingAgeConfirmation mirrors the terms-rejection
// tests (PSY-1023): passkey signup must be blocked when the age confirmation is
// absent, even with terms accepted.
func TestBeginSignupHandler_MissingAgeConfirmation(t *testing.T) {
	h := testPasskeyHandler()
	input := &BeginSignupRequest{}
	input.Body.Email = "test@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = false

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeAgeConfirmationRequired {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeAgeConfirmationRequired, resp.Body.ErrorCode)
	}
}

// TestBeginSignupHandler_AgeBelowMinimum guards the server-side age floor for
// the passkey path.
func TestBeginSignupHandler_AgeBelowMinimum(t *testing.T) {
	h := testPasskeyHandler()
	input := &BeginSignupRequest{}
	input.Body.Email = "test@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge - 1

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeAgeConfirmationRequired {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeAgeConfirmationRequired, resp.Body.ErrorCode)
	}
}

func TestBeginSignupHandler_EmailCheckFails(t *testing.T) {
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testPasskeyHandlerWithMocks(&testhelpers.MockWebAuthnService{}, &testhelpers.MockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

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
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &email}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(&testhelpers.MockWebAuthnService{}, &testhelpers.MockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = email
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

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
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, nil // no existing user
		},
	}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return nil, nil, fmt.Errorf("webauthn init error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

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

func TestBeginSignupHandler_StoreSignupChallengeFails(t *testing.T) {
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, nil
		},
	}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreSignupChallengeFn: func(signup contracts.SignupChallenge, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("storage error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

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
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, nil // no existing user
		},
	}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			capturedEmail = email
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreSignupChallengeFn: func(signup contracts.SignupChallenge, session *webauthn.SessionData, operation string) (string, error) {
			if operation != "signup" {
				t.Errorf("expected operation 'signup', got '%s'", operation)
			}
			return "signup-challenge-id", nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &BeginSignupRequest{}
	input.Body.Email = "new@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return nil, contracts.SignupChallenge{}, fmt.Errorf("challenge expired or not found")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, currentSignupChallenge("new@example.com"), nil
		},
	}
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, currentSignupChallenge(email), nil
		},
	}
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(e string) (*authm.User, error) {
			// User was created between begin and finish (race condition)
			return &authm.User{ID: 1, Email: &email}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

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
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, currentSignupChallenge("new@example.com"), nil
		},
	}
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, nil // no existing user
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.Response = malformedCredentialJSON

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

// A challenge that carries no consent is refused however the finish body
// fills its consent fields, before any account is created.
func TestFinishSignupHandler_ChallengeWithoutConsent(t *testing.T) {
	body, _ := softRegistration(t)
	var finished bool
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, contracts.SignupChallenge{Email: "new@example.com"}, nil
		},
		FinishSignupRegistrationWithLegalFn: func(string, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string, contracts.LegalAcceptance) (*authm.User, error) {
			finished = true
			return &authm.User{ID: 9}, nil
		},
	}
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(email string) (*authm.User, error) {
			return nil, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge
	input.Body.Response = body

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || finished {
		t.Error("expected the signup to be refused before the account is created")
	}
	if resp.Body.ErrorCode != autherrors.CodeTermsAcceptanceRequired {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeTermsAcceptanceRequired, resp.Body.ErrorCode)
	}
}

// ============================================================================
// BeginDiscoverableLogin (tested via BeginLoginHandler with no email)
// ============================================================================

func TestBeginLoginHandler_Discoverable_ChallengeStoreFails(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		BeginDiscoverableLoginFn: func() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("redis unavailable")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

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
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationFn: func(user *authm.User) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			capturedUserID = user.ID
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 42}, time.Now())

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
	mockUS := &testhelpers.MockUserService{
		GetUserByEmailFn: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &email}, nil
		},
	}
	mockWA := &testhelpers.MockWebAuthnService{
		BeginLoginFn: func(user *authm.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
			return &protocol.CredentialAssertion{}, &webauthn.SessionData{}, nil
		},
		StoreChallengeFn: func(userID uint, session *webauthn.SessionData, operation string) (string, error) {
			return "", fmt.Errorf("storage failure")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

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

// A challenge issued with no user is a discoverable login; an assertion the
// service refuses is answered as invalid credentials, with no session.
func TestFinishLoginHandler_DiscoverableLogin_RefusedAssertion(t *testing.T) {
	_, device := softRegistration(t)
	var reachedDiscoverable bool
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, uint, error) {
			return &webauthn.SessionData{}, 0, nil
		},
		FinishDiscoverableLoginFn: func(*webauthn.SessionData, *protocol.ParsedCredentialAssertionData) (*authm.User, *authm.WebAuthnCredential, error) {
			reachedDiscoverable = true
			return nil, nil, autherrors.NewAuthError(autherrors.CodeInvalidCredentials, "Invalid passkey", fmt.Errorf("validate discoverable login"))
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

	input := &FinishLoginRequest{}
	input.Body.ChallengeID = "disc-challenge"
	input.Body.Response = softAssertion(t, device)

	resp, err := h.FinishLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reachedDiscoverable {
		t.Fatal("expected the discoverable login path")
	}
	if resp.Body.Success || resp.SetCookie.Value != "" {
		t.Error("expected no session")
	}
	if resp.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
}

// TestBeginSignupHandler_RejectsMalformedEmail covers the second address-write
// path: the address stored in the challenge here is what FinishSignupHandler
// writes to the users row, so it is refused before the lookup that would
// otherwise be the first thing a malformed address reaches.
func TestBeginSignupHandler_RejectsMalformedEmail(t *testing.T) {
	cases := []struct {
		name  string
		email string
	}{
		{"display-name form", "Name <a@b.com>"},
		{"header injection", "a@b.com\r\nBcc: evil@example.com"},
		{"rfc comment", "a@b.com (comment)"},
		{"no at sign", "not-an-email"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var lookedUp bool
			us := &testhelpers.MockUserService{
				GetUserByEmailFn: func(email string) (*authm.User, error) {
					lookedUp = true
					return nil, nil
				},
			}
			h := testPasskeyHandlerWithMocks(nil, nil, us)
			input := &BeginSignupRequest{}
			input.Body.Email = tc.email
			input.Body.TermsAccepted = true
			input.Body.TermsVersion = legal.CurrentTermsVersion
			input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
			input.Body.AgeConfirmed = true
			input.Body.MinAgeAttested = MinSignupAge

			resp, err := h.BeginSignupHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
				t.Errorf("expected validation failure, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
			}
			if lookedUp {
				t.Error("expected no user lookup for a refused address")
			}
		})
	}
}
