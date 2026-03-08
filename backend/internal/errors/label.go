package errors

import (
	"fmt"
)

// Label error codes
const (
	CodeLabelNotFound = "LABEL_NOT_FOUND"
	CodeLabelExists   = "LABEL_EXISTS"
)

// LabelError represents a label-related error with additional context.
type LabelError struct {
	Code      string
	Message   string
	Internal  error
	RequestID string
	LabelID   uint
}

// Error implements the error interface.
func (e *LabelError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *LabelError) Unwrap() error {
	return e.Internal
}

// ErrLabelNotFound creates a label not found error.
func ErrLabelNotFound(labelID uint) *LabelError {
	return &LabelError{
		Code:    CodeLabelNotFound,
		Message: "Label not found",
		LabelID: labelID,
	}
}

// ErrLabelExists creates a label-already-exists error.
func ErrLabelExists(name string) *LabelError {
	return &LabelError{
		Code:    CodeLabelExists,
		Message: fmt.Sprintf("Label with name '%s' already exists", name),
	}
}
