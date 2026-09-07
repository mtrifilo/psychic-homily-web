package catalog

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

func dayMock(capture *string) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			if slug == "phoenix-az" {
				return "Phoenix", "AZ", nil
			}
			return "", "", apperrors.ErrSceneNotFound(fmt.Sprintf("unknown scene slug: %s", slug))
		},
		GetSceneDayFn: func(city, state, dateKey string) (*contracts.SceneDayResponse, error) {
			if capture != nil {
				*capture = dateKey
			}
			return &contracts.SceneDayResponse{
				Slug:      "phoenix-az",
				SceneName: "Phoenix, AZ",
				Date:      "2026-07-31",
				Shows:     []contracts.SceneShowSummary{},
			}, nil
		},
	}
}

// The two day routes exist as separate handlers for the same reason the two week
// routes do — huma treats every declared path param as REQUIRED, so a shared
// request type makes /scenes/{slug}/day fail validation with 422 before the
// handler runs. These tests pin that split: the current-night handler must pass
// an EMPTY date key (meaning "resolve it server-side, in the scene's timezone
// and against its 6am night boundary"), and the keyed handler must pass the date
// through verbatim.
func TestGetSceneCurrentDayHandler_PassesEmptyDateKey(t *testing.T) {
	var got string
	h := NewSceneHandler(dayMock(&got))

	resp, err := h.GetSceneCurrentDayHandler(context.Background(), &GetSceneCurrentDayRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("current-day handler passed dateKey %q, want \"\" (server resolves tonight)", got)
	}
	if resp.Body.SceneName != "Phoenix, AZ" {
		t.Errorf("unexpected scene name %q", resp.Body.SceneName)
	}
}

func TestGetSceneDayHandler_PassesDateKeyThrough(t *testing.T) {
	var got string
	h := NewSceneHandler(dayMock(&got))

	if _, err := h.GetSceneDayHandler(context.Background(), &GetSceneDayRequest{
		Slug: "phoenix-az",
		Date: "2026-07-31",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2026-07-31" {
		t.Errorf("keyed handler passed dateKey %q, want 2026-07-31", got)
	}
}

// The STATUS is what matters, not merely that an error came back: the proxy's
// HEAD existence check reads it, and anything other than a 404 makes it fail
// open — turning an unknown scene into a 404 body served at HTTP 200.
func TestSceneDayHandlers_UnknownSlugIs404(t *testing.T) {
	h := NewSceneHandler(dayMock(nil))

	_, currentErr := h.GetSceneCurrentDayHandler(context.Background(), &GetSceneCurrentDayRequest{Slug: "nowhere-zz"})
	assertHTTPStatus(t, currentErr, 404, "current-day handler")

	_, keyedErr := h.GetSceneDayHandler(context.Background(), &GetSceneDayRequest{Slug: "nowhere-zz", Date: "2026-07-31"})
	assertHTTPStatus(t, keyedErr, 404, "keyed handler")
}

func assertHTTPStatus(t *testing.T, err error, want int, who string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s returned no error, want HTTP %d", who, want)
	}
	se, ok := err.(interface{ GetStatus() int })
	if !ok {
		t.Fatalf("%s error does not carry an HTTP status: %T", who, err)
	}
	if se.GetStatus() != want {
		t.Errorf("%s returned HTTP %d, want %d", who, se.GetStatus(), want)
	}
}

// A scene below the venue threshold, or an impossible date, surfaces as a
// scene-not-found error from the service and must map to 404 rather than
// bubbling up as a 500 — the proxy's HEAD existence check reads that status to
// decide whether the page is real, so a 500 here becomes a soft-404 there.
func TestSceneDayHandlers_SceneNotFoundMapsTo404(t *testing.T) {
	mock := dayMock(nil)
	mock.GetSceneDayFn = func(city, state, dateKey string) (*contracts.SceneDayResponse, error) {
		return nil, apperrors.ErrSceneNotFound("date \"2026-02-30\" does not exist")
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneDayHandler(context.Background(), &GetSceneDayRequest{Slug: "phoenix-az", Date: "2026-02-30"})
	assertHTTPStatus(t, err, 404, "keyed handler on an impossible date")
}
