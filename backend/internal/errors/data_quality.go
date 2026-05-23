package errors

import (
	"fmt"
)

// Data-quality error codes.
//
// Listing the items in a data-quality / contribution category can fail
// because the category key is not recognised (semantic validation) or
// because of a database fault (internal). The summary endpoint only fails on
// internal faults. This error type is shared by the admin data-quality
// dashboard and the public contribute-opportunities surface, which both
// consume DataQualityService.
const (
	// CodeDataQualityUnknownCategory indicates an unrecognised category key.
	CodeDataQualityUnknownCategory = "DATA_QUALITY_UNKNOWN_CATEGORY"
	// CodeDataQualityInternal indicates a database or infrastructure failure.
	CodeDataQualityInternal = "DATA_QUALITY_INTERNAL"
)

// DataQualityError represents a data-quality error with additional context.
type DataQualityError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *DataQualityError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *DataQualityError) Unwrap() error {
	return e.Internal
}

// ErrDataQualityUnknownCategory creates an unknown-category error.
func ErrDataQualityUnknownCategory(category string) *DataQualityError {
	return &DataQualityError{
		Code:    CodeDataQualityUnknownCategory,
		Message: fmt.Sprintf("unknown category: %s", category),
	}
}

// ErrDataQualityInternal wraps a database or infrastructure failure.
func ErrDataQualityInternal(internal error) *DataQualityError {
	return &DataQualityError{
		Code:     CodeDataQualityInternal,
		Message:  "failed to query data quality",
		Internal: internal,
	}
}
