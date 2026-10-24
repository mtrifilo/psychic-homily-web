package auth

import (
	"context"
	"encoding/json"
	"errors"
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
	"psychic-homily-backend/internal/testutil/signupconsent"
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

// requireServiceUnavailableError asserts a handler answered with the
// SERVICE_UNAVAILABLE error the API layer writes as a 503.
func requireServiceUnavailableError(t *testing.T, err error) {
	t.Helper()
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) || authErr.Code != autherrors.CodeServiceUnavailable {
		t.Fatalf("expected a %s error, got %v", autherrors.CodeServiceUnavailable, err)
	}
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

	_, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	requireServiceUnavailableError(t, err)
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

	_, err := h.BeginRegisterHandler(ctx, &BeginRegisterRequest{})
	requireServiceUnavailableError(t, err)
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

	input := &BeginLoginRequest{}

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

// ============================================================================
// FinishLoginHandler
// ============================================================================

func TestFinishLoginHandler_InvalidChallenge(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, error) {
			return nil, autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", fmt.Errorf("challenge expired"))
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
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, error) {
			return &webauthn.SessionData{}, nil
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

	_, err := h.ListCredentialsHandler(ctx, &struct{}{})
	requireServiceUnavailableError(t, err)
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

func TestDeleteCredentialHandler_CredentialNotHeld(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		DeleteCredentialFn: func(userID uint, credentialID uint) error {
			return autherrors.ErrCredentialNotHeld(fmt.Errorf("credential not held"))
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

func TestDeleteCredentialHandler_ServiceError(t *testing.T) {
	mockWA := &testhelpers.MockWebAuthnService{
		DeleteCredentialFn: func(userID uint, credentialID uint) error {
			return fmt.Errorf("failed to delete credential: db connection error")
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteCredentialHandler(ctx, &DeleteCredentialRequest{CredentialID: 999})
	requireServiceUnavailableError(t, err)
}

func TestDeleteCredentialHandler_AnotherUsersCredential(t *testing.T) {
	// The service layer should enforce ownership, but we can test the handler
	// passes the correct user ID through
	var capturedUserID uint
	mockWA := &testhelpers.MockWebAuthnService{
		DeleteCredentialFn: func(userID uint, credentialID uint) error {
			capturedUserID = userID
			return autherrors.ErrCredentialNotHeld(fmt.Errorf("credential not held"))
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

// beginSignupRequestWithConsent is a passkey signup/begin request that passes
// every check before the age one.
func beginSignupRequestWithConsent() *BeginSignupRequest {
	input := &BeginSignupRequest{}
	input.Body.Email = "test@example.com"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	return input
}

// A signup/begin without a date of birth is refused for its age, whatever
// legacy attestation fields it carries, and sets no refusal marker.
func TestBeginSignupHandler_RefusesWithoutDateOfBirth(t *testing.T) {
	cases := map[string]func(*BeginSignupRequest){
		"no age at all": func(*BeginSignupRequest) {},
		"legacy attestation fields only": func(in *BeginSignupRequest) {
			in.Body.AgeConfirmed = true
			in.Body.MinAgeAttested = legal.MinSignupAge
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := testPasskeyHandler()
			input := beginSignupRequestWithConsent()
			mutate(input)

			resp, err := h.BeginSignupHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeAgeConfirmationRequired {
				t.Fatalf("got success=%v code=%q, want %s", resp.Body.Success, resp.Body.ErrorCode, autherrors.CodeAgeConfirmationRequired)
			}
			if resp.SetCookie.Name != "" {
				t.Errorf("set cookies %+v; a request that gave no age leaves no marker", resp.SetCookie)
			}
		})
	}
}

// A date of birth under the minimum age is refused before any challenge is
// issued, and sets the refusal marker.
func TestBeginSignupHandler_UnderAgeSetsRefusalMarker(t *testing.T) {
	h := testPasskeyHandler()
	input := beginSignupRequestWithConsent()
	input.Body.DateOfBirth = signupconsent.UnderAgeDateOfBirth(time.Now())

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeAgeConfirmationRequired {
		t.Fatalf("got success=%v code=%q, want %s", resp.Body.Success, resp.Body.ErrorCode, autherrors.CodeAgeConfirmationRequired)
	}
	if resp.Body.ChallengeID != "" || resp.Body.Options != nil {
		t.Error("a challenge was issued for a refused sign-up")
	}
	if !refusalMarkerSet(resp.SetCookie) {
		t.Errorf("Set-Cookie = %+v, want the refusal marker alone", resp.SetCookie)
	}
}

// A browser carrying the refusal marker is refused for its age even with an
// adult date of birth.
func TestBeginSignupHandler_RefusesBrowserCarryingMarker(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(fmt.Sprintf("secure=%v", secure), func(t *testing.T) {
			cfg := testConfig()
			cfg.Session.Secure = secure
			h := NewPasskeyHandler(nil, nil, nil, nil, cfg)
			input := beginSignupRequestWithConsent()
			input.Body.DateOfBirth = signupconsent.AdultDateOfBirth
			if secure {
				input.AgeRefusedHost = "1"
			} else {
				input.AgeRefused = "1"
			}

			resp, err := h.BeginSignupHandler(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Body.ErrorCode != autherrors.CodeAgeConfirmationRequired {
				t.Fatalf("code = %q, want %s", resp.Body.ErrorCode, autherrors.CodeAgeConfirmationRequired)
			}
			if resp.SetCookie.Name != signupAgeRefusalCookieNameFor(secure) {
				t.Errorf("Set-Cookie = %+v, want the marker renewed as %q", resp.SetCookie, signupAgeRefusalCookieNameFor(secure))
			}
		})
	}
}

// A date of birth that is not a plausible calendar date is refused with the
// form's sentence and no marker.
func TestBeginSignupHandler_RefusesImplausibleDateOfBirth(t *testing.T) {
	h := testPasskeyHandler()
	input := beginSignupRequestWithConsent()
	input.Body.DateOfBirth = "2001-02-30"

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed || resp.Body.Message != dateOfBirthInvalidMessage {
		t.Errorf("got code=%q message=%q, want %s %q", resp.Body.ErrorCode, resp.Body.Message, autherrors.CodeValidationFailed, dateOfBirthInvalidMessage)
	}
	if resp.SetCookie.Name != "" {
		t.Errorf("set cookies %+v on a field refusal", resp.SetCookie)
	}
}

// The challenge signup/begin stores carries the date of birth and the age
// computed from it, so finish records them.
func TestBeginSignupHandler_StoresDateOfBirthInChallenge(t *testing.T) {
	var stored contracts.SignupChallenge
	mockWA := &testhelpers.MockWebAuthnService{
		BeginRegistrationForEmailFn: func(string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
			return &protocol.CredentialCreation{}, &webauthn.SessionData{}, nil
		},
		StoreSignupChallengeFn: func(signup contracts.SignupChallenge, _ *webauthn.SessionData, _ string) (string, error) {
			stored = signup
			return "signup-challenge-id", nil
		},
	}
	mockUS := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)
	input := beginSignupRequestWithConsent()
	input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

	resp, err := h.BeginSignupHandler(context.Background(), input)
	if err != nil || !resp.Body.Success {
		t.Fatalf("begin refused: err=%v code=%q", err, resp.Body.ErrorCode)
	}
	wantDate, wantAge := signupconsent.AdultDateOfBirthAt(time.Now())
	acceptance := stored.Acceptance
	if acceptance.DateOfBirth == nil || !acceptance.DateOfBirth.Equal(wantDate) {
		t.Errorf("stored DateOfBirth = %v, want %v", acceptance.DateOfBirth, wantDate)
	}
	if acceptance.MinAgeAttested != wantAge || acceptance.AgeConfirmedAt.IsZero() {
		t.Errorf("stored age %d at %v, want %d at the acceptance instant", acceptance.MinAgeAttested, acceptance.AgeConfirmedAt, wantAge)
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
	input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

	_, err := h.BeginSignupHandler(context.Background(), input)
	requireServiceUnavailableError(t, err)
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
	input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

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
	input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

	_, err := h.BeginSignupHandler(context.Background(), input)
	requireServiceUnavailableError(t, err)
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
	input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

	_, err := h.BeginSignupHandler(context.Background(), input)
	requireServiceUnavailableError(t, err)
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
	input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

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
			return nil, contracts.SignupChallenge{}, autherrors.NewAuthError(autherrors.CodeValidationFailed, "Invalid or expired challenge", fmt.Errorf("challenge not found"))
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

// A browser refused for its age after a ceremony began cannot finish it: the
// marker refuses finish before the challenge is spent.
func TestFinishSignupHandler_RefusesBrowserCarryingMarker(t *testing.T) {
	consumed := false
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			consumed = true
			return &webauthn.SessionData{}, currentSignupChallenge("new@example.com"), nil
		},
	}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, &testhelpers.MockUserService{})

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.AgeRefused = "1"

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeAgeConfirmationRequired {
		t.Errorf("got success=%v code=%q, want %s", resp.Body.Success, resp.Body.ErrorCode, autherrors.CodeAgeConfirmationRequired)
	}
	if consumed {
		t.Error("the challenge was spent for a refused browser")
	}
}

// The service's own consent refusal is the client's refusal, coded as such,
// never a fault.
func TestFinishSignupHandler_ServiceTermsRefusalIsReported(t *testing.T) {
	body, _ := softRegistration(t)
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeSignupChallengeFn: func(string, string) (*webauthn.SessionData, contracts.SignupChallenge, error) {
			return &webauthn.SessionData{}, currentSignupChallenge("new@example.com"), nil
		},
		FinishSignupRegistrationWithLegalFn: func(string, *webauthn.SessionData, *protocol.ParsedCredentialCreationData, string, contracts.LegalAcceptance) (*authm.User, error) {
			return nil, autherrors.ErrTermsAcceptanceRequired("injected")
		},
	}
	mockUS := &testhelpers.MockUserService{GetUserByEmailFn: func(string) (*authm.User, error) { return nil, nil }}
	h := testPasskeyHandlerWithMocks(mockWA, &testhelpers.MockJWTService{}, mockUS)

	input := &FinishSignupRequest{}
	input.Body.ChallengeID = "valid-challenge"
	input.Body.Response = body

	resp, err := h.FinishSignupHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error %v; the refusal answers 200", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeTermsAcceptanceRequired {
		t.Errorf("got success=%v code=%q, want %s", resp.Body.Success, resp.Body.ErrorCode, autherrors.CodeTermsAcceptanceRequired)
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

// A challenge issued with no user is a discoverable login; an assertion the
// service refuses is answered as invalid credentials, with no session.
func TestFinishLoginHandler_DiscoverableLogin_RefusedAssertion(t *testing.T) {
	_, device := softRegistration(t)
	var reachedDiscoverable bool
	mockWA := &testhelpers.MockWebAuthnService{
		ConsumeChallengeFn: func(challengeID string, operation string) (*webauthn.SessionData, error) {
			return &webauthn.SessionData{}, nil
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
			input.Body.DateOfBirth = signupconsent.AdultDateOfBirth

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
