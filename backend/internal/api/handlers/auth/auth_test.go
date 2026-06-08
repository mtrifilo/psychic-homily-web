package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
	usersvc "psychic-homily-backend/internal/services/user"
)

// --- helpers ---

func testConfig() *config.Config {
	return &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-at-least-32-characters-long",
			Expiry:    24,
		},
		Session: config.SessionConfig{
			Path:     "/",
			Domain:   "",
			MaxAge:   86400,
			HttpOnly: true,
			Secure:   false,
			SameSite: "lax",
		},
	}
}

func testAuthHandler() *AuthHandler {
	return NewAuthHandler(nil, nil, nil, nil, nil, nil, testConfig())
}

func strPtr(s string) *string {
	return &s
}

func testJWTService() *auth.JWTService {
	return auth.NewJWTService(nil, testConfig(), usersvc.NewUserService(nil))
}

// --- LoginHandler ---

func TestLoginHandler_EmptyCredentials(t *testing.T) {
	h := testAuthHandler()
	input := &LoginRequest{}
	// email and password are zero-value strings (empty)

	resp, err := h.LoginHandler(context.Background(), input)
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

func TestLoginHandler_MissingPassword(t *testing.T) {
	h := testAuthHandler()
	input := &LoginRequest{}
	input.Body.Email = "user@example.com"

	resp, err := h.LoginHandler(context.Background(), input)
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

// --- OAuthLoginHandler ---

func TestOAuthLoginHandler_InvalidProvider(t *testing.T) {
	h := testAuthHandler()
	input := &OAuthLoginRequest{Provider: "facebook"}

	resp, err := h.OAuthLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if !strings.Contains(resp.Body.Message, "Invalid provider") {
		t.Errorf("expected message to contain 'Invalid provider', got %q", resp.Body.Message)
	}
}

func TestOAuthLoginHandler_NilAuthService(t *testing.T) {
	h := testAuthHandler() // authService is nil
	input := &OAuthLoginRequest{Provider: "google"}

	resp, err := h.OAuthLoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if !strings.Contains(resp.Body.Message, "not configured") {
		t.Errorf("expected message about not configured, got %q", resp.Body.Message)
	}
}

// --- LogoutHandler ---

func TestLogoutHandler_Success(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.LogoutHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.Message != "Logout successful" {
		t.Errorf("expected 'Logout successful', got %q", resp.Body.Message)
	}
	// Verify clear cookie
	if resp.SetCookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 (clear cookie), got %d", resp.SetCookie.MaxAge)
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("expected cookie name=%s, got %s", config.AuthCookieName, resp.SetCookie.Name)
	}
}

func TestLogoutHandler_WithUser(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.LogoutHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.SetCookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 (clear cookie), got %d", resp.SetCookie.MaxAge)
	}
}

// --- RefreshTokenHandler ---

