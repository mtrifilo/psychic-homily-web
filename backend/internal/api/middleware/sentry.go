package middleware

import (
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/logger"
)

// SentryRequestIDTag tags the request's Sentry scope with the request ID, so
// every event from a route registered on chi or a Huma operation carries the
// ID its response and log lines carry. Register it after RequestIDMiddleware
// and the sentryhttp handler, whose hub it tags.
func SentryRequestIDTag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			if requestID := logger.GetRequestID(r.Context()); requestID != "" {
				hub.Scope().SetTag("request_id", requestID)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// SentryPanicEmailScrub redacts email addresses from the panic value on the
// event the sentryhttp handler captures for a handler panic, as RecoverPanic
// redacts them from its log line. Every other event from the request keeps its
// text. Register it right after the sentryhttp handler, whose per-request hub
// it configures; the BeforeSend scrub runs after it.
func SentryPanicEmailScrub(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().AddEventProcessor(redactPanicEventEmails)
		}
		next.ServeHTTP(w, r)
	})
}

// redactPanicEventEmails applies redactPanicText to a recovered panic's event:
// its message (a non-error panic value) and its exception values (an error and
// the errors it wraps). The BeforeSend scrub caps them afterwards. An event
// with no recovered panic in its hint, which includes the request's
// transaction, passes through unchanged.
func redactPanicEventEmails(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if hint == nil || hint.RecoveredException == nil {
		return event
	}
	event.Message = redactPanicText(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = redactPanicText(event.Exception[i].Value)
	}
	return event
}

// HumaSentryContextMiddleware enriches the Sentry scope with HTTP metadata and
// user information (if authenticated). The request ID tag comes from
// SentryRequestIDTag in the chi chain. It works with the hub injected by the
// sentryhttp Chi middleware.
func HumaSentryContextMiddleware(ctx huma.Context, next func(huma.Context)) {
	if hub := sentry.GetHubFromContext(ctx.Context()); hub != nil {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			// HTTP context. The raw query string is deliberately NOT
			// tagged: secret-bearing routes (magic-link ?token=, digest
			// unsubscribe ?sig=) would otherwise land in searchable Sentry
			// retention. Sentry's default request integration still captures
			// the full URL, so method+path remain sufficient for triage.
			scope.SetTag("http.method", ctx.Method())
			scope.SetTag("http.path", ctx.URL().Path)

			// User context (nil if not authenticated yet). Email is masked
			// via logger.HashEmail so the observability tier never holds
			// plaintext addresses; ID + masked email keep events correlatable.
			if user := GetUserFromContext(ctx.Context()); user != nil {
				sentryUser := sentry.User{
					ID: fmt.Sprintf("%d", user.ID),
				}
				if user.Email != nil {
					sentryUser.Email = logger.HashEmail(*user.Email)
				}
				scope.SetUser(sentryUser)
				scope.SetTag("user.is_admin", fmt.Sprintf("%t", user.IsAdmin))
			}
		})
	}

	next(ctx)
}
