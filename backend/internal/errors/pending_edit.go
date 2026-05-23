package errors

import (
	"fmt"
)

// Pending-edit error codes.
//
// Creating a pending edit can fail because the target entity does not exist
// (not found), the entity type is not editable, the request is malformed (no
// changes / missing or oversize summary — semantic validation), the submitter
// already has a pending edit for this entity (conflict, surfaced from the DB
// unique constraint), or a database fault.
//
// Approve/reject/cancel can fail because the edit row does not exist (not
// found), it has already been reviewed (conflict), the caller is not the
// submitter (forbidden — cancel only), or the entity was deleted between
// submission and approval (unprocessable — the edit can no longer be applied).
//
// Note: the disallowed-fields auto-rejection on the approve path is its own
// sentinel (adminm.ErrPendingEditDisallowedFields, mapped to 400 by the
// handler via errors.Is) and is intentionally NOT modelled here.
const (
	// CodePendingEditEntityNotFound indicates the target entity does not exist
	// at submission time (create path → 404).
	CodePendingEditEntityNotFound = "PENDING_EDIT_ENTITY_NOT_FOUND"
	// CodePendingEditEntityGone indicates the entity was deleted between
	// submission and approval (approve path → 422, the edit is unprocessable).
	CodePendingEditEntityGone = "PENDING_EDIT_ENTITY_GONE"
	// CodePendingEditNotFound indicates the pending-edit row does not exist.
	CodePendingEditNotFound = "PENDING_EDIT_NOT_FOUND"
	// CodePendingEditNotPending indicates the edit has already been reviewed.
	CodePendingEditNotPending = "PENDING_EDIT_NOT_PENDING"
	// CodePendingEditNotSubmitter indicates a non-submitter tried to cancel.
	CodePendingEditNotSubmitter = "PENDING_EDIT_NOT_SUBMITTER"
	// CodePendingEditDuplicate indicates the submitter already has a pending
	// edit for this entity (DB unique-constraint violation).
	CodePendingEditDuplicate = "PENDING_EDIT_DUPLICATE"
	// CodePendingEditInvalidEntityType indicates an unsupported entity type.
	CodePendingEditInvalidEntityType = "PENDING_EDIT_INVALID_ENTITY_TYPE"
	// CodePendingEditInvalidRequest indicates a malformed request (no changes,
	// missing or oversize summary/reason — semantic validation).
	CodePendingEditInvalidRequest = "PENDING_EDIT_INVALID_REQUEST"
	// CodePendingEditInternal indicates a database or infrastructure failure.
	CodePendingEditInternal = "PENDING_EDIT_INTERNAL"
)

// PendingEditError represents a pending-edit error with additional context.
type PendingEditError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *PendingEditError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *PendingEditError) Unwrap() error {
	return e.Internal
}

// ErrPendingEditEntityNotFound creates an entity-not-found error for the
// create path.
func ErrPendingEditEntityNotFound(entityType string, entityID uint) *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditEntityNotFound,
		Message: fmt.Sprintf("entity not found: %s %d", entityType, entityID),
	}
}

// ErrPendingEditEntityGone creates an entity-deleted-before-approval error for
// the approve path.
func ErrPendingEditEntityGone(entityType string, entityID uint) *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditEntityGone,
		Message: fmt.Sprintf("entity not found: %s %d", entityType, entityID),
	}
}

// ErrPendingEditNotFound creates a pending-edit-not-found error.
func ErrPendingEditNotFound() *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditNotFound,
		Message: "pending edit not found",
	}
}

// ErrPendingEditNotPending creates an already-reviewed error.
func ErrPendingEditNotPending(status string) *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditNotPending,
		Message: fmt.Sprintf("edit is not pending (status: %s)", status),
	}
}

// ErrPendingEditNotSubmitter creates a not-the-submitter error.
func ErrPendingEditNotSubmitter() *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditNotSubmitter,
		Message: "only the submitter can cancel their own edit",
	}
}

// ErrPendingEditDuplicate creates a duplicate-pending-edit error. internal
// carries the underlying unique-constraint violation for logging.
func ErrPendingEditDuplicate(internal error) *PendingEditError {
	return &PendingEditError{
		Code:     CodePendingEditDuplicate,
		Message:  "you already have a pending edit for this entity",
		Internal: internal,
	}
}

// ErrPendingEditInvalidEntityType creates an invalid-entity-type error.
func ErrPendingEditInvalidEntityType(entityType string) *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditInvalidEntityType,
		Message: fmt.Sprintf("invalid entity type: %s", entityType),
	}
}

// ErrPendingEditInvalidRequest creates a malformed-request error. The message
// is user-facing.
func ErrPendingEditInvalidRequest(message string) *PendingEditError {
	return &PendingEditError{
		Code:    CodePendingEditInvalidRequest,
		Message: message,
	}
}

// ErrPendingEditInternal wraps a database or infrastructure failure.
func ErrPendingEditInternal(internal error) *PendingEditError {
	return &PendingEditError{
		Code:     CodePendingEditInternal,
		Message:  "failed to process pending edit",
		Internal: internal,
	}
}
