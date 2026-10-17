package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/notification"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// --- Suite ---

type AuthHandlerIntegrationSuite struct {
	suite.Suite
	deps *testhelpers.IntegrationDeps
	cfg  *config.Config
}

func TestAuthHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(AuthHandlerIntegrationSuite))
}

func (s *AuthHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
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
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *AuthHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

// --- Helpers ---

// resendInvalidAPIKeyBody is the body Resend's API answers a request carrying
// an unknown API key with, alongside HTTP 401.
const resendInvalidAPIKeyBody = `{"message":"API key is invalid","name":"validation_error","statusCode":401}`

// fakeResend answers Resend API calls in-process, so no test in this suite
// depends on a live third-party response. It counts the calls it answers.
type fakeResend struct {
	respond func(*http.Request) (*http.Response, error)
	calls   atomic.Int32
}

func (f *fakeResend) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return f.respond(r)
}

// resendAnswering returns a fakeResend that answers every call with status
// and a JSON body, the way Resend's API does.
func resendAnswering(status int, body string) *fakeResend {
	return &fakeResend{respond: func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}}
}

// resendRejectingKey returns a fakeResend that answers every call the way
// Resend answers an unknown API key.
func resendRejectingKey() *fakeResend {
	return resendAnswering(http.StatusUnauthorized, resendInvalidAPIKeyBody)
}

// resendDialRefused returns a fakeResend whose every call fails the way a
// refused TCP dial fails.
func resendDialRefused() *fakeResend {
	return &fakeResend{respond: func(*http.Request) (*http.Response, error) {
		return nil, dialRefused()
	}}
}

// resendForbidden returns a fakeResend that fails t on any call, for a test
// whose handler must not send email.
func resendForbidden(t *testing.T) *fakeResend {
	return &fakeResend{respond: func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected Resend call: %s %s", r.Method, r.URL)
		return nil, errors.New("unexpected Resend call")
	}}
}

// newAuthHandler builds the handler around the real services. With
// emailConfigured, the email service is configured but any send fails the
// test; a test whose handler sends uses newAuthHandlerWithResend. Any breach
// check fails the test; a test whose handler validates a password uses
// newAuthHandlerWithPwnedPasswords.
func (s *AuthHandlerIntegrationSuite) newAuthHandler(emailConfigured bool) *AuthHandler {
	if !emailConfigured {
		return s.buildAuthHandler(s.cfg, nil, pwnedPasswordsForbidden(s.T()))
	}
	return s.newAuthHandlerWithResend(resendForbidden(s.T()))
}

// newAuthHandlerWithPwnedPasswords builds the handler with email unconfigured
// and every breach-check request answered by pwned.
func (s *AuthHandlerIntegrationSuite) newAuthHandlerWithPwnedPasswords(pwned *fakePwnedPasswords) *AuthHandler {
	return s.buildAuthHandler(s.cfg, nil, pwned)
}

// newAuthHandlerWithResend builds the handler with email configured and every
// Resend API call answered by resend.
func (s *AuthHandlerIntegrationSuite) newAuthHandlerWithResend(resend *fakeResend) *AuthHandler {
	emailCfg := &config.Config{
		JWT:     s.cfg.JWT,
		Session: s.cfg.Session,
		Email: config.EmailConfig{
			ResendAPIKey: "re_fake_key_for_testing",
			FromEmail:    "test@psychichomily.com",
			FrontendURL:  "http://localhost:3000",
		},
	}
	return s.buildAuthHandler(emailCfg, resend, pwnedPasswordsForbidden(s.T()))
}

