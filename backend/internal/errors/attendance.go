package errors

import (
	"fmt"
)

// Attendance error codes.
//
// Setting attendance ("going"/"interested") is an idempotent upsert and
// clearing it is an idempotent delete, so there is no "already attending"
// conflict. The genuine failures are: the show does not exist (not found),
// an invalid status value (semantic validation), and a database fault
// (internal).
const (
	// CodeAttendanceShowNotFound indicates the target show does not exist.
	CodeAttendanceShowNotFound = "ATTENDANCE_SHOW_NOT_FOUND"
	// CodeAttendanceInvalidStatus indicates an unsupported attendance status.
	CodeAttendanceInvalidStatus = "ATTENDANCE_INVALID_STATUS"
	// CodeAttendanceInternal indicates a database or infrastructure failure.
	CodeAttendanceInternal = "ATTENDANCE_INTERNAL"
)

// AttendanceError represents an attendance-related error with additional context.
type AttendanceError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *AttendanceError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *AttendanceError) Unwrap() error {
	return e.Internal
}

// ErrAttendanceShowNotFound creates a show-not-found error.
func ErrAttendanceShowNotFound() *AttendanceError {
	return &AttendanceError{
		Code:    CodeAttendanceShowNotFound,
		Message: "show not found",
	}
}

// ErrAttendanceInvalidStatus creates an invalid-status error.
func ErrAttendanceInvalidStatus(status string) *AttendanceError {
	return &AttendanceError{
		Code:    CodeAttendanceInvalidStatus,
		Message: fmt.Sprintf("invalid attendance status: %s", status),
	}
}

// ErrAttendanceInternal wraps a database or infrastructure failure.
func ErrAttendanceInternal(internal error) *AttendanceError {
	return &AttendanceError{
		Code:     CodeAttendanceInternal,
		Message:  "failed to update attendance",
		Internal: internal,
	}
}
