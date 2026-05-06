// Package testhelpers exposes test fixtures shared across all handler
// sub-packages (catalog, engagement, admin, auth, community, notification,
// pipeline, system). It is a regular (non-`_test.go`) package so any test
// file in any handler sub-package can import it.
//
// It must NOT import any handler sub-package — that would create a cycle.
// Helpers here construct services + DB only; handler construction stays in
// the calling test file (which lives next to its handler and has direct
// access to the `New*Handler` constructor).
package testhelpers

import (
	"context"
	"errors"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
)

// AssertHumaError checks that an error is a *huma.ErrorModel with the
// expected HTTP status. Used by every handler unit test that exercises
// huma's error path.
func AssertHumaError(t *testing.T, err error, expectedStatus int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *huma.ErrorModel
	if !errors.As(err, &he) {
		t.Fatalf("expected *huma.ErrorModel, got %T: %v", err, err)
	}
	if he.Status != expectedStatus {
		t.Errorf("expected status %d, got %d (detail: %s)", expectedStatus, he.Status, he.Detail)
	}
}

// AssertHumaErrorWithDetail asserts both the HTTP status AND that the
// huma.ErrorModel's Detail message matches expectedDetail exactly.
// Use this when the precise error message is part of the contract
// (e.g. PSY-592 reply-permission errors that distinguish "missing" vs
// "invalid value"). Prefer AssertHumaError when only the status code
// matters.
func AssertHumaErrorWithDetail(t *testing.T, err error, expectedStatus int, expectedDetail string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *huma.ErrorModel
	if !errors.As(err, &he) {
		t.Fatalf("expected *huma.ErrorModel, got %T: %v", err, err)
	}
	if he.Status != expectedStatus {
		t.Errorf("expected status %d, got %d (detail: %s)", expectedStatus, he.Status, he.Detail)
	}
	if he.Detail != expectedDetail {
		t.Errorf("expected detail %q, got %q", expectedDetail, he.Detail)
	}
}

// CtxWithUser returns a context with the given user attached at
// middleware.UserContextKey. Mirrors what the auth middleware does in
// production so handler unit tests can simulate authenticated requests
// without spinning up the middleware stack.
func CtxWithUser(user *authm.User) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, user)
}
