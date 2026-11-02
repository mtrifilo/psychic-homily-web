package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"

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

// --- SentryPanicEmailScrub tests ---

// servePanicThroughSentry runs handler behind the sentryhttp handler and
// SentryPanicEmailScrub, in production order, on a hub whose client records
// every event it would send (error events and transactions).
func servePanicThroughSentry(t *testing.T, handler http.HandlerFunc) []*sentry.Event {
	t.Helper()
	var events []*sentry.Event
	record := func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		events = append(events, event)
		return nil
	}
	client, err := sentry.NewClient(sentry.ClientOptions{
		EnableTracing:         true,
		TracesSampleRate:      1,
		BeforeSend:            record,
		BeforeSendTransaction: record,
	})
	if err != nil {
		t.Fatalf("failed to create sentry test client: %v", err)
	}
	hub := sentry.NewHub(client, sentry.NewScope())
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req = req.WithContext(sentry.SetHubOnContext(req.Context(), hub))

	chain := sentryhttp.New(sentryhttp.Options{}).Handle(SentryPanicEmailScrub(handler))
	chain.ServeHTTP(httptest.NewRecorder(), req)
	return events
}

// The panic event carries no email from the panic value, for an error (its
// exception values) and a plain value (its message), and a secret key the
// email runs into stays redacted. Another event the request captures, and the
// transaction, keep their text.
func TestSentryPanicEmailScrub_RedactsOnlyThePanicEvent(t *testing.T) {
	const email = "jane.doe@example.com"
	values := map[string]any{
		"error":  fmt.Errorf("lookup failed for %s&password=hunter2: %w", email, errors.New("not found")),
		"string": "lookup failed for " + email + "&password=hunter2",
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			events := servePanicThroughSentry(t, func(_ http.ResponseWriter, r *http.Request) {
				sentry.GetHubFromContext(r.Context()).CaptureMessage("note for " + email)
				panic(value)
			})

			var note, panicEvent, transaction *sentry.Event
			for _, event := range events {
				switch {
				case event.Type == "transaction":
					transaction = event
				case strings.HasPrefix(event.Message, "note for"):
					note = event
				default:
					panicEvent = event
				}
			}
			if note == nil || panicEvent == nil || transaction == nil {
				t.Fatalf("events = %+v, want a note, a panic event and a transaction", events)
			}

			texts := []string{panicEvent.Message}
			for _, exception := range panicEvent.Exception {
				texts = append(texts, exception.Value)
			}
			joined := strings.Join(texts, "\n")
			if strings.Contains(joined, "jane.doe") || strings.Contains(joined, "example.com") || strings.Contains(joined, "hunter2") {
				t.Errorf("panic event text %q carries the email or the password", joined)
			}
			if !strings.Contains(joined, "lookup failed for "+redactedEmail) {
				t.Errorf("panic event text %q, want the panic value with the email replaced", joined)
			}
			if note.Message != "note for "+email {
				t.Errorf("note message = %q, want it unchanged", note.Message)
			}
		})
	}
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
