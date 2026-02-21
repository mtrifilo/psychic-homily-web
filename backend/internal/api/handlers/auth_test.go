package handlers

import (
	"context"
	"fmt"
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

// ============================================================================
// Mock-based auth handler tests
// ============================================================================

// authHandler builds an AuthHandler with mock services and optional overrides.
func authHandler(opts ...func(*AuthHandler)) *AuthHandler {
	h := &AuthHandler{
		authService:       &mockAuthService{},
		jwtService:        &mockJWTService{},
		userService:       &mockUserService{},
		emailService:      &mockEmailService{},
		discordService:    &mockDiscordService{},
		passwordValidator: &mockPasswordValidator{},
		config:            testConfig(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// --- LoginHandler mock tests ---

func TestLoginHandler_Success(t *testing.T) {
	user := &models.User{ID: 1, Email: strPtr("test@example.com")}
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			authenticateUserWithPasswordFn: func(email, password string) (*models.User, error) {
				return user, nil
			},
		}
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
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
		ah.userService = &mockUserService{
			authenticateUserWithPasswordFn: func(email, password string) (*models.User, error) {
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
		ah.userService = &mockUserService{
			authenticateUserWithPasswordFn: func(email, password string) (*models.User, error) {
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

func TestLoginHandler_TokenFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			authenticateUserWithPasswordFn: func(email, password string) (*models.User, error) {
				return &models.User{ID: 1}, nil
			},
		}
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
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
	user := &models.User{ID: 10, Email: strPtr("new@example.com")}
	var discordCalled bool
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			createUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance services.LegalAcceptance) (*models.User, error) {
				return user, nil
			},
		}
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
				return "reg-token", nil
			},
		}
		ah.discordService = &mockDiscordService{
			notifyNewUserFn: func(u *models.User) {
				discordCalled = true
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "new@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"

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

func TestRegisterHandler_UserExists(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			createUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance services.LegalAcceptance) (*models.User, error) {
				return nil, autherrors.ErrUserExists(email)
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "existing@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"

	resp, err := h.RegisterHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != autherrors.CodeUserExists {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeUserExists, resp.Body.ErrorCode)
	}
}

func TestRegisterHandler_WeakPasswordMock(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.passwordValidator = &mockPasswordValidator{
			validatePasswordFn: func(password string) (*services.PasswordValidationResult, error) {
				return &services.PasswordValidationResult{
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
		ah.userService = &mockUserService{
			createUserWithPasswordWithLegalFn: func(email, password, firstName, lastName string, acceptance services.LegalAcceptance) (*models.User, error) {
				return &models.User{ID: 1}, nil
			},
		}
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
				return "", fmt.Errorf("jwt error")
			},
		}
	})

	input := &RegisterRequest{}
	input.Body.Email = "user@example.com"
	input.Body.Password = "a-valid-password-123"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"

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
	user := &models.User{ID: 1, Email: strPtr("test@example.com")}
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &mockAuthService{
			getUserProfileFn: func(userID uint) (*models.User, error) {
				return user, nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

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

func TestGetProfileHandler_ServiceError(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &mockAuthService{
			getUserProfileFn: func(userID uint) (*models.User, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetProfileHandler(ctx, &struct{}{})
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

// --- RefreshTokenHandler mock tests ---

func TestRefreshTokenHandler_Success(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &mockAuthService{
			getUserProfileFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: userID}, nil
			},
			refreshUserTokenFn: func(user *models.User) (string, error) {
				return "new-token", nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

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

func TestRefreshTokenHandler_ProfileFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &mockAuthService{
			getUserProfileFn: func(userID uint) (*models.User, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})
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

func TestRefreshTokenHandler_TokenFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &mockAuthService{
			getUserProfileFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: userID}, nil
			},
			refreshUserTokenFn: func(user *models.User) (string, error) {
				return "", fmt.Errorf("token error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})
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

// --- SendVerificationEmailHandler mock tests ---

func TestSendVerificationEmailHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return true },
			sendVerificationEmailFn: func(toEmail, token string) error {
				return nil
			},
		}
		ah.jwtService = &mockJWTService{
			createVerificationTokenFn: func(userID uint, e string) (string, error) {
				return "verify-token", nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, Email: &email, EmailVerified: false})

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
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return false },
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, Email: &email, EmailVerified: false})

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
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return true },
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, Email: nil, EmailVerified: false})

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

func TestSendVerificationEmailHandler_TokenFails(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return true },
		}
		ah.jwtService = &mockJWTService{
			createVerificationTokenFn: func(userID uint, e string) (string, error) {
				return "", fmt.Errorf("token error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, Email: &email, EmailVerified: false})

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

// --- ConfirmVerificationHandler mock tests ---

func TestConfirmVerificationHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &mockJWTService{
			validateVerificationTokenFn: func(tokenString string) (*services.VerificationTokenClaims, error) {
				return &services.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, EmailVerified: false}, nil
			},
			setEmailVerifiedFn: func(userID uint, verified bool) error {
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
		ah.jwtService = &mockJWTService{
			validateVerificationTokenFn: func(tokenString string) (*services.VerificationTokenClaims, error) {
				return &services.VerificationTokenClaims{UserID: 999, Email: "test@example.com"}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
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
		ah.jwtService = &mockJWTService{
			validateVerificationTokenFn: func(tokenString string) (*services.VerificationTokenClaims, error) {
				return &services.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, EmailVerified: true}, nil
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

func TestConfirmVerificationHandler_SetVerifiedFails(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &mockJWTService{
			validateVerificationTokenFn: func(tokenString string) (*services.VerificationTokenClaims, error) {
				return &services.VerificationTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, EmailVerified: false}, nil
			},
			setEmailVerifiedFn: func(userID uint, verified bool) error {
				return fmt.Errorf("db error")
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
	if resp.Body.ErrorCode != autherrors.CodeServiceUnavailable {
		t.Errorf("expected error_code=%s, got %s", autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
	}
}

// --- SendMagicLinkHandler mock tests ---

func TestSendMagicLinkHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &mockEmailService{
			isConfiguredFn:       func() bool { return true },
			sendMagicLinkEmailFn: func(toEmail, token string) error { return nil },
		}
		ah.userService = &mockUserService{
			getUserByEmailFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, EmailVerified: true}, nil
			},
		}
		ah.jwtService = &mockJWTService{
			createMagicLinkTokenFn: func(userID uint, e string) (string, error) {
				return "magic-token", nil
			},
		}
	})

	input := &SendMagicLinkRequest{}
	input.Body.Email = email

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
}

func TestSendMagicLinkHandler_UserNotFound(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return true },
		}
		ah.userService = &mockUserService{
			getUserByEmailFn: func(e string) (*models.User, error) {
				return nil, nil // user not found
			},
		}
	})

	input := &SendMagicLinkRequest{}
	input.Body.Email = "nobody@example.com"

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return success to prevent email enumeration
	if !resp.Body.Success {
		t.Error("expected success=true (silent failure)")
	}
}

func TestSendMagicLinkHandler_EmailNotVerified(t *testing.T) {
	email := "unverified@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return true },
		}
		ah.userService = &mockUserService{
			getUserByEmailFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, EmailVerified: false}, nil
			},
		}
	})

	input := &SendMagicLinkRequest{}
	input.Body.Email = email

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "EMAIL_NOT_VERIFIED" {
		t.Errorf("expected error_code=EMAIL_NOT_VERIFIED, got %s", resp.Body.ErrorCode)
	}
}

