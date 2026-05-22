package errors

import (
	"fmt"
)

// Contributor-profile (section) error codes.
//
// Profile sections are user-owned. Mutating one can fail because the section
// does not exist (or isn't owned by the caller — surfaced as not-found so we
// don't leak existence), because a field value is rejected by the section
// validation rules (semantic validation: title length, content length,
// position range, max-section count), or because of a database fault.
const (
	// CodeProfileSectionNotFound indicates the section does not exist for the
	// requesting user.
	CodeProfileSectionNotFound = "PROFILE_SECTION_NOT_FOUND"
	// CodeProfileSectionInvalid indicates a section field failed validation.
	CodeProfileSectionInvalid = "PROFILE_SECTION_INVALID"
	// CodeProfileInternal indicates a database or infrastructure failure.
	CodeProfileInternal = "PROFILE_INTERNAL"
)

// ProfileError represents a contributor-profile error with context.
type ProfileError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *ProfileError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *ProfileError) Unwrap() error {
	return e.Internal
}

// ErrProfileSectionNotFound creates a section-not-found error.
func ErrProfileSectionNotFound() *ProfileError {
	return &ProfileError{
		Code:    CodeProfileSectionNotFound,
		Message: "profile section not found",
	}
}

// ErrProfileSectionInvalid creates a section-validation error. The message is
// user-facing — it is surfaced verbatim by the handler so the contributor
// sees which rule was violated.
func ErrProfileSectionInvalid(message string) *ProfileError {
	return &ProfileError{
		Code:    CodeProfileSectionInvalid,
		Message: message,
	}
}

// ErrProfileInternal wraps a database or infrastructure failure.
func ErrProfileInternal(internal error) *ProfileError {
	return &ProfileError{
		Code:     CodeProfileInternal,
		Message:  "failed to update profile section",
		Internal: internal,
	}
}
