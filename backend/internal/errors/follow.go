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
//
// The alert-subscription sub-resource hanging off a follow does have a
// not-found path: reading or configuring the alerts of a follow that does not
// exist is a genuine 404, not a silent no-op.
const (
	// CodeFollowInvalidEntityType indicates the entity type is not followable
	// (must be artist, venue, label, or festival).
	CodeFollowInvalidEntityType = "FOLLOW_INVALID_ENTITY_TYPE"
	// CodeFollowInternal indicates a database or infrastructure failure.
	CodeFollowInternal = "FOLLOW_INTERNAL"
	// CodeFollowNotFound indicates the follow whose sub-resource was addressed
	// does not exist. Never returned by follow/unfollow themselves.
	CodeFollowNotFound = "FOLLOW_NOT_FOUND"
	// CodeFollowInvalidAlertSettings indicates an alert-subscription update the
	// follow's entity type cannot carry (an axis it does not have, or an
	// unrecognized value). Distinct from CodeFollowInvalidEntityType: the
	// entity type is followable, the requested setting is not valid for it.
	CodeFollowInvalidAlertSettings = "FOLLOW_INVALID_ALERT_SETTINGS"
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

// ErrFollowNotFound creates a missing-follow error for a follow sub-resource.
func ErrFollowNotFound(entityType string, entityID uint) *FollowError {
	return &FollowError{
		Code:    CodeFollowNotFound,
		Message: fmt.Sprintf("not following %s %d", entityType, entityID),
	}
}

// ErrFollowInvalidAlertSettings creates an invalid-alert-settings error. The
// message is the caller's verbatim, since it is user-visible detail on a 422.
func ErrFollowInvalidAlertSettings(message string) *FollowError {
	return &FollowError{
		Code:    CodeFollowInvalidAlertSettings,
		Message: message,
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
