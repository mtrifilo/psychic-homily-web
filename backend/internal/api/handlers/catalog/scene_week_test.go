package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

func weekMock(capture *string) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			if slug == "phoenix-az" {
				return "Phoenix", "AZ", nil
			}
			return "", "", apperrors.ErrSceneNotFound(fmt.Sprintf("unknown scene slug: %s", slug))
		},
		GetSceneWeekFn: func(city, state, weekKey string) (*contracts.SceneWeekResponse, error) {
			if capture != nil {
				*capture = weekKey
			}
			return &contracts.SceneWeekResponse{
				Slug:      "phoenix-az",
				SceneName: "Phoenix, AZ",
				ISOWeek:   "2026-W31",
				Days:      []contracts.SceneWeekDay{},
			}, nil
		},
	}
}

// The two week routes exist as separate handlers because huma treats every
// declared path param as REQUIRED — a single shared request type makes
// /scenes/{slug}/week fail validation with 422 before the handler runs, and a
// pointer path param panics. These tests pin that split: the current-week
// handler must pass an EMPTY week key (meaning "resolve it server-side, in the
// scene's timezone"), and the keyed handler must pass the key through verbatim.
func TestGetSceneCurrentWeekHandler_PassesEmptyWeekKey(t *testing.T) {
	var got string
	h := NewSceneHandler(weekMock(&got))

	resp, err := h.GetSceneCurrentWeekHandler(context.Background(), &GetSceneCurrentWeekRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("current-week handler passed weekKey %q, want \"\" (server resolves current week)", got)
	}
	if resp.Body.SceneName != "Phoenix, AZ" {
		t.Errorf("unexpected scene name %q", resp.Body.SceneName)
	}
}

func TestGetSceneWeekHandler_PassesWeekKeyThrough(t *testing.T) {
	var got string
	h := NewSceneHandler(weekMock(&got))

	if _, err := h.GetSceneWeekHandler(context.Background(), &GetSceneWeekRequest{
		Slug: "phoenix-az",
		Week: "2026-W31",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2026-W31" {
		t.Errorf("keyed handler passed weekKey %q, want 2026-W31", got)
	}
}

func TestSceneWeekHandlers_UnknownSlugIs404(t *testing.T) {
	h := NewSceneHandler(weekMock(nil))

	if _, err := h.GetSceneCurrentWeekHandler(context.Background(), &GetSceneCurrentWeekRequest{Slug: "nowhere-zz"}); err == nil {
		t.Error("current-week handler accepted an unknown slug, want 404")
	}
	if _, err := h.GetSceneWeekHandler(context.Background(), &GetSceneWeekRequest{Slug: "nowhere-zz", Week: "2026-W31"}); err == nil {
		t.Error("keyed handler accepted an unknown slug, want 404")
	}
}

// A scene below the venue threshold, or a week key that does not exist, surfaces
// as a scene-not-found error from the service and must map to 404 rather than
// bubbling up as a 500.
func TestSceneWeekHandlers_SceneNotFoundMapsTo404(t *testing.T) {
	mock := weekMock(nil)
	mock.GetSceneWeekFn = func(city, state, weekKey string) (*contracts.SceneWeekResponse, error) {
		return nil, apperrors.ErrSceneNotFound("scene not found: Phoenix, AZ")
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneWeekHandler(context.Background(), &GetSceneWeekRequest{Slug: "phoenix-az", Week: "2025-W53"})
	if err == nil {
		t.Fatal("expected an error for a nonexistent week, got nil")
	}
	if se, ok := err.(interface{ GetStatus() int }); ok {
		if se.GetStatus() != 404 {
			t.Errorf("expected HTTP 404, got %d", se.GetStatus())
		}
	} else {
		t.Errorf("error does not carry an HTTP status: %T", err)
	}
}
