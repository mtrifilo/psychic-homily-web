package errors

import (
	"fmt"
)

// Auto-promotion error codes.
//
// Evaluating a single user for tier auto-promotion can fail because the user
// does not exist (not found) or because of a database fault (internal). The
// batch evaluation only fails on internal faults.
const (
	// CodeAutoPromotionUserNotFound indicates the target user does not exist.
	CodeAutoPromotionUserNotFound = "AUTO_PROMOTION_USER_NOT_FOUND"
	// CodeAutoPromotionInternal indicates a database or infrastructure failure.
	CodeAutoPromotionInternal = "AUTO_PROMOTION_INTERNAL"
)

// AutoPromotionError represents an auto-promotion error with context.
type AutoPromotionError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *AutoPromotionError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *AutoPromotionError) Unwrap() error {
	return e.Internal
}

// ErrAutoPromotionUserNotFound creates a user-not-found error.
func ErrAutoPromotionUserNotFound() *AutoPromotionError {
	return &AutoPromotionError{
		Code:    CodeAutoPromotionUserNotFound,
		Message: "user not found",
	}
}

// ErrAutoPromotionInternal wraps a database or infrastructure failure.
func ErrAutoPromotionInternal(internal error) *AutoPromotionError {
	return &AutoPromotionError{
		Code:     CodeAutoPromotionInternal,
		Message:  "failed to evaluate user for auto-promotion",
		Internal: internal,
	}
}
