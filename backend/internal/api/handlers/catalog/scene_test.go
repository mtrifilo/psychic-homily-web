package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// ListScenesHandler Tests
// ============================================================================

func TestListScenes_Success(t *testing.T) {
	mock := &mockSceneService{
		listScenesFn: func() ([]*contracts.SceneListResponse, error) {
			return []*contracts.SceneListResponse{
				{City: "Phoenix", State: "AZ", Slug: "phoenix-az", VenueCount: 5, UpcomingShowCount: 12},
			}, nil
		},
	}
	h := NewSceneHandler(mock)
	resp, err := h.ListScenesHandler(context.Background(), &ListScenesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
	if resp.Body.Scenes[0].City != "Phoenix" {
		t.Errorf("expected Phoenix, got %s", resp.Body.Scenes[0].City)
	}
}

func TestListScenes_Empty(t *testing.T) {
	mock := &mockSceneService{
		listScenesFn: func() ([]*contracts.SceneListResponse, error) {
			return nil, nil
		},
	}
	h := NewSceneHandler(mock)
	resp, err := h.ListScenesHandler(context.Background(), &ListScenesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Body.Count)
	}
}

func TestListScenes_ServiceError(t *testing.T) {
	mock := &mockSceneService{
		listScenesFn: func() ([]*contracts.SceneListResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := NewSceneHandler(mock)
	_, err := h.ListScenesHandler(context.Background(), &ListScenesRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// GetSceneDetailHandler Tests
// ============================================================================

func TestGetSceneDetail_Success(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getSceneDetailFn: func(city, state string) (*contracts.SceneDetailResponse, error) {
			return &contracts.SceneDetailResponse{
				City:  city,
				State: state,
				Slug:  "phoenix-az",
				Stats: contracts.SceneStats{
					VenueCount:        5,
					ArtistCount:       30,
					UpcomingShowCount: 12,
					FestivalCount:     2,
				},
				Pulse: contracts.ScenePulse{
					ShowsThisMonth: 8,
					ShowsPrevMonth: 5,
					ShowsTrend:     "+3",
					ShowsByMonth:   []int{3, 4, 5, 6, 5, 8},
				},
			}, nil
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneDetailRequest{Slug: "phoenix-az"}
	resp, err := h.GetSceneDetailHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.City != "Phoenix" {
		t.Errorf("expected Phoenix, got %s", resp.Body.City)
	}
	if resp.Body.Stats.VenueCount != 5 {
		t.Errorf("expected venue count 5, got %d", resp.Body.Stats.VenueCount)
	}
}

func TestGetSceneDetail_SlugNotFound(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", fmt.Errorf("scene not found for slug: %s", slug)
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneDetailRequest{Slug: "nonexistent-xx"}
	_, err := h.GetSceneDetailHandler(context.Background(), req)
	assertHumaError(t, err, 404)
}

func TestGetSceneDetail_SceneNotFound(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Tiny", "XX", nil
		},
		getSceneDetailFn: func(city, state string) (*contracts.SceneDetailResponse, error) {
			return nil, fmt.Errorf("scene not found: %s, %s", city, state)
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneDetailRequest{Slug: "tiny-xx"}
	_, err := h.GetSceneDetailHandler(context.Background(), req)
	assertHumaError(t, err, 404)
}

func TestGetSceneDetail_ServiceError(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getSceneDetailFn: func(city, state string) (*contracts.SceneDetailResponse, error) {
			return nil, fmt.Errorf("database connection lost")
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneDetailRequest{Slug: "phoenix-az"}
	_, err := h.GetSceneDetailHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// GetSceneActiveArtistsHandler Tests
// ============================================================================

func TestGetSceneActiveArtists_Success(t *testing.T) {
	phoenix := "Phoenix"
	az := "AZ"
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getActiveArtistsFn: func(city, state string, periodDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
			return []*contracts.SceneArtistResponse{
				{ID: 1, Slug: "band-a", Name: "Band A", City: &phoenix, State: &az, ShowCount: 5},
				{ID: 2, Slug: "band-b", Name: "Band B", City: &phoenix, State: &az, ShowCount: 3},
			}, 2, nil
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneActiveArtistsRequest{Slug: "phoenix-az", Period: 90, Limit: 20}
	resp, err := h.GetSceneActiveArtistsHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Artists) != 2 {
		t.Errorf("expected 2 artists, got %d", len(resp.Body.Artists))
	}
}

func TestGetSceneActiveArtists_SlugNotFound(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", fmt.Errorf("scene not found for slug: %s", slug)
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneActiveArtistsRequest{Slug: "nonexistent-xx"}
	_, err := h.GetSceneActiveArtistsHandler(context.Background(), req)
	assertHumaError(t, err, 404)
}

func TestGetSceneActiveArtists_SceneNotFound(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Tiny", "XX", nil
		},
		getActiveArtistsFn: func(city, state string, periodDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
			return nil, 0, fmt.Errorf("scene not found: %s, %s", city, state)
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneActiveArtistsRequest{Slug: "tiny-xx"}
	_, err := h.GetSceneActiveArtistsHandler(context.Background(), req)
	assertHumaError(t, err, 404)
}

func TestGetSceneActiveArtists_DefaultPeriodAndLimit(t *testing.T) {
	var capturedPeriod, capturedLimit int
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getActiveArtistsFn: func(city, state string, periodDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
			capturedPeriod = periodDays
			capturedLimit = limit
			return []*contracts.SceneArtistResponse{}, 0, nil
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneActiveArtistsRequest{Slug: "phoenix-az", Period: 0, Limit: 0}
	_, err := h.GetSceneActiveArtistsHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPeriod != 90 {
		t.Errorf("expected default period 90, got %d", capturedPeriod)
	}
	if capturedLimit != 20 {
		t.Errorf("expected default limit 20, got %d", capturedLimit)
	}
}

func TestGetSceneActiveArtists_ServiceError(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getActiveArtistsFn: func(city, state string, periodDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
			return nil, 0, fmt.Errorf("database connection lost")
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneActiveArtistsRequest{Slug: "phoenix-az", Period: 90, Limit: 20}
	_, err := h.GetSceneActiveArtistsHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// isSceneNotFoundErr Tests
// ============================================================================

func TestIsSceneNotFoundErr(t *testing.T) {
	if !isSceneNotFoundErr(fmt.Errorf("scene not found: Phoenix, AZ")) {
		t.Error("expected true for scene not found error")
	}
	if !isSceneNotFoundErr(fmt.Errorf("scene not found for slug: phoenix-az")) {
		t.Error("expected true for scene not found for slug")
	}
	if isSceneNotFoundErr(fmt.Errorf("database error")) {
		t.Error("expected false for non-scene error")
	}
	if isSceneNotFoundErr(nil) {
		t.Error("expected false for nil error")
	}
}

// ============================================================================
// GetSceneGenresHandler Tests
// ============================================================================

func TestGetSceneGenres_Success(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getSceneGenreDistributionFn: func(city, state string) ([]contracts.GenreCount, error) {
			return []contracts.GenreCount{
				{TagID: 1, Name: "punk", Slug: "punk", Count: 20},
				{TagID: 2, Name: "indie rock", Slug: "indie-rock", Count: 15},
			}, nil
		},
		getGenreDiversityIndexFn: func(city, state string) (float64, error) {
			return 0.85, nil
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneGenresRequest{Slug: "phoenix-az"}
	resp, err := h.GetSceneGenresHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Genres) != 2 {
		t.Errorf("expected 2 genres, got %d", len(resp.Body.Genres))
	}
	if resp.Body.DiversityIndex != 0.85 {
		t.Errorf("expected diversity index 0.85, got %f", resp.Body.DiversityIndex)
	}
	if resp.Body.DiversityLabel != "Highly diverse" {
		t.Errorf("expected 'Highly diverse', got '%s'", resp.Body.DiversityLabel)
	}
}

func TestGetSceneGenres_SlugNotFound(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "", "", fmt.Errorf("scene not found for slug: %s", slug)
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneGenresRequest{Slug: "nonexistent-xx"}
	_, err := h.GetSceneGenresHandler(context.Background(), req)
	assertHumaError(t, err, 404)
}

func TestGetSceneGenres_Empty(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getSceneGenreDistributionFn: func(city, state string) ([]contracts.GenreCount, error) {
			return nil, nil
		},
		getGenreDiversityIndexFn: func(city, state string) (float64, error) {
			return -1, nil
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneGenresRequest{Slug: "phoenix-az"}
	resp, err := h.GetSceneGenresHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Genres) != 0 {
		t.Errorf("expected 0 genres, got %d", len(resp.Body.Genres))
	}
	if resp.Body.DiversityIndex != -1 {
		t.Errorf("expected diversity index -1, got %f", resp.Body.DiversityIndex)
	}
	if resp.Body.DiversityLabel != "" {
		t.Errorf("expected empty diversity label, got '%s'", resp.Body.DiversityLabel)
	}
}

func TestGetSceneGenres_ServiceError(t *testing.T) {
	mock := &mockSceneService{
		parseSceneSlugFn: func(slug string) (string, string, error) {
			return "Phoenix", "AZ", nil
		},
		getSceneGenreDistributionFn: func(city, state string) ([]contracts.GenreCount, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := NewSceneHandler(mock)
	req := &GetSceneGenresRequest{Slug: "phoenix-az"}
	_, err := h.GetSceneGenresHandler(context.Background(), req)
	assertHumaError(t, err, 500)
}