// TestRefreshTokenHandler_NilAuthServiceReturnsServerError locks in the
// fail-closed convention for the unwired-auth-service early exit: a nil
// authService is a server-config defect, and the handler must surface it as a
// 5xx (non-nil returned error wrapping an *AuthError of CodeServiceUnavailable)
// so monitoring sees the same uniform shape as the post-service-call failure
// branches. The body shape stays byte-identical with the prior HTTP-200
// SERVICE_UNAVAILABLE path; only the transport status flips.
func TestRefreshTokenHandler_NilAuthServiceReturnsServerError(t *testing.T) {
	h := testAuthHandler() // authService is nil

	resp, err := h.RefreshTokenHandler(context.Background(), &struct{}{})

	// Handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx with the structured body.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape so
	// callers do not see an internal-step leak (e.g. "Auth service not available").
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

func TestRefreshTokenHandler_NoUserContext(t *testing.T) {
	// Need a non-nil authService to pass the nil check
	authSvc := auth.NewAuthService(nil, testConfig(), usersvc.NewUserService(nil))
	h := NewAuthHandler(authSvc, nil, nil, nil, nil, nil, testConfig())

	resp, err := h.RefreshTokenHandler(context.Background(), &struct{}{})
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

// --- GetProfileHandler ---

// TestGetProfileHandler_NilAuthServiceReturnsServerError locks in the
// fail-closed convention for the unwired-auth-service early exit: a nil
// authService is a server-config defect, and the handler must surface it as a
// 5xx (non-nil returned error wrapping an *AuthError of CodeServiceUnavailable)
// so monitoring sees the same uniform shape as the post-service-call failure
// branch. The body shape stays byte-identical with the prior HTTP-200
// SERVICE_UNAVAILABLE path; only the transport status flips.
func TestGetProfileHandler_NilAuthServiceReturnsServerError(t *testing.T) {
	h := testAuthHandler() // authService is nil

	resp, err := h.GetProfileHandler(context.Background(), &struct{}{})

	// Handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx with the structured body.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape so
	// callers do not see an internal-step leak (e.g. "Auth service not available").
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}


func TestGetProfileHandler_NoUserContext(t *testing.T) {
	authSvc := auth.NewAuthService(nil, testConfig(), usersvc.NewUserService(nil))
	h := NewAuthHandler(authSvc, nil, nil, nil, nil, nil, testConfig())

	resp, err := h.GetProfileHandler(context.Background(), &struct{}{})
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

// --- RegisterHandler ---

func TestRegisterHandler_EmptyCredentials(t *testing.T) {
	h := testAuthHandler()
	input := &RegisterRequest{}

	resp, err := h.RegisterHandler(context.Background(), input)
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

func TestRegisterHandler_MissingEmail(t *testing.T) {
	h := testAuthHandler()
	input := &RegisterRequest{}
	input.Body.Password = "somepassword123"

	resp, err := h.RegisterHandler(context.Background(), input)
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

func TestRegisterHandler_MissingTermsAcceptance(t *testing.T) {
	h := testAuthHandler()
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = false

	resp, err := h.RegisterHandler(context.Background(), input)
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

// TestRegisterHandler_MissingAgeConfirmation mirrors the terms-rejection test
// (PSY-1023): a signup with terms accepted but no age confirmation must be
// rejected with the AGE_CONFIRMATION_REQUIRED code.
func TestRegisterHandler_MissingAgeConfirmation(t *testing.T) {
	h := testAuthHandler()
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = false

	resp, err := h.RegisterHandler(context.Background(), input)
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

// TestRegisterHandler_AgeBelowMinimum guards the server-side age floor: a client
// that attests an age below MinSignupAge (e.g. tampered request) must be
// rejected even though the age_confirmed flag is set.
func TestRegisterHandler_AgeBelowMinimum(t *testing.T) {
	h := testAuthHandler()
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge - 1

	resp, err := h.RegisterHandler(context.Background(), input)
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

// TestRegisterHandler_RecordsAgeConfirmation asserts the handler forwards the
// age confirmation into the LegalAcceptance passed to the user service, so the
// evidence is persisted on the new user.
func TestRegisterHandler_RecordsAgeConfirmation(t *testing.T) {
	var captured contracts.LegalAcceptance
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			CreateUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
				captured = acceptance
				return &authm.User{ID: 11, Email: strPtr(email)}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) { return "tok", nil },
		}
		ah.discordService = &testhelpers.MockDiscordService{
			NotifyNewUserFn: func(u *authm.User) {},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "new@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatalf("expected success=true, got message=%q", resp.Body.Message)
	}
	if captured.MinAgeAttested != MinSignupAge {
		t.Errorf("expected MinAgeAttested=%d on LegalAcceptance, got %d", MinSignupAge, captured.MinAgeAttested)
	}
	if captured.AgeConfirmedAt.IsZero() {
		t.Error("expected AgeConfirmedAt to be set on LegalAcceptance")
	}
}

func TestRegisterHandler_NilUserService(t *testing.T) {
	// No passwordValidator, no userService → hits userService nil check
	h := testAuthHandler()
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge
	input.Body.PrivacyVersion = "2026-02-15"

	resp, err := h.RegisterHandler(context.Background(), input)
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

func TestRegisterHandler_WeakPassword(t *testing.T) {
	// NewPasswordValidator works standalone; the HIBP call may fail in tests
	// but the length check happens first and short-circuits the result.
	pv := auth.NewPasswordValidator()
	h := NewAuthHandler(nil, nil, nil, nil, nil, pv, testConfig())
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "abc" // too short (min 12)
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge
	input.Body.PrivacyVersion = "2026-02-15"

	resp, err := h.RegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if !strings.Contains(resp.Body.Message, "at least") {
		t.Errorf("expected message about minimum length, got %q", resp.Body.Message)
	}
}

// --- SendVerificationEmailHandler ---

func TestSendVerificationEmailHandler_NoUserContext(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.SendVerificationEmailHandler(context.Background(), &struct{}{})
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

func TestSendVerificationEmailHandler_AlreadyVerified(t *testing.T) {
	h := testAuthHandler()
	user := &authm.User{ID: 1, EmailVerified: true}
	ctx := testhelpers.CtxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "ALREADY_VERIFIED" {
		t.Errorf("expected error_code=ALREADY_VERIFIED, got %s", resp.Body.ErrorCode)
	}
}

// --- ConfirmVerificationHandler ---

func TestConfirmVerificationHandler_EmptyToken(t *testing.T) {
	h := testAuthHandler()
	input := &ConfirmVerificationRequest{}

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
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

func TestConfirmVerificationHandler_InvalidToken(t *testing.T) {
	jwtSvc := testJWTService()
	h := NewAuthHandler(nil, jwtSvc, nil, nil, nil, nil, testConfig())
	input := &ConfirmVerificationRequest{}
	input.Body.Token = "invalid.garbage.token"

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "INVALID_TOKEN" {
		t.Errorf("expected error_code=INVALID_TOKEN, got %s", resp.Body.ErrorCode)
	}
}

// --- SendMagicLinkHandler ---

func TestSendMagicLinkHandler_EmptyEmail(t *testing.T) {
	h := testAuthHandler()
	input := &SendMagicLinkRequest{}

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
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

// --- VerifyMagicLinkHandler ---

func TestVerifyMagicLinkHandler_EmptyToken(t *testing.T) {
	h := testAuthHandler()
	input := &VerifyMagicLinkRequest{}

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
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

func TestVerifyMagicLinkHandler_InvalidToken(t *testing.T) {
	jwtSvc := testJWTService()
	h := NewAuthHandler(nil, jwtSvc, nil, nil, nil, nil, testConfig())
	input := &VerifyMagicLinkRequest{}
	input.Body.Token = "invalid.garbage.token"

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "INVALID_TOKEN" {
		t.Errorf("expected error_code=INVALID_TOKEN, got %s", resp.Body.ErrorCode)
	}
}

// --- ChangePasswordHandler ---

func TestChangePasswordHandler_NoUserContext(t *testing.T) {
	h := testAuthHandler()
	input := &ChangePasswordRequest{}

	resp, err := h.ChangePasswordHandler(context.Background(), input)
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

func TestChangePasswordHandler_EmptyPasswords(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	input := &ChangePasswordRequest{}

	resp, err := h.ChangePasswordHandler(ctx, input)
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

func TestChangePasswordHandler_SamePassword(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "samepassword123"
	input.Body.NewPassword = "samepassword123"

	resp, err := h.ChangePasswordHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if !strings.Contains(resp.Body.Message, "must be different") {
		t.Errorf("expected message about different password, got %q", resp.Body.Message)
	}
}

// --- GetDeletionSummaryHandler ---

func TestGetDeletionSummaryHandler_NoUserContext(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.GetDeletionSummaryHandler(context.Background(), &struct{}{})
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

// --- DeleteAccountHandler ---

func TestDeleteAccountHandler_NoUserContext(t *testing.T) {
	h := testAuthHandler()
	input := &DeleteAccountRequest{}

	resp, err := h.DeleteAccountHandler(context.Background(), input)
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

func TestDeleteAccountHandler_OAuthOnlyUser(t *testing.T) {
	h := testAuthHandler()
	// User with nil PasswordHash (OAuth-only)
	user := &authm.User{ID: 1, PasswordHash: nil, IsActive: true}
	ctx := testhelpers.CtxWithUser(user)
	input := &DeleteAccountRequest{}

	resp, err := h.DeleteAccountHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if !strings.Contains(resp.Body.Message, "OAuth") {
		t.Errorf("expected message about OAuth, got %q", resp.Body.Message)
	}
}

func TestDeleteAccountHandler_EmptyPassword(t *testing.T) {
	h := testAuthHandler()
	hash := "$2a$10$fakehashvalue"
	user := &authm.User{ID: 1, PasswordHash: &hash, IsActive: true}
	ctx := testhelpers.CtxWithUser(user)
	input := &DeleteAccountRequest{}
	// password is empty

	resp, err := h.DeleteAccountHandler(ctx, input)
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

// --- ExportDataHandler ---

func TestExportDataHandler_NoUserContext(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.ExportDataHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ContentType != "application/json" {
		t.Errorf("expected content-type application/json, got %s", resp.ContentType)
	}
	if !strings.Contains(string(resp.Body), "unauthorized") {
		t.Errorf("expected body to contain 'unauthorized', got %q", string(resp.Body))
	}
}

// --- RecoverAccountHandler ---

func TestRecoverAccountHandler_EmptyCredentials(t *testing.T) {
	h := testAuthHandler()
	input := &RecoverAccountRequest{}

	resp, err := h.RecoverAccountHandler(context.Background(), input)
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

// --- RequestAccountRecoveryHandler ---

func TestRequestAccountRecoveryHandler_EmptyEmail(t *testing.T) {
	h := testAuthHandler()
	input := &RequestAccountRecoveryRequest{}

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
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

// --- ConfirmAccountRecoveryHandler ---

func TestConfirmAccountRecoveryHandler_EmptyToken(t *testing.T) {
	h := testAuthHandler()
	input := &ConfirmAccountRecoveryRequest{}

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
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

func TestConfirmAccountRecoveryHandler_InvalidToken(t *testing.T) {
	jwtSvc := testJWTService()
	h := NewAuthHandler(nil, jwtSvc, nil, nil, nil, nil, testConfig())
	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "invalid.garbage.token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "INVALID_TOKEN" {
		t.Errorf("expected error_code=INVALID_TOKEN, got %s", resp.Body.ErrorCode)
	}
}

// --- GenerateCLITokenHandler ---

func TestGenerateCLITokenHandler_NoUserContext(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.GenerateCLITokenHandler(context.Background(), &struct{}{})
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

// ============================================================================
// Mock-based auth handler tests
// ============================================================================

// authHandler builds an AuthHandler with mock services and optional overrides.
func authHandler(opts ...func(*AuthHandler)) *AuthHandler {
	h := &AuthHandler{
		authService:       &testhelpers.MockAuthService{},
		jwtService:        &testhelpers.MockJWTService{},
		userService:       &testhelpers.MockUserService{},
		emailService:      &testhelpers.MockEmailService{},
		discordService:    &testhelpers.MockDiscordService{},
		passwordValidator: &testhelpers.MockPasswordValidator{},
		config:            testConfig(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// --- LoginHandler mock tests ---

func TestLoginHandler_Success(t *testing.T) {
	user := &authm.User{ID: 1, Email: strPtr("test@example.com")}
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return user, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "jwt-token-123", nil
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "password123"

	resp, err := h.LoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.Token != "jwt-token-123" {
		t.Errorf("expected token=jwt-token-123, got %s", resp.Body.Token)
	}
	if resp.Body.User == nil || resp.Body.User.ID != 1 {
		t.Error("expected user in response")
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("expected cookie name=%s, got %s", config.AuthCookieName, resp.SetCookie.Name)
	}
}

func TestLoginHandler_InvalidCredentials(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return nil, autherrors.ErrInvalidCredentials(nil)
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "wrong"

	resp, err := h.LoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.ToExternalCode(autherrors.CodeInvalidCredentials) {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
}

func TestLoginHandler_AccountLocked(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return nil, autherrors.ErrAccountLockedWithMinutes(15)
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "password"

	resp, err := h.LoginHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeAccountLocked {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeAccountLocked, resp.Body.ErrorCode)
	}
}

// TestLoginHandler_AccountInactive asserts the PSY-866 contract: when
// AuthenticateUserWithPassword returns a CodeAccountInactive AuthError
// (deactivated account, is_active = false), the handler returns HTTP 200
// (nil error) with Success=false, the distinct ACCOUNT_INACTIVE code, and the
// chosen contact-support message — NOT a fail-closed 5xx via the default arm.
func TestLoginHandler_AccountInactive(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return nil, autherrors.ErrAccountInactive()
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "inactive@example.com"
	input.Body.Password = "CorrectPassword1!"

	resp, err := h.LoginHandler(context.Background(), input)

	// HTTP 200: the account state is known and the service is healthy, so the
	// handler must NOT return an error (which would push Huma into the 5xx band).
	if err != nil {
		t.Fatalf("expected nil error (HTTP 200), got %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeAccountInactive {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeAccountInactive, resp.Body.ErrorCode)
	}
	if resp.Body.Message != "Account unavailable. Please contact support." {
		t.Errorf("expected contact-support message, got %q", resp.Body.Message)
	}
	// Regression guard: an inactive account must NOT be downgraded to the
	// fail-closed SERVICE_UNAVAILABLE shape (the default-arm behavior before
	// the explicit case was added).
	if resp.Body.ErrorCode == autherrors.CodeServiceUnavailable {
		t.Error("regression: ACCOUNT_INACTIVE must not be downgraded to SERVICE_UNAVAILABLE")
	}
}

// TestLoginHandler_ServiceUnavailable asserts that a CodeServiceUnavailable
// AuthError from AuthenticateUserWithPassword (the PSY-861 fail-closed signal
// emitted when IncrementFailedAttempts errors) propagates as a real error to
// Huma — NOT silently downgraded to INVALID_CREDENTIALS via the fallback path.
// The body still carries SERVICE_UNAVAILABLE so clients can branch, and the
// returned error drives the HTTP status into the 5xx band.
func TestLoginHandler_ServiceUnavailable(t *testing.T) {
	svcErr := autherrors.ErrServiceUnavailable("user_increment_failed_attempts", fmt.Errorf("db hiccup"))
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return nil, svcErr
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "wrong"

	resp, err := h.LoginHandler(context.Background(), input)

	// The handler MUST return the AuthError so Huma emits a 5xx.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// Regression guard: the fall-through path used to map every unknown
	// AuthError to INVALID_CREDENTIALS, which would hand an attacker a free
	// guess. Lock that branch out.
	if resp.Body.ErrorCode == autherrors.CodeInvalidCredentials {
		t.Error("regression: SERVICE_UNAVAILABLE must not be downgraded to INVALID_CREDENTIALS")
	}
}

// TestLoginHandler_UnknownAuthCodeFailsClosed asserts that an AuthError whose
// Code is not explicitly handled by the LoginHandler switch propagates as a
// 5xx instead of falling through to an INVALID_CREDENTIALS HTTP 200 downgrade.
// Locks in the fail-closed convention: any new AuthError code added without a
// dedicated handler case must surface as SERVICE_UNAVAILABLE — adding an
// explicit 401-shaped case requires a UX decision in code review.
func TestLoginHandler_UnknownAuthCodeFailsClosed(t *testing.T) {
	// CodeUnknown is the canonical "we have an AuthError but the code is not
	// one we explicitly route" signal. Any other unrouted code (e.g. a future
	// addition without a switch arm) would exercise the same default branch.
	unknownErr := &autherrors.AuthError{Code: autherrors.CodeUnknown, Message: "unrouted auth code"}
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return nil, unknownErr
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "any"

	resp, err := h.LoginHandler(context.Background(), input)

	// The handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// Regression guard: an unrouted AuthError code must not silently downgrade
	// to INVALID_CREDENTIALS — that was the pre-existing fall-through behavior
	// and the convention this test locks in.
	if resp.Body.ErrorCode == autherrors.CodeInvalidCredentials {
		t.Error("regression: unknown AuthError code must not be downgraded to INVALID_CREDENTIALS")
	}
	// External message must match the existing SERVICE_UNAVAILABLE shape — no
	// leak about whether the unknown was an AuthError sub-code or a raw error.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestLoginHandler_NonAuthErrorFailsClosed asserts that a raw (non-AuthError)
// error returned by the user service — e.g. the config / DB / account-state
// fmt.Errorf sites in AuthenticateUserWithPassword that have not been promoted
// to typed AuthErrors — propagates as a 5xx instead of being silently mapped
// to INVALID_CREDENTIALS HTTP 200. The outer fallback used to swallow these
// errors and hand attackers a free guess on DB-stress; this regression test
// locks that branch out.
func TestLoginHandler_NonAuthErrorFailsClosed(t *testing.T) {
	rawErr := fmt.Errorf("simulated config error")
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return nil, rawErr
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "any"

	resp, err := h.LoginHandler(context.Background(), input)

	// The handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must itself be an AuthError of CodeServiceUnavailable
	// so the apperror mapper translates it to a 5xx. A non-AuthError leaking out
	// here would be downstream-classified by Huma into a generic 500 with no
	// structured error code — fine, but the explicit shape is what callers
	// (and clients) branch on. The handler wraps via ErrServiceUnavailable.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected response error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// Regression guard: a non-AuthError must not silently downgrade to
	// INVALID_CREDENTIALS — that was the pre-existing outer-fallback behavior
	// and the convention this test locks in.
	if resp.Body.ErrorCode == autherrors.CodeInvalidCredentials {
		t.Error("regression: non-AuthError must not be downgraded to INVALID_CREDENTIALS")
	}
	// External message must match the existing SERVICE_UNAVAILABLE shape — no
	// leak about whether the cause was a config error, raw DB failure, or
	// some other internal step.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

func TestLoginHandler_TokenFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			AuthenticateUserWithPasswordFn: func(email, password string) (*authm.User, error) {
				return &authm.User{ID: 1}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "", fmt.Errorf("jwt error")
			},
		}
	})

	input := &LoginRequest{}
	input.Body.Email = "test@example.com"
	input.Body.Password = "password123"

	resp, err := h.LoginHandler(context.Background(), input)
	// LoginHandler returns a huma error on token failure
	if err == nil {
		// The handler returns (resp, authErr) where authErr is non-nil
		if resp != nil && resp.Body.Success {
			t.Error("expected success=false")
		}
	}
	// Either way, we should not have success
	if resp != nil && resp.Body.Success {
		t.Error("expected success=false on token failure")
	}
}

// --- RegisterHandler mock tests ---

func TestRegisterHandler_Success(t *testing.T) {
	user := &authm.User{ID: 10, Email: strPtr("new@example.com")}
	var discordCalled bool
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			CreateUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
				return user, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "reg-token", nil
			},
		}
		ah.discordService = &testhelpers.MockDiscordService{
			NotifyNewUserFn: func(u *authm.User) {
				discordCalled = true
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "new@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
	if resp.Body.Token != "reg-token" {
		t.Errorf("expected token=reg-token, got %s", resp.Body.Token)
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("expected cookie set, got name=%s", resp.SetCookie.Name)
	}
	if !discordCalled {
		t.Error("expected Discord notification for new user")
	}
}

// TestRegisterHandler_UserExists pins the INTENTIONAL enumeration-UX arm
// (PSY-775 Option A): an already-registered email must stay HTTP 200 (nil
// returned error) with the explicit "account already exists" message and the
// USER_EXISTS code. This is the dual-direction counterpart to
// TestRegisterHandler_UnknownServiceErrorFailsClosed — it guards against a
// future fail-closed pass accidentally flipping this arm to a 5xx, which would
// regress the "log in instead" guidance a returning user depends on.
func TestRegisterHandler_UserExists(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			CreateUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
				return nil, autherrors.ErrUserExists(email)
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "existing@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	// Option A: HTTP 200, so the handler returns a nil error. A non-nil error
	// here would mean the arm regressed into the fail-closed 5xx path.
	if err != nil {
		t.Fatalf("expected nil error (HTTP 200 enumeration-UX arm), got %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUserExists {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUserExists, resp.Body.ErrorCode)
	}
	// The explicit message is the user-facing guidance; it must not be replaced
	// by the generic SERVICE_UNAVAILABLE copy.
	if resp.Body.Message != autherrors.ErrUserExists(input.Body.Email).UserMessage() {
		t.Errorf("expected explicit user-exists message, got %q", resp.Body.Message)
	}
}

// TestRegisterHandler_UnknownServiceErrorFailsClosed locks in the PSY-900
// fail-closed convention for the registration path: any CreateUser failure
// OTHER than the intentional USER_EXISTS arm is an unexpected backend/service
// error and MUST surface as a 5xx (non-nil returned error wrapping an
// *AuthError of CodeServiceUnavailable) so monitoring sees the same uniform
// shape as the rest of this handler family — not the prior silent HTTP-200 +
// CodeUnknown body. The body stays byte-identical with the canonical
// SERVICE_UNAVAILABLE shape; only the transport status flips.
func TestRegisterHandler_UnknownServiceErrorFailsClosed(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			CreateUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
				return nil, errors.New("db connection refused")
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)

	// Handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx with the structured body.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape, not
	// the prior "Failed to create user" leak.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

func TestRegisterHandler_WeakPasswordMock(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.passwordValidator = &testhelpers.MockPasswordValidator{
			ValidatePasswordFn: func(password string) (*contracts.PasswordValidationResult, error) {
				return &contracts.PasswordValidationResult{
					Valid:  false,
					Errors: []string{"Password must be at least 12 characters"},
				}, nil
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "short"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	}
	if !strings.Contains(resp.Body.Message, "12 characters") {
		t.Errorf("expected message about 12 characters, got %q", resp.Body.Message)
	}
}

func TestRegisterHandler_TokenFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			CreateUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance contracts.LegalAcceptance) (*authm.User, error) {
				return &authm.User{ID: 1}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "", fmt.Errorf("jwt error")
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
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

// --- GetProfileHandler mock tests ---

func TestGetProfileHandler_Success(t *testing.T) {
	user := &authm.User{ID: 1, Email: strPtr("test@example.com")}
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return user, nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetProfileHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.User == nil || resp.Body.User.ID != 1 {
		t.Error("expected user in response")
	}
}

// TestGetProfileHandler_ServiceError asserts that a service-call failure from
// authService.GetUserProfile propagates as a 5xx rather than the prior silent
// HTTP-200 SERVICE_UNAVAILABLE downgrade. Locks in the fail-closed convention
// so an unexpected backend defect surfaces in monitoring instead of being
// read by the client as the canonical body shape.
func TestGetProfileHandler_ServiceError(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetProfileHandler(ctx, &struct{}{})

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestGetProfileHandler_DeletedUserReturns401 asserts the typed
// CodeUserNotFound path: when GetUserProfile reports the principal was
// deleted between token issuance and this profile fetch, the handler routes
// to HTTP 401 + CodeUnauthorized rather than the fail-closed 5xx +
// CodeServiceUnavailable reserved for generic backend failures.
//
// The 5xx contract for non-typed errors stays covered by
// TestGetProfileHandler_ServiceError. Both directions must remain asserted
// to prevent a future fail-closed pass from collapsing the 401
// discrimination back into the generic 5xx bucket.
func TestGetProfileHandler_DeletedUserReturns401(t *testing.T) {
	notFoundErr := autherrors.ErrUserNotFoundByID(1, fmt.Errorf("record not found"))
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return nil, notFoundErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetProfileHandler(ctx, &struct{}{})

	// Handler MUST return a non-nil StatusError so Huma emits HTTP 401.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits HTTP 401")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected returned error to satisfy huma.StatusError, got %T (%v)", err, err)
	}
	if got := statusErr.GetStatus(); got != 401 {
		t.Errorf("expected HTTP status 401, got %d", got)
	}
	// Body shape carries the canonical CodeUnauthorized envelope. Huma drops
	// the resp body when the handler returns a non-nil error, but the body
	// fields are still asserted here so a downstream change that switches
	// to a body-preserving error-emit (e.g. embedding the resp shape in a
	// custom StatusError) cannot quietly drop the discriminator.
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeUnauthorized) {
		t.Errorf("expected canonical UNAUTHORIZED message, got %q", resp.Body.Message)
	}
}

// --- RefreshTokenHandler mock tests ---

func TestRefreshTokenHandler_Success(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: userID}, nil
			},
			RefreshUserTokenFn: func(user *authm.User) (string, error) {
				return "new-token", nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.Token != "new-token" {
		t.Errorf("expected token=new-token, got %s", resp.Body.Token)
	}
}

// TestRefreshTokenHandler_ProfileFails asserts that an unexpected service
// failure in GetUserProfile (DB outage, raw error from the user service)
// propagates as a 5xx so the HTTP layer does not hand callers an HTTP 200
// with a SERVICE_UNAVAILABLE body. The body shape stays the same as the old
// HTTP-200 path; only the transport status flips. Locks in the fail-closed
// convention for the token-refresh session-lifecycle surface.
func TestRefreshTokenHandler_ProfileFails(t *testing.T) {
	rawErr := fmt.Errorf("db error")
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return nil, rawErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})

	// Handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx with the structured body.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape so
	// callers do not see "Failed to refresh token" as an internal-step leak.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestRefreshTokenHandler_DeletedUserReturns401 asserts the typed
// CodeUserNotFound path: when GetUserProfile reports the principal was
// deleted between token issuance and this refresh, the handler routes to
// HTTP 401 + CodeUnauthorized rather than the fail-closed 5xx +
// CodeServiceUnavailable reserved for generic backend failures.
//
// The 5xx contract for non-typed errors stays covered by
// TestRefreshTokenHandler_ProfileFails. Both directions must remain
// asserted to prevent a future fail-closed pass from collapsing the 401
// discrimination back into the generic 5xx bucket.
func TestRefreshTokenHandler_DeletedUserReturns401(t *testing.T) {
	notFoundErr := autherrors.ErrUserNotFoundByID(1, fmt.Errorf("record not found"))
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return nil, notFoundErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})

	// Handler MUST return a non-nil StatusError so Huma emits HTTP 401.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits HTTP 401")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	var statusErr huma.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected returned error to satisfy huma.StatusError, got %T (%v)", err, err)
	}
	if got := statusErr.GetStatus(); got != 401 {
		t.Errorf("expected HTTP status 401, got %d", got)
	}
	// Body shape carries the canonical CodeUnauthorized envelope. Huma drops
	// the resp body when the handler returns a non-nil error, but the body
	// fields are still asserted here so a downstream change that switches
	// to a body-preserving error-emit (e.g. embedding the resp shape in a
	// custom StatusError) cannot quietly drop the discriminator.
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeUnauthorized) {
		t.Errorf("expected canonical UNAUTHORIZED message, got %q", resp.Body.Message)
	}
}

// TestRefreshTokenHandler_TokenFails asserts that an unexpected failure in
// RefreshUserToken (JWT service outage, signing failure) propagates as a 5xx
// for the same reason as the profile-failure branch — a session-lifecycle
// surface returning HTTP 200 on a transport-level outage hides a real
// incident from monitoring.
func TestRefreshTokenHandler_TokenFails(t *testing.T) {
	rawErr := fmt.Errorf("token error")
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: userID}, nil
			},
			RefreshUserTokenFn: func(user *authm.User) (string, error) {
				return "", rawErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})

	// Handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx with the structured body.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape so
	// callers do not see "Failed to generate new token" as an internal-step
	// leak.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// --- SendVerificationEmailHandler mock tests ---

func TestSendVerificationEmailHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return true },
			SendVerificationEmailFn: func(toEmail, token string) error {
				return nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateVerificationTokenFn: func(userID uint, e string) (string, error) {
				return "verify-token", nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, Email: &email, EmailVerified: false})

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
}

func TestSendVerificationEmailHandler_EmailNotConfigured(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return false },
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, Email: &email, EmailVerified: false})

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
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

func TestSendVerificationEmailHandler_NoEmail(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return true },
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, Email: nil, EmailVerified: false})

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "NO_EMAIL" {
		t.Errorf("expected error_code=NO_EMAIL, got %s", resp.Body.ErrorCode)
	}
}

// TestSendVerificationEmailHandler_TokenFails asserts that a JWT-service
// failure at the verification-token mint step propagates as a 5xx rather
// than the prior silent HTTP-200 SERVICE_UNAVAILABLE downgrade. Locks in
// the fail-closed convention for the post-input-validation backend-failure
// branches; the intentional UX shapes (ALREADY_VERIFIED, NO_EMAIL, etc.)
// still return HTTP 200 with structured ErrorCode.
func TestSendVerificationEmailHandler_TokenFails(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return true },
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateVerificationTokenFn: func(userID uint, e string) (string, error) {
				return "", fmt.Errorf("token error")
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, Email: &email, EmailVerified: false})

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestSendVerificationEmailHandler_EmailSendFailsClosed asserts that an
// email-service failure at the send step propagates as a 5xx rather than the
// prior silent HTTP-200 SERVICE_UNAVAILABLE downgrade. Same convention as the
// token-mint branch above; both backend-failure paths now fail closed.
func TestSendVerificationEmailHandler_EmailSendFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return true },
			SendVerificationEmailFn: func(toEmail, token string) error {
				return fmt.Errorf("smtp connection refused")
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateVerificationTokenFn: func(userID uint, e string) (string, error) {
				return "valid-token", nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, Email: &email, EmailVerified: false})

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// --- ConfirmVerificationHandler mock tests ---

func TestConfirmVerificationHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateVerificationTokenFn: func(tokenString string) (*contracts.VerificationTokenClaims, error) {
				return &contracts.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, EmailVerified: false}, nil
			},
			SetEmailVerifiedFn: func(userID uint, verified bool) error {
				return nil
			},
		}
	})

	input := &ConfirmVerificationRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
}

