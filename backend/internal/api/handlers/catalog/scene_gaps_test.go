package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// GetSceneGapsHandler Tests (PSY-1845)
// ============================================================================

func sceneGapsMock(fn func(city, state string) (*contracts.SceneGapsResponse, error)) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(string) (string, string, error) { return "Phoenix", "AZ", nil },
		GetSceneGapsFn:   fn,
	}
}

func TestGetSceneGaps_Success(t *testing.T) {
	mock := sceneGapsMock(func(city, state string) (*contracts.SceneGapsResponse, error) {
		if city != "Phoenix" || state != "AZ" {
			t.Fatalf("handler must pass the PARSED city/state through, got %q, %q", city, state)
		}
		return &contracts.SceneGapsResponse{
			City:                          "Phoenix",
			State:                         "AZ",
			Slug:                          "phoenix-az",
			ArtistsMissingListenLink:      11,
			ArtistsOnBillsMissingLocation: 4,
		}, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneGapsHandler(context.Background(), &GetSceneGapsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ArtistsMissingListenLink != 11 {
		t.Errorf("ArtistsMissingListenLink = %d, want 11", resp.Body.ArtistsMissingListenLink)
	}
	if resp.Body.ArtistsOnBillsMissingLocation != 4 {
		t.Errorf("ArtistsOnBillsMissingLocation = %d, want 4", resp.Body.ArtistsOnBillsMissingLocation)
	}
	if resp.Body.Slug != "phoenix-az" {
		t.Errorf("Slug = %q, want phoenix-az", resp.Body.Slug)
	}
}

// A complete scene is a 200 with zeros, NOT a 404 and not an empty body. The
// frontend decides whether to render the line from the numbers; the endpoint
// must hand it numbers to decide on.
func TestGetSceneGaps_ZeroGapsIsSuccessNotNotFound(t *testing.T) {
	mock := sceneGapsMock(func(string, string) (*contracts.SceneGapsResponse, error) {
		return &contracts.SceneGapsResponse{City: "Phoenix", State: "AZ", Slug: "phoenix-az"}, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneGapsHandler(context.Background(), &GetSceneGapsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ArtistsMissingListenLink != 0 || resp.Body.ArtistsOnBillsMissingLocation != 0 {
		t.Errorf("expected zeros, got %+v", resp.Body)
	}
}

func TestGetSceneGaps_UnknownSlugIsNotFound(t *testing.T) {
	mock := &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", apperrors.ErrSceneNotFound(fmt.Sprintf("invalid scene slug: %s", slug))
		},
		GetSceneGapsFn: func(string, string) (*contracts.SceneGapsResponse, error) {
			t.Fatal("service must not be called for an unparseable slug")
			return nil, nil
		},
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneGapsHandler(context.Background(), &GetSceneGapsRequest{Slug: "not-a-scene"})
	testhelpers.AssertHumaError(t, err, 404)
}

// The service DOES gate on the verified-venue threshold, so this path is live —
// a place below it must reach the reader as a 404, not a 500.
func TestGetSceneGaps_SceneNotFoundFromServiceIsNotFound(t *testing.T) {
	mock := sceneGapsMock(func(string, string) (*contracts.SceneGapsResponse, error) {
		return nil, apperrors.ErrSceneNotFound("scene not found: Phoenix, AZ")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneGapsHandler(context.Background(), &GetSceneGapsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetSceneGaps_ServiceErrorIs500(t *testing.T) {
	mock := sceneGapsMock(func(string, string) (*contracts.SceneGapsResponse, error) {
		return nil, fmt.Errorf("database error")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneGapsHandler(context.Background(), &GetSceneGapsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 500)
}
