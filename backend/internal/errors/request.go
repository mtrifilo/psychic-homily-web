package errors

import (
	"fmt"
)

// Request error codes
const (
	CodeRequestNotFound         = "REQUEST_NOT_FOUND"
	CodeRequestForbidden        = "REQUEST_FORBIDDEN"
	CodeRequestAlreadyFulfilled = "REQUEST_ALREADY_FULFILLED"
)

// RequestError represents a request-related error with additional context.
type RequestError struct {
	Code      string
	Message   string
	Internal  error
	RequestID uint
}

// Error implements the error interface.
func (e *RequestError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *RequestError) Unwrap() error {
	return e.Internal
}

// ErrRequestNotFound creates a request not found error.
func ErrRequestNotFound(requestID uint) *RequestError {
	return &RequestError{
		Code:      CodeRequestNotFound,
		Message:   fmt.Sprintf("Request %d not found", requestID),
		RequestID: requestID,
	}
}

// ErrRequestForbidden creates a request forbidden error.
func ErrRequestForbidden(requestID uint) *RequestError {
	return &RequestError{
		Code:      CodeRequestForbidden,
		Message:   fmt.Sprintf("Access denied for request %d", requestID),
		RequestID: requestID,
	}
}

// ErrRequestAlreadyFulfilled creates an already-fulfilled error.
func ErrRequestAlreadyFulfilled(requestID uint) *RequestError {
	return &RequestError{
		Code:      CodeRequestAlreadyFulfilled,
		Message:   fmt.Sprintf("Request %d is already fulfilled", requestID),
		RequestID: requestID,
	}
}