func (s *AuthHandlerIntegrationSuite) buildAuthHandler(emailCfg *config.Config, resendTransport, pwnedPasswordsTransport http.RoundTripper) *AuthHandler {
	authSvc := auth.NewAuthService(s.deps.DB, emailCfg, s.deps.UserService)
	jwtSvc := auth.NewJWTService(s.deps.DB, emailCfg, s.deps.UserService)
	emailSvc := notification.NewEmailServiceWithTransport(emailCfg, resendTransport)
	discordSvc := notification.NewDiscordService(emailCfg)
	pv := auth.NewPasswordValidatorWithTransport(pwnedPasswordsTransport)

	return NewAuthHandler(
		authSvc,
		jwtSvc,
		s.deps.UserService,
		emailSvc,
		auth.NewAuthEmailOutbox(s.deps.DB, s.deps.UserService, jwtSvc, emailSvc),
		discordSvc,
		pv,
		emailCfg,
	)
}

// newAuthHandlerCapturingEmail builds the handler with the REAL jwt + user
// services (so tokens are genuinely signed and validated) but a capturing email
// service, which is the only way to observe the verification token that
// registration puts in the user's inbox. Every breach-check request is answered
// by pwned. Returns the handler plus a pointer to the captured token.
func (s *AuthHandlerIntegrationSuite) newAuthHandlerCapturingEmail(pwned *fakePwnedPasswords) (*AuthHandler, *string) {
	cfg := &config.Config{
		JWT:     s.cfg.JWT,
		Session: s.cfg.Session,
		Email: config.EmailConfig{
			ResendAPIKey: "re_fake_key_for_testing",
			FromEmail:    "test@psychichomily.com",
			FrontendURL:  "http://localhost:3000",
		},
	}

	captured := new(string)
	emailSvc := &testhelpers.MockEmailService{
		IsConfiguredFn: func() bool { return true },
		SendVerificationEmailFn: func(_ string, token string) error {
			*captured = token
			return nil
		},
	}

	jwtSvc := auth.NewJWTService(s.deps.DB, cfg, s.deps.UserService)
	h := NewAuthHandler(
		auth.NewAuthService(s.deps.DB, cfg, s.deps.UserService),
		jwtSvc,
		s.deps.UserService,
		emailSvc,
		auth.NewAuthEmailOutbox(s.deps.DB, s.deps.UserService, jwtSvc, emailSvc),
		notification.NewDiscordService(cfg),
		auth.NewPasswordValidatorWithTransport(pwned),
		cfg,
	)
	return h, captured
}

// runOutbox runs one tick of the outbox h queues into.
func (s *AuthHandlerIntegrationSuite) runOutbox(h *AuthHandler) {
	outbox, ok := h.emailOutbox.(*auth.AuthEmailOutbox)
	s.Require().True(ok, "the handler is built around the real outbox")
	outbox.RunNow(context.Background())
}

func (s *AuthHandlerIntegrationSuite) createUserWithPassword(email, password string) *authm.User {
	user, err := s.deps.UserService.CreateUserWithPasswordWithLegal(email, password, "Test", "User", signupconsent.CurrentLegalAcceptance())
	s.Require().NoError(err)
	return user
}

func (s *AuthHandlerIntegrationSuite) ctxWithUser(user *authm.User) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, user)
}

func (s *AuthHandlerIntegrationSuite) softDeleteUser(userID uint) {
	err := s.deps.DB.Model(&authm.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"deleted_at": time.Now(),
		}).Error
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) deactivateUser(userID uint) {
	err := s.deps.DB.Model(&authm.User{}).Where("id = ?", userID).
		Update("is_active", false).Error
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) softDeleteUserExpired(userID uint) {
	s.Require().NoError(softDeletePastRecoveryWindow(s.deps.DB, userID))
}

// softDeletePastRecoveryWindow soft-deletes the account with a deleted_at a day
// older than the recovery window, so it is deleted and no longer recoverable.
func softDeletePastRecoveryWindow(db *gorm.DB, userID uint) error {
	return db.Model(&authm.User{}).Where("id = ?", userID).Updates(map[string]any{
		"is_active":  false,
		"deleted_at": time.Now().Add(-usersvc.AccountRecoveryGracePeriod - 24*time.Hour),
	}).Error
}

