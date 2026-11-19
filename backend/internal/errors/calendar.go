package errors

import (
	"errors"
	"fmt"
	"time"
)

// Calendar error codes.
//
// Status mapping:
//   - CalendarStepsRateLimitedHourly → 429 with Retry-After set to the whole
//     seconds until the account may ask again (CalendarStepsHandler in the
//     engagement handlers).
const (
	// CodeCalendarStepsRateLimitedHourly indicates the account has already
	// asked for the calendar steps email as many times as the rolling hour
	// allows.
	CodeCalendarStepsRateLimitedHourly = "CALENDAR_STEPS_RATE_LIMITED_HOURLY"
)

// The approved copy for the calendar panel's two refusals. A client keys its
// own copy on the code; these are what a reader of the raw response sees, so
// they say the same thing.
const (
	calendarStepsRateLimitedMessage       = "You've asked for this a few times. Try again in an hour."
	calendarStepsAddressUnverifiedMessage = "Verify your email address first, then try again."
)

// CalendarStepsRateLimitedError refuses a calendar steps email because the
// account has asked for Limit of them within the last hour. RetryAfter is how
// long until the oldest of those requests leaves the hour.
type CalendarStepsRateLimitedError struct {
	Limit      int
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *CalendarStepsRateLimitedError) Error() string {
	return fmt.Sprintf("%s: %d calendar steps emails in the last hour (retry after %s)",
		CodeCalendarStepsRateLimitedHourly, e.Limit, e.RetryAfter)
}

// UserMessage returns the user-safe message.
func (e *CalendarStepsRateLimitedError) UserMessage() string {
	return calendarStepsRateLimitedMessage
}

// ErrCalendarStepsAddressUnverified refuses a calendar steps email to an
// account whose address is not verified, or that has none: nothing shows
// that its holder reads that mailbox. It carries the code connecting a
// provider is refused with, because the remedy is the same.
func ErrCalendarStepsAddressUnverified() *AuthError {
	return NewAuthError(
		CodeEmailVerificationRequired,
		calendarStepsAddressUnverifiedMessage,
		errors.New("account address is not verified: calendar steps email"),
	)
}