func TestConfirmVerificationHandler_UserNotFound(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateVerificationTokenFn: func(tokenString string) (*contracts.VerificationTokenClaims, error) {
				return &contracts.VerificationTokenClaims{UserID: 999, Email: "test@example.com"}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})

	input := &ConfirmVerificationRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
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

func TestConfirmVerificationHandler_AlreadyVerified(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateVerificationTokenFn: func(tokenString string) (*contracts.VerificationTokenClaims, error) {
				return &contracts.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, EmailVerified: true}, nil
			},
		}
	})

	input := &ConfirmVerificationRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true for already verified")
	}
	if !strings.Contains(resp.Body.Message, "already verified") {
		t.Errorf("expected message about already verified, got %q", resp.Body.Message)
	}
}

// TestConfirmVerificationHandler_SetVerifiedFailsClosed asserts that a
// SetEmailVerified failure at the final write step propagates as a 5xx
// rather than the prior silent HTTP-200 SERVICE_UNAVAILABLE downgrade. The
// token, user, and email have all been validated by this point — a write
// failure is an unexpected backend defect, not a UX condition. The
// intentional UX shapes earlier in the handler (INVALID_TOKEN, UNAUTHORIZED,
// EMAIL_MISMATCH, already-verified) still return HTTP 200 with structured
// ErrorCode so the frontend can render the corresponding form state.
func TestConfirmVerificationHandler_SetVerifiedFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateVerificationTokenFn: func(tokenString string) (*contracts.VerificationTokenClaims, error) {
				return &contracts.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, EmailVerified: false}, nil
			},
			SetEmailVerifiedFn: func(userID uint, verified bool) error {
				return fmt.Errorf("db error")
			},
		}
	})

	input := &ConfirmVerificationRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestConfirmVerificationHandler_AlreadyVerifiedStillSucceeds locks in the