// --- VerifyMagicLinkHandler mock tests ---

func TestVerifyMagicLinkHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &mockJWTService{
			validateMagicLinkTokenFn: func(tokenString string) (*services.MagicLinkTokenClaims, error) {
				return &services.MagicLinkTokenClaims{UserID: 1, Email: email}, nil
			},
			createTokenFn: func(u *models.User) (string, error) {
				return "session-token", nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: true}, nil
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
		ah.jwtService = &mockJWTService{
			validateMagicLinkTokenFn: func(tokenString string) (*services.MagicLinkTokenClaims, error) {
				return &services.MagicLinkTokenClaims{UserID: 999, Email: "test@example.com"}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
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
		ah.jwtService = &mockJWTService{
			validateMagicLinkTokenFn: func(tokenString string) (*services.MagicLinkTokenClaims, error) {
				return &services.MagicLinkTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: false}, nil
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

// --- ChangePasswordHandler mock tests ---

func TestChangePasswordHandler_Success(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			updatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

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
		ah.userService = &mockUserService{
			updatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return autherrors.ErrInvalidCredentials(nil)
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

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
		ah.userService = &mockUserService{
			updatePasswordFn: func(userID uint, currentPassword, newPassword string) error {
				return autherrors.ErrNoPasswordSet()
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

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

// --- GetDeletionSummaryHandler mock tests ---

func TestGetDeletionSummaryHandler_Success(t *testing.T) {
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getDeletionSummaryFn: func(userID uint) (*services.DeletionSummary, error) {
				return &services.DeletionSummary{
					ShowsCount:      5,
					SavedShowsCount: 12,
					PasskeysCount:   2,
				}, nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, PasswordHash: &hash})

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

func TestGetDeletionSummaryHandler_ServiceError(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getDeletionSummaryFn: func(userID uint) (*services.DeletionSummary, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})
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

// --- DeleteAccountHandler mock tests ---

func TestDeleteAccountHandler_Success(t *testing.T) {
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			verifyPasswordFn: func(hashedPassword, password string) error {
				return nil
			},
			softDeleteAccountFn: func(userID uint, reason *string) error {
				return nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, PasswordHash: &hash})

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
		ah.userService = &mockUserService{
			verifyPasswordFn: func(hashedPassword, password string) error {
				return fmt.Errorf("mismatch")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, PasswordHash: &hash})

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

func TestDeleteAccountHandler_SoftDeleteFails(t *testing.T) {
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			verifyPasswordFn: func(hashedPassword, password string) error {
				return nil
			},
			softDeleteAccountFn: func(userID uint, reason *string) error {
				return fmt.Errorf("db error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, PasswordHash: &hash})

	input := &DeleteAccountRequest{}
	input.Body.Password = "correct"

	resp, err := h.DeleteAccountHandler(ctx, input)
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

// --- ExportDataHandler mock tests ---

func TestExportDataHandler_Success(t *testing.T) {
	exportData := []byte(`{"user":{"id":1},"shows":[]}`)
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			exportUserDataJSONFn: func(userID uint) ([]byte, error) {
				return exportData, nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

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

func TestExportDataHandler_ServiceError(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			exportUserDataJSONFn: func(userID uint) ([]byte, error) {
				return nil, fmt.Errorf("db error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1})

	resp, err := h.ExportDataHandler(ctx, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(resp.Body), "export_failed") {
		t.Errorf("expected error body, got %s", string(resp.Body))
	}
}

// --- RecoverAccountHandler mock tests ---

func TestRecoverAccountHandler_Success(t *testing.T) {
	email := "test@example.com"
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, PasswordHash: &hash, IsActive: false}, nil
			},
			isAccountRecoverableFn: func(user *models.User) bool {
				return true
			},
			verifyPasswordFn: func(hashedPassword, password string) error {
				return nil
			},
			restoreAccountFn: func(userID uint) error {
				return nil
			},
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
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

func TestRecoverAccountHandler_UserNotFound(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return nil, nil // user not found
			},
		}
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = "nobody@example.com"
	input.Body.Password = "password"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
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

func TestRecoverAccountHandler_AccountActive(t *testing.T) {
	email := "active@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = email
	input.Body.Password = "password"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
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

func TestRecoverAccountHandler_NotRecoverable(t *testing.T) {
	email := "expired@example.com"
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, PasswordHash: &hash, IsActive: false}, nil
			},
			isAccountRecoverableFn: func(user *models.User) bool {
				return false
			},
		}
	})

	input := &RecoverAccountRequest{}
	input.Body.Email = email
	input.Body.Password = "password"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Success {
		t.Error("expected success=false")
	}
	if resp.Body.ErrorCode != "ACCOUNT_NOT_RECOVERABLE" {
		t.Errorf("expected error_code=ACCOUNT_NOT_RECOVERABLE, got %s", resp.Body.ErrorCode)
	}
}

// --- RequestAccountRecoveryHandler mock tests ---

func TestRequestAccountRecoveryHandler_Success(t *testing.T) {
	email := "deleted@example.com"
	hash := "$2a$10$fakehash"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, PasswordHash: &hash, IsActive: false}, nil
			},
			isAccountRecoverableFn: func(user *models.User) bool {
				return true
			},
			getDaysUntilPermanentDeletionFn: func(user *models.User) int {
				return 20
			},
		}
		ah.emailService = &mockEmailService{
			isConfiguredFn: func() bool { return true },
			sendAccountRecoveryEmailFn: func(toEmail, token string, daysRemaining int) error {
				return nil
			},
		}
		ah.jwtService = &mockJWTService{
			createAccountRecoveryTokenFn: func(userID uint, e string) (string, error) {
				return "recovery-token", nil
			},
		}
	})

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = email

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Errorf("expected success=true, got message=%q", resp.Body.Message)
	}
	if resp.Body.DaysRemaining != 20 {
		t.Errorf("expected days_remaining=20, got %d", resp.Body.DaysRemaining)
	}
}

func TestRequestAccountRecoveryHandler_UserNotFound(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return nil, nil
			},
		}
	})

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "nobody@example.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return success to prevent email enumeration
	if !resp.Body.Success {
		t.Error("expected success=true (silent failure)")
	}
}

