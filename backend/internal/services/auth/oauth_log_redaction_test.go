package auth

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/mock"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testlog"
)

// Sentinels are improbable literals, so a hit in the captured log is proof the
// value came from the OAuth payload and not from unrelated log text.
const (
	sentinelAccessToken  = "SENTINEL-ACCESS-TOKEN-9f2b41"
	sentinelRefreshToken = "SENTINEL-REFRESH-TOKEN-4c7a18"
	sentinelIDToken      = "SENTINEL-ID-TOKEN-1e5d63"
	sentinelRawData      = "SENTINEL-RAW-DATA-8a3c07"
	sentinelEmail        = "sentinel-oauth-user@example.invalid"
	sentinelAuthzCode    = "SENTINEL-AUTHZ-CODE-6b1f92"
	sentinelState        = "SENTINEL-STATE-2d9e50"
	sentinelProviderUser = "sentinel-provider-user-id"
)

// oauthUserServiceStub resolves the OAuth callback to a fixed user so the whole
// callback body runs under the log capture. Every other method keeps the
// nil-database behaviour.
type oauthUserServiceStub struct {
	nilDBUserService
	user *authm.User
}

func (s *oauthUserServiceStub) FindOrCreateUserWithConsent(
	goth.User,
	string,
	*contracts.OAuthSignupConsent,
) (*authm.User, error) {
	return s.user, nil
}

// sentinelGothUser is a completed provider identity whose every credential and
// PII field is a distinct sentinel.
func sentinelGothUser() goth.User {
	return goth.User{
		Provider:     "google",
		UserID:       sentinelProviderUser,
		Email:        sentinelEmail,
		Name:         "Sentinel OAuth User",
		AccessToken:  sentinelAccessToken,
		RefreshToken: sentinelRefreshToken,
		IDToken:      sentinelIDToken,
		RawData:      map[string]any{"raw_payload": sentinelRawData},
	}
}

// assertCaptureIsLive fails when the captured stream lacks a line the code
// under test always emits. Without it an empty capture would make every
// absence assertion below pass vacuously.
func assertCaptureIsLive(t *testing.T, output string) {
	t.Helper()

	const marker = "DEBUG: OAuth callback path: /auth/callback/google"
	if !strings.Contains(output, marker) {
		t.Fatalf("captured log is missing %q, so testlog.Capture no longer intercepts this path and the secret assertions below are vacuous; captured log:\n%s", marker, output)
	}
}

func assertNoSecretsLogged(t *testing.T, output string) {
	t.Helper()

	for _, secret := range []struct {
		label string
		value string
	}{
		{"access token", sentinelAccessToken},
		{"refresh token", sentinelRefreshToken},
		{"ID token", sentinelIDToken},
		{"raw provider payload", sentinelRawData},
		{"email address", sentinelEmail},
		{"authorization code", sentinelAuthzCode},
		{"state nonce", sentinelState},
	} {
		if strings.Contains(output, secret.value) {
			t.Errorf("the OAuth callback logged the %s; captured log:\n%s", secret.label, output)
		}
	}
}

// TestOAuthCallbackNeverLogsCredentials asserts the invariant that the OAuth
// completion path logs neither goth.User nor the callback URL as a value. Both
// carry live credentials: goth.User holds the access, refresh, and ID tokens
// plus the raw provider payload and the email, and the callback query string
// holds the single-use authorization code and the state nonce.
func TestOAuthCallbackNeverLogsCredentials(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	callbackURL := "/auth/callback/google?code=" + sentinelAuthzCode + "&state=" + sentinelState

	// OAuthCallbackWithConsent is the entry point the HTTP handler calls.
	t.Run("completed_login", func(t *testing.T) {
		userService := &oauthUserServiceStub{user: &authm.User{ID: 42}}
		authService := NewAuthService(nil, cfg, userService)

		completer := &MockOAuthCompleter{}
		completer.On("CompleteUserAuth", mock.Anything, mock.Anything).
			Return(sentinelGothUser(), nil)
		authService.SetOAuthCompleter(completer)

		var token string
		var err error
		output := testlog.Capture(t, func() {
			_, token, err = authService.OAuthCallbackWithConsent(
				httptest.NewRecorder(),
				httptest.NewRequest("GET", callbackURL, nil),
				"google",
				nil,
			)
		})

		if err != nil {
			t.Fatalf("expected the stubbed callback to succeed, got %v", err)
		}
		if token == "" {
			t.Fatal("expected a session token from the completed callback")
		}

		assertCaptureIsLive(t, output)
		assertNoSecretsLogged(t, output)

		if !strings.Contains(output, sentinelProviderUser) {
			t.Errorf("expected the provider user id to survive redaction; its absence means the diagnostic was dropped, not redacted; captured log:\n%s", output)
		}
		if strings.Contains(output, token) {
			t.Errorf("the OAuth callback logged the minted session token; captured log:\n%s", output)
		}
	})

	t.Run("failed_completion", func(t *testing.T) {
		authService := NewAuthService(nil, cfg, newNilDBUserService())

		completer := &MockOAuthCompleter{}
		completer.On("CompleteUserAuth", mock.Anything, mock.Anything).
			Return(goth.User{}, errors.New("provider rejected the exchange"))
		authService.SetOAuthCompleter(completer)

		var err error
		output := testlog.Capture(t, func() {
			_, _, err = authService.OAuthCallbackWithConsent(
				httptest.NewRecorder(),
				httptest.NewRequest("GET", callbackURL, nil),
				"google",
				nil,
			)
		})

		if err == nil {
			t.Fatal("expected the stubbed completion failure to surface")
		}

		assertCaptureIsLive(t, output)
		assertNoSecretsLogged(t, output)
	})
}