// intentional UX shape for the "already verified" branch: the response stays
// HTTP 200 with success=true and a human-readable message even though no
// write happens. Regression guard against the fail-closed conversion
// accidentally tipping over the intentional 200-shape branches.
func TestConfirmVerificationHandler_AlreadyVerifiedStillSucceeds(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateVerificationTokenFn: func(tokenString string) (*contracts.VerificationTokenClaims, error) {
				return &contracts.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, EmailVerified: true}, nil
			},
			// SetEmailVerifiedFn intentionally absent — the handler must not
			// reach this call when the email is already verified.
			SetEmailVerifiedFn: func(userID uint, verified bool) error {
				t.Fatal("SetEmailVerified must not be called when email is already verified")
				return nil
			},
		}
	})

	input := &ConfirmVerificationRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error on already-verified path: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true on already-verified branch")
	}
	if !strings.Contains(resp.Body.Message, "already verified") {
		t.Errorf("expected message about already verified, got %q", resp.Body.Message)
	}
}

// --- SendMagicLinkHandler mock tests ---

// magicLinkAccountState models a GetUserByEmail outcome for the enumeration test.
type magicLinkAccountState struct {
	name string
	user func(email string) (*authm.User, error)
}

// magicLinkAccountStates enumerates the three states whose responses must be
// indistinguishable (PSY-749): unknown email, known-unverified, known-verified.
func magicLinkAccountStates(email string) []magicLinkAccountState {
	return []magicLinkAccountState{
		{name: "unknown_email", user: func(string) (*authm.User, error) { return nil, nil }},
		{name: "known_unverified", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, EmailVerified: false}, nil
		}},
		{name: "known_verified", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, EmailVerified: true}, nil
		}},
	}
}

// TestSendMagicLinkHandler_ResponseIdenticalAcrossAccountStates is the PSY-749
// regression guard: the response body must be byte-identical regardless of
// whether the email is unregistered, registered-unverified, or
// registered-verified, so the endpoint can't be used as an enumeration oracle.
func TestSendMagicLinkHandler_ResponseIdenticalAcrossAccountStates(t *testing.T) {
	email := "probe@example.com"

	respFor := func(state magicLinkAccountState) SendMagicLinkResponse {
		h := authHandler(func(ah *AuthHandler) {
			ah.emailService = &testhelpers.MockEmailService{
				IsConfiguredFn:          func() bool { return true },
				SendMagicLinkEmailFn:    func(string, string) error { return nil },
				SendVerificationEmailFn: func(string, string) error { return nil },
			}
			ah.userService = &testhelpers.MockUserService{GetUserByEmailFn: state.user}
			ah.jwtService = &testhelpers.MockJWTService{
				CreateMagicLinkTokenFn:    func(uint, string) (string, error) { return "magic-token", nil },
				CreateVerificationTokenFn: func(uint, string) (string, error) { return "verify-token", nil },
			}
		})
		input := &SendMagicLinkRequest{}
		input.Body.Email = email
		resp, err := h.SendMagicLinkHandler(context.Background(), input)
		if err != nil {
			t.Fatalf("[%s] unexpected error: %v", state.name, err)
		}
		return *resp
	}

	states := magicLinkAccountStates(email)
	want := respFor(states[0])
	if !want.Body.Success {
		t.Fatalf("expected success=true for enumeration-safe response, got message=%q", want.Body.Message)
	}
	if want.Body.ErrorCode != "" {
		t.Fatalf("expected empty error_code, got %q", want.Body.ErrorCode)
	}
	for _, state := range states[1:] {
		got := respFor(state)
		if got.Body != want.Body {
			t.Errorf("response body for %s differs from %s:\n  got  %+v\n  want %+v",
				state.name, states[0].name, got.Body, want.Body)
		}
	}
}

