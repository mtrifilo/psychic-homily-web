package errors

import (
	"fmt"
)

// Venue error codes
const (
	CodeVenueNotFound         = "VENUE_NOT_FOUND"
	CodeVenueHasShows         = "VENUE_HAS_SHOWS"
	CodeVenuePendingEditExists = "VENUE_PENDING_EDIT_EXISTS"
)

// VenueError represents a venue-related error with additional context.
type VenueError struct {
	Code      string
	Message   string
	Internal  error
	RequestID string
	VenueID   uint
}

// Error implements the error interface.
func (e *VenueError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *VenueError) Unwrap() error {
	return e.Internal
}

// ErrVenueNotFound creates a venue not found error.
func ErrVenueNotFound(venueID uint) *VenueError {
	return &VenueError{
		Code:    CodeVenueNotFound,
		Message: "Venue not found",
		VenueID: venueID,
	}
}

// ErrVenueHasShows creates a venue-has-shows error.
func ErrVenueHasShows(venueID uint, count int64) *VenueError {
	return &VenueError{
		Code:    CodeVenueHasShows,
		Message: fmt.Sprintf("Cannot delete venue: associated with %d shows", count),
		VenueID: venueID,
	}
}

// ErrVenuePendingEditExists creates a pending-edit-exists error.
func ErrVenuePendingEditExists(venueID uint) *VenueError {
	return &VenueError{
		Code:    CodeVenuePendingEditExists,
		Message: "You already have a pending edit for this venue",
		VenueID: venueID,
	}
}
