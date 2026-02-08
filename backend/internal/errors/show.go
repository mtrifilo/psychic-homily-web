// Package errors provides custom error types for the application.
package errors

import (
	"fmt"
)

// Show error codes
const (
	// CodeShowNotFound indicates the show was not found
	CodeShowNotFound = "SHOW_NOT_FOUND"
	// CodeShowCreateFailed indicates show creation failed
	CodeShowCreateFailed = "SHOW_CREATE_FAILED"
	// CodeShowUpdateFailed indicates show update failed
	CodeShowUpdateFailed = "SHOW_UPDATE_FAILED"
	// CodeShowDeleteFailed indicates show deletion failed
	CodeShowDeleteFailed = "SHOW_DELETE_FAILED"
	// CodeShowDeleteUnauthorized indicates user is not authorized to delete the show
	CodeShowDeleteUnauthorized = "SHOW_DELETE_UNAUTHORIZED"
	// CodeShowUpdateUnauthorized indicates user is not authorized to update the show
	CodeShowUpdateUnauthorized = "SHOW_UPDATE_UNAUTHORIZED"
	// CodeShowInvalidID indicates an invalid show ID
	CodeShowInvalidID = "SHOW_INVALID_ID"
	// CodeShowValidationFailed indicates validation failed
	CodeShowValidationFailed = "SHOW_VALIDATION_FAILED"
	// CodeVenueRequired indicates at least one venue is required
	CodeVenueRequired = "VENUE_REQUIRED"
	// CodeArtistRequired indicates at least one artist is required
	CodeArtistRequired = "ARTIST_REQUIRED"
	// CodeInvalidEventDate indicates an invalid event date
	CodeInvalidEventDate = "INVALID_EVENT_DATE"
	// CodeShowUnpublishUnauthorized indicates user is not authorized to unpublish the show
	CodeShowUnpublishUnauthorized = "SHOW_UNPUBLISH_UNAUTHORIZED"
	// CodeShowMakePrivateUnauthorized indicates user is not authorized to make the show private
	CodeShowMakePrivateUnauthorized = "SHOW_MAKE_PRIVATE_UNAUTHORIZED"
	// CodeShowPublishUnauthorized indicates user is not authorized to publish the show
	CodeShowPublishUnauthorized = "SHOW_PUBLISH_UNAUTHORIZED"
)

// ShowError represents a show-related error with additional context.
type ShowError struct {
	Code      string // Error code (e.g., "SHOW_NOT_FOUND")
	Message   string // User-facing message
	Internal  error  // Original error (logged, not exposed to client)
	RequestID string // Request ID for correlation
	ShowID    uint   // Show ID if applicable
}

// Error implements the error interface.
func (e *ShowError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *ShowError) Unwrap() error {
	return e.Internal
}

// UserMessage returns the user-safe message (without internal details).
func (e *ShowError) UserMessage() string {
	return e.Message
}

// WithRequestID returns a copy of the error with the request ID set.
func (e *ShowError) WithRequestID(requestID string) *ShowError {
	return &ShowError{
		Code:      e.Code,
		Message:   e.Message,
		Internal:  e.Internal,
		RequestID: requestID,
		ShowID:    e.ShowID,
	}
}

// NewShowError creates a new ShowError with the given parameters.
func NewShowError(code, message string, internal error) *ShowError {
	return &ShowError{
		Code:     code,
		Message:  message,
		Internal: internal,
	}
}

// Predefined error constructors for common show errors

// ErrShowNotFound creates a show not found error.
func ErrShowNotFound(showID uint) *ShowError {
	return &ShowError{
		Code:    CodeShowNotFound,
		Message: "Show not found",
		ShowID:  showID,
	}
}

// ErrShowCreateFailed creates a show creation failed error.
func ErrShowCreateFailed(internal error) *ShowError {
	return NewShowError(CodeShowCreateFailed, "Failed to create show", internal)
}

// ErrShowUpdateFailed creates a show update failed error.
func ErrShowUpdateFailed(showID uint, internal error) *ShowError {
	return &ShowError{
		Code:     CodeShowUpdateFailed,
		Message:  "Failed to update show",
		Internal: internal,
		ShowID:   showID,
	}
}

