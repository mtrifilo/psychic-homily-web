package errors

import (
	"fmt"
)

// Field-note error codes.
//
// FieldNote-specific failures beyond the shared CommentError surface. Body
// validation, sound_quality / crowd_energy range checks, and rate limits
// all stay on CommentError. These cover the show / past-show / artist-on-
// bill gates unique to field-note creation.
//
// Status mapping (see shared.MapFieldNoteError):
//   - ShowNotFound      → 404
//   - ShowFuture        → 400 (semantic 400 — the show exists, the action
//     is invalid for its date)
//   - ArtistNotOnBill   → 400 (semantic 400 — the artist exists, just not
//     on this show)
const (
	// CodeFieldNoteShowNotFound indicates the target show does not exist.
	CodeFieldNoteShowNotFound = "FIELD_NOTE_SHOW_NOT_FOUND"
	// CodeFieldNoteShowFuture indicates the show is in the future
	// (field notes can only be attached to past shows).
	CodeFieldNoteShowFuture = "FIELD_NOTE_SHOW_FUTURE"
	// CodeFieldNoteArtistNotOnBill indicates the supplied show_artist_id
	// is not part of this show's bill.
	CodeFieldNoteArtistNotOnBill = "FIELD_NOTE_ARTIST_NOT_ON_BILL"
)

// FieldNoteError represents a field-note-specific error.
type FieldNoteError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *FieldNoteError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *FieldNoteError) Unwrap() error {
	return e.Internal
}

// ErrFieldNoteShowNotFound creates a show-not-found error.
func ErrFieldNoteShowNotFound() *FieldNoteError {
	return &FieldNoteError{
		Code:    CodeFieldNoteShowNotFound,
		Message: "show not found",
	}
}

// ErrFieldNoteShowFuture creates a future-show rejection.
func ErrFieldNoteShowFuture() *FieldNoteError {
	return &FieldNoteError{
		Code:    CodeFieldNoteShowFuture,
		Message: "field notes can only be added to past shows",
	}
}

// ErrFieldNoteArtistNotOnBill creates an artist-not-on-show-bill rejection.
func ErrFieldNoteArtistNotOnBill() *FieldNoteError {
	return &FieldNoteError{
		Code:    CodeFieldNoteArtistNotOnBill,
		Message: "artist is not on this show's bill",
	}
}

// Note: field-note creation shares CommentError for body / sound_quality /
// crowd_energy / rate-limit / user-not-found codes. Only the show /
// past-show / artist-on-bill gates are FieldNoteError.