func (s *AuthHandlerIntegrationSuite) lockAccount(userID uint) {
	err := s.deps.DB.Model(&authm.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"failed_login_attempts": 5,
			"locked_until":          time.Now().Add(15 * time.Minute),
		}).Error
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) verifyEmail(userID uint) {
	err := s.deps.UserService.SetEmailVerified(userID, true)
	s.Require().NoError(err)
}

func (s *AuthHandlerIntegrationSuite) reloadUser(userID uint) *authm.User {
	user, err := s.deps.UserService.GetUserByID(userID)
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

// TestRegister_Success also pins the breach check's "suffix absent" answer:
// the password's range is checked once and its suffix is not listed.
func (s *AuthHandlerIntegrationSuite) TestRegister_Success() {
	pwned := pwnedPasswordsListing(s.T())
	h := s.newAuthHandlerWithPwnedPasswords(pwned)

	input := &RegisterRequest{}
	input.Body.Email = "new-user@test.com"
	input.Body.Password = "very-strong-password-123!"
	input.Body.FirstName = testhelpers.StringPtr("Jane")
	input.Body.LastName = testhelpers.StringPtr("Doe")
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.NotEmpty(resp.Body.Token)
	s.NotNil(resp.Body.User)
	s.Equal("new-user@test.com", *resp.Body.User.Email)
	s.NotEmpty(resp.SetCookie.Name)
	s.Equal(int32(1), pwned.calls.Load(), "registration must check the password's range once")

	// PSY-1023: the age confirmation is persisted on the new user.
	created := s.reloadUser(resp.Body.User.ID)
	s.Require().NotNil(created.AgeConfirmedAt, "expected age_confirmed_at recorded on new user")
	s.Require().NotNil(created.MinAgeAttested, "expected min_age_attested recorded on new user")
	s.Equal(MinSignupAge, *created.MinAgeAttested)
	s.Require().NotNil(created.TermsVersion)
	s.Require().NotNil(created.PrivacyVersion)
	s.Require().NotNil(created.TermsAcceptedAt)
	s.Equal(legal.CurrentTermsVersion, *created.TermsVersion)
	s.Equal(legal.CurrentPrivacyVersion, *created.PrivacyVersion)
	s.WithinDuration(time.Now(), *created.TermsAcceptedAt, time.Minute)
}

// TestRegister_MissingAgeConfirmation asserts the integration-level register
// path blocks signup when the age confirmation is absent (PSY-1023).
func (s *AuthHandlerIntegrationSuite) TestRegister_MissingAgeConfirmation() {
	h := s.newAuthHandler(false)

	input := &RegisterRequest{}
	input.Body.Email = "no-age@test.com"
	input.Body.Password = "very-strong-password-123!"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = false

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeAgeConfirmationRequired, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRegister_DuplicateEmail() {
	h := s.newAuthHandlerWithPwnedPasswords(pwnedPasswordsListing(s.T()))
	s.createUserWithPassword("dup@test.com", "strong-password-123!")

	input := &RegisterRequest{}
	input.Body.Email = "dup@test.com"
	input.Body.Password = "another-strong-pass-456!"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUserExists, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestRegister_WeakPassword() {
	h := s.newAuthHandlerWithPwnedPasswords(pwnedPasswordsListing(s.T()))

	input := &RegisterRequest{}
	input.Body.Email = "weak-pass@test.com"
	input.Body.Password = "short" // too short
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeValidationFailed, resp.Body.ErrorCode)
}

// breachedPasswordMessage is the validator's refusal for a password whose
// SHA-1 suffix the breach check finds in its range.
const breachedPasswordMessage = "This password has been exposed in a data breach and should not be used"

func (s *AuthHandlerIntegrationSuite) TestRegister_BreachedPassword() {
	input := validRegisterInput("breached@test.com")
	pwned := pwnedPasswordsListing(s.T(), input.Body.Password)
	h := s.newAuthHandlerWithPwnedPasswords(pwned)

	resp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	s.Equal(breachedPasswordMessage, resp.Body.Message)
	s.Equal(int32(1), pwned.calls.Load())

	user, err := s.deps.UserService.GetUserByEmail("breached@test.com")
	s.Require().NoError(err)
	s.Nil(user, "a refused registration must not create the account")
}

// TestRegister_BreachCheckUnreachable pins the breach check as fail-open: when
// the range request fails before any response, registration goes ahead.
func (s *AuthHandlerIntegrationSuite) TestRegister_BreachCheckUnreachable() {
	pwned := pwnedPasswordsUnreachable(s.T())
	h := s.newAuthHandlerWithPwnedPasswords(pwned)

	resp, err := h.RegisterHandler(context.Background(), validRegisterInput("hibp-down@test.com"))
	s.Require().NoError(err)
	s.True(resp.Body.Success, resp.Body.Message)
	s.Equal(int32(1), pwned.calls.Load())
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
	sqlDB, _ := s.deps.DB.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_preferences WHERE user_id = $1", user.ID)
	_, _ = sqlDB.Exec("DELETE FROM oauth_accounts WHERE user_id = $1", user.ID)
	_, _ = sqlDB.Exec("DELETE FROM users WHERE id = $1", user.ID)

	resp, err := h.RefreshTokenHandler(ctx, &struct{}{})

	// Deleted-user-during-refresh is the canonical 401 case: the token is
	// cryptographically valid but the principal it represents no longer
	// exists, so the right transport status is 401 (client clears session +
	// redirects to login). A 5xx would (incorrectly) tell the client
	// "backend is broken; retry". Generic backend failures stay on the 5xx
	// fail-closed path (covered by TestRefreshTokenHandler_ProfileFails).
	s.Require().Error(err)
	var statusErr huma.StatusError
	s.Require().True(errors.As(err, &statusErr), "expected returned error to satisfy huma.StatusError")
	s.Equal(401, statusErr.GetStatus())
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, resp.Body.ErrorCode)
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
	user := &authm.User{
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

// TestSendVerification_SendFails, _ResendUnreachable and _ResendAccepts pin
// the handler's answer to three Resend outcomes: a 401 with Resend's JSON
// error body, a transport error before any response, and a 200 with a JSON
// body.
func (s *AuthHandlerIntegrationSuite) TestSendVerification_SendFails() {
	resend := resendRejectingKey()
	h := s.newAuthHandlerWithResend(resend)
	user := s.createUserWithPassword("verify-fail@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})

	// Fail-closed: a real email-send failure surfaces as a 5xx so monitoring
	// sees the incident; the response body carries the canonical
	// SERVICE_UNAVAILABLE shape even though the transport-level status is 5xx.
	s.Equal(int32(1), resend.calls.Load(), "the send must reach Resend")
	s.Require().Error(err)
	s.ErrorContains(err, "API key is invalid")
	var authErr *autherrors.AuthError
	s.Require().True(errors.As(err, &authErr), "expected returned error to wrap *AuthError")
	s.Equal(autherrors.CodeServiceUnavailable, authErr.Code)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestSendVerification_ResendUnreachable() {
	resend := resendDialRefused()
	h := s.newAuthHandlerWithResend(resend)
	user := s.createUserWithPassword("verify-unreachable@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})

	s.Equal(int32(1), resend.calls.Load(), "the send must reach the transport")
	s.Require().Error(err)
	s.ErrorIs(err, syscall.ECONNREFUSED)
	var authErr *autherrors.AuthError
	s.Require().True(errors.As(err, &authErr), "expected returned error to wrap *AuthError")
	s.Equal(autherrors.CodeServiceUnavailable, authErr.Code)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeServiceUnavailable, resp.Body.ErrorCode)
}

func (s *AuthHandlerIntegrationSuite) TestSendVerification_ResendAccepts() {
	resend := resendAnswering(http.StatusOK, `{}`)
	h := s.newAuthHandlerWithResend(resend)
	user := s.createUserWithPassword("verify-sent@test.com", "strong-password-123!")
	ctx := s.ctxWithUser(user)

	resp, err := h.SendVerificationEmailHandler(ctx, &struct{}{})

	s.Equal(int32(1), resend.calls.Load(), "the send must reach Resend")
	s.Require().NoError(err)
	s.True(resp.Body.Success, resp.Body.Message)
	s.Empty(resp.Body.ErrorCode)
}

// TestRegisterThenVerify_RoundTrip is the PSY-1871 acceptance test: signing up
// must put a working verification link in the inbox, and following that link
// must flip email_verified in the database (which is what unlocks show
// submission). It exercises the real JWT service end to end rather than a
// stubbed token, so a mismatch between what CreateVerificationToken mints and
// what ValidateVerificationToken accepts would fail here.
func (s *AuthHandlerIntegrationSuite) TestRegisterThenVerify_RoundTrip() {
	h, capturedToken := s.newAuthHandlerCapturingEmail(pwnedPasswordsListing(s.T()))

	input := &RegisterRequest{}
	input.Body.Email = "roundtrip-v@test.com"
	input.Body.Password = "strong-password-123!"
	input.Body.TermsAccepted = true
	input.Body.TermsVersion = legal.CurrentTermsVersion
	input.Body.PrivacyVersion = legal.CurrentPrivacyVersion
	input.Body.AgeConfirmed = true
	input.Body.MinAgeAttested = MinSignupAge

	regResp, err := h.RegisterHandler(context.Background(), input)
	s.Require().NoError(err)
	s.Require().True(regResp.Body.Success, regResp.Body.Message)
	s.Require().NotNil(regResp.Body.User)

	// The account starts unverified, which is what gates submission.
	s.False(s.reloadUser(regResp.Body.User.ID).EmailVerified)

	// Registration must have sent a verification token.
	s.Require().NotEmpty(*capturedToken, "signup must send a verification email")

	confirm := &ConfirmVerificationRequest{}
	confirm.Body.Token = *capturedToken

	confirmResp, err := h.ConfirmVerificationHandler(context.Background(), confirm)
	s.Require().NoError(err)
	s.True(confirmResp.Body.Success, confirmResp.Body.Message)

	s.True(s.reloadUser(regResp.Body.User.ID).EmailVerified,
		"following the emailed link must verify the account")
}

// --- ConfirmVerificationHandler ---

func (s *AuthHandlerIntegrationSuite) TestConfirmVerification_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("confirm-v@test.com", "strong-password-123!")

	// Generate a real verification token
	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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
	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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
	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

// TestSendMagicLink_EmailNotVerified verifies an unverified account gets the
// unknown-email response, whole body, and that its verification email is
// queued rather than sent: the poller sends it.
func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_EmailNotVerified() {
	resend := resendAnswering(http.StatusOK, `{}`)
	h := s.newAuthHandlerWithResend(resend)
	user := s.createUserWithPassword("unverified@test.com", "strong-password-123!")

	send := func(email string) *SendMagicLinkResponse {
		input := &SendMagicLinkRequest{}
		input.Body.Email = email
		resp, err := h.SendMagicLinkHandler(context.Background(), input)
		s.Require().NoError(err)
		return resp
	}
	unknown := send("nobody@test.com")
	known := send("unverified@test.com")
	s.Equal(unknown.Body, known.Body, "an unverified account must not change the response")
	s.Equal(int32(0), resend.calls.Load(), "the handler must not reach Resend")

	s.runOutbox(h)
	s.Equal(int32(1), resend.calls.Load(), "the poller sends exactly one email")
	rows := s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailKindVerification, rows[0].Kind)
	s.Equal(authm.AuthEmailStatusSent, rows[0].Status)
}

// TestSendMagicLink_SendFails verifies a known-verified account whose send
// will fail gets the unknown-email response, whole body: the handler only
// queues the email, and the outbox poller meets the failure. Resend answers
// the send with 401.
func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_SendFails() {
	resend := resendRejectingKey()
	h := s.newAuthHandlerWithResend(resend)
	user := s.createUserWithPassword("magic-fail@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)

	send := func(email string) *SendMagicLinkResponse {
		input := &SendMagicLinkRequest{}
		input.Body.Email = email
		resp, err := h.SendMagicLinkHandler(context.Background(), input)
		s.Require().NoError(err)
		return resp
	}
	unknown := send("nobody@test.com")
	known := send("magic-fail@test.com")
	s.Equal(unknown.Body, known.Body, "a failing send must not change the response")
	s.True(known.Body.Success)
	s.Empty(known.Body.ErrorCode)
	s.Equal(int32(0), resend.calls.Load(), "the handler must not reach Resend")

	s.runOutbox(h)
	s.Equal(int32(1), resend.calls.Load(), "the poller's send must reach Resend")
	rows := s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailStatusPending, rows[0].Status, "a failed send is retried")
	s.Equal(1, rows[0].Attempts)
	s.Require().NotNil(rows[0].LastError)
	s.True(rows[0].NextAttemptAt.After(rows[0].CreatedAt), "the retry waits out the backoff")
}

// --- VerifyMagicLinkHandler ---

func (s *AuthHandlerIntegrationSuite) TestVerifyMagicLink_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("ml-ok@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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
	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)

	states := []struct {
		name  string
		apply func(userID uint)
	}{
		{name: "deactivated", apply: s.deactivateUser},
		{name: "soft-deleted", apply: s.softDeleteUser},
	}

	for _, state := range states {
		s.Run(state.name, func() {
			address := "ml-inactive-" + state.name + "@test.com"
			user := s.createUserWithPassword(address, "strong-password-123!")
			s.verifyEmail(user.ID)
			state.apply(user.ID)

			token, err := jwtSvc.CreateMagicLinkToken(user.ID, address)
			s.Require().NoError(err)

			input := &VerifyMagicLinkRequest{}
			input.Body.Token = token

			resp, err := h.VerifyMagicLinkHandler(context.Background(), input)
			s.Require().NoError(err)
			s.False(resp.Body.Success)
			s.Equal(autherrors.CodeAccountInactive, resp.Body.ErrorCode)
			s.Equal(autherrors.ErrAccountInactive().UserMessage(), resp.Body.Message)
			s.Empty(resp.Body.Token)
			s.Nil(resp.Body.User)
			s.Empty(resp.SetCookie.Value)
		})
	}
}

// --- ChangePasswordHandler ---

// TestChangePassword_Success also pins the breach check's "suffix absent"
// answer: the new password's range is checked once and its suffix is not listed.
func (s *AuthHandlerIntegrationSuite) TestChangePassword_Success() {
	pwned := pwnedPasswordsListing(s.T())
	h := s.newAuthHandlerWithPwnedPasswords(pwned)
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
	s.Equal(int32(1), pwned.calls.Load(), "a password change must check the new password's range once")

	// Verify new password works
	loginInput := &LoginRequest{}
	loginInput.Body.Email = "cp-ok@test.com"
	loginInput.Body.Password = "new-password-456!!"
	loginResp, err := h.LoginHandler(context.Background(), loginInput)
	s.Require().NoError(err)
	s.True(loginResp.Body.Success)
}

func (s *AuthHandlerIntegrationSuite) TestChangePassword_WrongCurrent() {
	h := s.newAuthHandlerWithPwnedPasswords(pwnedPasswordsListing(s.T()))
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
	h := s.newAuthHandlerWithPwnedPasswords(pwnedPasswordsListing(s.T()))
	// Create an OAuth-only user (no password hash)
	user := testhelpers.CreateTestUser(s.deps.DB) // EmailVerified=true, no password
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
	h := s.newAuthHandlerWithPwnedPasswords(pwnedPasswordsListing(s.T()))
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

// changePasswordAs creates a user with oldPassword and asks h to change it to
// newPassword on that user's behalf.
func (s *AuthHandlerIntegrationSuite) changePasswordAs(h *AuthHandler, email, oldPassword, newPassword string) *ChangePasswordResponse {
	user := s.reloadUser(s.createUserWithPassword(email, oldPassword).ID)

	input := &ChangePasswordRequest{}
	input.Body.CurrentPassword = oldPassword
	input.Body.NewPassword = newPassword

	resp, err := h.ChangePasswordHandler(s.ctxWithUser(user), input)
	s.Require().NoError(err)
	return resp
}

// loginSucceeds reports whether h accepts email and password.
func (s *AuthHandlerIntegrationSuite) loginSucceeds(h *AuthHandler, email, password string) bool {
	input := &LoginRequest{}
	input.Body.Email = email
	input.Body.Password = password
	resp, err := h.LoginHandler(context.Background(), input)
	s.Require().NoError(err)
	return resp.Body.Success
}

func (s *AuthHandlerIntegrationSuite) TestChangePassword_BreachedNew() {
	const newPassword = "breached-but-long-password-1!"
	pwned := pwnedPasswordsListing(s.T(), newPassword)
	h := s.newAuthHandlerWithPwnedPasswords(pwned)

	resp := s.changePasswordAs(h, "cp-breached@test.com", "old-password-123!", newPassword)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeValidationFailed, resp.Body.ErrorCode)
	s.Equal(breachedPasswordMessage, resp.Body.Message)
	s.Equal(int32(1), pwned.calls.Load())

	s.True(s.loginSucceeds(h, "cp-breached@test.com", "old-password-123!"), "a refused change must keep the old password")
	s.False(s.loginSucceeds(h, "cp-breached@test.com", newPassword))
}

// TestChangePassword_BreachCheckUnreachable pins the breach check as fail-open:
// when the range request fails before any response, the change goes ahead.
func (s *AuthHandlerIntegrationSuite) TestChangePassword_BreachCheckUnreachable() {
	pwned := pwnedPasswordsUnreachable(s.T())
	h := s.newAuthHandlerWithPwnedPasswords(pwned)

	resp := s.changePasswordAs(h, "cp-hibp-down@test.com", "old-password-123!", "new-password-456!!")
	s.True(resp.Body.Success, resp.Body.Message)
	s.Equal(int32(1), pwned.calls.Load())

	s.True(s.loginSucceeds(h, "cp-hibp-down@test.com", "new-password-456!!"))
}

// --- GetDeletionSummaryHandler ---

func (s *AuthHandlerIntegrationSuite) TestGetDeletionSummary_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("del-sum@test.com", "strong-password-123!")
	user = s.reloadUser(user.ID)

	// Create a show for the user
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")
	ctx := s.ctxWithUser(user)

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Equal(int64(1), resp.Body.ShowsCount)
	s.True(resp.Body.HasPassword)
}

func (s *AuthHandlerIntegrationSuite) TestGetDeletionSummary_FreshUser() {
	h := s.newAuthHandler(false)
	user := testhelpers.CreateTestUser(s.deps.DB) // no password, no data
	ctx := s.ctxWithUser(user)

	resp, err := h.GetDeletionSummaryHandler(ctx, &struct{}{})
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Equal(int64(0), resp.Body.ShowsCount)
	s.Equal(int64(0), resp.Body.SavedShowsCount)
	s.Equal(int64(0), resp.Body.SavedReleasesCount)
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
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Export Show")
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

// TestRecoverAccount_AccountActive: PSY-774 — an active account must
// collapse to the same "Invalid credentials" body as an unknown email so
// the endpoint can't be used as an existence/state oracle.
func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_AccountActive() {
	h := s.newAuthHandler(false)
	s.createUserWithPassword("recover-active@test.com", "strong-password-123!")

	input := &RecoverAccountRequest{}
	input.Body.Email = "recover-active@test.com"
	input.Body.Password = "strong-password-123!"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
}

// TestRecoverAccount_Expired: PSY-774 — an expired recovery window must
// collapse to "Invalid credentials" too (was ACCOUNT_NOT_RECOVERABLE).
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
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
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

// TestRecoverAccount_NoPassword: PSY-774 — OAuth-only accounts must
// collapse to "Invalid credentials" too (was NO_PASSWORD).
func (s *AuthHandlerIntegrationSuite) TestRecoverAccount_NoPassword() {
	h := s.newAuthHandler(false)
	// Create an OAuth-only user, then soft-delete
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.softDeleteUser(user.ID)

	input := &RecoverAccountRequest{}
	input.Body.Email = *user.Email
	input.Body.Password = "doesnt-matter-12345"

	resp, err := h.RecoverAccountHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeInvalidCredentials, resp.Body.ErrorCode)
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

// TestRequestRecovery_AccountActive: PSY-774 — an active account must
// collapse to the same generic "if eligible, sent" response as an unknown
// email so the endpoint can't be used as an account-state oracle.
func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_AccountActive() {
	h := s.newAuthHandler(true)
	s.createUserWithPassword("req-active@test.com", "strong-password-123!")

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "req-active@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Empty(resp.Body.ErrorCode)
}

// TestRequestRecovery_Expired: PSY-774 — an expired recovery window must
// also collapse to the generic success body (was ACCOUNT_NOT_RECOVERABLE).
func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_Expired() {
	h := s.newAuthHandler(true)
	user := s.createUserWithPassword("req-exp@test.com", "strong-password-123!")
	s.softDeleteUserExpired(user.ID)

	input := &RequestAccountRecoveryRequest{}
	input.Body.Email = "req-exp@test.com"

	resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.True(resp.Body.Success)
	s.Empty(resp.Body.ErrorCode)
}

// TestRequestRecovery_EmailNotConfigured: pre-lookup config error fires for
// every caller regardless of account state, so SERVICE_UNAVAILABLE is not
// an existence oracle.
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

// TestRequestRecovery_SendFails verifies a recoverable account whose send
// will fail gets the unknown-email response, whole body: the handler only
// queues the email, and the outbox poller meets the failure. Resend answers
// the send with 401.
func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_SendFails() {
	resend := resendRejectingKey()
	h := s.newAuthHandlerWithResend(resend)
	user := s.createUserWithPassword("req-fail@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	request := func(email string) *RequestAccountRecoveryResponse {
		input := &RequestAccountRecoveryRequest{}
		input.Body.Email = email
		resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
		s.Require().NoError(err)
		return resp
	}
	unknown := request("nobody@test.com")
	known := request("req-fail@test.com")
	s.Equal(unknown.Body, known.Body, "a failing send must not change the response")
	s.True(known.Body.Success)
	s.Empty(known.Body.ErrorCode)
	s.Equal(int32(0), resend.calls.Load(), "the handler must not reach Resend")

	s.runOutbox(h)
	s.Equal(int32(1), resend.calls.Load(), "the poller's send must reach Resend")
	rows := s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailStatusPending, rows[0].Status, "a failed send is retried")
	s.Equal(1, rows[0].Attempts)
	s.Require().NotNil(rows[0].LastError)
}

// --- ConfirmAccountRecoveryHandler ---

func (s *AuthHandlerIntegrationSuite) TestConfirmRecovery_Success() {
	h := s.newAuthHandler(false)
	user := s.createUserWithPassword("confirm-r@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
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

	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
	token, err := jwtSvc.CreateAccountRecoveryToken(99999, fmt.Sprintf("ghost-%d@test.com", time.Now().UnixNano()))
	s.Require().NoError(err)

	input := &ConfirmAccountRecoveryRequest{}
	input.Body.Token = token

	resp, err := h.ConfirmAccountRecoveryHandler(context.Background(), input)
	s.Require().NoError(err)
	s.False(resp.Body.Success)
	s.Equal(autherrors.CodeUnauthorized, resp.Body.ErrorCode)
}
