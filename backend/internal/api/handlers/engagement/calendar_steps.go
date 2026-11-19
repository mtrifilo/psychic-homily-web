package engagement

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// CalendarStepsHandler sends the signed-in account the email that walks it
// through adding its saved shows to Google Calendar on a computer.
type CalendarStepsHandler struct {
	emailOutbox contracts.AuthEmailOutboxInterface
}

// NewCalendarStepsHandler creates the handler over the outbox that queues the
// email.
func NewCalendarStepsHandler(emailOutbox contracts.AuthEmailOutboxInterface) *CalendarStepsHandler {
	return &CalendarStepsHandler{emailOutbox: emailOutbox}
}

// SendCalendarStepsEmailRequest is empty: the account is the caller's, and the
// email goes to its own address.
type SendCalendarStepsEmailRequest struct{}

// SendCalendarStepsEmailResponse has no body: success is 204.
type SendCalendarStepsEmailResponse struct{}

// SendCalendarStepsEmailHandler queues the calendar steps email for the
// caller's own address and answers 204.
//
// It needs no feed token and reads none, so it is not a credential mint. It
// still requires a session credential: the protected group also admits
// API-token principals, and sending mail to the account's inbox is the
// signed-in person's request to make. Refusals:
//
//   - an API-token principal: 403 SESSION_CREDENTIAL_REQUIRED;
//   - an address that is not verified, or none: 403 EMAIL_VERIFICATION_REQUIRED;
//   - the account's hourly cap reached: 429 CALENDAR_STEPS_RATE_LIMITED_HOURLY
//     with Retry-After;
//   - the outbox switched off, no email provider, or a failed queue write:
//     503 SERVICE_UNAVAILABLE.
//
// 204 means the account's email is queued, by this request or by one of the
// account's still queued or in flight; repeats add no second email.
func (h *CalendarStepsHandler) SendCalendarStepsEmailHandler(ctx context.Context, _ *SendCalendarStepsEmailRequest) (*SendCalendarStepsEmailResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if err := middleware.RequireSessionCredential(ctx, "send_calendar_steps_email"); err != nil {
		return nil, err
	}

	err := h.emailOutbox.EnqueueCalendarSteps(ctx, user)

	var authErr *apperrors.AuthError
	var limited *apperrors.CalendarStepsRateLimitedError
	switch {
	case err == nil:
		// Accepted, not necessarily a new row: a repeat while one is queued
		// is absorbed (EnqueueCalendarSteps).
		logger.FromContext(ctx).Info("calendar_steps_email_accepted", "user_id", user.ID)
		return &SendCalendarStepsEmailResponse{}, nil
	case errors.As(err, &limited):
		logger.FromContext(ctx).Warn("calendar_steps_email_rate_limited",
			"user_id", user.ID,
			"retry_after", limited.RetryAfter,
		)
		return nil, calendarStepsRateLimited(ctx, limited)
	case errors.As(err, &authErr):
		logger.FromContext(ctx).Warn("calendar_steps_email_refused",
			"user_id", user.ID,
			"code", authErr.Code,
		)
		return nil, authErr
	default:
		logger.FromContext(ctx).Error("calendar_steps_email_enqueue_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		return nil, apperrors.ErrServiceUnavailable("calendar_steps_email", err)
	}
}

// calendarStepsRateLimitedResponse is the 429 refusal in the envelope the
// other refusals of this route answer in (success, message, error_code,
// request_id), so a client reads every refusal's code from one field.
type calendarStepsRateLimitedResponse struct {
	middleware.JWTErrorResponse
}

func (e *calendarStepsRateLimitedResponse) Error() string { return e.Message }

// GetStatus satisfies huma.StatusError, so huma writes this value as the body.
func (e *calendarStepsRateLimitedResponse) GetStatus() int { return http.StatusTooManyRequests }

// calendarStepsRateLimited answers the hourly cap with 429 and a Retry-After
// of the whole seconds, rounded up and at least one, until the account may ask
// again.
func calendarStepsRateLimited(ctx context.Context, limited *apperrors.CalendarStepsRateLimitedError) error {
	retryAfter := max(int64(math.Ceil(limited.RetryAfter.Seconds())), 1)
	return huma.ErrorWithHeaders(&calendarStepsRateLimitedResponse{JWTErrorResponse: middleware.JWTErrorResponse{
		Message:   limited.UserMessage(),
		ErrorCode: apperrors.CodeCalendarStepsRateLimitedHourly,
		RequestID: logger.GetRequestID(ctx),
	}}, http.Header{"Retry-After": {strconv.FormatInt(retryAfter, 10)}})
}
