package middleware

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/getsentry/sentry-go"

	"psychic-homily-backend/internal/logger"
)

// HumaSentryContextMiddleware enriches the Sentry scope with request-level
// context: request ID, HTTP metadata, and user information (if authenticated).
// It should be applied after HumaRequestIDMiddleware and works with the hub
// injected by the sentryhttp Chi middleware.
func HumaSentryContextMiddleware(ctx huma.Context, next func(huma.Context)) {
	if hub := sentry.GetHubFromContext(ctx.Context()); hub != nil {
		hub.ConfigureScope(func(scope *sentry.Scope) {
			// Request ID for log correlation
			if reqID := logger.GetRequestID(ctx.Context()); reqID != "" {
				scope.SetTag("request_id", reqID)
			}

			// HTTP context
			scope.SetTag("http.method", ctx.Method())
			scope.SetTag("http.path", ctx.URL().Path)
			if q := ctx.URL().RawQuery; q != "" {
				scope.SetTag("http.query", q)
			}

			// User context (nil if not authenticated yet)
			if user := GetUserFromContext(ctx.Context()); user != nil {
				sentryUser := sentry.User{
					ID: fmt.Sprintf("%d", user.ID),
				}
				if user.Email != nil {
					sentryUser.Email = *user.Email
				}
				scope.SetUser(sentryUser)
				scope.SetTag("user.is_admin", fmt.Sprintf("%t", user.IsAdmin))
			}
		})
	}

	next(ctx)
}
