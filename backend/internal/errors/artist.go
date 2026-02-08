package errors

import (
	"fmt"
)

// Artist error codes
const (
	CodeArtistNotFound = "ARTIST_NOT_FOUND"
	CodeArtistHasShows = "ARTIST_HAS_SHOWS"
)

// ArtistError represents an artist-related error with additional context.
type ArtistError struct {
	Code      string
	Message   string
	Internal  error
	RequestID string
	ArtistID  uint
}

// Error implements the error interface.
func (e *ArtistError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *ArtistError) Unwrap() error {
	return e.Internal
}

// ErrArtistNotFound creates an artist not found error.
func ErrArtistNotFound(artistID uint) *ArtistError {
	return &ArtistError{
		Code:     CodeArtistNotFound,
		Message:  "Artist not found",
		ArtistID: artistID,
	}
}

// ErrArtistHasShows creates an artist-has-shows error.
func ErrArtistHasShows(artistID uint, count int64) *ArtistError {
	return &ArtistError{
		Code:     CodeArtistHasShows,
		Message:  fmt.Sprintf("Cannot delete artist: associated with %d shows", count),
		ArtistID: artistID,
	}
}
