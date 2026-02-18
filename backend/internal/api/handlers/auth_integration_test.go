package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// --- Suite ---

type AuthHandlerIntegrationSuite struct {
	suite.Suite
	deps *handlerIntegrationDeps
	cfg  *config.Config
}

func TestAuthHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(AuthHandlerIntegrationSuite))
}

func (s *AuthHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.cfg = &config.Config{
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

func (s *AuthHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *AuthHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

// --- Helpers ---

func (s *AuthHandlerIntegrationSuite) newAuthHandler(emailConfigured bool) *AuthHandler {
	emailCfg := s.cfg
	if emailConfigured {
		emailCfg = &config.Config{
			JWT:     s.cfg.JWT,
			Session: s.cfg.Session,
			Email: config.EmailConfig{
				ResendAPIKey: "re_fake_key_for_testing",
				FromEmail:    "test@psychichomily.com",
				FrontendURL:  "http://localhost:3000",
			},
		}
	}

	authSvc := services.NewAuthService(s.deps.db, emailCfg)
	jwtSvc := services.NewJWTService(s.deps.db, emailCfg)
	emailSvc := services.NewEmailService(emailCfg)
	discordSvc := services.NewDiscordService(emailCfg)
	pv := services.NewPasswordValidator()

	return NewAuthHandler(
		authSvc,
		jwtSvc,
		s.deps.userService,
		emailSvc,
		discordSvc,
		pv,
		emailCfg,
	)
}

func (s *AuthHandlerIntegrationSuite) createUserWithPassword(email, password string) *models.User {
	user, err := s.deps.userService.CreateUserWithPassword(email, password, "Test", "User")
	s.Require().NoError(err)
	return user
}

func (s *AuthHandlerIntegrationSuite) ctxWithUser(user *models.User) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, user)
}

func (s *AuthHandlerIntegrationSuite) softDeleteUser(userID uint) {
	err := s.deps.db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"deleted_at": time.Now(),
		}).Error
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) softDeleteUserExpired(userID uint) {
	err := s.deps.db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"deleted_at": time.Now().AddDate(0, 0, -31),
		}).Error
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) lockAccount(userID uint) {
	err := s.deps.db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"failed_login_attempts": 5,
			"locked_until":          time.Now().Add(15 * time.Minute),
		}).Error
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) verifyEmail(userID uint) {
	err := s.deps.userService.SetEmailVerified(userID, true)
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) reloadUser(userID uint) *models.User {
	user, err := s.deps.userService.GetUserByID(userID)
	s.Require().NoError(err)
	return user
}

// --- LoginHandler ---

func (s *AuthHandlerIntegrationSuite) TestLogin_Success() {
	h := s.newAuthHandler(false)
	s.createUserWithPassword("login-ok@test.com", "strong-password-123!")

	input := &LoginRequest{}
	input.Body.Email = "login-ok@test.com"
	input.Body.Password = "strong-password-123!"

	resp, err := h.LoginHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotEmpty(resp.Body.Token)
	s.NotNil(resp.Body.User)
	s.Equal("login-ok@test.com", *resp.Body.User.Email)
	s.NotEmpty(resp.SetCookie.Name)
}

func (s *AuthHandlerIntegrationSuite) TestLogin_InvalidPassword() {
	h := s.newAuthHandler(false)
	s.createUserWithPassword("login-bad@test.com", "strong-password-123!")

	input := &LoginRequest{}
	input.Body.Email = "login-bad@test.com"
	input.Body.Password = "wrong-password-999!"

	resp, err := h.LoginHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestLogin_UserNotFound() {
	h := s.newAuthHandler(false)

	input := &LoginRequest{}
	input.Body.Email = "nobody@test.com"
	input.Body.Password = "doesnt-matter-12345"

	resp, err := h.LoginHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestLogin_AccountLocked() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("locked@test.com", "strong-password-123!")
	s.lockAccount(user.ID)

	input := &LoginRequest{}
	input.Body.Email = "locked@test.com"
	input.Body.Password = "strong-password-123!"

	resp, err := h.LoginHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeAccountLocked, resp.Body.ErrorCode)
}

// --- RegisterHandler ---

func (s *AuthHandlerIntegrationSuite) TestRegister_Success() {
	h := s.newAuthHandler(false)

	input := &RegisterRequest{}
	input.Body.Email = "new-user@test.com"
	input.Body.Password = "very-strong-password-123!"
	input.Body.FirstName = stringPtr("Jane")
	input.Body.LastName = stringPtr("Doe")
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.PrivacyVersion = "2026-02-15"

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotEmpty(resp.Body.Token)
	s.NotNil(resp.Body.User)
	s.Equal("new-user@test.com", *resp.Body.User.Email)
	s.NotEmpty(resp.SetCookie.Name)
}

