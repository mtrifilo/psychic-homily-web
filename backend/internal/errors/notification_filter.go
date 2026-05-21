package errors

import (
	"fmt"
)

// Notification filter error codes.
//
// Filter CRUD has three genuine failure shapes: the target filter does not
// exist or is owned by another user (not found), a request that parses but
// violates a domain rule — no criteria set, per-user filter cap reached
// (validation), and a database/infrastructure fault (internal). Matching the
// PSY-761 engagement convention so the handlers can return 404 / 422 / 500
// instead of 422-for-everything.
const (
	// CodeFilterNotFound indicates the filter does not exist for this user.
	CodeFilterNotFound = "FILTER_NOT_FOUND"
	// CodeFilterValidation indicates a well-formed request rejected by a
	// domain rule (no criteria set, per-user filter limit reached).
	CodeFilterValidation = "FILTER_VALIDATION"
	// CodeFilterInternal indicates a database or infrastructure failure.
	CodeFilterInternal = "FILTER_INTERNAL"
)

// NotificationFilterError represents a notification-filter error with context.
type NotificationFilterError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *NotificationFilterError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *NotificationFilterError) Unwrap() error {
	return e.Internal
}

// ErrFilterNotFound creates a filter-not-found error.
func ErrFilterNotFound() *NotificationFilterError {
	return &NotificationFilterError{
		Code:    CodeFilterNotFound,
		Message: "filter not found",
	}
}

// ErrFilterValidation creates a domain-validation error with a caller-supplied
// message (e.g. "at least one filter criteria is required").
func ErrFilterValidation(message string) *NotificationFilterError {
	return &NotificationFilterError{
		Code:    CodeFilterValidation,
		Message: message,
	}
}

// ErrFilterInternal wraps a database or infrastructure failure.
func ErrFilterInternal(internal error) *NotificationFilterError {
	return &NotificationFilterError{
		Code:     CodeFilterInternal,
		Message:  "failed to process notification filter",
		Internal: internal,
	}
}
