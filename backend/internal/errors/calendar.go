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
//   - CalendarLinkRateLimitedHourly → 429 with Retry-After set to the whole
//     seconds until the counted clock hour ends (CalendarHandler in the
//     engagement handlers).
const (
	// CodeCalendarStepsRateLimitedHourly indicates the account has already
	// asked for the calendar steps email as many times as the rolling hour
	// allows.
	CodeCalendarStepsRateLimitedHourly = "CALENDAR_STEPS_RATE_LIMITED_HOURLY"

	// CodeCalendarLinkRateLimitedHourly indicates the account has already
	// made or replaced its calendar link as many times as the current clock
	// hour allows.
	CodeCalendarLinkRateLimitedHourly = "CALENDAR_LINK_RATE_LIMITED_HOURLY"
)

// The approved copy for the calendar panel's refusals. A client keys its own
// copy on the code; these are what a reader of the raw response sees, so they
// say the same thing. The link cap has no copy of its own: each surface shows
// its own line for a link it could not make, and the response carries the
// saved-shows panel's.
const (
	calendarStepsRateLimitedMessage       = "You've asked for this a few times. Try again in an hour."
	calendarStepsAddressUnverifiedMessage = "Verify your email address first, then try again."
	calendarLinkRateLimitedMessage        = "Couldn't create your calendar link. Try again."
)

// CalendarLinkRateLimitedError refuses making or replacing a calendar link
// because the account has already made Limit of them in the current clock
// hour. RetryAfter is how long until the end of the hour the stored count
// belongs to: the next clock hour, unless this server's clock is behind the
// one that last counted.
type CalendarLinkRateLimitedError struct {
	Limit      int
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *CalendarLinkRateLimitedError) Error() string {
	return fmt.Sprintf("%s: %d calendar links made this hour (retry after %s)",
		CodeCalendarLinkRateLimitedHourly, e.Limit, e.RetryAfter)
}

// UserMessage returns the user-safe message.
func (e *CalendarLinkRateLimitedError) UserMessage() string {
	return calendarLinkRateLimitedMessage
}

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
