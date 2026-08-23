package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

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
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requests <- req
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "test-email-id"})
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
		_, _ = w.Write([]byte(`{"message": "internal error"}`))
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
	assert.Equal(t, "Verify your email", email.Subject)
	assert.Contains(t, email.Html, "http://localhost:3000/verify-email?token=abc-token-123")
}

// TestSendVerificationEmail_Structure pins the parts of the rendered template
// that carry meaning: the framing, the alerts-first body, and the two ways a
// recipient can reach the link. Copy edits should update these assertions
// deliberately.
func TestSendVerificationEmail_Structure(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	require.NoError(t, svc.SendVerificationEmail("user@test.com", "abc-token-123"))
	body := (<-emails).Html

	assert.Contains(t, body, "PSYCHIC HOMILY", "masthead")
	assert.Contains(t, body, "YOUR ACCOUNT · PENDING VERIFICATION", "kicker")
	assert.Contains(t, body, "Verify your email.", "headline")
	assert.Contains(t, body, "A verified email is what lets the index reach you.", "body lead")
	assert.Contains(t, body, "alerts for the artists and venues you follow", "alerts-first framing")
	assert.Contains(t, body, "THIS LINK EXPIRES IN 24 HOURS", "expiry note")
	assert.Contains(t, body, "Not you? Ignore this email and nothing happens.", "reassurance")
	assert.Contains(t, body, "If the button fails, paste this link into your browser:",
		"plain-link fallback label")

	// The link appears twice: once as the button href, once as pasteable text.
	assert.Equal(t, 2,
		strings.Count(body, "http://localhost:3000/verify-email?token=abc-token-123"),
		"button href plus plain-link fallback")

	// Design-system palette, inlined as hex because email cannot read CSS vars.
	for _, hex := range []string{"#f4f1ea", "#1a1714", "#6b5e4f", "#cabe9f", "#d2541b"} {
		assert.Contains(t, body, hex, "expected DS color %s", hex)
	}

	// Truth boundary. No send path gates alert delivery on email_verified:
	// sendFilterEmail resolves a recipient with a bare email lookup and never
	// reads the flag, and the only enforcement anywhere is show submission in
	// the catalog create handler. So the copy may say what a verified address is
	// for, and may claim submission in the present tense, but must not claim that
	// verifying switches alert delivery on. Loosen these only alongside a send
	// path that actually checks the flag.
	assert.Contains(t, body, "will land", "alert benefit stays future-tense")
	assert.Contains(t, body, "unlocks submitting shows",
		"submission gating is real, so it can be claimed in the present tense")
	assert.NotContains(t, body, "switch on email alerts",
		"claims verification activates alert delivery; nothing enforces that")
	assert.NotContains(t, body, "Once verified, you can",
		"present-tense mechanism claim")
	assert.NotContains(t, body, "YOUR ALERTS",
		"kicker must not imply alerts are held pending verification")

	// The pre-redesign template said this; the rebuilt one must not.
	assert.NotContains(t, body, "Arizona music calendar")
	assert.NotContains(t, body, "Verify Your Email Address")
	assert.NotContains(t, body, "#f97316", "stale orange")

	// Owner mandate: no em dashes in user-facing copy.
	assert.NotContains(t, body, "\u2014", "em dash")
	assert.NotContains(t, body, "&mdash;", "em dash entity")
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

// =============================================================================
// SendShowReminderEmail
// =============================================================================

func TestSendShowReminderEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendShowReminderEmail(
		"user@test.com",
		"Rock Night",
		"http://localhost:3000/shows/rock-night",
		"http://localhost:3000/unsubscribe?uid=1&sig=abc",
		time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC),
		[]string{"Valley Bar"},
	)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Contains(t, email.Subject, "Reminder")
	assert.Contains(t, email.Subject, "Rock Night")
	assert.Contains(t, email.Html, "Rock Night")
	assert.Contains(t, email.Html, "Valley Bar")
	assert.Contains(t, email.Html, "http://localhost:3000/shows/rock-night")
	assert.Contains(t, email.Html, "Unsubscribe")
}

func TestSendShowReminderEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendShowReminderEmail("user@test.com", "Show", "url", "unsub", time.Now(), nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendShowReminderEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendShowReminderEmail("user@test.com", "Show", "url", "unsub", time.Now(), nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send show reminder email")
}

func TestSendShowReminderEmail_MultipleVenues(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendShowReminderEmail(
		"user@test.com", "Show", "url", "unsub", time.Now(),
		[]string{"Valley Bar", "Crescent Ballroom"},
	)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.Html, "Valley Bar, Crescent Ballroom")
}

func TestSendShowReminderEmail_NoVenues(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	err := svc.SendShowReminderEmail(
		"user@test.com", "Show", "url", "unsub", time.Now(),
		[]string{},
	)

	require.NoError(t, err)
	email := <-emails
	// Should not contain "Venue:" text when no venues
	assert.NotContains(t, email.Html, "Venue:")
}

// =============================================================================
// SendFilterNotificationEmail
// =============================================================================

func TestSendFilterNotificationEmail_Success(t *testing.T) {
	svc, emails, _ := setupEmailTest(t)

	htmlBody := "<h1>New show matches your filter!</h1><p>Rock Night at Valley Bar</p>"
	err := svc.SendFilterNotificationEmail(
		"user@test.com",
		"New show: Rock Night",
		htmlBody,
		"http://localhost:3000/unsubscribe?uid=1&sig=abc",
	)

	require.NoError(t, err)
	email := <-emails
	assert.Contains(t, email.From, "noreply@test.com")
	assert.Equal(t, []string{"user@test.com"}, email.To)
	assert.Equal(t, "New show: Rock Night", email.Subject)
	assert.Contains(t, email.Html, "Rock Night at Valley Bar")
}

func TestSendFilterNotificationEmail_NotConfigured(t *testing.T) {
	svc := &EmailService{client: nil, fromEmail: ""}

	err := svc.SendFilterNotificationEmail("user@test.com", "sub", "body", "unsub")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSendFilterNotificationEmail_APIError(t *testing.T) {
	svc := setupEmailTestError(t)

	err := svc.SendFilterNotificationEmail("user@test.com", "sub", "body", "unsub")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send filter notification email")
}
