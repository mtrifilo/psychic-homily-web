package handlers

import (
	"context"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
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

func ctxWithUser(user *models.User) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, user)
}

func strPtr(s string) *string {
	return &s
}

func testJWTService() *services.JWTService {
	return services.NewJWTService(nil, testConfig())
}

// --- TestNewAuthHandler ---

func TestNewAuthHandler(t *testing.T) {
	h := NewAuthHandler(nil, nil, nil, nil, nil, nil, testConfig())
	if h == nil {
		t.Fatal("expected non-nil AuthHandler")
	}
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
	ctx := ctxWithUser(&models.User{ID: 1})

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

func TestRefreshTokenHandler_NilAuthService(t *testing.T) {
	h := testAuthHandler() // authService is nil

	resp, err := h.RefreshTokenHandler(context.Background(), &struct{}{})
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

func TestRefreshTokenHandler_NoUserContext(t *testing.T) {
	// Need a non-nil authService to pass the nil check
	authSvc := services.NewAuthService(nil, testConfig())
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

func TestGetProfileHandler_NilAuthService(t *testing.T) {
	h := testAuthHandler()

	resp, err := h.GetProfileHandler(context.Background(), &struct{}{})
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

func TestGetProfileHandler_NoUserContext(t *testing.T) {
	authSvc := services.NewAuthService(nil, testConfig())
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

func TestRegisterHandler_NilUserService(t *testing.T) {
	// No passwordValidator, no userService → hits userService nil check
	h := testAuthHandler()
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
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
	pv := services.NewPasswordValidator()
	h := NewAuthHandler(nil, nil, nil, nil, nil, pv, testConfig())
	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "abc" // too short (min 12)
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
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
	user := &models.User{ID: 1, EmailVerified: true}
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(&models.User{ID: 1})
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
	ctx := ctxWithUser(&models.User{ID: 1})
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
	user := &models.User{ID: 1, PasswordHash: nil, IsActive: true}
	ctx := ctxWithUser(user)
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
	user := &models.User{ID: 1, PasswordHash: &hash, IsActive: true}
	ctx := ctxWithUser(user)
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

func TestGenerateCLITokenHandler_NonAdmin(t *testing.T) {
	h := testAuthHandler()
	user := &models.User{ID: 1, IsAdmin: false}
	ctx := ctxWithUser(user)

	resp, err := h.GenerateCLITokenHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUnauthorized {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUnauthorized, resp.Body.ErrorCode)
	}
	if !strings.Contains(resp.Body.Message, "admin") {
		t.Errorf("expected message about admin, got %q", resp.Body.Message)
	}
}
