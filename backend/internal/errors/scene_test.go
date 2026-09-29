package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsSceneNotFound(t *testing.T) {
	notFound := ErrSceneNotFound("scene not found for slug: x")
	if !IsSceneNotFound(notFound) {
		t.Error("a scene-not-found error must match")
	}
	if !IsSceneNotFound(fmt.Errorf("wrapped: %w", notFound)) {
		t.Error("a wrapped scene-not-found error must match")
	}
	if IsSceneNotFound(&SceneError{Code: "OTHER"}) {
		t.Error("another scene error code must not match")
	}
	if IsSceneNotFound(errors.New("database is down")) {
		t.Error("a plain error must not match")
	}
	if IsSceneNotFound(nil) {
		t.Error("nil must not match")
	}
}
