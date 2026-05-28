package errors

import (
	"fmt"
)

// Request error codes
const (
	CodeRequestNotFound           = "REQUEST_NOT_FOUND"
	CodeRequestForbidden          = "REQUEST_FORBIDDEN"
	CodeRequestAlreadyFulfilled   = "REQUEST_ALREADY_FULFILLED"
	CodeRequestEntityTypeMismatch = "REQUEST_ENTITY_TYPE_MISMATCH"
	CodeRequestEntityNotFound     = "REQUEST_ENTITY_NOT_FOUND"
	CodeRequestInvalidState       = "REQUEST_INVALID_STATE"
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

// ErrRequestEntityTypeMismatch creates an error for when the proposed
// fulfilling entity's type doesn't match the request's declared entity type.
// e.g. caller submits a venue ID for an "artist" request.
func ErrRequestEntityTypeMismatch(requestID uint, requestType, providedType string) *RequestError {
	return &RequestError{
		Code:      CodeRequestEntityTypeMismatch,
		Message:   fmt.Sprintf("Request %d expects an entity of type %q but the provided entity is of type %q", requestID, requestType, providedType),
		RequestID: requestID,
	}
}

// ErrRequestEntityNotFound creates an error for when the proposed
// fulfilling entity does not exist (or has been soft-deleted, depending
// on the lookup table). Surfaces 400 rather than 404 because the request
// itself was found — only the linked entity payload is wrong.
func ErrRequestEntityNotFound(requestID uint, entityType string, entityID uint) *RequestError {
	return &RequestError{
		Code:      CodeRequestEntityNotFound,
		Message:   fmt.Sprintf("Request %d cannot be fulfilled: %s entity %d does not exist", requestID, entityType, entityID),
		RequestID: requestID,
	}
}

// ErrRequestInvalidState creates an error for when an operation
// requires the request to be in a specific state. e.g. approving a
// fulfillment when the request is not in pending_fulfillment.
func ErrRequestInvalidState(requestID uint, current, expected string) *RequestError {
	return &RequestError{
		Code:      CodeRequestInvalidState,
		Message:   fmt.Sprintf("Request %d is in state %q; expected %q", requestID, current, expected),
		RequestID: requestID,
	}
}
