package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// GetSceneCollectionsHandler Tests (PSY-1847)
// ============================================================================

func sceneCollectionsMock(
	fn func(city, state string, limit int) ([]contracts.SceneCollectionSummary, error),
) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn:      func(string) (string, string, error) { return "Phoenix", "AZ", nil },
		GetSceneCollectionsFn: fn,
	}
}

func TestGetSceneCollections_Success(t *testing.T) {
	updated := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	cover := "https://example.com/cover.jpg"
	mock := sceneCollectionsMock(func(city, state string, limit int) ([]contracts.SceneCollectionSummary, error) {
		return []contracts.SceneCollectionSummary{{
			ID:                  9,
			Slug:                "phoenix-heavy",
			Title:               "Phoenix Heavy",
			CoverImageURL:       &cover,
			SceneLocalItemCount: 12,
			ItemCount:           14,
			ContributorCount:    3,
			UpdatedAt:           updated,
		}}, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneCollectionsHandler(context.Background(),
		&GetSceneCollectionsRequest{Slug: "phoenix-az", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(resp.Body.Collections))
	}
	got := resp.Body.Collections[0]
	if got.Title != "Phoenix Heavy" || got.Slug != "phoenix-heavy" {
		t.Errorf("identity fields did not survive: %+v", got)
	}
	// Both counts must reach the wire: the rail's claim is "this collection is
	// about your city", and 12-of-14 is only checkable with the denominator.
	if got.SceneLocalItemCount != 12 || got.ItemCount != 14 {
		t.Errorf("counts = %d of %d, want 12 of 14", got.SceneLocalItemCount, got.ItemCount)
	}
	if got.ContributorCount != 3 {
		t.Errorf("contributor_count = %d, want 3 (the rail's \"Built by N\")", got.ContributorCount)
	}
	if got.CoverImageURL == nil || *got.CoverImageURL != cover {
		t.Errorf("cover did not survive: %+v", got.CoverImageURL)
	}
	if !got.UpdatedAt.Equal(updated) {
		t.Errorf("updated_at = %v, want %v", got.UpdatedAt, updated)
	}
}

// The parsed scene and the limit must reach the service unchanged — the handler
// owns slug parsing, the service owns the rule.
func TestGetSceneCollections_SlugAndLimitPassedThrough(t *testing.T) {
	var gotCity, gotState string
	var gotLimit int
	mock := sceneCollectionsMock(func(city, state string, limit int) ([]contracts.SceneCollectionSummary, error) {
		gotCity, gotState, gotLimit = city, state, limit
		return nil, nil
	})
	h := NewSceneHandler(mock)

	if _, err := h.GetSceneCollectionsHandler(context.Background(),
		&GetSceneCollectionsRequest{Slug: "phoenix-az", Limit: 3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCity != "Phoenix" || gotState != "AZ" {
		t.Errorf("scene = %q, %q; want Phoenix, AZ", gotCity, gotState)
	}
	if gotLimit != 3 {
		t.Errorf("limit = %d, want 3", gotLimit)
	}
}

// A scene with no qualifying collection is a 200 with `[]`, never a 404: the
// rail hides itself, and a 404 would take the whole scene page's fetch down.
// The generated client type is `SceneCollectionSummary[] | null`, so a handler
// that let the service's nil through would ship a null the consumer must
// survive.
func TestGetSceneCollections_EmptyIsOKNotNotFound(t *testing.T) {
	mock := sceneCollectionsMock(func(string, string, int) ([]contracts.SceneCollectionSummary, error) {
		return nil, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneCollectionsHandler(context.Background(),
		&GetSceneCollectionsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Collections == nil {
		t.Fatal("collections must marshal as [], not null")
	}
	if len(resp.Body.Collections) != 0 {
		t.Errorf("expected an empty rail, got %d", len(resp.Body.Collections))
	}
}

func TestGetSceneCollections_UnknownSlugIsNotFound(t *testing.T) {
	mock := &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", apperrors.ErrSceneNotFound(fmt.Sprintf("invalid scene slug: %s", slug))
		},
		GetSceneCollectionsFn: func(string, string, int) ([]contracts.SceneCollectionSummary, error) {
			t.Fatal("service must not be called for an unparseable slug")
			return nil, nil
		},
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneCollectionsHandler(context.Background(),
		&GetSceneCollectionsRequest{Slug: "not-a-scene"})
	testhelpers.AssertHumaError(t, err, 404)
}

// Unlike /new-artists, this service DOES gate on the scene venue threshold, so
// this path is live rather than defensive: a parseable place that is not a
// scene must 404 here exactly as /shows and /graph do.
func TestGetSceneCollections_SceneNotFoundFromServiceIsNotFound(t *testing.T) {
	mock := sceneCollectionsMock(func(string, string, int) ([]contracts.SceneCollectionSummary, error) {
		return nil, apperrors.ErrSceneNotFound("scene not found: Bisbee, AZ")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneCollectionsHandler(context.Background(),
		&GetSceneCollectionsRequest{Slug: "bisbee-az"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetSceneCollections_ServiceErrorIs500(t *testing.T) {
	mock := sceneCollectionsMock(func(string, string, int) ([]contracts.SceneCollectionSummary, error) {
		return nil, fmt.Errorf("database error")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneCollectionsHandler(context.Background(),
		&GetSceneCollectionsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 500)
}
