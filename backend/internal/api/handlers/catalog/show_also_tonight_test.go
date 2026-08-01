package catalog

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// The address reaches the service verbatim. The path segment is documented as
// "id OR slug", and the service is what decides between them — a handler that
// coerced it would silently break every slug-addressed show page.
func TestGetShowAlsoTonightHandler_PassesTheAddressThrough(t *testing.T) {
	var got string
	h := NewSceneHandler(&testhelpers.MockSceneService{
		GetShowAlsoTonightFn: func(idOrSlug string) (*contracts.ShowAlsoTonightResponse, error) {
			got = idOrSlug
			return &contracts.ShowAlsoTonightResponse{
				SceneSlug: "chicago-il",
				Date:      "2026-09-18",
				Shows:     []contracts.SceneShowSummary{},
			}, nil
		},
	})

	resp, err := h.GetShowAlsoTonightHandler(context.Background(), &GetShowAlsoTonightRequest{
		ShowID: "desert-doom-night",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "desert-doom-night" {
		t.Errorf("handler passed %q to the service, want the path segment verbatim", got)
	}
	if resp.Body.SceneSlug != "chicago-il" {
		t.Errorf("unexpected scene slug %q", resp.Body.SceneSlug)
	}
}

// An unknown (or hidden) show must be a 404, not a 500. The status is the whole
// answer for any client deciding whether the rail is missing or the server is
// broken.
func TestGetShowAlsoTonightHandler_NotFoundMapsTo404(t *testing.T) {
	h := NewSceneHandler(&testhelpers.MockSceneService{
		GetShowAlsoTonightFn: func(string) (*contracts.ShowAlsoTonightResponse, error) {
			return nil, apperrors.ErrShowNotFound(0)
		},
	})

	_, err := h.GetShowAlsoTonightHandler(context.Background(), &GetShowAlsoTonightRequest{ShowID: "999999"})
	assertHTTPStatus(t, err, 404, "also-tonight handler")
}

// Anything else is a fault, and must not be laundered into a 404 — a rail that
// 404s on a database outage would teach clients to hide a real show page.
func TestGetShowAlsoTonightHandler_UnexpectedErrorIs500(t *testing.T) {
	h := NewSceneHandler(&testhelpers.MockSceneService{
		GetShowAlsoTonightFn: func(string) (*contracts.ShowAlsoTonightResponse, error) {
			return nil, errors.New("database not initialized")
		},
	})

	_, err := h.GetShowAlsoTonightHandler(context.Background(), &GetShowAlsoTonightRequest{ShowID: "1"})
	assertHTTPStatus(t, err, 500, "also-tonight handler")
}
