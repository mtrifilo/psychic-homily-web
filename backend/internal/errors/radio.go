package errors

import (
	"fmt"
)

// Radio error codes
const (
	CodeRadioStationNotFound = "RADIO_STATION_NOT_FOUND"
	CodeRadioShowNotFound    = "RADIO_SHOW_NOT_FOUND"
	CodeRadioEpisodeNotFound = "RADIO_EPISODE_NOT_FOUND"
)

// RadioError represents a radio-related error with additional context.
type RadioError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *RadioError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *RadioError) Unwrap() error {
	return e.Internal
}

// ErrRadioStationNotFound creates a radio station not found error.
func ErrRadioStationNotFound(stationID uint) *RadioError {
	return &RadioError{
		Code:    CodeRadioStationNotFound,
		Message: fmt.Sprintf("Radio station %d not found", stationID),
	}
}

// ErrRadioShowNotFound creates a radio show not found error.
func ErrRadioShowNotFound(showID uint) *RadioError {
	return &RadioError{
		Code:    CodeRadioShowNotFound,
		Message: fmt.Sprintf("Radio show %d not found", showID),
	}
}

// ErrRadioEpisodeNotFound creates a radio episode not found error.
func ErrRadioEpisodeNotFound(episodeID uint) *RadioError {
	return &RadioError{
		Code:    CodeRadioEpisodeNotFound,
		Message: fmt.Sprintf("Radio episode %d not found", episodeID),
	}
}