// ErrShowDeleteFailed creates a show deletion failed error.
func ErrShowDeleteFailed(showID uint, internal error) *ShowError {
	return &ShowError{
		Code:     CodeShowDeleteFailed,
		Message:  "Failed to delete show",
		Internal: internal,
		ShowID:   showID,
	}
}

// ErrShowDeleteUnauthorized creates a show delete unauthorized error.
func ErrShowDeleteUnauthorized(showID uint) *ShowError {
	return &ShowError{
		Code:    CodeShowDeleteUnauthorized,
		Message: "You are not authorized to delete this show",
		ShowID:  showID,
	}
}

// ErrShowUpdateUnauthorized creates a show update unauthorized error.
func ErrShowUpdateUnauthorized(showID uint) *ShowError {
	return &ShowError{
		Code:    CodeShowUpdateUnauthorized,
		Message: "You are not authorized to update this show",
		ShowID:  showID,
	}
}

// ErrShowInvalidID creates an invalid show ID error.
func ErrShowInvalidID(idStr string) *ShowError {
	return NewShowError(CodeShowInvalidID, "Invalid show ID", fmt.Errorf("invalid ID: %s", idStr))
}

// ErrShowValidationFailed creates a validation failed error.
func ErrShowValidationFailed(message string) *ShowError {
	return NewShowError(CodeShowValidationFailed, message, nil)
}

// ErrVenueRequired creates a venue required error.
func ErrVenueRequired() *ShowError {
	return NewShowError(CodeVenueRequired, "At least one venue is required", nil)
}

// ErrArtistRequired creates an artist required error.
func ErrArtistRequired() *ShowError {
	return NewShowError(CodeArtistRequired, "At least one artist is required", nil)
}

// ErrInvalidEventDate creates an invalid event date error.
func ErrInvalidEventDate(message string) *ShowError {
	return NewShowError(CodeInvalidEventDate, message, nil)
}

// ErrShowUnpublishUnauthorized creates a show unpublish unauthorized error.
func ErrShowUnpublishUnauthorized(showID uint) *ShowError {
	return &ShowError{
		Code:    CodeShowUnpublishUnauthorized,
		Message: "You are not authorized to unpublish this show",
		ShowID:  showID,
	}
}

// ErrShowMakePrivateUnauthorized creates a show make-private unauthorized error.
func ErrShowMakePrivateUnauthorized(showID uint) *ShowError {
	return &ShowError{
		Code:    CodeShowMakePrivateUnauthorized,
		Message: "You are not authorized to make this show private",
		ShowID:  showID,
	}
}

// ErrShowPublishUnauthorized creates a show publish unauthorized error.
func ErrShowPublishUnauthorized(showID uint) *ShowError {
	return &ShowError{
		Code:    CodeShowPublishUnauthorized,
		Message: "You are not authorized to publish this show",
		ShowID:  showID,
	}
}

// GetShowErrorMessage returns a user-friendly message for an error code.
func GetShowErrorMessage(code string) string {
	switch code {
	case CodeShowNotFound:
		return "Show not found"
	case CodeShowCreateFailed:
		return "Failed to create show. Please try again."
	case CodeShowUpdateFailed:
		return "Failed to update show. Please try again."
	case CodeShowDeleteFailed:
		return "Failed to delete show. Please try again."
	case CodeShowDeleteUnauthorized:
		return "You are not authorized to delete this show."
	case CodeShowUpdateUnauthorized:
		return "You are not authorized to update this show."
	case CodeShowInvalidID:
		return "Invalid show ID"
	case CodeShowValidationFailed:
		return "Validation failed"
	case CodeVenueRequired:
		return "At least one venue is required"
	case CodeArtistRequired:
		return "At least one artist is required"
	case CodeInvalidEventDate:
		return "Invalid event date"
	case CodeShowUnpublishUnauthorized:
		return "You are not authorized to unpublish this show."
	case CodeShowMakePrivateUnauthorized:
		return "You are not authorized to make this show private."
	case CodeShowPublishUnauthorized:
		return "You are not authorized to publish this show."
	default:
		return "An error occurred"
	}
}
