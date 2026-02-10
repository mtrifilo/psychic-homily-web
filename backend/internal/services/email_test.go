package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"

	resend "github.com/resend/resend-go/v2"
)

// =============================================================================
// HELPERS
// =============================================================================

type capturedEmail struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Html    string   `json:"html"`
}

func setupEmailTest(t *testing.T) (*EmailService, chan capturedEmail, *httptest.Server) {
	t.Helper()
	requests := make(chan capturedEmail, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req capturedEmail
		json.NewDecoder(r.Body).Decode(&req)
		requests <- req
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "test-email-id"})
	}))
	t.Cleanup(server.Close)

	client := resend.NewCustomClient(server.Client(), "test-api-key")
	serverURL, _ := url.Parse(server.URL)
	client.BaseURL = serverURL

	service := &EmailService{
		client:      client,
		fromEmail:   "noreply@test.com",
		frontendURL: "http://localhost:3000",
	}
	return service, requests, server
}

func setupEmailTestError(t *testing.T) *EmailService {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "internal error"}`))
	}))
	t.Cleanup(server.Close)

	client := resend.NewCustomClient(server.Client(), "test-api-key")
	serverURL, _ := url.Parse(server.URL)
	client.BaseURL = serverURL

	return &EmailService{
		client:      client,
		fromEmail:   "noreply@test.com",
		frontendURL: "http://localhost:3000",
	}
}

// =============================================================================
// Constructor & IsConfigured
// =============================================================================

func TestNewEmailService_Configured(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			ResendAPIKey: "re_123abc",
			FromEmail:    "noreply@example.com",
			FrontendURL:  "http://localhost:3000",
		},
	}
	svc := NewEmailService(cfg)

	assert.NotNil(t, svc.client)
	assert.Equal(t, "noreply@example.com", svc.fromEmail)
	assert.Equal(t, "http://localhost:3000", svc.frontendURL)
}

func TestNewEmailService_NotConfigured(t *testing.T) {
	cfg := &config.Config{
		Email: config.EmailConfig{
			ResendAPIKey: "",
			FromEmail:    "noreply@example.com",
			FrontendURL:  "http://localhost:3000",
		},
	}
	svc := NewEmailService(cfg)

	assert.Nil(t, svc.client)
}

func TestEmailIsConfigured_True(t *testing.T) {
	svc := &EmailService{
		client:    resend.NewClient("fake-key"),
		fromEmail: "noreply@test.com",
	}
	assert.True(t, svc.IsConfigured())
}

func TestEmailIsConfigured_False_NilClient(t *testing.T) {
	svc := &EmailService{
		client:    nil,
		fromEmail: "noreply@test.com",
	}
	assert.False(t, svc.IsConfigured())
}

func TestEmailIsConfigured_False_EmptyFrom(t *testing.T) {
	svc := &EmailService{
		client:    resend.NewClient("fake-key"),
		fromEmail: "",
	}
	assert.False(t, svc.IsConfigured())
}

// =============================================================================
// SendVerificationEmail
// =============================================================================

func TestSendVerificationEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendVerificationEmail("user@test.com", "abc-token-123")

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Contains(t, email.Subject, "Verify your email")
	assert.Contains(t, email.Html, "http://localhost:3000/verify-email?token=abc-token-123")
}

func TestSendVerificationEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendVerificationEmail("user@test.com", "token")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendVerificationEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendVerificationEmail("user@test.com", "token")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send verification email")
}

// =============================================================================
// SendMagicLinkEmail
// =============================================================================

func TestSendMagicLinkEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendMagicLinkEmail("user@test.com", "magic-token-456")

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Contains(t, email.Subject, "Sign in")
	assert.Contains(t, email.Html, "http://localhost:3000/auth/magic-link?token=magic-token-456")
}

func TestSendMagicLinkEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendMagicLinkEmail("user@test.com", "token")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendMagicLinkEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendMagicLinkEmail("user@test.com", "token")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send magic link email")
}

// =============================================================================
// SendAccountRecoveryEmail
// =============================================================================

func TestSendAccountRecoveryEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendAccountRecoveryEmail("user@test.com", "recovery-token-789", 14)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Contains(t, email.Subject, "Recover")
	assert.Contains(t, email.Html, "http://localhost:3000/auth/recover?token=recovery-token-789")
	assert.True(t, strings.Contains(email.Html, "14 days remaining"),
		"should include days remaining in body")
}

func TestSendAccountRecoveryEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendAccountRecoveryEmail("user@test.com", "token", 7)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendAccountRecoveryEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendAccountRecoveryEmail("user@test.com", "token", 7)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send account recovery email")
}