// TestSendMagicLinkHandler_UnverifiedResendsVerification asserts the side
// effect: an unverified account triggers a re-sent verification email (not a
// magic link), so the user can still proceed (PSY-749).
func TestSendMagicLinkHandler_UnverifiedResendsVerification(t *testing.T) {
	email := "unverified@example.com"
	var verificationSent, magicLinkSent bool
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn:          func() bool { return true },
			SendVerificationEmailFn: func(string, string) error { verificationSent = true; return nil },
			SendMagicLinkEmailFn:    func(string, string) error { magicLinkSent = true; return nil },
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &e, EmailVerified: false}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateVerificationTokenFn: func(uint, string) (string, error) { return "verify-token", nil },
		}
	})

	input := &SendMagicLinkRequest{}
	input.Body.Email = email

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if !verificationSent {
		t.Error("expected verification email to be re-sent for unverified account")
	}
	if magicLinkSent {
		t.Error("expected no magic link email for unverified account")
	}
}

// TestSendMagicLinkHandler_VerifiedSendsMagicLink asserts the verified path
// sends a magic link (and not a verification email).
func TestSendMagicLinkHandler_VerifiedSendsMagicLink(t *testing.T) {
	email := "verified@example.com"
	var verificationSent, magicLinkSent bool
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn:          func() bool { return true },
			SendVerificationEmailFn: func(string, string) error { verificationSent = true; return nil },
			SendMagicLinkEmailFn:    func(string, string) error { magicLinkSent = true; return nil },
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &e, EmailVerified: true}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateMagicLinkTokenFn: func(uint, string) (string, error) { return "magic-token", nil },
		}
	})

	input := &SendMagicLinkRequest{}
	input.Body.Email = email

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if !magicLinkSent {
		t.Error("expected magic link email for verified account")
	}
	if verificationSent {
		t.Error("expected no verification email for verified account")
	}
}

// TestSendMagicLinkHandler_SendFailureStaysGeneric asserts that a downstream
// send failure for a known-verified account does NOT change the response —
// otherwise the failure response would leak that the account exists (PSY-749).
func TestSendMagicLinkHandler_SendFailureStaysGeneric(t *testing.T) {
	email := "verified@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn:       func() bool { return true },
			SendMagicLinkEmailFn: func(string, string) error { return fmt.Errorf("resend exploded") },
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &e, EmailVerified: true}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateMagicLinkTokenFn: func(uint, string) (string, error) { return "magic-token", nil },
		}
	})

	input := &SendMagicLinkRequest{}
	input.Body.Email = email

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true even when send fails (no enumeration leak)")
	}
	if resp.Body.ErrorCode != "" {
		t.Errorf("expected empty error_code on send failure, got %q", resp.Body.ErrorCode)
	}
}

// --- VerifyMagicLinkHandler mock tests ---

func TestVerifyMagicLinkHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateMagicLinkTokenFn: func(tokenString string) (*contracts.MagicLinkTokenClaims, error) {
				return &contracts.MagicLinkTokenClaims{UserID: 1, Email: email}, nil
			},
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "session-token", nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = "valid-magic-token"

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
	if resp.Body.Token != "session-token" {
		t.Errorf("expected token=session-token, got %s", resp.Body.Token)
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("expected cookie set, got name=%s", resp.SetCookie.Name)
	}
}

func TestVerifyMagicLinkHandler_UserNotFound(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateMagicLinkTokenFn: func(tokenString string) (*contracts.MagicLinkTokenClaims, error) {
				return &contracts.MagicLinkTokenClaims{UserID: 999, Email: "test@example.com"}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
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

func TestVerifyMagicLinkHandler_InactiveUser(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateMagicLinkTokenFn: func(tokenString string) (*contracts.MagicLinkTokenClaims, error) {
				return &contracts.MagicLinkTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: false}, nil
			},
		}
	})

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
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

// TestVerifyMagicLinkHandler_SessionTokenMintFailsClosed asserts that once a
// magic-link request has cleared every enumeration-safety gate (token valid,
// email matches, account active), a downstream JWT mint failure surfaces as
// a 5xx instead of a silent HTTP 200 + SERVICE_UNAVAILABLE downgrade. The
// gates above this branch deliberately return HTTP 200 so the response shape
// does not leak whether an account exists or its state; this branch is past
// those gates and a failure here is a genuine internal fault that on-call
// must see.
func TestVerifyMagicLinkHandler_SessionTokenMintFailsClosed(t *testing.T) {
	email := "test@example.com"
	mintErr := fmt.Errorf("simulated jwt mint failure")
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateMagicLinkTokenFn: func(tokenString string) (*contracts.MagicLinkTokenClaims, error) {
				return &contracts.MagicLinkTokenClaims{UserID: 1, Email: email}, nil
			},
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "", mintErr
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = "valid-magic-token"

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)

	// The handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected response error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape so
	// no leak about which internal step failed.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
	// Regression guard: the previous branch hard-coded the message
	// "Failed to create session" and returned a nil error so the response
	// rode HTTP 200. The handler must no longer surface that literal — the
	// canonical SERVICE_UNAVAILABLE external message is the only acceptable
	// shape now that the branch is fail-closed.
	if resp.Body.Message == "Failed to create session" {
		t.Error("regression: handler must not surface the pre-fix \"Failed to create session\" message")
	}
}

// --- ChangePasswordHandler mock tests ---

func TestChangePasswordHandler_Success(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "old-password-123"
	input.Body.NewPassword = "new-password-456"

	resp, err := h.ChangePasswordHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
}

func TestChangePasswordHandler_InvalidCurrent(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return autherrors.ErrInvalidCredentials(nil)
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "wrong-password"
	input.Body.NewPassword = "new-password-456"

	resp, err := h.ChangePasswordHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
}

func TestChangePasswordHandler_NoPasswordSet(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return autherrors.ErrNoPasswordSet()
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "anything"
	input.Body.NewPassword = "new-password-456"

	resp, err := h.ChangePasswordHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeNoPasswordSet {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeNoPasswordSet, resp.Body.ErrorCode)
	}
}

