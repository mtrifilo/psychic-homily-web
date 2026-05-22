package errors

import (
	"fmt"
)

// Entity-report error codes.
//
// Reporting an entity can fail because the target entity does not exist
// (not found), the reporter already has a pending report for it (conflict),
// the entity type or report type is not valid for the taxonomy (semantic
// validation), or a database fault. Admin resolve/dismiss can fail because
// the report row does not exist (not found) or has already been reviewed
// (conflict).
const (
	// CodeEntityReportEntityNotFound indicates the reported entity does not exist.
	CodeEntityReportEntityNotFound = "ENTITY_REPORT_ENTITY_NOT_FOUND"
	// CodeEntityReportNotFound indicates the report row itself does not exist.
	CodeEntityReportNotFound = "ENTITY_REPORT_NOT_FOUND"
	// CodeEntityReportDuplicatePending indicates the reporter already has a
	// pending report for this entity.
	CodeEntityReportDuplicatePending = "ENTITY_REPORT_DUPLICATE_PENDING"
	// CodeEntityReportAlreadyReviewed indicates the report has already been
	// resolved or dismissed.
	CodeEntityReportAlreadyReviewed = "ENTITY_REPORT_ALREADY_REVIEWED"
	// CodeEntityReportInvalidEntityType indicates an unsupported entity type.
	CodeEntityReportInvalidEntityType = "ENTITY_REPORT_INVALID_ENTITY_TYPE"
	// CodeEntityReportInvalidReportType indicates a report type not valid for
	// the given entity type.
	CodeEntityReportInvalidReportType = "ENTITY_REPORT_INVALID_REPORT_TYPE"
	// CodeEntityReportInternal indicates a database or infrastructure failure.
	CodeEntityReportInternal = "ENTITY_REPORT_INTERNAL"
)

// EntityReportError represents an entity-report error with additional context.
type EntityReportError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *EntityReportError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *EntityReportError) Unwrap() error {
	return e.Internal
}

// ErrEntityReportEntityNotFound creates a reported-entity-not-found error.
func ErrEntityReportEntityNotFound(entityType string, entityID uint) *EntityReportError {
	return &EntityReportError{
		Code:    CodeEntityReportEntityNotFound,
		Message: fmt.Sprintf("entity not found: %s %d", entityType, entityID),
	}
}

// ErrEntityReportNotFound creates a report-not-found error.
func ErrEntityReportNotFound() *EntityReportError {
	return &EntityReportError{
		Code:    CodeEntityReportNotFound,
		Message: "report not found",
	}
}

// ErrEntityReportDuplicatePending creates an already-pending-report error.
func ErrEntityReportDuplicatePending() *EntityReportError {
	return &EntityReportError{
		Code:    CodeEntityReportDuplicatePending,
		Message: "you already have a pending report for this entity",
	}
}

// ErrEntityReportAlreadyReviewed creates an already-reviewed error.
func ErrEntityReportAlreadyReviewed(status string) *EntityReportError {
	return &EntityReportError{
		Code:    CodeEntityReportAlreadyReviewed,
		Message: fmt.Sprintf("report has already been reviewed (status: %s)", status),
	}
}

// ErrEntityReportInvalidEntityType creates an invalid-entity-type error.
func ErrEntityReportInvalidEntityType(entityType string) *EntityReportError {
	return &EntityReportError{
		Code:    CodeEntityReportInvalidEntityType,
		Message: fmt.Sprintf("invalid entity type: %s", entityType),
	}
}

// ErrEntityReportInvalidReportType creates an invalid-report-type error.
func ErrEntityReportInvalidReportType(reportType, entityType string) *EntityReportError {
	return &EntityReportError{
		Code:    CodeEntityReportInvalidReportType,
		Message: fmt.Sprintf("invalid report type '%s' for entity type '%s'", reportType, entityType),
	}
}

// ErrEntityReportInternal wraps a database or infrastructure failure.
func ErrEntityReportInternal(internal error) *EntityReportError {
	return &EntityReportError{
		Code:     CodeEntityReportInternal,
		Message:  "failed to process entity report",
		Internal: internal,
	}
}
