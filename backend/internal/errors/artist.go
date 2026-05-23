package errors

import (
	"fmt"
)

// Artist error codes
const (
	CodeArtistNotFound = "ARTIST_NOT_FOUND"
	CodeArtistHasShows = "ARTIST_HAS_SHOWS"
	// CodeArtistExists indicates an artist with the same name already exists.
	CodeArtistExists = "ARTIST_EXISTS"
	// CodeArtistAliasExists indicates the alias collides with an existing alias
	// or artist name.
	CodeArtistAliasExists = "ARTIST_ALIAS_EXISTS"
	// CodeArtistAliasNotFound indicates the alias to remove does not exist.
	CodeArtistAliasNotFound = "ARTIST_ALIAS_NOT_FOUND"
	// CodeArtistMergeSelf indicates an attempt to merge an artist into itself.
	CodeArtistMergeSelf = "ARTIST_MERGE_SELF"
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

// ErrArtistExists creates an artist-already-exists error.
func ErrArtistExists(name string) *ArtistError {
	return &ArtistError{
		Code:    CodeArtistExists,
		Message: fmt.Sprintf("artist with name '%s' already exists", name),
	}
}

// ErrArtistAliasExists creates an alias-conflict error. The message describes
// whether the alias collided with another alias or an existing artist name.
func ErrArtistAliasExists(message string) *ArtistError {
	return &ArtistError{
		Code:    CodeArtistAliasExists,
		Message: message,
	}
}

// ErrArtistAliasNotFound creates an alias-not-found error.
func ErrArtistAliasNotFound() *ArtistError {
	return &ArtistError{
		Code:    CodeArtistAliasNotFound,
		Message: "alias not found",
	}
}

// ErrArtistMergeSelf creates a merge-into-self error.
func ErrArtistMergeSelf() *ArtistError {
	return &ArtistError{
		Code:    CodeArtistMergeSelf,
		Message: "cannot merge an artist with itself",
	}
}