// TestChangePasswordHandler_UnknownAuthCodeFailsClosed asserts that an
// AuthError whose Code is not explicitly handled by the ChangePasswordHandler
// switch propagates as a 5xx instead of falling through to the prior silent
// CodeUnknown HTTP-200 downgrade. Locks in the convention so a new AuthError
// code added without a dedicated handler case must surface as 5xx — code
// review forces a UX decision per code.
func TestChangePasswordHandler_UnknownAuthCodeFailsClosed(t *testing.T) {
	unknownErr := &autherrors.AuthError{Code: autherrors.CodeUnknown, Message: "unrouted auth code"}
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return unknownErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "old-password-123"
	input.Body.NewPassword = "new-password-456"

	resp, err := h.ChangePasswordHandler(ctx, input)

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestChangePasswordHandler_NonAuthErrorFailsClosed asserts that a raw
// (non-AuthError) service-call failure propagates as a 5xx via the apperror
// wrapper rather than the prior silent CodeUnknown HTTP-200 downgrade. Locks
// in the fail-closed outer fallback so unexpected backend defects surface in
// monitoring instead of being read by the client as a canonical sad-path body.
func TestChangePasswordHandler_NonAuthErrorFailsClosed(t *testing.T) {
	rawErr := fmt.Errorf("simulated db outage")
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			UpdatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return rawErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "old-password-123"
	input.Body.NewPassword = "new-password-456"

	resp, err := h.ChangePasswordHandler(ctx, input)

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// --- GetDeletionSummaryHandler mock tests ---

func TestGetDeletionSummaryHandler_Success(t *testing.T) {
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			GetDeletionSummaryFn: func(userID uint) (*contracts.DeletionSummary, error) {
				return &contracts.DeletionSummary{
					ShowsCount:      5,
					SavedShowsCount: 12,
					PasskeysCount:   2,
				}, nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, PasswordHash: &hash})

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.ShowsCount != 5 {
		t.Errorf("expected shows_count=5, got %d", resp.Body.ShowsCount)
	}
	if resp.Body.SavedShowsCount != 12 {
		t.Errorf("expected saved_shows_count=12, got %d", resp.Body.SavedShowsCount)
	}
	if resp.Body.PasskeysCount != 2 {
		t.Errorf("expected passkeys_count=2, got %d", resp.Body.PasskeysCount)
	}
	if !resp.Body.HasPassword {
		t.Error("expected has_password=true")
	}
}

// TestGetDeletionSummaryHandler_ServiceError asserts that a service-call
// failure from userService.GetDeletionSummary propagates as a 5xx rather
// than the prior silent HTTP-200 SERVICE_UNAVAILABLE downgrade. Locks in the
// fail-closed convention so an unexpected backend defect surfaces in
// monitoring instead of being read by the client as the canonical body shape.
func TestGetDeletionSummaryHandler_ServiceError(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			GetDeletionSummaryFn: func(userID uint) (*contracts.DeletionSummary, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// --- DeleteAccountHandler mock tests ---

func TestDeleteAccountHandler_Success(t *testing.T) {
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			VerifyPasswordFn: func(hashedPassword, password string) error {
				return nil
			},
			SoftDeleteAccountFn: func(userID uint, reason *string) error {
				return nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, PasswordHash: &hash})

	input := &DeleteAccountRequest{}
	input.Body.Password = "correct-password"

	resp, err := h.DeleteAccountHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
	if resp.SetCookie.MaxAge != -1 {
		t.Errorf("expected cleared cookie (MaxAge=-1), got %d", resp.SetCookie.MaxAge)
	}
	if resp.Body.GracePeriodDays != 30 {
		t.Errorf("expected grace_period_days=30, got %d", resp.Body.GracePeriodDays)
	}
}

func TestDeleteAccountHandler_WrongPassword(t *testing.T) {
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			VerifyPasswordFn: func(hashedPassword, password string) error {
				return fmt.Errorf("mismatch")
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, PasswordHash: &hash})

	input := &DeleteAccountRequest{}
	input.Body.Password = "wrong"

	resp, err := h.DeleteAccountHandler(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
	}
}

// TestDeleteAccountHandler_SoftDeleteFailsClosed asserts that a SoftDeleteAccount
// service failure propagates as a 5xx instead of being silently returned as
// HTTP 200 + SERVICE_UNAVAILABLE in the body. By the time execution reaches
// this branch the user is authenticated and password-verified, so this is the
// DESTRUCTIVE step — an unexpected service failure here must fire alerting
// rather than masquerade as a successful response. Locks in the fail-closed
// convention so a future "silent 200 on destructive op" regression is caught.
func TestDeleteAccountHandler_SoftDeleteFailsClosed(t *testing.T) {
	rawErr := fmt.Errorf("db error")
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			VerifyPasswordFn: func(hashedPassword, password string) error {
				return nil
			},
			SoftDeleteAccountFn: func(userID uint, reason *string) error {
				return rawErr
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, PasswordHash: &hash})

	input := &DeleteAccountRequest{}
	input.Body.Password = "correct"

	resp, err := h.DeleteAccountHandler(ctx, input)

	// The handler MUST return a non-nil error so Huma emits a 5xx HTTP status.
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp == nil {
		t.Fatal("expected non-nil response body")
	}
	// The returned error must wrap an *AuthError of CodeServiceUnavailable so
	// the apperror mapper translates it to a 5xx with the structured shape
	// clients branch on.
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected response error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	// External message must match the canonical SERVICE_UNAVAILABLE shape — no
	// leak about which internal step failed and consistent with the LoginHandler
	// fail-closed branches so clients render one error consistently.
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// --- ExportDataHandler mock tests ---

func TestExportDataHandler_Success(t *testing.T) {
	exportData := []byte(`{"user":{"id":1},"shows":[]}`)
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			ExportUserDataJSONFn: func(userID uint) ([]byte, error) {
				return exportData, nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.ExportDataHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ContentType != "application/json" {
		t.Errorf("expected content-type application/json, got %s", resp.ContentType)
	}
	if resp.ContentDisposition == "" {
		t.Error("expected content-disposition header")
	}
	if string(resp.Body) != string(exportData) {
		t.Errorf("expected export data, got %s", string(resp.Body))
	}
}

// TestExportDataHandler_ServiceErrorFailsClosed asserts that a service-call
// failure from userService.ExportUserDataJSON propagates as a 5xx via the
// apperror wrapper rather than the prior silent HTTP-200 with an inline JSON
// error body. The byte-body file-download shape is the user-visible behavior
// of the happy path; on failure, Huma's error mapper builds a canonical JSON
// 5xx from the returned error directly so monitoring sees the incident.
func TestExportDataHandler_ServiceErrorFailsClosed(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			ExportUserDataJSONFn: func(userID uint) ([]byte, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.ExportDataHandler(ctx, &struct{}{})

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	// The response Body must NOT carry the prior inline JSON "export_failed"
	// shape — Huma builds the response from the returned error directly. The
	// happy-path file-download shape is reserved for success.
	if len(resp.Body) != 0 {
		t.Errorf("expected empty resp.Body on service failure (Huma builds the body), got %q", string(resp.Body))
	}
}

// TestExportDataHandler_NoUserContextPreservesByteBodyShape locks in the
// intentional middleware-contract-violation path: a nil contextUser is NOT
// a service failure, so the response stays HTTP 200 with the inline JSON
// "unauthorized" body. Regression guard against the fail-closed conversion
// tipping over the unauthorized branch as well.
func TestExportDataHandler_NoUserContextPreservesByteBodyShape(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.ExportDataHandler(context.Background(), &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error on no-user-context path: %v", err)
	}
	if resp.ContentType != "application/json" {
		t.Errorf("expected content-type application/json, got %s", resp.ContentType)
	}
	if !strings.Contains(string(resp.Body), "unauthorized") {
		t.Errorf("expected body to contain 'unauthorized', got %q", string(resp.Body))
	}
}

// --- RecoverAccountHandler mock tests ---

func TestRecoverAccountHandler_Success(t *testing.T) {
	email := "test@example.com"
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, PasswordHash: &hash, IsActive: false}, nil
			},
			IsAccountRecoverableFn: func(user *authm.User) bool {
				return true
			},
			VerifyPasswordFn: func(hashedPassword, password string) error {
				return nil
			},
			RestoreAccountFn: func(userID uint) error {
				return nil
			},
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "recover-token", nil
			},
		}
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = email
	input.Body.Password = "correct"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
	if resp.Body.User == nil {
		t.Error("expected user in response")
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("expected auth cookie set, got name=%s", resp.SetCookie.Name)
	}
}

// recoverAccountState models a soft-deleted-account lookup outcome for the
// RecoverAccountHandler enumeration test (PSY-774).
type recoverAccountState struct {
	name string
	user func(email string) (*authm.User, error)
}

// recoverAccountStates enumerates the pre-success states whose responses must
// be indistinguishable: unknown email, active account, expired recovery
// window, no-password account, and known-recoverable account with the wrong
// password. All five must collapse to a single "Invalid credentials" body.
func recoverAccountStates(email string) []recoverAccountState {
	hash := "$2a$10$fakehash"
	return []recoverAccountState{
		{name: "unknown_email", user: func(string) (*authm.User, error) { return nil, nil }},
		{name: "active_account", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, IsActive: true}, nil
		}},
		{name: "expired_account", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, PasswordHash: &hash, IsActive: false}, nil
		}},
		{name: "no_password_account", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, IsActive: false}, nil
		}},
		{name: "wrong_password", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, PasswordHash: &hash, IsActive: false}, nil
		}},
	}
}

// TestRecoverAccountHandler_ResponseIdenticalAcrossAccountStates is the
// PSY-774 regression guard: the response body must be byte-identical
// regardless of account state, so the endpoint can't be used as an
// existence/state oracle.
func TestRecoverAccountHandler_ResponseIdenticalAcrossAccountStates(t *testing.T) {
	email := "probe@example.com"

	respFor := func(state recoverAccountState) RecoverAccountResponse {
		h := authHandler(func(ah *AuthHandler) {
			ah.userService = &testhelpers.MockUserService{
				GetUserByEmailIncludingDeletedFn: state.user,
				IsAccountRecoverableFn: func(user *authm.User) bool {
					// Expired branch must collapse onto the same failure body.
					return state.name != "expired_account"
				},
				VerifyPasswordFn: func(hashedPassword, password string) error {
					return fmt.Errorf("bcrypt mismatch")
				},
			}
		})
		input := &RecoverAccountRequest{}
		input.Body.Email = email
		input.Body.Password = "any-password"
		resp, err := h.RecoverAccountHandler(context.Background(), input)
		if err != nil {
			t.Fatalf("[%s] unexpected error: %v", state.name, err)
		}
		return *resp
	}

	states := recoverAccountStates(email)
	want := respFor(states[0])
	if want.Body.Success {
		t.Fatalf("expected success=false for enumeration-safe failure, got message=%q", want.Body.Message)
	}
	if want.Body.ErrorCode != autherrors.CodeInvalidCredentials {
		t.Fatalf("expected error_code=%s, got %q", autherrors.CodeInvalidCredentials, want.Body.ErrorCode)
	}
	for _, state := range states[1:] {
		got := respFor(state)
		if got.Body != want.Body {
			t.Errorf("response body for %s differs from %s:\n  got  %+v\n  want %+v",
				state.name, states[0].name, got.Body, want.Body)
		}
	}
}

// recoverAccountSuccessfulSetup builds an authHandler whose pre-success
// branches are all green (known recoverable account, correct password). The
// `tweak` callback flips one of the three post-success-eligibility service
// calls into a failure so each fail-closed regression test exercises a
// single specific branch.
func recoverAccountSuccessfulSetup(email string, tweak func(*testhelpers.MockUserService, *testhelpers.MockJWTService)) *AuthHandler {
	hash := "$2a$10$fakehash"
	return authHandler(func(ah *AuthHandler) {
		us := &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, PasswordHash: &hash, IsActive: false}, nil
			},
			IsAccountRecoverableFn: func(*authm.User) bool { return true },
			VerifyPasswordFn:       func(string, string) error { return nil },
			RestoreAccountFn:       func(uint) error { return nil },
			GetUserByIDFn: func(uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
		js := &testhelpers.MockJWTService{
			CreateTokenFn: func(*authm.User) (string, error) { return "session-token", nil },
		}
		tweak(us, js)
		ah.userService = us
		ah.jwtService = js
	})
}

