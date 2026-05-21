package errors

import (
	"fmt"
)

// Follow error codes.
//
// Follow/unfollow are idempotent toggles backed by user_bookmarks: re-following
// an already-followed entity, or unfollowing one that isn't followed, is a
// no-op success — there is no "already following" conflict and no not-found
// path (the bookmark table does not verify the target entity exists). The only
// genuine failures are an invalid entity type (semantic validation) and a
// database/infrastructure fault (internal).
const (
	// CodeFollowInvalidEntityType indicates the entity type is not followable
	// (must be artist, venue, label, or festival).
	CodeFollowInvalidEntityType = "FOLLOW_INVALID_ENTITY_TYPE"
	// CodeFollowInternal indicates a database or infrastructure failure.
	CodeFollowInternal = "FOLLOW_INTERNAL"
)

// FollowError represents a follow-related error with additional context.
type FollowError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *FollowError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *FollowError) Unwrap() error {
	return e.Internal
}

// ErrFollowInvalidEntityType creates an invalid-entity-type error.
func ErrFollowInvalidEntityType(entityType string) *FollowError {
	return &FollowError{
		Code:    CodeFollowInvalidEntityType,
		Message: fmt.Sprintf("invalid entity type for follow: %s", entityType),
	}
}

// ErrFollowInternal wraps a database or infrastructure failure.
func ErrFollowInternal(internal error) *FollowError {
	return &FollowError{
		Code:     CodeFollowInternal,
		Message:  "failed to update follow",
		Internal: internal,
	}
}
