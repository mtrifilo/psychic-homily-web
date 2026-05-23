package errors

import (
	"fmt"
)

// Comment error codes.
//
// CommentService produces these from the comment-CRUD, threading,
// validation, rate-limit, and edit-permission paths consumed by the
// engagement comment / reply / field-note / subscription handlers.
//
// Status mapping (see shared.MapCommentError):
//   - NotFound / ParentNotFound / EntityNotFound      → 404
//   - Forbidden (author-only, edit-window expired,
//     replies-disabled, followers-only-denied)        → 403
//   - InvalidEntityType / InvalidReplyPermission /
//     BodyRequired / BodyTooLong / MaxDepthExceeded /
//     ParentMismatch / NotThreadRoot / FieldValidation → 400
//   - RateLimitedEntity                                → 429 with Retry-After: 60
//   - RateLimitedHourly                                → 429 with Retry-After: 3600
//   - Internal                                         → 500
const (
	// CodeCommentNotFound indicates the target comment does not exist.
	CodeCommentNotFound = "COMMENT_NOT_FOUND"
	// CodeCommentParentNotFound indicates the parent comment does not exist.
	CodeCommentParentNotFound = "COMMENT_PARENT_NOT_FOUND"
	// CodeCommentEntityNotFound indicates the entity being commented on
	// does not exist (e.g. show, artist, venue).
	CodeCommentEntityNotFound = "COMMENT_ENTITY_NOT_FOUND"
	// CodeCommentNotThreadRoot indicates GetThread was called on a non-root
	// comment.
	CodeCommentNotThreadRoot = "COMMENT_NOT_THREAD_ROOT"
	// CodeCommentForbidden indicates the caller is not allowed to perform
	// this action (not the author, edit window expired, replies disabled,
	// followers-only gate rejected).
	CodeCommentForbidden = "COMMENT_FORBIDDEN"
	// CodeCommentInvalidEntityType indicates the entity type is not
	// supported for commenting.
	CodeCommentInvalidEntityType = "COMMENT_INVALID_ENTITY_TYPE"
	// CodeCommentInvalidReplyPermission indicates the reply_permission
	// value is not one of {anyone, followers, author_only}.
	CodeCommentInvalidReplyPermission = "COMMENT_INVALID_REPLY_PERMISSION"
	// CodeCommentBodyRequired indicates the body was empty or whitespace.
	CodeCommentBodyRequired = "COMMENT_BODY_REQUIRED"
	// CodeCommentBodyTooLong indicates the body exceeds the max length.
	CodeCommentBodyTooLong = "COMMENT_BODY_TOO_LONG"
	// CodeCommentMaxDepthExceeded indicates the reply depth would exceed
	// the configured maximum.
	CodeCommentMaxDepthExceeded = "COMMENT_MAX_DEPTH_EXCEEDED"
	// CodeCommentParentMismatch indicates the parent belongs to a
	// different entity than the requested reply target.
	CodeCommentParentMismatch = "COMMENT_PARENT_MISMATCH"
	// CodeCommentFieldValidation indicates a 1-5 rating field
	// (sound_quality / crowd_energy) was out of range.
	CodeCommentFieldValidation = "COMMENT_FIELD_VALIDATION"
	// CodeCommentRateLimitedEntity indicates the 60s per-entity cooldown
	// applies. Maps to 429 with Retry-After: 60.
	CodeCommentRateLimitedEntity = "COMMENT_RATE_LIMITED_ENTITY"
	// CodeCommentRateLimitedHourly indicates the tier-based hourly cap
	// applies. Maps to 429 with Retry-After: 3600.
	CodeCommentRateLimitedHourly = "COMMENT_RATE_LIMITED_HOURLY"
	// CodeCommentInternal indicates a database or infrastructure failure.
	CodeCommentInternal = "COMMENT_INTERNAL"
)

// CommentError represents a comment-related error with additional context.
type CommentError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface. The Message is included verbatim
// so service-layer substring assertions on the human copy continue to
// match.
func (e *CommentError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *CommentError) Unwrap() error {
	return e.Internal
}

// ErrCommentNotFound creates a comment-not-found error.
func ErrCommentNotFound() *CommentError {
	return &CommentError{
		Code:    CodeCommentNotFound,
		Message: "comment not found",
	}
}

// ErrCommentParentNotFound creates a parent-comment-not-found error.
func ErrCommentParentNotFound() *CommentError {
	return &CommentError{
		Code:    CodeCommentParentNotFound,
		Message: "parent comment not found",
	}
}

// ErrCommentEntityNotFound creates an entity-not-found error in the form
// `<type> with ID <id> not found`.
func ErrCommentEntityNotFound(entityType string, entityID uint) *CommentError {
	return &CommentError{
		Code:    CodeCommentEntityNotFound,
		Message: fmt.Sprintf("%s with ID %d not found", entityType, entityID),
	}
}