func TestRequestAccountRecoveryHandler_AccountActive(t *testing.T) {
	email := "active@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: true}, nil
			},
		}
	})

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = email

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
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

// --- ConfirmAccountRecoveryHandler mock tests ---

func TestConfirmAccountRecoveryHandler_Success(t *testing.T) {
	email := "test@example.com"
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &mockJWTService{
			validateAccountRecoveryTokenFn: func(tokenString string) (*services.AccountRecoveryTokenClaims, error) {
				return &services.AccountRecoveryTokenClaims{UserID: 1, Email: email}, nil
			},
			createTokenFn: func(u *models.User) (string, error) {
				return "session-token", nil
			},
		}
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: false}, nil
			},
			isAccountRecoverableFn: func(user *models.User) bool {
				return true
			},
			restoreAccountFn: func(userID uint) error {
				return nil
			},
			getUserByIDFn: func(userID uint) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: true}, nil
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
		ah.jwtService = &mockJWTService{
			validateAccountRecoveryTokenFn: func(tokenString string) (*services.AccountRecoveryTokenClaims, error) {
				return &services.AccountRecoveryTokenClaims{UserID: 999, Email: "test@example.com"}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
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
		ah.jwtService = &mockJWTService{
			validateAccountRecoveryTokenFn: func(tokenString string) (*services.AccountRecoveryTokenClaims, error) {
				return &services.AccountRecoveryTokenClaims{UserID: 1, Email: email}, nil
			},
		}
		ah.userService = &mockUserService{
			getUserByEmailIncludingDeletedFn: func(e string) (*models.User, error) {
				return &models.User{ID: 1, Email: &email, IsActive: true}, nil
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

// --- GenerateCLITokenHandler mock tests ---

func TestGenerateCLITokenHandler_Success(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
				return "cli-token-123", nil
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

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

func TestGenerateCLITokenHandler_TokenFails(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.jwtService = &mockJWTService{
			createTokenFn: func(u *models.User) (string, error) {
				return "", fmt.Errorf("jwt error")
			},
		}
	})
	ctx := ctxWithUser(&models.User{ID: 1, IsAdmin: true})

	resp, err := h.GenerateCLITokenHandler(ctx, &struct{}{})
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
