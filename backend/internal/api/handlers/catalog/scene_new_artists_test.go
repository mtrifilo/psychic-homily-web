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
// GetSceneNewArtistsHandler Tests (PSY-1781)
// ============================================================================

func newArtistsMock(fn func(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtistRow, int, error)) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn:     func(string) (string, string, error) { return "Phoenix", "AZ", nil },
		GetSceneNewArtistsFn: fn,
	}
}

func TestGetSceneNewArtists_Success(t *testing.T) {
	listed := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	mock := newArtistsMock(func(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtistRow, int, error) {
		return []contracts.SceneNewArtistRow{{
			SceneNewArtist: contracts.SceneNewArtist{ID: 7, Slug: "saguaro-teeth", Name: "Saguaro Teeth", FirstListedAt: listed},
			Show: &contracts.SceneNewArtistShow{
				ID: 42, Slug: "saguaro-teeth-nile", EventDate: "2026-08-20", VenueName: "Nile Theater", IsUpcoming: true,
			},
		}}, 4, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az", Days: 30, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(resp.Body.Artists))
	}
	if resp.Body.Artists[0].Name != "Saguaro Teeth" {
		t.Errorf("expected Saguaro Teeth, got %s", resp.Body.Artists[0].Name)
	}
	if !resp.Body.Artists[0].FirstListedAt.Equal(listed) {
		t.Errorf("first_listed_at = %v, want %v", resp.Body.Artists[0].FirstListedAt, listed)
	}
	if resp.Body.Artists[0].Show == nil || resp.Body.Artists[0].Show.VenueName != "Nile Theater" {
		t.Errorf("expected the attached show's venue, got %+v", resp.Body.Artists[0].Show)
	}
	// The uncapped total is what drives "+N more"; a handler that returned
	// len(Artists) would silently hide the bands the cap dropped.
	if resp.Body.Total != 4 {
		t.Errorf("total = %d, want 4", resp.Body.Total)
	}
}

// The window is derived from `days`, ending now — the handler owns the clock so
// the service can stay a pure (since, now] range query.
func TestGetSceneNewArtists_WindowAndLimitPassedThrough(t *testing.T) {
	var gotSince, gotNow time.Time
	var gotLimit int
	mock := newArtistsMock(func(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtistRow, int, error) {
		gotSince, gotNow, gotLimit = since, now, limit
		return nil, 0, nil
	})
	h := NewSceneHandler(mock)

	before := time.Now().UTC()
	if _, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az", Days: 7, Limit: 3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != 3 {
		t.Errorf("limit = %d, want 3", gotLimit)
	}
	if gotNow.Before(before) {
		t.Errorf("window end %v predates the call", gotNow)
	}
	if want := gotNow.AddDate(0, 0, -7); !gotSince.Equal(want) {
		t.Errorf("since = %v, want %v (now - 7 days)", gotSince, want)
	}
}

// Huma applies the declared defaults on a real request; a direct handler call
// arrives with zero values, so the handler substitutes the same numbers.
func TestGetSceneNewArtists_ZeroValuesUseDefaults(t *testing.T) {
	var gotSince, gotNow time.Time
	var gotLimit int
	mock := newArtistsMock(func(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtistRow, int, error) {
		gotSince, gotNow, gotLimit = since, now, limit
		return nil, 0, nil
	})
	h := NewSceneHandler(mock)

	if _, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != 10 {
		t.Errorf("default limit = %d, want 10", gotLimit)
	}
	if want := gotNow.AddDate(0, 0, -30); !gotSince.Equal(want) {
		t.Errorf("default window = %v, want %v (now - 30 days)", gotSince, want)
	}
}

// A scene with no new bands is a 200 with `[]`, never a 404: the module hides
// itself, and a 404 would take the whole scene page's fetch down with it.
func TestGetSceneNewArtists_EmptyIsOKNotNotFound(t *testing.T) {
	mock := newArtistsMock(func(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtistRow, int, error) {
		return nil, 0, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Artists == nil {
		t.Fatal("artists must marshal as [], not null")
	}
	if len(resp.Body.Artists) != 0 || resp.Body.Total != 0 {
		t.Errorf("expected an empty module, got %d artists / total %d", len(resp.Body.Artists), resp.Body.Total)
	}
}

func TestGetSceneNewArtists_UnknownSlugIsNotFound(t *testing.T) {
	mock := &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", fmt.Errorf("invalid scene slug: %s", slug)
		},
		GetSceneNewArtistsFn: func(string, string, time.Time, time.Time, int) ([]contracts.SceneNewArtistRow, int, error) {
			t.Fatal("service must not be called for an unparseable slug")
			return nil, 0, nil
		},
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "not-a-scene"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetSceneNewArtists_SceneNotFoundFromServiceIsNotFound(t *testing.T) {
	mock := newArtistsMock(func(string, string, time.Time, time.Time, int) ([]contracts.SceneNewArtistRow, int, error) {
		return nil, 0, apperrors.ErrSceneNotFound("scene not found: Phoenix, AZ")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetSceneNewArtists_ServiceErrorIs500(t *testing.T) {
	mock := newArtistsMock(func(string, string, time.Time, time.Time, int) ([]contracts.SceneNewArtistRow, int, error) {
		return nil, 0, fmt.Errorf("database error")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 500)
}
