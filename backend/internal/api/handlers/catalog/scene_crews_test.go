package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// GetSceneCrewsHandler Tests (PSY-1884)
// ============================================================================

func sceneCrewsMock(fn func(city, state string) ([]contracts.SceneCrewSummary, error)) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(string) (string, string, error) { return "Phoenix", "AZ", nil },
		GetSceneCrewsFn:  fn,
	}
}

func TestGetSceneCrews_Success(t *testing.T) {
	mock := sceneCrewsMock(func(city, state string) ([]contracts.SceneCrewSummary, error) {
		if city != "Phoenix" || state != "AZ" {
			t.Fatalf("handler must pass the PARSED city/state through, got %q, %q", city, state)
		}
		return []contracts.SceneCrewSummary{
			{Slug: "rubber-brother-records", Name: "Rubber Brother Records", ShowCount: 12},
			{Slug: "ascetic-house", Name: "Ascetic House", ShowCount: 3},
		}, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneCrewsHandler(context.Background(), &GetSceneCrewsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Crews) != 2 {
		t.Fatalf("Crews length = %d, want 2", len(resp.Body.Crews))
	}
	// The handler must not re-rank: the service owns the order.
	if resp.Body.Crews[0].Slug != "rubber-brother-records" || resp.Body.Crews[0].ShowCount != 12 {
		t.Errorf("first crew = %+v, want rubber-brother-records with 12", resp.Body.Crews[0])
	}
	if resp.Body.Crews[1].Name != "Ascetic House" {
		t.Errorf("second crew name = %q, want Ascetic House", resp.Body.Crews[1].Name)
	}
}

// A scene with no crews is a 200 with an empty list, not a 404: the scene
// exists, and a 404 would deny a place the sibling routes just served. The
// nil-to-empty conversion is asserted on the MARSHALLED body, because a nil
// slice and an empty one are indistinguishable in Go and the wire is where the
// difference lands: `null` breaks a client that maps over the field.
func TestGetSceneCrews_NoCrewsIsEmptyArrayNotNull(t *testing.T) {
	mock := sceneCrewsMock(func(string, string) ([]contracts.SceneCrewSummary, error) {
		return nil, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneCrewsHandler(context.Background(), &GetSceneCrewsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	encoded, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if got, want := string(encoded), `{"crews":[]}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestGetSceneCrews_UnknownSlugIsNotFound(t *testing.T) {
	mock := &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", fmt.Errorf("invalid scene slug: %s", slug)
		},
		GetSceneCrewsFn: func(string, string) ([]contracts.SceneCrewSummary, error) {
			t.Fatal("service must not be called for an unparseable slug")
			return nil, nil
		},
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneCrewsHandler(context.Background(), &GetSceneCrewsRequest{Slug: "not-a-scene"})
	testhelpers.AssertHumaError(t, err, 404)
}

// The service gates on the verified-venue threshold, so this path is live: a
// parseable place below it must reach the reader as a 404, not a 500.
func TestGetSceneCrews_SceneNotFoundFromServiceIsNotFound(t *testing.T) {
	mock := sceneCrewsMock(func(string, string) ([]contracts.SceneCrewSummary, error) {
		return nil, apperrors.ErrSceneNotFound("scene not found: Phoenix, AZ")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneCrewsHandler(context.Background(), &GetSceneCrewsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetSceneCrews_ServiceErrorIs500(t *testing.T) {
	mock := sceneCrewsMock(func(string, string) ([]contracts.SceneCrewSummary, error) {
		return nil, fmt.Errorf("database error")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneCrewsHandler(context.Background(), &GetSceneCrewsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 500)
}
