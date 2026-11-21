package engagement

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
)

// calendarRateLimitedResponse is the 429 refusal of a calendar route's hourly
// cap, in the envelope the other refusals of these routes answer in (success,
// message, error_code, request_id), so a client reads every refusal's code
// from one field.
type calendarRateLimitedResponse struct {
	middleware.JWTErrorResponse
}

func (e *calendarRateLimitedResponse) Error() string { return e.Message }

// GetStatus satisfies huma.StatusError, so huma writes this value as the body.
func (e *calendarRateLimitedResponse) GetStatus() int { return http.StatusTooManyRequests }

// calendarRateLimited answers an hourly cap with 429, code and message, and a
// Retry-After of the whole seconds in retryAfter, rounded up and at least one.
func calendarRateLimited(ctx context.Context, code, message string, retryAfter time.Duration) error {
	seconds := max(int64(math.Ceil(retryAfter.Seconds())), 1)
	return huma.ErrorWithHeaders(&calendarRateLimitedResponse{JWTErrorResponse: middleware.JWTErrorResponse{
		Message:   message,
		ErrorCode: code,
		RequestID: logger.GetRequestID(ctx),
	}}, http.Header{"Retry-After": {strconv.FormatInt(seconds, 10)}})
}
