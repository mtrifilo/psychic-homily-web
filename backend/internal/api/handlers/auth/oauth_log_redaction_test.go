package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testlog"
)

// cliCallbackID reads back the cli_callback_id the login handler minted. That
// value is the correlation key gating the token-bearing CLI redirect.
func cliCallbackID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	for _, c := range w.Result().Cookies() {
		if c.Name == "cli_callback_id" && c.Value != "" {
			return c.Value
		}
	}
	t.Fatal("expected the login handler to mint a cli_callback_id cookie")
	return ""
}

// TestOAuthLoginNeverLogsCredentials asserts the invariant that the OAuth
// initiation handler logs cookie NAMES, the request PATH, and the loopback
// callback URL, and never a cookie value, a query string, or the CLI callback
// ID. A signed-in user carries the session JWT in config.AuthCookieName, and
// the CLI callback ID gates a redirect that later carries a session JWT.
func TestOAuthLoginNeverLogsCredentials(t *testing.T) {
	const sentinelSessionJWT = "SENTINEL-SESSION-JWT-VALUE-7c3a55"
	const sentinelConsentCookie = "SENTINEL-CONSENT-COOKIE-b0d419"
	const loopbackCallback = "http://localhost:8765/cli-oauth-return"

	handler := NewOAuthHTTPHandler(nil, &config.Config{})

	// The cli_callback branch writes to the package-global store.
	t.Cleanup(cleanCLICallbackStore)

	w, req := oauthLoginRequest("google")
	// Reach the cliCallback branch, which the bare helper request skips.
	req.URL.RawQuery = "cli_callback=" + loopbackCallback
	req.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: sentinelSessionJWT})
	req.AddCookie(&http.Cookie{Name: oauthSignupConsentCookieName, Value: sentinelConsentCookie})

	output := testlog.Capture(t, func() {
		handler.OAuthLoginHTTPHandler(w, req)
	})

	mintedCallbackID := cliCallbackID(t, w)

	for _, secret := range []struct {
		label string
		value string
	}{
		{"session JWT cookie value", sentinelSessionJWT},
		{"signup consent cookie value", sentinelConsentCookie},
		{"minted CLI callback ID", mintedCallbackID},
	} {
		if strings.Contains(output, secret.value) {
			t.Errorf("the OAuth login handler logged the %s; captured log:\n%s", secret.label, output)
		}
	}

	// Pin the rendered fragments so an unrelated line cannot satisfy these.
	for _, want := range []string{
		// Not pinned to a position: cookie order is not the invariant.
		"Login - Request cookie names BEFORE: [",
		config.AuthCookieName,
		"Login - Request path: /auth/login/google",
		"CLI callback stored: " + loopbackCallback,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in the log; captured log:\n%s", want, output)
		}
	}
}

