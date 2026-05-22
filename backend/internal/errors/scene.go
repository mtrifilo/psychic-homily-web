package errors

import (
	"fmt"
)

// Scene error codes.
//
// Scenes are city/state aggregations computed from shows. The only genuine
// not-an-infra failure is a scene that does not exist (no qualifying shows for
// the city/state or an unparseable slug). Database faults fall through to the
// generic 500 in the handler.
const (
	// CodeSceneNotFound indicates the requested scene does not exist.
	CodeSceneNotFound = "SCENE_NOT_FOUND"
)

// SceneError represents a scene-related error with additional context.
type SceneError struct {
	Code     string
	Message  string
	Internal error
}

// Error implements the error interface.
func (e *SceneError) Error() string {
	if e.Internal != nil {
		return fmt.Sprintf("%s: %s (internal: %v)", e.Code, e.Message, e.Internal)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the internal error for errors.Is/As compatibility.
func (e *SceneError) Unwrap() error {
	return e.Internal
}

// ErrSceneNotFound creates a scene-not-found error. The message is preserved
// from the caller so the existing "scene not found: <city>, <state>" /
// "scene not found for slug: <slug>" copy (and the service tests asserting it)
// are unchanged.
func ErrSceneNotFound(message string) *SceneError {
	return &SceneError{
		Code:    CodeSceneNotFound,
		Message: message,
	}
}
