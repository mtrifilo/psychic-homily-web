package auth

import (
	"net/http"
	"strings"
	"testing"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/testlog"
)

// TestOAuthLoginNeverLogsCookieValues asserts the invariant that the OAuth
// initiation handler logs cookie NAMES and never cookie values. A signed-in
// user carries the session JWT in config.AuthCookieName, so a cookie value in
// the log stream is a live session credential.
func TestOAuthLoginNeverLogsCookieValues(t *testing.T) {
	const sentinelSessionJWT = "SENTINEL-SESSION-JWT-VALUE-7c3a55"
	const sentinelCallbackID = "SENTINEL-CLI-CALLBACK-ID-b0d419"

	handler := NewOAuthHTTPHandler(nil, &config.Config{})

	w, req := oauthLoginRequest("google")
	req.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: sentinelSessionJWT})
	req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: sentinelCallbackID})

	output := testlog.Capture(t, func() {
		handler.OAuthLoginHTTPHandler(w, req)
	})

	for _, secret := range []struct {
		label string
		value string
	}{
		{"session JWT cookie value", sentinelSessionJWT},
		{"CLI callback ID cookie value", sentinelCallbackID},
	} {
		if strings.Contains(output, secret.value) {
			t.Errorf("the OAuth login handler logged the %s; captured log:\n%s", secret.label, output)
		}
	}

	if !strings.Contains(output, config.AuthCookieName) {
		t.Errorf("expected the cookie NAME in the log; captured log:\n%s", output)
	}
}
