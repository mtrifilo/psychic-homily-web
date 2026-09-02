package errors

import (
	"fmt"
)

// Artist error codes
const (
	CodeArtistNotFound = "ARTIST_NOT_FOUND"
	CodeArtistHasShows = "ARTIST_HAS_SHOWS"
	// CodeArtistHasOtherUsersEngagement indicates a NON-ADMIN tried to delete an
	// artist that other people have followed, tagged, saved or subscribed to.
	// Deleting it would destroy those rows, so only an admin may.
	CodeArtistHasOtherUsersEngagement = "ARTIST_HAS_OTHER_USERS_ENGAGEMENT"
	// CodeArtistExists indicates an artist with the same name already exists.
	CodeArtistExists = "ARTIST_EXISTS"
	// CodeArtistAliasExists indicates the alias collides with an existing alias
	// or artist name.
	CodeArtistAliasExists = "ARTIST_ALIAS_EXISTS"
	// CodeArtistAliasNotFound indicates the alias to remove does not exist.
	CodeArtistAliasNotFound = "ARTIST_ALIAS_NOT_FOUND"
	// CodeArtistMergeSelf indicates an attempt to merge an artist into itself.
	CodeArtistMergeSelf = "ARTIST_MERGE_SELF"

	// CodeArtistInvalidField indicates a field value the service refused. It
	// exists so a service-layer validation refusal reaches the caller as a 422
	// carrying its own message, rather than as the generic 500 every
	// non-ArtistError maps to — which would swallow the one sentence that tells
	// the submitter how to fix the value.
	CodeArtistInvalidField = "ARTIST_INVALID_FIELD"
	// CodeArtistRelationshipNotFound indicates no connection (stored or
	// query-time) exists between the artist pair.
	CodeArtistRelationshipNotFound = "ARTIST_RELATIONSHIP_NOT_FOUND"
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

// ErrArtistHasOtherUsersEngagement creates the refusal a non-admin gets for an
// artist that is not inert.
//
// The message names the SHAPE of the engagement rather than the tables that held
// it: the caller's next move is the same either way (ask an admin), and the
// table list goes to the log where an operator can act on it.
//
// It stays general on purpose. The gate refuses on seven tables, and the ones a
// short list would leave out are real (a comment subscription, a read cursor on
// the artist's thread, a tag vote), so naming three of them would tell some
// callers a reason that is not their reason.
func ErrArtistHasOtherUsersEngagement(artistID uint) *ArtistError {
	return &ArtistError{
		Code: CodeArtistHasOtherUsersEngagement,
		Message: "Cannot delete artist: other people have engaged with it, by saving, " +
			"tagging or following its comments. Ask an admin to delete it.",
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

// ErrArtistRelationshipNotFound creates a no-relationship-between-pair error.
func ErrArtistRelationshipNotFound() *ArtistError {
	return &ArtistError{
		Code:    CodeArtistRelationshipNotFound,
		Message: "No relationship found between these artists",
	}
}

// ErrArtistMergeSelf creates a merge-into-self error.
// ErrArtistInvalidField wraps a validation failure raised inside the service.
// The message is the validator's own, so the wording a curator sees is the same
// whichever layer refused the value.
func ErrArtistInvalidField(err error) *ArtistError {
	return &ArtistError{
		Code:    CodeArtistInvalidField,
		Message: err.Error(),
	}
}

func ErrArtistMergeSelf() *ArtistError {
	return &ArtistError{
		Code:    CodeArtistMergeSelf,
		Message: "cannot merge an artist with itself",
	}
}