// assertRecoverAccountFailedClosed factors out the regression-shape assertion
// reused by each of the three RecoverAccount post-success-eligibility failure
// branches: handler must return a non-nil error wrapping an *AuthError of
// CodeServiceUnavailable, and the response body must match the canonical
// SERVICE_UNAVAILABLE shape.
func assertRecoverAccountFailedClosed(t *testing.T, resp *RecoverAccountResponse, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestRecoverAccountHandler_RestoreFailsClosed locks in the post-success-
// eligibility fail-closed convention for the RestoreAccount branch: the
// caller has already authenticated (pre-success enumeration safety holds via
// invalidCredentials), so a service-level failure here is no longer an
// existence oracle and must surface as 5xx instead of HTTP 200.
func TestRecoverAccountHandler_RestoreFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := recoverAccountSuccessfulSetup(email, func(us *testhelpers.MockUserService, _ *testhelpers.MockJWTService) {
		us.RestoreAccountFn = func(uint) error { return fmt.Errorf("db error") }
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = email
	input.Body.Password = "correct"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	assertRecoverAccountFailedClosed(t, resp, err)
}

// TestRecoverAccountHandler_FetchFailsClosed mirrors the RestoreFailsClosed
// regression for the GetUserByID branch.
func TestRecoverAccountHandler_FetchFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := recoverAccountSuccessfulSetup(email, func(us *testhelpers.MockUserService, _ *testhelpers.MockJWTService) {
		us.GetUserByIDFn = func(uint) (*authm.User, error) { return nil, fmt.Errorf("db error") }
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = email
	input.Body.Password = "correct"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	assertRecoverAccountFailedClosed(t, resp, err)
}

// TestRecoverAccountHandler_TokenFailsClosed mirrors the RestoreFailsClosed
// regression for the JWT-token-mint branch.
func TestRecoverAccountHandler_TokenFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := recoverAccountSuccessfulSetup(email, func(_ *testhelpers.MockUserService, js *testhelpers.MockJWTService) {
		js.CreateTokenFn = func(*authm.User) (string, error) { return "", fmt.Errorf("jwt error") }
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = email
	input.Body.Password = "correct"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	assertRecoverAccountFailedClosed(t, resp, err)
}

// --- RequestAccountRecoveryHandler mock tests ---

// requestRecoveryAccountState models a soft-deleted-account lookup outcome
// for the RequestAccountRecoveryHandler enumeration test (PSY-774).
type requestRecoveryAccountState struct {
	name string
	user func(email string) (*authm.User, error)
}

// requestRecoveryAccountStates enumerates the four post-lookup states whose
// responses must be indistinguishable: unknown email, active account, expired
// recovery window, and a recoverable account (which triggers a real send).
func requestRecoveryAccountStates(email string) []requestRecoveryAccountState {
	hash := "$2a$10$fakehash"
	return []requestRecoveryAccountState{
		{name: "unknown_email", user: func(string) (*authm.User, error) { return nil, nil }},
		{name: "active_account", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, IsActive: true}, nil
		}},
		{name: "expired_account", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, PasswordHash: &hash, IsActive: false}, nil
		}},
		{name: "recoverable_account", user: func(e string) (*authm.User, error) {
			return &authm.User{ID: 1, Email: &e, PasswordHash: &hash, IsActive: false}, nil
		}},
	}
}

// TestRequestAccountRecoveryHandler_ResponseIdenticalAcrossAccountStates is
// the PSY-774 regression guard: the response body must be byte-identical
// regardless of whether the email is unknown, registered-active,
// registered-expired, or registered-recoverable, so the endpoint can't be
// used as an account-existence/recoverability oracle.
func TestRequestAccountRecoveryHandler_ResponseIdenticalAcrossAccountStates(t *testing.T) {
	email := "probe@example.com"

	respFor := func(state requestRecoveryAccountState) RequestAccountRecoveryResponse {
		h := authHandler(func(ah *AuthHandler) {
			ah.emailService = &testhelpers.MockEmailService{
				IsConfiguredFn:             func() bool { return true },
				SendAccountRecoveryEmailFn: func(string, string, int) error { return nil },
			}
			ah.userService = &testhelpers.MockUserService{
				GetUserByEmailIncludingDeletedFn: state.user,
				IsAccountRecoverableFn: func(user *authm.User) bool {
					// Expired branch must collapse onto the generic success body.
					return state.name != "expired_account"
				},
				GetDaysUntilPermanentDeletionFn: func(*authm.User) int { return 20 },
			}
			ah.jwtService = &testhelpers.MockJWTService{
				CreateAccountRecoveryTokenFn: func(uint, string) (string, error) { return "recovery-token", nil },
			}
		})
		input := &RequestAccountRecoveryRequest{}
		input.Body.Email = email
		resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
		if err != nil {
			t.Fatalf("[%s] unexpected error: %v", state.name, err)
		}
		return *resp
	}

	states := requestRecoveryAccountStates(email)
	want := respFor(states[0])
	if !want.Body.Success {
		t.Fatalf("expected success=true for enumeration-safe response, got message=%q", want.Body.Message)
	}
	if want.Body.ErrorCode != "" {
		t.Fatalf("expected empty error_code, got %q", want.Body.ErrorCode)
	}
	for _, state := range states[1:] {
		got := respFor(state)
		if got.Body != want.Body {
			t.Errorf("response body for %s differs from %s:\n  got  %+v\n  want %+v",
				state.name, states[0].name, got.Body, want.Body)
		}
	}
}

// TestRequestAccountRecoveryHandler_RecoverableSendsEmail asserts the side
// effect: a recoverable account triggers a real recovery email (PSY-774).
func TestRequestAccountRecoveryHandler_RecoverableSendsEmail(t *testing.T) {
	email := "recoverable@example.com"
	hash := "$2a$10$fakehash"
	var recoverySent bool
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return true },
			SendAccountRecoveryEmailFn: func(toEmail, token string, daysRemaining int) error {
				recoverySent = true
				return nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &e, PasswordHash: &hash, IsActive: false}, nil
			},
			IsAccountRecoverableFn:          func(*authm.User) bool { return true },
			GetDaysUntilPermanentDeletionFn: func(*authm.User) int { return 20 },
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateAccountRecoveryTokenFn: func(uint, string) (string, error) { return "recovery-token", nil },
		}
	})

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = email

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if !recoverySent {
		t.Error("expected recovery email to be sent for recoverable account")
	}
}

// TestRequestAccountRecoveryHandler_UnknownDoesNotSend asserts an unknown
// email triggers NO email send — the response stays generic but no side
// effect leaks downstream (PSY-774).
func TestRequestAccountRecoveryHandler_UnknownDoesNotSend(t *testing.T) {
	var recoverySent bool
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn: func() bool { return true },
			SendAccountRecoveryEmailFn: func(string, string, int) error {
				recoverySent = true
				return nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(string) (*authm.User, error) { return nil, nil },
		}
	})

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "nobody@example.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true (silent failure)")
	}
	if recoverySent {
		t.Error("expected NO recovery email for unknown user")
	}
}

// TestRequestAccountRecoveryHandler_SendFailureStaysGeneric asserts that a
// downstream send failure for a recoverable account does NOT change the
// response — otherwise the failure response would leak account state
// (PSY-774).
func TestRequestAccountRecoveryHandler_SendFailureStaysGeneric(t *testing.T) {
	email := "recoverable@example.com"
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &testhelpers.MockEmailService{
			IsConfiguredFn:             func() bool { return true },
			SendAccountRecoveryEmailFn: func(string, string, int) error { return fmt.Errorf("send exploded") },
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &e, PasswordHash: &hash, IsActive: false}, nil
			},
			IsAccountRecoverableFn:          func(*authm.User) bool { return true },
			GetDaysUntilPermanentDeletionFn: func(*authm.User) int { return 20 },
		}
		ah.jwtService = &testhelpers.MockJWTService{
			CreateAccountRecoveryTokenFn: func(uint, string) (string, error) { return "recovery-token", nil },
		}
	})

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = email

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true even when send fails (no enumeration leak)")
	}
	if resp.Body.ErrorCode != "" {
		t.Errorf("expected empty error_code on send failure, got %q", resp.Body.ErrorCode)
	}
}

// --- ConfirmAccountRecoveryHandler mock tests ---

func TestConfirmAccountRecoveryHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateAccountRecoveryTokenFn: func(tokenString string) (*contracts.AccountRecoveryTokenClaims, error) {
				return &contracts.AccountRecoveryTokenClaims{UserID: 1, Email: email}, nil
			},
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "session-token", nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: false}, nil
			},
			IsAccountRecoverableFn: func(user *authm.User) bool {
				return true
			},
			RestoreAccountFn: func(userID uint) error {
				return nil
			},
			GetUserByIDFn: func(userID uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "valid-recovery-token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
	if resp.Body.User == nil {
		t.Error("expected user in response")
	}
	if resp.SetCookie.Name != config.AuthCookieName {
		t.Errorf("expected auth cookie set, got name=%s", resp.SetCookie.Name)
	}
}

func TestConfirmAccountRecoveryHandler_UserNotFound(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateAccountRecoveryTokenFn: func(tokenString string) (*contracts.AccountRecoveryTokenClaims, error) {
				return &contracts.AccountRecoveryTokenClaims{UserID: 999, Email: "test@example.com"}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(e string) (*authm.User, error) {
				return nil, fmt.Errorf("not found")
			},
		}
	})

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
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

