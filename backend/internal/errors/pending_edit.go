package errors

import (
	"fmt"
	"strings"
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
	// CodePendingEditStaleValue indicates a recorded previous value disagrees
	// with the entity, so the edit was composed against a value the entity no
	// longer holds. Raised on both apply-relevant paths: the submitter's claim
	// at submit time, and the stored old_value at approve time.
	CodePendingEditStaleValue = "PENDING_EDIT_STALE_VALUE"
)

// StaleFieldValue names a field whose recorded previous value no longer
// describes the entity, paired with the value the entity holds now as its
// READER observes it.
//
// Current is the same derivation a successful submission stores and serves back
// in the pending-edit response, so returning it discloses nothing a caller
// could not already obtain by submitting a matching claim: a column the entity
// withholds derives as its withheld view, not as the column.
type StaleFieldValue struct {
	Field   string `json:"field"`
	Current any    `json:"current_value"`
}

// PendingEditError represents a pending-edit error with additional context.
//
// StaleFields is populated only on CodePendingEditStaleValue and carries the
// entity's current value for each field the refusal names, so a client can
// re-seed the form it composed the edit in rather than parse the message.
type PendingEditError struct {
	Code        string
	Message     string
	Internal    error
	StaleFields []StaleFieldValue
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

// ErrPendingEditStaleValue creates a stale-value conflict error for the SUBMIT
// path, naming the fields whose claimed previous value disagrees with the
// entity.
//
// The message says only that the field moved, never what it moved to. Field
// NAMES are safe there because a submission can only name a field its entity's
// edit allowlist exposes. The value travels in StaleFields instead, where it is
// the derived reader's view rather than the column: deriveOldValues holds that
// half of the rule, deriving the withheld view for exactly the fields the entity
// withholds, so the pair carries no bit about a column its reader may not see.
func ErrPendingEditStaleValue(stale []StaleFieldValue) *PendingEditError {
	subject := "This field has"
	if len(stale) > 1 {
		subject = "These fields have"
	}
	return &PendingEditError{
		Code: CodePendingEditStaleValue,
		Message: fmt.Sprintf(
			"%s changed since you loaded the form: %s. Reload to see the current values, then submit your edit again.",
			subject, strings.Join(staleFieldNames(stale), ", ")),
		StaleFields: stale,
	}
}

// ErrPendingEditStaleValueAtApprove creates a stale-value conflict error for the
// APPROVE path, naming the fields whose STORED previous value no longer
// describes the entity.
//
// Same code, so the same 409, and the same disclosure rule as the submit
// constructor above. The copy differs because the reader differs: the moderator
// cannot make this edit applicable, since the stored previous value is never
// re-stamped, so it points at the only two moves that resolve the row.
func ErrPendingEditStaleValueAtApprove(stale []StaleFieldValue) *PendingEditError {
	subject, values := "This field", "a different value"
	if len(stale) > 1 {
		subject, values = "These fields", "different values"
	}
	return &PendingEditError{
		Code: CodePendingEditStaleValue,
		Message: fmt.Sprintf(
			"%s changed since this edit was submitted: %s. The edit cannot be applied over %s. Reject it and ask the contributor to resubmit.",
			subject, strings.Join(staleFieldNames(stale), ", "), values),
		StaleFields: stale,
	}
}

// staleFieldNames lists the field names of stale, in the order given. Both
// constructors join them into a message, and both are handed an
// already-sorted list so the message is stable across runs.
func staleFieldNames(stale []StaleFieldValue) []string {
	names := make([]string, len(stale))
	for i, f := range stale {
		names[i] = f.Field
	}
	return names
}

// ErrPendingEditInternal wraps a database or infrastructure failure.
func ErrPendingEditInternal(internal error) *PendingEditError {
	return &PendingEditError{
		Code:     CodePendingEditInternal,
		Message:  "failed to process pending edit",
		Internal: internal,
	}
}
