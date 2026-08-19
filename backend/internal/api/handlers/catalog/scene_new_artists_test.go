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
// GetSceneNewArtistsHandler Tests (PSY-1781, redefined by PSY-1844)
// ============================================================================

func newArtistsMock(fn func(city, state string, now time.Time, limit int) ([]contracts.SceneNewArtistRow, error)) *testhelpers.MockSceneService {
	return &testhelpers.MockSceneService{
		ParseSceneSlugFn:        func(string) (string, string, error) { return "Phoenix", "AZ", nil },
		GetSceneLatestArtistsFn: fn,
	}
}

func TestGetSceneNewArtists_Success(t *testing.T) {
	listed := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	mock := newArtistsMock(func(city, state string, now time.Time, limit int) ([]contracts.SceneNewArtistRow, error) {
		return []contracts.SceneNewArtistRow{{
			SceneNewArtist: contracts.SceneNewArtist{ID: 7, Slug: "saguaro-teeth", Name: "Saguaro Teeth", FirstListedAt: listed},
			Show: &contracts.SceneNewArtistShow{
				ID: 42, Slug: "saguaro-teeth-nile", EventDate: "2026-08-20", VenueName: "Nile Theater", IsUpcoming: true,
			},
		}}, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az", Limit: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(resp.Body.Artists))
	}
	if resp.Body.Artists[0].Name != "Saguaro Teeth" {
		t.Errorf("expected Saguaro Teeth, got %s", resp.Body.Artists[0].Name)
	}
	// first_listed_at survives to the wire because it is the fact the ordering
	// selected on — the row's date and its position come from one column.
	if !resp.Body.Artists[0].FirstListedAt.Equal(listed) {
		t.Errorf("first_listed_at = %v, want %v", resp.Body.Artists[0].FirstListedAt, listed)
	}
	if resp.Body.Artists[0].Show == nil || resp.Body.Artists[0].Show.VenueName != "Nile Theater" {
		t.Errorf("expected the attached show's venue, got %+v", resp.Body.Artists[0].Show)
	}
}

// PSY-1844 removed the window. The handler still owns the clock — the service
// needs `now` to split the attached show into upcoming vs past — but it derives
// no `since` from it, and the limit travels verbatim.
func TestGetSceneNewArtists_ClockAndLimitPassedThrough(t *testing.T) {
	var gotNow time.Time
	var gotLimit int
	var gotCity, gotState string
	mock := newArtistsMock(func(city, state string, now time.Time, limit int) ([]contracts.SceneNewArtistRow, error) {
		gotCity, gotState, gotNow, gotLimit = city, state, now, limit
		return nil, nil
	})
	h := NewSceneHandler(mock)

	before := time.Now().UTC()
	if _, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az", Limit: 3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCity != "Phoenix" || gotState != "AZ" {
		t.Errorf("scope = %q/%q, want Phoenix/AZ", gotCity, gotState)
	}
	if gotLimit != 3 {
		t.Errorf("limit = %d, want 3", gotLimit)
	}
	if gotNow.Before(before) {
		t.Errorf("clock %v predates the call", gotNow)
	}
	if gotNow.Location() != time.UTC {
		t.Errorf("clock must be UTC, got %v", gotNow.Location())
	}
}

// The handler does NOT substitute a default limit. huma fills the declared
// `default:"5"` on a real request, and the service owns the fallback for
// everything else — pinning the pass-through here keeps the number from being
// decided in two places and drifting.
func TestGetSceneNewArtists_ZeroLimitIsForwardedNotSubstituted(t *testing.T) {
	gotLimit := -1
	mock := newArtistsMock(func(city, state string, now time.Time, limit int) ([]contracts.SceneNewArtistRow, error) {
		gotLimit = limit
		return nil, nil
	})
	h := NewSceneHandler(mock)

	if _, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != 0 {
		t.Errorf("limit = %d, want 0 forwarded verbatim — the service decides the default", gotLimit)
	}
}

// A scene with no bands based in it is a 200 with `[]`, never a 404: the module
// hides itself, and a 404 would take the whole scene page's fetch down with it.
func TestGetSceneNewArtists_EmptyIsOKNotNotFound(t *testing.T) {
	mock := newArtistsMock(func(city, state string, now time.Time, limit int) ([]contracts.SceneNewArtistRow, error) {
		return nil, nil
	})
	h := NewSceneHandler(mock)

	resp, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Artists == nil {
		t.Fatal("artists must marshal as [], not null")
	}
	if len(resp.Body.Artists) != 0 {
		t.Errorf("expected an empty module, got %d artists", len(resp.Body.Artists))
	}
}

func TestGetSceneNewArtists_UnknownSlugIsNotFound(t *testing.T) {
	mock := &testhelpers.MockSceneService{
		ParseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", fmt.Errorf("invalid scene slug: %s", slug)
		},
		GetSceneLatestArtistsFn: func(string, string, time.Time, int) ([]contracts.SceneNewArtistRow, error) {
			t.Fatal("service must not be called for an unparseable slug")
			return nil, nil
		},
	}
	h := NewSceneHandler(mock)

	_, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "not-a-scene"})
	testhelpers.AssertHumaError(t, err, 404)
}

// Wiring only: the service behind this endpoint has no venue-count gate, so it
// does not raise SceneError today. This pins that the mapper is in the path, so
// a future gate cannot arrive as an accidental 500.
func TestGetSceneNewArtists_SceneNotFoundFromServiceIsNotFound(t *testing.T) {
	mock := newArtistsMock(func(string, string, time.Time, int) ([]contracts.SceneNewArtistRow, error) {
		return nil, apperrors.ErrSceneNotFound("scene not found: Phoenix, AZ")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetSceneNewArtists_ServiceErrorIs500(t *testing.T) {
	mock := newArtistsMock(func(string, string, time.Time, int) ([]contracts.SceneNewArtistRow, error) {
		return nil, fmt.Errorf("database error")
	})
	h := NewSceneHandler(mock)

	_, err := h.GetSceneNewArtistsHandler(context.Background(), &GetSceneNewArtistsRequest{Slug: "phoenix-az"})
	testhelpers.AssertHumaError(t, err, 500)
}