func TestConfirmAccountRecoveryHandler_AlreadyActive(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			ValidateAccountRecoveryTokenFn: func(tokenString string) (*contracts.AccountRecoveryTokenClaims, error) {
				return &contracts.AccountRecoveryTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(e string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "ACCOUNT_ACTIVE" {
		t.Errorf("expected error_code=ACCOUNT_ACTIVE, got %s", resp.Body.ErrorCode)
	}
}

// confirmAccountRecoverySuccessfulSetup builds an authHandler whose pre-
// success branches are all green (valid token, matching user, recoverable
// inactive account). The `tweak` callback flips one of the three post-
// success-eligibility service calls into a failure so each fail-closed
// regression test exercises a single specific branch.
func confirmAccountRecoverySuccessfulSetup(email string, tweak func(*testhelpers.MockUserService, *testhelpers.MockJWTService)) *AuthHandler {
	return authHandler(func(ah *AuthHandler) {
		us := &testhelpers.MockUserService{
			GetUserByEmailIncludingDeletedFn: func(string) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: false}, nil
			},
			IsAccountRecoverableFn: func(*authm.User) bool { return true },
			RestoreAccountFn:       func(uint) error { return nil },
			GetUserByIDFn: func(uint) (*authm.User, error) {
				return &authm.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
		js := &testhelpers.MockJWTService{
			ValidateAccountRecoveryTokenFn: func(string) (*contracts.AccountRecoveryTokenClaims, error) {
				return &contracts.AccountRecoveryTokenClaims{UserID: 1, Email: email}, nil
			},
			CreateTokenFn: func(*authm.User) (string, error) { return "session-token", nil },
		}
		tweak(us, js)
		ah.userService = us
		ah.jwtService = js
	})
}

// assertConfirmAccountRecoveryFailedClosed factors out the regression-shape
// assertion reused by each of the three ConfirmAccountRecovery post-success-
// eligibility failure branches.
func assertConfirmAccountRecoveryFailedClosed(t *testing.T, resp *ConfirmAccountRecoveryResponse, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestConfirmAccountRecoveryHandler_RestoreFailsClosed asserts that a
// RestoreAccount service failure at the final write step propagates as a 5xx
// rather than the prior silent HTTP-200 SERVICE_UNAVAILABLE downgrade. By
// this point the token + user-ID + recoverability checks have all passed, so
// a service failure is an unexpected backend defect — not a UX condition.
// The intentional UX shapes earlier in the handler (INVALID_TOKEN,
// UNAUTHORIZED, ACCOUNT_ACTIVE, ACCOUNT_NOT_RECOVERABLE) still return HTTP
// 200 with structured ErrorCode so the frontend can render the corresponding
// form state.
func TestConfirmAccountRecoveryHandler_RestoreFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := confirmAccountRecoverySuccessfulSetup(email, func(us *testhelpers.MockUserService, _ *testhelpers.MockJWTService) {
		us.RestoreAccountFn = func(uint) error { return fmt.Errorf("db error") }
	})

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	assertConfirmAccountRecoveryFailedClosed(t, resp, err)
}

// TestConfirmAccountRecoveryHandler_FetchFailsClosed mirrors the
// RestoreFailsClosed regression for the GetUserByID branch.
func TestConfirmAccountRecoveryHandler_FetchFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := confirmAccountRecoverySuccessfulSetup(email, func(us *testhelpers.MockUserService, _ *testhelpers.MockJWTService) {
		us.GetUserByIDFn = func(uint) (*authm.User, error) { return nil, fmt.Errorf("db error") }
	})

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	assertConfirmAccountRecoveryFailedClosed(t, resp, err)
}

// TestConfirmAccountRecoveryHandler_TokenFailsClosed mirrors the
// RestoreFailsClosed regression for the JWT-token-mint branch.
func TestConfirmAccountRecoveryHandler_TokenFailsClosed(t *testing.T) {
	email := "test@example.com"
	h := confirmAccountRecoverySuccessfulSetup(email, func(_ *testhelpers.MockUserService, js *testhelpers.MockJWTService) {
		js.CreateTokenFn = func(*authm.User) (string, error) { return "", fmt.Errorf("jwt error") }
	})

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = "valid-token"

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	assertConfirmAccountRecoveryFailedClosed(t, resp, err)
}

// --- GenerateCLITokenHandler mock tests ---

func TestGenerateCLITokenHandler_Success(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "cli-token-123", nil
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	resp, err := h.GenerateCLITokenHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if resp.Body.Token != "cli-token-123" {
		t.Errorf("expected token=cli-token-123, got %s", resp.Body.Token)
	}
	if resp.Body.ExpiresIn != 86400 {
		t.Errorf("expected expires_in=86400, got %d", resp.Body.ExpiresIn)
	}
}

// TestGenerateCLITokenHandler_TokenFails asserts that a JWT-service failure
// at the CLI-token mint step propagates as a 5xx rather than the prior
// silent HTTP-200 SERVICE_UNAVAILABLE downgrade. Locks in the fail-closed
// convention so an unexpected backend defect surfaces in monitoring instead
// of being read by the CLI client as the canonical body shape.
func TestGenerateCLITokenHandler_TokenFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &testhelpers.MockJWTService{
			CreateTokenFn: func(u *authm.User) (string, error) {
				return "", fmt.Errorf("jwt error")
			},
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})

	resp, err := h.GenerateCLITokenHandler(ctx, &struct{}{})

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// --- UpdateProfileHandler ---
//
// UpdateProfileHandler returns a 200 with a structured Success/ErrorCode body
// rather than huma errors, so assertions read those fields instead of an HTTP
// status.

func TestUpdateProfileHandler_NoAuth(t *testing.T) {
	h := testAuthHandler()
	resp, err := h.UpdateProfileHandler(context.Background(), &UpdateProfileRequest{})
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

func TestUpdateProfileHandler_UsernameTooShort(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.Username = strPtr("ab") // < 3 chars

	resp, err := h.UpdateProfileHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected validation failure, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
	}
}

func TestUpdateProfileHandler_UsernameInvalidChars(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.Username = strPtr("bad name!") // space + punctuation

	resp, err := h.UpdateProfileHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected validation failure, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
	}
}

func TestUpdateProfileHandler_BioTooLong(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.Bio = strPtr(strings.Repeat("x", 501))

	resp, err := h.UpdateProfileHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected validation failure, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
	}
}

func TestUpdateProfileHandler_NoFields(t *testing.T) {
	h := testAuthHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.UpdateProfileHandler(ctx, &UpdateProfileRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected validation failure for empty update, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
	}
}

func TestUpdateProfileHandler_Success(t *testing.T) {
	mock := &testhelpers.MockUserService{
		UpdateUserFn: func(userID uint, updates map[string]any) (*authm.User, error) {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			if updates["username"] != "new_name" {
				t.Errorf("expected username=new_name, got %v", updates["username"])
			}
			return &authm.User{ID: 1, Username: strPtr("new_name")}, nil
		},
	}
	h := NewAuthHandler(nil, nil, mock, nil, nil, nil, testConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.Username = strPtr("new_name")

	resp, err := h.UpdateProfileHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatalf("expected success=true, got message=%q code=%s", resp.Body.Message, resp.Body.ErrorCode)
	}
	if resp.Body.User == nil || resp.Body.User.Username == nil || *resp.Body.User.Username != "new_name" {
		t.Errorf("expected updated user with username=new_name, got %+v", resp.Body.User)
	}
}

func TestUpdateProfileHandler_DuplicateUsername(t *testing.T) {
	mock := &testhelpers.MockUserService{
		UpdateUserFn: func(_ uint, _ map[string]any) (*authm.User, error) {
			return nil, autherrors.ErrUsernameTaken(fmt.Errorf("duplicate key value violates unique constraint"))
		},
	}
	h := NewAuthHandler(nil, nil, mock, nil, nil, nil, testConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.Username = strPtr("taken_name")

	resp, err := h.UpdateProfileHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeValidationFailed {
		t.Errorf("expected validation failure for duplicate username, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
	}
}

// TestUpdateProfileHandler_ServiceError asserts that a raw (non-AuthError)
// service-call failure propagates as a 5xx via the apperror wrapper rather
// than the prior silent HTTP-200 SERVICE_UNAVAILABLE downgrade. Locks in the
// fail-closed outer fallback so unexpected backend defects surface in
// monitoring instead of being read by the client as a canonical sad-path body.
func TestUpdateProfileHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockUserService{
		UpdateUserFn: func(_ uint, _ map[string]any) (*authm.User, error) {
			return nil, fmt.Errorf("db connection lost")
		},
	}
	h := NewAuthHandler(nil, nil, mock, nil, nil, nil, testConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.FirstName = strPtr("Jane")

	resp, err := h.UpdateProfileHandler(ctx, req)

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	var authErr *autherrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap an *AuthError, got %T", err)
	}
	if authErr.Code != autherrors.CodeServiceUnavailable {
		t.Errorf("expected wrapped error code=%s, got %s", autherrors.CodeServiceUnavailable, authErr.Code)
	}
	if resp.Body.Success || resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected service-unavailable failure, got success=%v code=%s", resp.Body.Success, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}

// TestUpdateProfileHandler_UnknownAuthCodeFailsClosed asserts that an
// AuthError whose Code is not explicitly handled by the UpdateProfileHandler
// switch propagates as a 5xx instead of falling through to the silent
// SERVICE_UNAVAILABLE downgrade. Locks in the convention so a new AuthError
// code added without a dedicated handler case must surface as 5xx — code
// review forces a UX decision per code.
func TestUpdateProfileHandler_UnknownAuthCodeFailsClosed(t *testing.T) {
	unknownErr := &autherrors.AuthError{Code: autherrors.CodeUnknown, Message: "unrouted auth code"}
	mock := &testhelpers.MockUserService{
		UpdateUserFn: func(_ uint, _ map[string]any) (*authm.User, error) {
			return nil, unknownErr
		},
	}
	h := NewAuthHandler(nil, nil, mock, nil, nil, nil, testConfig())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UpdateProfileRequest{}
	req.Body.FirstName = strPtr("Jane")

	resp, err := h.UpdateProfileHandler(ctx, req)

	if err == nil {
		t.Fatal("expected non-nil error so Huma emits a 5xx HTTP status")
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
	if resp.Body.Message != autherrors.ToExternalMessage(autherrors.CodeServiceUnavailable) {
		t.Errorf("expected generic SERVICE_UNAVAILABLE message, got %q", resp.Body.Message)
	}
}