// ErrCommentUserNotFound creates an authenticated-user-not-found error.
// Same 404 semantics as ErrCommentEntityNotFound but with the bare
// "user not found" copy.
func ErrCommentUserNotFound() *CommentError {
	return &CommentError{
		Code:    CodeCommentEntityNotFound,
		Message: "user not found",
	}
}

// ErrCommentNotThreadRoot creates a not-thread-root error.
func ErrCommentNotThreadRoot() *CommentError {
	return &CommentError{
		Code:    CodeCommentNotThreadRoot,
		Message: "comment is not a thread root",
	}
}

// ErrCommentThreadRootNotFound creates an error for GetThread when the
// root itself does not exist. Uses CodeCommentNotFound (same 404) with a
// distinct message identifying the GetThread path.
func ErrCommentThreadRootNotFound() *CommentError {
	return &CommentError{
		Code:    CodeCommentNotFound,
		Message: "thread root comment not found",
	}
}

// ErrCommentForbidden creates a generic forbidden error. The caller-
// supplied message names the specific gate (author-only, edit window
// expired, replies disabled, followers-only denied).
func ErrCommentForbidden(message string) *CommentError {
	return &CommentError{
		Code:    CodeCommentForbidden,
		Message: message,
	}
}

// ErrCommentInvalidEntityType creates an invalid-entity-type error.
func ErrCommentInvalidEntityType(entityType string) *CommentError {
	return &CommentError{
		Code:    CodeCommentInvalidEntityType,
		Message: fmt.Sprintf("unsupported entity type: %s", entityType),
	}
}

// ErrCommentInvalidReplyPermission creates an invalid-reply-permission error.
func ErrCommentInvalidReplyPermission(value string) *CommentError {
	return &CommentError{
		Code:    CodeCommentInvalidReplyPermission,
		Message: fmt.Sprintf("invalid reply_permission: %s", value),
	}
}

// ErrCommentUnsupportedReplyPermission creates an error for a
// reply_permission value stored on a parent comment that is no longer
// recognized. Distinct message from ErrCommentInvalidReplyPermission to
// distinguish "user supplied a bad value" from "stored value is bad".
func ErrCommentUnsupportedReplyPermission(value string) *CommentError {
	return &CommentError{
		Code:    CodeCommentInvalidReplyPermission,
		Message: fmt.Sprintf("unsupported reply_permission on parent: %s", value),
	}
}

// ErrCommentBodyRequired creates an empty-body error. The caller supplies
// the message so field notes can use "field note body is required" while
// comments use "comment body is required".
func ErrCommentBodyRequired(message string) *CommentError {
	return &CommentError{
		Code:    CodeCommentBodyRequired,
		Message: message,
	}
}

// ErrCommentBodyTooLong creates a body-exceeds-max-length error. The
// caller supplies the message so field notes can use their own copy.
func ErrCommentBodyTooLong(message string) *CommentError {
	return &CommentError{
		Code:    CodeCommentBodyTooLong,
		Message: message,
	}
}

// ErrCommentMaxDepthExceeded creates a depth-exceeded error.
func ErrCommentMaxDepthExceeded(maxDepth int) *CommentError {
	return &CommentError{
		Code:    CodeCommentMaxDepthExceeded,
		Message: fmt.Sprintf("maximum reply depth of %d exceeded", maxDepth),
	}
}

// ErrCommentParentMismatch creates a parent-entity-mismatch error.
func ErrCommentParentMismatch() *CommentError {
	return &CommentError{
		Code:    CodeCommentParentMismatch,
		Message: "parent comment belongs to a different entity",
	}
}

// ErrCommentFieldValidation creates a structured-data field validation
// error (sound_quality / crowd_energy out of range). The caller-supplied
// message names the specific field.
func ErrCommentFieldValidation(message string) *CommentError {
	return &CommentError{
		Code:    CodeCommentFieldValidation,
		Message: message,
	}
}

// ErrCommentRateLimitedEntity creates a per-entity-cooldown rate-limit error.
// Maps to 429 with Retry-After: 60.
func ErrCommentRateLimitedEntity() *CommentError {
	return &CommentError{
		Code:    CodeCommentRateLimitedEntity,
		Message: "Please wait 60 seconds between comments on the same entity",
	}
}

// ErrCommentRateLimitedHourly creates a tier-based hourly-cap rate-limit
// error. Maps to 429 with Retry-After: 3600.
func ErrCommentRateLimitedHourly(limit int, tier string) *CommentError {
	return &CommentError{
		Code:    CodeCommentRateLimitedHourly,
		Message: fmt.Sprintf("you've reached your hourly comment limit (%d/hour for %s users)", limit, tier),
	}
}

// ErrCommentInternal wraps a database or infrastructure failure.
func ErrCommentInternal(message string, internal error) *CommentError {
	return &CommentError{
		Code:     CodeCommentInternal,
		Message:  message,
		Internal: internal,
	}
}
