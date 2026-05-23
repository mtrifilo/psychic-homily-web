package errors

import (
	"fmt"
)

// Comment-admin error codes.
//
// Produced by CommentService's admin paths (Hide / Restore / Approve /
// Reject / GetCommentEditHistory) and consumed by
// `engagement/comment_admin.go`. Comment-not-found stays on CommentError
// (CodeCommentNotFound → 404); admin-specific state-transition failures
// get their own codes here.
//
// Status mapping (see shared.MapCommentAdminError):
//   - AlreadyVisible → 409 (Restore on a visible comment)
//   - NotPending     → 409 (Approve / Reject on a non-pending comment)
//   - AccessRequired → 403 (edit history on non-admin requester)
const (
	// CodeCommentAdminAlreadyVisible indicates a Restore was attempted on a
	// comment that is already visible.
	CodeCommentAdminAlreadyVisible = "COMMENT_ADMIN_ALREADY_VISIBLE"
	// CodeCommentAdminNotPending indicates an Approve / Reject was
	// attempted on a comment whose visibility is not pending_review.
	CodeCommentAdminNotPending = "COMMENT_ADMIN_NOT_PENDING"
	// CodeCommentAdminAccessRequired indicates the requester is not an
	// admin (edit-history endpoint).
	CodeCommentAdminAccessRequired = "COMMENT_ADMIN_ACCESS_REQUIRED"
)

// CommentAdminError represents a comment-admin moderation error with
// additional context.
type CommentAdminError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *CommentAdminError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *CommentAdminError) Unwrap() error {
	return e.Internal
}

// ErrCommentAdminAlreadyVisible creates an already-visible error (Restore).
func ErrCommentAdminAlreadyVisible() *CommentAdminError {
	return &CommentAdminError{
		Code:    CodeCommentAdminAlreadyVisible,
		Message: "comment is already visible",
	}
}

// ErrCommentAdminNotPending creates a not-pending error (Approve / Reject).
func ErrCommentAdminNotPending() *CommentAdminError {
	return &CommentAdminError{
		Code:    CodeCommentAdminNotPending,
		Message: "comment is not pending review",
	}
}

// ErrCommentAdminAccessRequired creates an admin-access-required error
// (edit-history endpoint).
func ErrCommentAdminAccessRequired() *CommentAdminError {
	return &CommentAdminError{
		Code:    CodeCommentAdminAccessRequired,
		Message: "admin access required",
	}
}
