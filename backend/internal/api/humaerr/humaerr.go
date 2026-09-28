// Package humaerr decides what huma writes into an error response body.
//
// huma builds every error body it writes through two package-level
// constructors, huma.NewError and huma.NewErrorWithContext: the huma.ErrorNNN
// helpers call the first, and a handler error that is not a huma.StatusError
// goes through the second. Their defaults put each non-ErrorDetailer error's
// Error() text into errors[].message, which for a wrapped service or driver
// error is internal text (SQL, schema names, service names). Install replaces
// both so that text goes to the log and never to the body.
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
)

// Install replaces huma's error constructors with ones that keep internal
// error text out of response bodies. The constructors are huma package
// globals, so this applies to every huma API in the process; call it at API
// setup, before any request is served.
//
// What reaches the body:
//   - a huma.ErrorDetailer (huma's own validation details, and the structured
//     *huma.ErrorDetail values handlers build) is kept as the handler built it;
//   - any other error is withheld from the body and logged;
//   - a handler error that is an *AuthError, directly or wrapped, answers with
//     its code's status and the auth envelope (success, message, error_code,
//     request_id) the auth middleware writes, carrying only the external code
//     and message.
func Install() {
	huma.NewError = newError
	huma.NewErrorWithContext = newErrorWithContext
}

// newError has no request context: it runs inside handler code, which calls
// the huma.ErrorNNN helpers without one. Withheld text is logged without a
// request ID for that reason.
func newError(status int, msg string, errs ...error) huma.StatusError {
	return buildErrorModel(context.Background(), slog.Default(), "", status, msg, errs)
}

func newErrorWithContext(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
	reqCtx := context.Background()
	if ctx != nil {
		reqCtx = ctx.Context()
	}
	log, requestID := logger.FromContext(reqCtx), logger.GetRequestID(reqCtx)

	if len(errs) == 1 {
		var authErr *autherrors.AuthError
		if errors.As(errs[0], &authErr) {
			return newAuthErrorResponse(reqCtx, log, requestID, authErr, errs[0])
		}
	}
	return buildErrorModel(reqCtx, log, requestID, status, msg, errs)
}

func buildErrorModel(ctx context.Context, log *slog.Logger, requestID string, status int, msg string, errs []error) huma.StatusError {
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
		withheld = append(withheld, err.Error())
	}

	if len(withheld) > 0 {
		log.Log(ctx, levelFor(status), "error_response_details_withheld",
			"status", status,
			"detail", msg,
			"errors", withheld,
			"request_id", requestID,
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
		"error", returned.Error(),
		"request_id", requestID,
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
