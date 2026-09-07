package auth

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
)

// captureStdLog redirects the standard logger for the duration of fn and
// returns everything written to it.
func captureStdLog(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()
	log.SetOutput(&buf)
	log.SetFlags(0)

	fn()
	return buf.String()
}

// TestOAuthLoginNeverLogsCookieValues pins the OAuth initiation handler against
// re-introducing a whole-value log of the request cookies. A user who is
// already signed in carries the session JWT in config.AuthCookieName, so
// logging cookie values writes a live session credential to stdout. Names stay
// loggable.
func TestOAuthLoginNeverLogsCookieValues(t *testing.T) {
	const sentinelSessionJWT = "SENTINEL-SESSION-JWT-VALUE-7c3a55"
	const sentinelGothicSession = "SENTINEL-GOTHIC-SESSION-b0d419"

	cfg := &config.Config{}
	handler := NewOAuthHTTPHandler(nil, cfg)

	req := httptest.NewRequest("GET", "/auth/login/google", nil)
	req.AddCookie(&http.Cookie{Name: config.AuthCookieName, Value: sentinelSessionJWT})
	req.AddCookie(&http.Cookie{Name: "_gothic_session", Value: sentinelGothicSession})

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("provider", "google")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

	output := captureStdLog(t, func() {
		handler.OAuthLoginHTTPHandler(httptest.NewRecorder(), req)
	})

	for _, secret := range []struct {
		label string
		value string
	}{
		{"session JWT cookie value", sentinelSessionJWT},
		{"gothic session cookie value", sentinelGothicSession},
	} {
		if strings.Contains(output, secret.value) {
			t.Errorf("the OAuth login handler logged the %s; captured log:\n%s", secret.label, output)
		}
	}

	if !strings.Contains(output, config.AuthCookieName) {
		t.Errorf("expected the cookie NAME in the log; captured log:\n%s", output)
	}
}
