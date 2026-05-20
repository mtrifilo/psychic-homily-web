package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
)

// captureWithMiddleware runs HumaSentryContextMiddleware against req with a
// Sentry hub whose client's BeforeSend hook captures the serialized event.
// Triggering CaptureException through the same hub applies the scope the
// middleware configured, so the returned event reflects the tags/user it set.
func captureWithMiddleware(t *testing.T, req *http.Request) *sentry.Event {
	t.Helper()

	var captured *sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{
		// No DSN: BeforeSend still runs, but nothing leaves the process.
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			captured = event
			return nil // drop — we only need the serialized shape
		},
	})
	if err != nil {
		t.Fatalf("failed to create sentry test client: %v", err)
	}
	hub := sentry.NewHub(client, sentry.NewScope())
	reqWithHub := req.WithContext(sentry.SetHubOnContext(req.Context(), hub))
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, reqWithHub, rr)

	HumaSentryContextMiddleware(ctx, func(huma.Context) {})

	hub.CaptureException(errors.New("boom"))
	if captured == nil {
		t.Fatal("expected an event to be captured")
	}
	return captured
}

// --- HumaSentryContextMiddleware tests ---

func TestHumaSentryContextMiddleware_DropsRawQueryTag(t *testing.T) {
	// A magic-link request with a secret token in the query string.
	req := httptest.NewRequest(http.MethodGet, "/auth/magic-link?token=supersecretjwt", nil)
	captured := captureWithMiddleware(t, req)

	// The raw query tag must be absent.
	if v, ok := captured.Tags["http.query"]; ok {
		t.Errorf("http.query tag should be dropped, got %q", v)
	}

	// Path tag must still be present and must not carry the secret.
	if got := captured.Tags["http.path"]; got != "/auth/magic-link" {
		t.Errorf("http.path = %q, want %q", got, "/auth/magic-link")
	}
	if got := captured.Tags["http.method"]; got != http.MethodGet {
		t.Errorf("http.method = %q, want GET", got)
	}

	// Defense in depth: no tag value anywhere should contain the secret.
	for k, v := range captured.Tags {
		if strings.Contains(v, "supersecretjwt") {
			t.Errorf("tag %q leaks the secret token: %q", k, v)
		}
	}
}

func TestHumaSentryContextMiddleware_HashesUserEmail(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)

	email := "matt.trifilo@gmail.com"
	user := &authm.User{ID: 42, Email: strPtr(email), IsAdmin: true}
	reqWithUser := req.WithContext(
		context.WithValue(req.Context(), UserContextKey, user),
	)
	captured := captureWithMiddleware(t, reqWithUser)

	// Email must be masked, never plaintext.
	if captured.User.Email == email {
		t.Errorf("user email is plaintext %q, want masked", captured.User.Email)
	}
	want := logger.HashEmail(email)
	if captured.User.Email != want {
		t.Errorf("user email = %q, want masked %q", captured.User.Email, want)
	}

	// ID must still be present so events remain correlatable by user.
	if captured.User.ID != "42" {
		t.Errorf("user ID = %q, want %q", captured.User.ID, "42")
	}
	if got := captured.Tags["user.is_admin"]; got != "true" {
		t.Errorf("user.is_admin = %q, want true", got)
	}
}

func TestHumaSentryContextMiddleware_NoUserNoEmailTag(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	captured := captureWithMiddleware(t, req)

	if !captured.User.IsEmpty() {
		t.Errorf("user should be empty for unauthenticated request, got %+v", captured.User)
	}
	if v, ok := captured.Tags["http.query"]; ok {
		t.Errorf("http.query tag should be dropped, got %q", v)
	}
}

func TestHumaSentryContextMiddleware_CallsNext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)

	called := false
	HumaSentryContextMiddleware(ctx, func(huma.Context) { called = true })

	if !called {
		t.Error("next handler was not called")
	}
}