// TestOAuthCallbackHandlerNeverLogsTheMintedToken asserts the invariant that
// the OAuth callback handler logs neither the session JWT it mints nor the CLI
// callback ID that routes it, on the branch that redirects the token to a
// loopback URL.
func TestOAuthCallbackHandlerNeverLogsTheMintedToken(t *testing.T) {
	const sentinelToken = "SENTINEL-MINTED-SESSION-JWT-4d81"
	const sentinelCallbackID = "SENTINEL-CLI-CALLBACK-ID-9a02"
	const loopbackCallback = "http://localhost:8765/cli-oauth-return"

	t.Cleanup(cleanCLICallbackStore)
	storeCLICallback(sentinelCallbackID, loopbackCallback)

	authService := &testhelpers.MockAuthService{
		OAuthCallbackWithConsentFn: func(
			http.ResponseWriter, *http.Request, string, *contracts.OAuthSignupConsent,
		) (*authm.User, string, error) {
			return &authm.User{ID: 9}, sentinelToken, nil
		},
	}
	handler := NewOAuthHTTPHandler(authService, &config.Config{})

	req := httptest.NewRequest("GET", "/auth/callback/google?code=abc&state=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: sentinelCallbackID})
	w := httptest.NewRecorder()

	output := testlog.Capture(t, func() {
		handler.OAuthCallbackHTTPHandler(w, req)
	})

	if got := w.Result().StatusCode; got != http.StatusTemporaryRedirect {
		t.Fatalf("expected the CLI redirect branch, got status %d", got)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, sentinelToken) {
		t.Fatalf("expected the minted token in the redirect, got %q", loc)
	}

	const marker = "CLI callback found:"
	if !strings.Contains(output, marker) {
		t.Fatalf("captured log is missing %q, so testlog.Capture no longer intercepts this "+
			"path and the assertions below are vacuous; captured log:\n%s", marker, output)
	}

	for _, secret := range []struct {
		label string
		value string
	}{
		{"minted session token", sentinelToken},
		{"CLI callback ID", sentinelCallbackID},
	} {
		if strings.Contains(output, secret.value) {
			t.Errorf("the OAuth callback handler logged the %s; captured log:\n%s", secret.label, output)
		}
	}
}

// TestOAuthCallbackHandlerRedactsTokenBearingErrorURL asserts the invariant
// that a transport error from the callback is logged with its URL query
// stripped. goth's Google provider fetches the profile with the access token in
// the query string, so an ordinary network fault produces a *url.Error whose
// message embeds a live token; no attacker action is needed to reach it.
func TestOAuthCallbackHandlerRedactsTokenBearingErrorURL(t *testing.T) {
	const sentinelAccessToken = "SENTINEL-LIVE-ACCESS-TOKEN-e71c"

	transportErr := &url.Error{
		Op:  "Get",
		URL: "https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + sentinelAccessToken,
		Err: errors.New("dial tcp: i/o timeout"),
	}

	authService := &testhelpers.MockAuthService{
		OAuthCallbackWithConsentFn: func(
			http.ResponseWriter, *http.Request, string, *contracts.OAuthSignupConsent,
		) (*authm.User, string, error) {
			return nil, "", fmt.Errorf("OAuth completion failed: %w", transportErr)
		},
	}
	handler := NewOAuthHTTPHandler(authService, &config.Config{})

	req := httptest.NewRequest("GET", "/auth/callback/google", nil)
	w := httptest.NewRecorder()

	output := testlog.Capture(t, func() {
		handler.OAuthCallbackHTTPHandler(w, req)
	})

	const marker = "OAuth callback failed:"
	if !strings.Contains(output, marker) {
		t.Fatalf("captured log is missing %q, so testlog.Capture no longer intercepts this "+
			"path and the assertion below is vacuous; captured log:\n%s", marker, output)
	}
	if strings.Contains(output, sentinelAccessToken) {
		t.Errorf("the OAuth callback handler logged a live access token from the error URL; "+
			"captured log:\n%s", output)
	}
	// The host survives redaction, so the log still names the endpoint that failed.
	if !strings.Contains(output, "www.googleapis.com") {
		t.Errorf("expected the failing host to survive redaction; captured log:\n%s", output)
	}
}

// TestOAuthCallbackHandlerScrubsNonURLProviderError asserts the invariant that
// a provider error which is NOT a *url.Error is still scrubbed and capped. A
// token-exchange failure renders the token endpoint's raw response body into
// the error text, which RedactErrorURL passes through untouched.
func TestOAuthCallbackHandlerScrubsNonURLProviderError(t *testing.T) {
	const sentinelBodyToken = "SENTINEL-BODY-ACCESS-TOKEN-5b30"

	// The shape x/oauth2 renders when the token endpoint returns a failure
	// status: a plain error carrying the response body verbatim.
	providerErr := fmt.Errorf(
		"oauth2: cannot fetch token: 400 Bad Request\nResponse: %s",
		`{"access_token":"`+sentinelBodyToken+`","error":"invalid_grant"}`+strings.Repeat("x", 4000),
	)

	authService := &testhelpers.MockAuthService{
		OAuthCallbackWithConsentFn: func(
			http.ResponseWriter, *http.Request, string, *contracts.OAuthSignupConsent,
		) (*authm.User, string, error) {
			return nil, "", fmt.Errorf("OAuth completion failed: %w", providerErr)
		},
	}
	handler := NewOAuthHTTPHandler(authService, &config.Config{})

	req := httptest.NewRequest("GET", "/auth/callback/google", nil)
	w := httptest.NewRecorder()

	output := testlog.Capture(t, func() {
		handler.OAuthCallbackHTTPHandler(w, req)
	})

	const marker = "OAuth callback failed:"
	if !strings.Contains(output, marker) {
		t.Fatalf("captured log is missing %q, so testlog.Capture no longer intercepts this "+
			"path and the assertions below are vacuous; captured log:\n%s", marker, output)
	}
	if strings.Contains(output, sentinelBodyToken) {
		t.Errorf("the OAuth callback handler logged a token from the provider response body; "+
			"captured log:\n%s", output)
	}
	// An unbounded provider body must not reach the log stream in full.
	if len(output) > 4000 {
		t.Errorf("expected the provider error to be capped, got %d bytes of log", len(output))
	}
}
