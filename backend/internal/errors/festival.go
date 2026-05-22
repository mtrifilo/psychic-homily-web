package errors

import (
	"fmt"
)

// Festival error codes
const (
	CodeFestivalNotFound = "FESTIVAL_NOT_FOUND"
	CodeFestivalExists   = "FESTIVAL_EXISTS"
	// CodeFestivalArtistNotFound indicates the artist being added to a festival
	// lineup does not exist.
	CodeFestivalArtistNotFound = "FESTIVAL_ARTIST_NOT_FOUND"
	// CodeFestivalArtistNotInLineup indicates the artist is not part of the
	// festival lineup (update/remove target missing).
	CodeFestivalArtistNotInLineup = "FESTIVAL_ARTIST_NOT_IN_LINEUP"
	// CodeFestivalVenueNotFound indicates the venue being added to a festival
	// does not exist.
	CodeFestivalVenueNotFound = "FESTIVAL_VENUE_NOT_FOUND"
	// CodeFestivalVenueNotInFestival indicates the venue is not associated with
	// the festival (remove target missing).
	CodeFestivalVenueNotInFestival = "FESTIVAL_VENUE_NOT_IN_FESTIVAL"
)

// FestivalError represents a festival-related error with additional context.
type FestivalError struct {
	Code       string
	Message    string
	Internal   error
	RequestID  string
	FestivalID uint
}

// Error implements the error interface.
func (e *FestivalError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *FestivalError) Unwrap() error {
	return e.Internal
}

// ErrFestivalNotFound creates a festival not found error.
func ErrFestivalNotFound(festivalID uint) *FestivalError {
	return &FestivalError{
		Code:       CodeFestivalNotFound,
		Message:    "Festival not found",
		FestivalID: festivalID,
	}
}

// ErrFestivalExists creates a festival-already-exists error.
func ErrFestivalExists(name string) *FestivalError {
	return &FestivalError{
		Code:    CodeFestivalExists,
		Message: fmt.Sprintf("Festival with name '%s' already exists", name),
	}
}

// ErrFestivalArtistNotFound creates a festival-artist-not-found error.
func ErrFestivalArtistNotFound() *FestivalError {
	return &FestivalError{
		Code:    CodeFestivalArtistNotFound,
		Message: "artist not found",
	}
}

// ErrFestivalArtistNotInLineup creates an artist-not-in-lineup error.
func ErrFestivalArtistNotInLineup() *FestivalError {
	return &FestivalError{
		Code:    CodeFestivalArtistNotInLineup,
		Message: "artist not found in festival lineup",
	}
}

// ErrFestivalVenueNotFound creates a festival-venue-not-found error.
func ErrFestivalVenueNotFound() *FestivalError {
	return &FestivalError{
		Code:    CodeFestivalVenueNotFound,
		Message: "venue not found",
	}
}

// ErrFestivalVenueNotInFestival creates a venue-not-in-festival error.
func ErrFestivalVenueNotInFestival() *FestivalError {
	return &FestivalError{
		Code:    CodeFestivalVenueNotInFestival,
		Message: "venue not found in festival",
	}
}
