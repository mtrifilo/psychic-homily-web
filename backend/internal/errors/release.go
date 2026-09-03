package errors

import (
	"fmt"
)

// Release error codes
const (
	CodeReleaseNotFound = "RELEASE_NOT_FOUND"
	CodeReleaseExists   = "RELEASE_EXISTS"
	// CodeReleaseInvalidField indicates a field value the service refused. It
	// exists so a service-layer validation refusal reaches the caller as a 422
	// carrying its own message, rather than as the generic 500 every
	// non-ReleaseError maps to, which would swallow the one sentence that tells
	// the submitter how to fix the value. Mirrors CodeArtistInvalidField.
	CodeReleaseInvalidField = "RELEASE_INVALID_FIELD"
)

// ReleaseError represents a release-related error with additional context.
type ReleaseError struct {
	Code      string
	Message   string
	Internal  error
	RequestID string
	ReleaseID uint
}

// Error implements the error interface.
func (e *ReleaseError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *ReleaseError) Unwrap() error {
	return e.Internal
}

// ErrReleaseNotFound creates a release not found error.
func ErrReleaseNotFound(releaseID uint) *ReleaseError {
	return &ReleaseError{
		Code:      CodeReleaseNotFound,
		Message:   "Release not found",
		ReleaseID: releaseID,
	}
}

// ErrReleaseInvalidField wraps a validation failure raised inside the service.
// The message is the validator's own, so the wording a curator sees is the same
// whichever layer refused the value.
func ErrReleaseInvalidField(err error) *ReleaseError {
	return &ReleaseError{
		Code:    CodeReleaseInvalidField,
		Message: err.Error(),
	}
}

// ErrReleaseExists creates a release-already-exists error.
func ErrReleaseExists(title string) *ReleaseError {
	return &ReleaseError{
		Code:    CodeReleaseExists,
		Message: fmt.Sprintf("Release with title '%s' already exists", title),
	}
}
