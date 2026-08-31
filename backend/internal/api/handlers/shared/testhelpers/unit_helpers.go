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
	"psychic-homily-backend/internal/services/contracts"
)

// humaErrorModel asserts err is a *huma.ErrorModel with the expected
// status and returns it so callers can inspect Detail or other fields.
// On any failure it calls t.Fatal and returns nil.
func humaErrorModel(t *testing.T, err error, expectedStatus int) *huma.ErrorModel {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
		return nil
	}
	var he *huma.ErrorModel
	if !errors.As(err, &he) {
		t.Fatalf("expected *huma.ErrorModel, got %T: %v", err, err)
		return nil
	}
	if he.Status != expectedStatus {
		t.Errorf("expected status %d, got %d (detail: %s)", expectedStatus, he.Status, he.Detail)
	}
	return he
}

// AssertHumaError checks that an error is a *huma.ErrorModel with the
// expected HTTP status. Used by every handler unit test that exercises
// huma's error path.
func AssertHumaError(t *testing.T, err error, expectedStatus int) {
	t.Helper()
	// humaErrorModel returns *huma.ErrorModel (which satisfies the error
	// interface, so errcheck flags it). The pointer is only useful to
	// callers that want to inspect Detail — this helper deliberately
	// discards it. Use AssertHumaErrorWithDetail when the message matters.
	_ = humaErrorModel(t, err, expectedStatus)
}

// AssertHumaErrorWithDetail asserts both the HTTP status AND that the
// huma.ErrorModel's Detail message matches expectedDetail exactly.
// Use this when the precise error message is part of the contract
// (e.g. error copy that disambiguates "missing field" from "invalid
// value"). Prefer AssertHumaError when only the status code matters.
func AssertHumaErrorWithDetail(t *testing.T, err error, expectedStatus int, expectedDetail string) {
	t.Helper()
	he := humaErrorModel(t, err, expectedStatus)
	if he != nil && he.Detail != expectedDetail {
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

// AllShowsVisible returns an entity-visibility gate that grants every GATED
// entity — shows and collections alike — to every caller.
//
// For handler tests whose subject is NOT the visibility rule. Named rather than
// spelled inline at each call site so a reader can see at a glance which tests
// have deliberately switched the gate off, and so the tests that DO exercise it
// stand out by passing their own MockShowVisibility instead.
//
// The zero MockShowVisibility answers false for everything, which is the right
// default for a security boundary. This is the opt-out, and it exists as an
// explicit call precisely so nothing is accidentally permissive.
//
// EVERY method must be filled in here, not just the ones a given test needs.
// The helper's meaning is "this test is not about the gate", and a method left
// nil would quietly refuse one entity type while granting the rest — a test
// failure that reads as a product bug. The name predates the collection arm
// (PSY-1987) and is left alone for the reason contracts.ShowVisibilityInterface
// gives.
func AllShowsVisible() *MockShowVisibility {
	return &MockShowVisibility{
		ShowVisibleToFn:       func(uint, contracts.ShowViewer) bool { return true },
		CollectionVisibleToFn: func(uint, contracts.ShowViewer) bool { return true },
	}
}