func (s *AuthHandlerIntegrationSuite) TestRegister_DuplicateEmail() {
	h := s.newAuthHandler(false)
	s.createUserWithPassword("dup@test.com", "strong-password-123!")

	input := &RegisterRequest{}
	input.Body.Email = "dup@test.com"
	input.Body.Password = "another-strong-pass-456!"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.PrivacyVersion = "2026-02-15"

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUserExists, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRegister_WeakPassword() {
	h := s.newAuthHandler(false)

	input := &RegisterRequest{}
	input.Body.Email = "weak-pass@test.com"
	input.Body.Password = "short" // too short
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = "2026-01-31"
	input.Body.PrivacyVersion = "2026-02-15"

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeValidationFailed, resp.Body.ErrorCode)
}

// --- RefreshTokenHandler ---

func (s *AuthHandlerIntegrationSuite) TestRefreshToken_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("refresh@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotEmpty(resp.Body.Token)
}

func (s *AuthHandlerIntegrationSuite) TestRefreshToken_UserDeletedFromDB() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("refresh-del@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	// Hard-delete the user (must clean FKs first)
	sqlDB, _ := s.deps.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_preferences WHERE user_id = $1", user.ID)
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts WHERE user_id = $1", user.ID)
	_, _ = sqlDB.Exec("DELETE FROM users WHERE id = $1", user.ID)

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

// --- SendVerificationEmailHandler ---

func (s *AuthHandlerIntegrationSuite) TestSendVerification_NotConfigured() {
	h := s.newAuthHandler(false) // email NOT configured
	user := s.createUserWithPassword("verify@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestSendVerification_NoEmail() {
	h := s.newAuthHandler(true) // email configured
	// Create a user with nil email via context
	user := &models.User{
		ID:            999,
		Email:         nil,
		EmailVerified: false,
		IsActive:      true,
	}
	ctx := s.ctxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("NO_EMAIL", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestSendVerification_SendFails() {
	h := s.newAuthHandler(true) // email configured with fake API key
	user := s.createUserWithPassword("verify-fail@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

// --- ConfirmVerificationHandler ---

func (s *AuthHandlerIntegrationSuite) TestConfirmVerification_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("confirm-v@test.com", "strong-password-123!")

	// Generate a real verification token
	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateVerificationToken(user.ID, "confirm-v@test.com")
	s.Require().NoError(err)

	input := &ConfirmVerificationRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)

	// Verify DB state
	updated := s.reloadUser(user.ID)
	s.True(updated.EmailVerified)
}

func (s *AuthHandlerIntegrationSuite) TestConfirmVerification_AlreadyVerified() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("already-v@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateVerificationToken(user.ID, "already-v@test.com")
	s.Require().NoError(err)

	input := &ConfirmVerificationRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success) // idempotent
	s.Contains(resp.Body.Message, "already verified")
}

func (s *AuthHandlerIntegrationSuite) TestConfirmVerification_EmailMismatch() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("mismatch@test.com", "strong-password-123!")

	// Token generated with a different email
	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateVerificationToken(user.ID, "different@test.com")
	s.Require().NoError(err)

	input := &ConfirmVerificationRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("EMAIL_MISMATCH", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestConfirmVerification_UserNotFound() {
	h := s.newAuthHandler(false)

	// Token for a user ID that doesn't exist
	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateVerificationToken(99999, "ghost@test.com")
	s.Require().NoError(err)

	input := &ConfirmVerificationRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmVerificationHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, resp.Body.ErrorCode)
}

// --- SendMagicLinkHandler ---

func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_NotConfigured() {
	h := s.newAuthHandler(false) // email NOT configured

	input := &SendMagicLinkRequest{}
	input.Body.Email = "magic@test.com"

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_UserNotFound() {
	h := s.newAuthHandler(true) // email configured

	input := &SendMagicLinkRequest{}
	input.Body.Email = "nobody@test.com"

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success) // enumeration prevention
}

func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_EmailNotVerified() {
	h := s.newAuthHandler(true)
	s.createUserWithPassword("unverified@test.com", "strong-password-123!")

	input := &SendMagicLinkRequest{}
	input.Body.Email = "unverified@test.com"

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("EMAIL_NOT_VERIFIED", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_SendFails() {
	h := s.newAuthHandler(true) // email configured but fake API key
	user := s.createUserWithPassword("magic-fail@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)

	input := &SendMagicLinkRequest{}
	input.Body.Email = "magic-fail@test.com"

	resp, err := h.SendMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

// --- VerifyMagicLinkHandler ---

func (s *AuthHandlerIntegrationSuite) TestVerifyMagicLink_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("ml-ok@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateMagicLinkToken(user.ID, "ml-ok@test.com")
	s.Require().NoError(err)

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = token

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotEmpty(resp.Body.Token)
	s.NotNil(resp.Body.User)
	s.NotEmpty(resp.SetCookie.Name)
}

func (s *AuthHandlerIntegrationSuite) TestVerifyMagicLink_UserNotFound() {
	h := s.newAuthHandler(false)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateMagicLinkToken(99999, "ghost@test.com")
	s.Require().NoError(err)

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = token

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestVerifyMagicLink_EmailMismatch() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("ml-mm@test.com", "strong-password-123!")

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateMagicLinkToken(user.ID, "different@test.com")
	s.Require().NoError(err)

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = token

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("INVALID_TOKEN", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestVerifyMagicLink_InactiveUser() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("ml-inactive@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)
	s.softDeleteUser(user.ID)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateMagicLinkToken(user.ID, "ml-inactive@test.com")
	s.Require().NoError(err)

	input := &VerifyMagicLinkRequest{}
	input.Body.Token = token

	resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, resp.Body.ErrorCode)
}

// --- ChangePasswordHandler ---

func (s *AuthHandlerIntegrationSuite) TestChangePassword_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("cp-ok@test.com", "old-password-123!")
	// Reload user to get password hash in context
	user = s.reloadUser(user.ID)
	ctx := s.ctxWithUser(user)

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "old-password-123!"
	input.Body.NewPassword = "new-password-456!!"

	resp, err := h.ChangePasswordHandler(ctx, input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)

	// Verify new password works
	loginInput := &LoginRequest{}
	loginInput.Body.Email = "cp-ok@test.com"
	loginInput.Body.Password = "new-password-456!!"
	loginResp, err := h.LoginHandler(context.Background(), loginInput)
	s.Require().NoError(err)
	s.True(loginResp.Body.Success)
}

func (s *AuthHandlerIntegrationSuite) TestChangePassword_WrongCurrent() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("cp-wrong@test.com", "real-password-123!")
	user = s.reloadUser(user.ID)
	ctx := s.ctxWithUser(user)

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "wrong-password-999!"
	input.Body.NewPassword = "new-password-456!!"

	resp, err := h.ChangePasswordHandler(ctx, input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestChangePassword_NoPasswordSet() {
	h := s.newAuthHandler(false)
	// Create an OAuth-only user (no password hash)
	user := createTestUser(s.deps.db) // EmailVerified=true, no password
	ctx := s.ctxWithUser(user)

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "doesnt-matter-12345"
	input.Body.NewPassword = "new-password-456!!"

	resp, err := h.ChangePasswordHandler(ctx, input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeNoPasswordSet, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestChangePassword_WeakNew() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("cp-weak@test.com", "old-password-123!")
	user = s.reloadUser(user.ID)
	ctx := s.ctxWithUser(user)

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = "old-password-123!"
	input.Body.NewPassword = "short" // too short

	resp, err := h.ChangePasswordHandler(ctx, input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeValidationFailed, resp.Body.ErrorCode)
}

// --- GetDeletionSummaryHandler ---

func (s *AuthHandlerIntegrationSuite) TestGetDeletionSummary_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("del-sum@test.com", "strong-password-123!")
	user = s.reloadUser(user.ID)

	// Create a show for the user
	createApprovedShow(s.deps.db, user.ID, "Test Show")
	ctx := s.ctxWithUser(user)

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Equal(int64(1), resp.Body.ShowsCount)
	s.True(resp.Body.HasPassword)
}

func (s *AuthHandlerIntegrationSuite) TestGetDeletionSummary_FreshUser() {
	h := s.newAuthHandler(false)
	user := createTestUser(s.deps.db) // no password, no data
	ctx := s.ctxWithUser(user)

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Equal(int64(0), resp.Body.ShowsCount)
	s.Equal(int64(0), resp.Body.SavedShowsCount)
	s.Equal(int64(0), resp.Body.PasskeysCount)
	s.False(resp.Body.HasPassword) // OAuth-only user
}

// --- ExportDataHandler ---

func (s *AuthHandlerIntegrationSuite) TestExportData_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("export@test.com", "strong-password-123!")
	user = s.reloadUser(user.ID)
	ctx := s.ctxWithUser(user)

	resp, err := h.ExportDataHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.Equal("application/json", resp.ContentType)
	s.Contains(resp.ContentDisposition, "attachment")
	s.Contains(string(resp.Body), "export@test.com")
}

func (s *AuthHandlerIntegrationSuite) TestExportData_WithShows() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("export-shows@test.com", "strong-password-123!")
	user = s.reloadUser(user.ID)
	createApprovedShow(s.deps.db, user.ID, "Export Show")
	ctx := s.ctxWithUser(user)

	resp, err := h.ExportDataHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.Equal("application/json", resp.ContentType)

	// Verify the export contains show data
	var exportData map[string]interface{}
	err = json.Unmarshal(resp.Body, &exportData)
	s.Require().NoError(err)
	s.Contains(string(resp.Body), "export-shows@test.com")
}

// --- RecoverAccountHandler ---

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("recover-ok@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	input := &RecoverAccountRequest{}
	input.Body.Email = "recover-ok@test.com"
	input.Body.Password = "strong-password-123!"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotNil(resp.Body.User)
	s.NotEmpty(resp.SetCookie.Name)

	// Verify user is active in DB
	updated := s.reloadUser(user.ID)
	s.True(updated.IsActive)
}

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_AccountActive() {
	h := s.newAuthHandler(false)
	s.createUserWithPassword("recover-active@test.com", "strong-password-123!")

	input := &RecoverAccountRequest{}
	input.Body.Email = "recover-active@test.com"
	input.Body.Password = "strong-password-123!"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("ACCOUNT_ACTIVE", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_Expired() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("recover-exp@test.com", "strong-password-123!")
	s.softDeleteUserExpired(user.ID)

	input := &RecoverAccountRequest{}
	input.Body.Email = "recover-exp@test.com"
	input.Body.Password = "strong-password-123!"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("ACCOUNT_NOT_RECOVERABLE", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_WrongPassword() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("recover-badpw@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	input := &RecoverAccountRequest{}
	input.Body.Email = "recover-badpw@test.com"
	input.Body.Password = "wrong-password-999!"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_NoPassword() {
	h := s.newAuthHandler(false)
	// Create an OAuth-only user, then soft-delete
	user := createTestUser(s.deps.db)
	s.softDeleteUser(user.ID)

	input := &RecoverAccountRequest{}
	input.Body.Email = *user.Email
	input.Body.Password = "doesnt-matter-12345"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("NO_PASSWORD", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_UserNotFound() {
	h := s.newAuthHandler(false)

	input := &RecoverAccountRequest{}
	input.Body.Email = "nobody@test.com"
	input.Body.Password = "doesnt-matter-12345"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
}

// --- RequestAccountRecoveryHandler ---

func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_UserNotFound() {
	h := s.newAuthHandler(true)

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "nobody@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success) // enumeration prevention
}

func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_AccountActive() {
	h := s.newAuthHandler(true)
	s.createUserWithPassword("req-active@test.com", "strong-password-123!")

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "req-active@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("ACCOUNT_ACTIVE", resp.Body.ErrorCode)
	s.True(resp.Body.HasPassword)
}

func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_Expired() {
	h := s.newAuthHandler(true)
	user := s.createUserWithPassword("req-exp@test.com", "strong-password-123!")
	s.softDeleteUserExpired(user.ID)

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "req-exp@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("ACCOUNT_NOT_RECOVERABLE", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_EmailNotConfigured() {
	h := s.newAuthHandler(false) // email NOT configured
	user := s.createUserWithPassword("req-nomail@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "req-nomail@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_SendFails() {
	h := s.newAuthHandler(true) // email configured but fake API key
	user := s.createUserWithPassword("req-fail@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "req-fail@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

// --- ConfirmAccountRecoveryHandler ---

func (s *AuthHandlerIntegrationSuite) TestConfirmRecovery_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("confirm-r@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateAccountRecoveryToken(user.ID, "confirm-r@test.com")
	s.Require().NoError(err)

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotNil(resp.Body.User)
	s.NotEmpty(resp.SetCookie.Name)

	// Verify user is active in DB
	updated := s.reloadUser(user.ID)
	s.True(updated.IsActive)
}

func (s *AuthHandlerIntegrationSuite) TestConfirmRecovery_AccountActive() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("confirm-act@test.com", "strong-password-123!")

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateAccountRecoveryToken(user.ID, "confirm-act@test.com")
	s.Require().NoError(err)

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("ACCOUNT_ACTIVE", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestConfirmRecovery_Expired() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("confirm-exp@test.com", "strong-password-123!")
	s.softDeleteUserExpired(user.ID)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateAccountRecoveryToken(user.ID, "confirm-exp@test.com")
	s.Require().NoError(err)

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal("ACCOUNT_NOT_RECOVERABLE", resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestConfirmRecovery_UserNotFound() {
	h := s.newAuthHandler(false)

	jwtSvc := services.NewJWTService(s.deps.db, s.cfg)
	token, err := jwtSvc.CreateAccountRecoveryToken(99999, fmt.Sprintf("ghost-%d@test.com", time.Now().UnixNano()))
	s.Require().NoError(err)

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, resp.Body.ErrorCode)
}
