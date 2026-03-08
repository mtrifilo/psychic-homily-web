package errors

import (
	"fmt"
)

// Festival error codes
const (
	CodeFestivalNotFound = "FESTIVAL_NOT_FOUND"
	CodeFestivalExists   = "FESTIVAL_EXISTS"
)

// FestivalError represents a festival-related error with additional context.
type FestivalError struct {
	Code       string
	Message    string
	Internal   error
	RequestID  string
	FestivalID uint
}

// Error implements the error interface.
func (e *FestivalError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *FestivalError) Unwrap() error {
	return e.Internal
}

// ErrFestivalNotFound creates a festival not found error.
func ErrFestivalNotFound(festivalID uint) *FestivalError {
	return &FestivalError{
		Code:       CodeFestivalNotFound,
		Message:    "Festival not found",
		FestivalID: festivalID,
	}
}

// ErrFestivalExists creates a festival-already-exists error.
func ErrFestivalExists(name string) *FestivalError {
	return &FestivalError{
		Code:    CodeFestivalExists,
		Message: fmt.Sprintf("Festival with name '%s' already exists", name),
	}
}
