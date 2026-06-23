package errors

import (
	"fmt"
)

// Radio error codes
const (
	CodeRadioStationNotFound     = "RADIO_STATION_NOT_FOUND"
	CodeRadioShowNotFound        = "RADIO_SHOW_NOT_FOUND"
	CodeRadioEpisodeNotFound     = "RADIO_EPISODE_NOT_FOUND"
	CodeRadioStationNameConflict = "RADIO_STATION_NAME_CONFLICT"
	CodeRadioScheduleInvalid     = "RADIO_SCHEDULE_INVALID"
	// CodeRadioLifecycleInvalid is returned when an admin lifecycle_state update is not
	// one of active|dormant|retired (PSY-1172). Mapped to HTTP 422.
	CodeRadioLifecycleInvalid = "RADIO_LIFECYCLE_INVALID"
	// CodeRadioSyncAlreadyRunning is returned when a manual sync trigger loses the
	// per-station advisory lock to an in-flight run (PSY-1135). Mapped to HTTP 409.
	CodeRadioSyncAlreadyRunning = "RADIO_SYNC_ALREADY_RUNNING"
	// CodeRadioSyncRunNotFound is returned when a sync-run poll/cancel targets a
	// run id that does not exist (PSY-1135). Mapped to HTTP 404.
	CodeRadioSyncRunNotFound = "RADIO_SYNC_RUN_NOT_FOUND"
	// CodeRadioSyncNotCancellable is returned when a cancel targets a run that is
	// no longer running (already terminal) (PSY-1135). Mapped to HTTP 409.
	CodeRadioSyncNotCancellable = "RADIO_SYNC_NOT_CANCELLABLE"
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

// ErrRadioStationNameConflict creates a duplicate-station-name conflict error.
// Station names are unique (case-insensitive); admin create rejects a dupe with
// a clean 409 instead of a raw DB-constraint 500.
func ErrRadioStationNameConflict(name string) *RadioError {
	return &RadioError{
		Code:    CodeRadioStationNameConflict,
		Message: fmt.Sprintf("A radio station named %q already exists", name),
	}
}

// ErrRadioScheduleInvalid creates a validation error for a radio show's
// schedule JSONB that does not match the catalog.RadioSchedule shape
// (PSY-1131). Mapped to HTTP 422 by the admin show handlers.
func ErrRadioScheduleInvalid(detail string) *RadioError {
	return &RadioError{
		Code:    CodeRadioScheduleInvalid,
		Message: fmt.Sprintf("Invalid schedule: %s", detail),
	}
}

// ErrRadioLifecycleInvalid creates a validation error for an admin-supplied
// lifecycle_state that is not one of active|dormant|retired (PSY-1172). Mapped to
// HTTP 422 by the admin show handler.
func ErrRadioLifecycleInvalid(value string) *RadioError {
	return &RadioError{
		Code:    CodeRadioLifecycleInvalid,
		Message: fmt.Sprintf("Invalid lifecycle_state %q (expected active, dormant, or retired)", value),
	}
}

// ErrRadioSyncAlreadyRunning creates a conflict error for a manual sync trigger
// that could not start because another run already holds the station's sync lock
// (PSY-1135). Mapped to HTTP 409.
func ErrRadioSyncAlreadyRunning(stationID uint) *RadioError {
	return &RadioError{
		Code:    CodeRadioSyncAlreadyRunning,
		Message: fmt.Sprintf("A sync is already running for station %d", stationID),
	}
}

// ErrRadioSyncRunNotFound creates a not-found error for a sync-run poll/cancel
// (PSY-1135). Mapped to HTTP 404.
func ErrRadioSyncRunNotFound(runID uint) *RadioError {
	return &RadioError{
		Code:    CodeRadioSyncRunNotFound,
		Message: fmt.Sprintf("Sync run %d not found", runID),
	}
}

// ErrRadioSyncNotCancellable creates a conflict error for cancelling a run that
// is no longer running (PSY-1135). Mapped to HTTP 409.
func ErrRadioSyncNotCancellable(runID uint, status string) *RadioError {
	return &RadioError{
		Code:    CodeRadioSyncNotCancellable,
		Message: fmt.Sprintf("Sync run %d cannot be cancelled (status: %s)", runID, status),
	}
}
