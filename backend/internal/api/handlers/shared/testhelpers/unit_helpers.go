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
	"encoding/json"
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

// HumaErrorModel extracts the *huma.ErrorModel from err without asserting a
// status, for a caller that wants to compare two refusals against each other
// rather than against a constant. Fatals if err is not one.
func HumaErrorModel(t *testing.T, err error) *huma.ErrorModel {
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
	return he
}

// AssertSameRefusal asserts that two refusals are ONE response.
//
// The acceptance criterion of every entity-visibility gate in this codebase: a
// gated entity and an id nobody has used must answer alike, or the route is an
// existence oracle over a dense id space. Lives here rather than being respelled
// per package so the definition of "same response" has one home and a field
// added to the comparison covers every gated route at once.
//
// normalize is applied to both Detail strings first, for routes whose refusal
// ECHOES the id the caller supplied. Echoing it back is not a disclosure, so
// those callers substitute their own id out and compare the shape. Pass nil when
// the two details must match byte for byte.
//
// It compares the whole rendered ErrorModel, so a field added to huma's error
// body is covered without an edit here. WHAT IT CANNOT SEE, stated so nobody
// reads it as a proof of indistinguishability: response HEADERS (huma carries
// Retry-After on the rate-limited arms through a wrapper this never unwraps),
// and TIMING, which is the channel a gate placed after an expensive load leaves
// open however identical the bodies are.
func AssertSameRefusal(t *testing.T, gotErr, wantErr error, normalize func(string) string) {
	t.Helper()
	got := HumaErrorModel(t, gotErr)
	want := HumaErrorModel(t, wantErr)
	if got == nil || want == nil {
		return
	}
	render := func(m *huma.ErrorModel) string {
		encoded, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal error model: %v", err)
		}
		if normalize != nil {
			return normalize(string(encoded))
		}
		return string(encoded)
	}
	if render(got) != render(want) {
		t.Errorf("the two refusals differ:\n  %s\n  %s\na caller who can tell them apart "+
			"can walk the id space", render(got), render(want))
	}
}

// GatedEntityVisibility is the REAL pair of visibility rules for entities owned
// by ownerID, for a test whose subject IS the gate.
//
// AllShowsVisible is the permissive opt-out; this is its counterpart, and the
// two rules deliberately DIFFER on the admin: an admin sees every show and no
// private collection (services/shared/collection_visibility.go). A gate proven
// only against a checker that refuses everything would pass with the entity type
// ignored and the viewer discarded, so the asymmetry is what makes a matrix
// using this non-vacuous.
func GatedEntityVisibility(ownerID uint) *MockShowVisibility {
	return &MockShowVisibility{
		ShowVisibleToFn: func(_ uint, viewer contracts.ShowViewer) bool {
			return viewer.IsAdmin || viewer.UserID == ownerID
		},
		CollectionVisibleToFn: func(_ uint, viewer contracts.ShowViewer) bool {
			return viewer.UserID == ownerID
		},
	}
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
// nil quietly refuses one entity type while granting the rest, which reads as a
// product bug in whichever unrelated test hits it.
// TestAllShowsVisibleFillsEveryGate enforces that by reflection, so a method
// added to the interface is covered without an edit here.
//
// The name says shows and the helper grants collections too, for the reason
// contracts.ShowVisibilityInterface gives about its own name.
func AllShowsVisible() *MockShowVisibility {
	return &MockShowVisibility{
		ShowVisibleToFn:       func(uint, contracts.ShowViewer) bool { return true },
		CollectionVisibleToFn: func(uint, contracts.ShowViewer) bool { return true },
	}
}
