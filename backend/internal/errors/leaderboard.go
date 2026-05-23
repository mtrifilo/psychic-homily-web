package errors

import (
	"fmt"
)

// Leaderboard error codes.
//
// Computing a leaderboard can fail because the requested dimension or period
// is not one of the supported values (semantic validation), or because of a
// database fault (internal). There is no not-found path — an empty board is a
// valid 200 result, not an error.
const (
	// CodeLeaderboardInvalidDimension indicates an unsupported dimension.
	CodeLeaderboardInvalidDimension = "LEADERBOARD_INVALID_DIMENSION"
	// CodeLeaderboardInvalidPeriod indicates an unsupported time period.
	CodeLeaderboardInvalidPeriod = "LEADERBOARD_INVALID_PERIOD"
	// CodeLeaderboardInternal indicates a database or infrastructure failure.
	CodeLeaderboardInternal = "LEADERBOARD_INTERNAL"
)

// LeaderboardError represents a leaderboard-related error with context.
type LeaderboardError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *LeaderboardError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *LeaderboardError) Unwrap() error {
	return e.Internal
}

// ErrLeaderboardInvalidDimension creates an invalid-dimension error.
func ErrLeaderboardInvalidDimension(dimension string) *LeaderboardError {
	return &LeaderboardError{
		Code:    CodeLeaderboardInvalidDimension,
		Message: fmt.Sprintf("invalid dimension: %s", dimension),
	}
}

// ErrLeaderboardInvalidPeriod creates an invalid-period error.
func ErrLeaderboardInvalidPeriod(period string) *LeaderboardError {
	return &LeaderboardError{
		Code:    CodeLeaderboardInvalidPeriod,
		Message: fmt.Sprintf("invalid period: %s", period),
	}
}

// ErrLeaderboardInternal wraps a database or infrastructure failure.
func ErrLeaderboardInternal(internal error) *LeaderboardError {
	return &LeaderboardError{
		Code:     CodeLeaderboardInternal,
		Message:  "failed to compute leaderboard",
		Internal: internal,
	}
}
