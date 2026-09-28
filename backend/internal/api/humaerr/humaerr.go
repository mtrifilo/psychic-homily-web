// Package humaerr controls which errors huma renders into an error response
// body.
//
// huma builds its error bodies through two package-level constructors,
// huma.NewError and huma.NewErrorWithContext: the huma.ErrorNNN helpers call
// the first, and a handler error that is not a huma.StatusError goes through
// the second. Their defaults put each non-ErrorDetailer error argument's
// Error() text into errors[].message, which for a wrapped service or driver
// error is internal text (SQL, schema names, service names). Install replaces
// both so that text is logged instead of rendered.
//
// Only the error arguments are filtered. The message argument becomes the
// body's detail verbatim, so a caller that formats an error into the message
// still puts that text in the body. A handler-defined huma.StatusError, such
// as middleware.ReauthRequiredError, is written as its own body and never
// reaches these constructors.
package humaerr

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/observability"
)

// Install replaces huma's error constructors. They are huma package globals,
// so this applies to every huma API in the process and stays in effect; call
// it at API setup, before any request is served.
//
// For the error arguments:
//   - a huma.ErrorDetailer (huma's own validation details, and the structured
//     *huma.ErrorDetail values handlers build) is rendered as built;
//   - any other error is left out of the body and logged, scrubbed;
//   - a handler error that huma could not resolve to a huma.StatusError and
//     that is, or wraps, an *AuthError answers with its code's status and the
//     auth middleware's envelope (success, message, error_code, request_id),
//     carrying only the external code and message. An *AuthError passed as a
//     huma.ErrorNNN argument is an error argument like any other: left out
//     and logged, at the status the caller chose.
func Install() {
	huma.NewError = newError
	huma.NewErrorWithContext = newErrorWithContext
}

// newError has no request context: handler code calls the huma.ErrorNNN
// helpers without one, so its log line carries no request ID, and it logs when
// the error is built whether or not the error is then written.
func newError(status int, msg string, errs ...error) huma.StatusError {
	return buildErrorModel(context.Background(), slog.Default(), status, msg, errs)
}

func newErrorWithContext(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
	reqCtx := context.Background()
	if ctx != nil {
		reqCtx = ctx.Context()
	}
	// The request's logger already carries its request ID.
	log := logger.FromContext(reqCtx)

	if len(errs) == 1 {
		var authErr *autherrors.AuthError
		if errors.As(errs[0], &authErr) {
			return newAuthErrorResponse(reqCtx, log, logger.GetRequestID(reqCtx), authErr, errs[0])
		}
	}
	return buildErrorModel(reqCtx, log, status, msg, errs)
}

func buildErrorModel(ctx context.Context, log *slog.Logger, status int, msg string, errs []error) huma.StatusError {
	var details []*huma.ErrorDetail
	var withheld []string
	for _, err := range errs {
		if err == nil {
			continue
		}
		if detailer, ok := err.(huma.ErrorDetailer); ok {
			details = append(details, detailer.ErrorDetail())
			continue
		}
		withheld = append(withheld, observability.ScrubText(err.Error()))
	}

	if len(withheld) > 0 {
		log.Log(ctx, levelFor(status), "error_response_details_withheld",
			"status", status,
			"detail", msg,
			"errors", withheld,
		)
	}

	return &huma.ErrorModel{
		Status: status,
		Title:  http.StatusText(status),
		Detail: msg,
		Errors: details,
	}
}

// authErrorResponse is the auth envelope at the AuthError's status. It has no
// field for the error's internal chain.
type authErrorResponse struct {
	middleware.JWTErrorResponse
	status int
}

func (e *authErrorResponse) Error() string { return e.Message }

// GetStatus makes this a huma.StatusError, so huma writes this value as the
// body at this status.
func (e *authErrorResponse) GetStatus() int { return e.status }

func newAuthErrorResponse(ctx context.Context, log *slog.Logger, requestID string, authErr *autherrors.AuthError, returned error) *authErrorResponse {
	status := authErr.HTTPStatus()
	log.Log(ctx, levelFor(status), "auth_error_response",
		"status", status,
		"auth_code", authErr.Code,
		"error", observability.ScrubText(returned.Error()),
	)
	return &authErrorResponse{
		JWTErrorResponse: middleware.JWTErrorResponse{
			Message:   authErr.ExternalMessage(),
			ErrorCode: autherrors.ToExternalCode(authErr.Code),
			RequestID: requestID,
		},
		status: status,
	}
}

func levelFor(status int) slog.Level {
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}
	return slog.LevelWarn
}
