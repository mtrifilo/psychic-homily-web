package errors

import (
	"fmt"
)

// Festival intelligence error codes.
//
// The intelligence endpoints (similar festivals, overlap, breakouts, artist
// trajectory, series comparison) read existing entities and compute analytics.
// Their genuine failures are: a referenced festival or artist does not exist
// (not found), a series query that matches no festivals (not found), and an
// insufficient-input semantic violation (too few years to compare). Database
// faults fall through to the generic 500 in the handler.
const (
	// CodeFestivalIntelNotFound indicates a referenced festival or artist does
	// not exist.
	CodeFestivalIntelNotFound = "FESTIVAL_INTEL_NOT_FOUND"
	// CodeFestivalIntelNoFestivals indicates a series query matched no festivals.
	CodeFestivalIntelNoFestivals = "FESTIVAL_INTEL_NO_FESTIVALS"
	// CodeFestivalIntelInsufficientYears indicates fewer than two years were
	// supplied for a series comparison.
	CodeFestivalIntelInsufficientYears = "FESTIVAL_INTEL_INSUFFICIENT_YEARS"
)

// FestivalIntelligenceError represents a festival-intelligence error with
// additional context.
type FestivalIntelligenceError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *FestivalIntelligenceError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *FestivalIntelligenceError) Unwrap() error {
	return e.Internal
}

// ErrFestivalIntelNotFound creates a not-found error. The message is preserved
// from the caller so existing client-facing copy (e.g. "festival A not found",
// "artist not found") is unchanged.
func ErrFestivalIntelNotFound(message string) *FestivalIntelligenceError {
	return &FestivalIntelligenceError{
		Code:    CodeFestivalIntelNotFound,
		Message: message,
	}
}

// ErrFestivalIntelNoFestivals creates a no-festivals-for-series error.
func ErrFestivalIntelNoFestivals(seriesSlug string) *FestivalIntelligenceError {
	return &FestivalIntelligenceError{
		Code:    CodeFestivalIntelNoFestivals,
		Message: fmt.Sprintf("no festivals found for series '%s' in the requested years", seriesSlug),
	}
}

// ErrFestivalIntelInsufficientYears creates an insufficient-years error.
func ErrFestivalIntelInsufficientYears() *FestivalIntelligenceError {
	return &FestivalIntelligenceError{
		Code:    CodeFestivalIntelInsufficientYears,
		Message: "at least 2 years required for comparison",
	}
}
