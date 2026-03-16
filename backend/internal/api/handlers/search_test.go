package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/services"
)

// ============================================================================
// Mock: ReleaseServiceInterface (minimal for search tests)
// ============================================================================

type mockReleaseServiceForSearch struct {
	searchReleasesFn func(query string) ([]*services.ReleaseListResponse, error)
}

func (m *mockReleaseServiceForSearch) CreateRelease(req *services.CreateReleaseRequest) (*services.ReleaseDetailResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) GetRelease(releaseID uint) (*services.ReleaseDetailResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) GetReleaseBySlug(slug string) (*services.ReleaseDetailResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) ListReleases(filters map[string]interface{}) ([]*services.ReleaseListResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) SearchReleases(query string) ([]*services.ReleaseListResponse, error) {
	if m.searchReleasesFn != nil {
		return m.searchReleasesFn(query)
	}
	return nil, nil
}
func (m *mockReleaseServiceForSearch) UpdateRelease(releaseID uint, req *services.UpdateReleaseRequest) (*services.ReleaseDetailResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) DeleteRelease(releaseID uint) error { return nil }
func (m *mockReleaseServiceForSearch) GetReleasesForArtist(artistID uint) ([]*services.ReleaseListResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) GetReleasesForArtistWithRoles(artistID uint) ([]*services.ArtistReleaseListResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) AddExternalLink(releaseID uint, platform, url string) (*services.ReleaseExternalLinkResponse, error) {
	return nil, nil
}
func (m *mockReleaseServiceForSearch) RemoveExternalLink(linkID uint) error { return nil }

// ============================================================================
// Mock: LabelServiceInterface (minimal for search tests)
// ============================================================================

type mockLabelServiceForSearch struct {
	searchLabelsFn func(query string) ([]*services.LabelListResponse, error)
}

func (m *mockLabelServiceForSearch) CreateLabel(req *services.CreateLabelRequest) (*services.LabelDetailResponse, error) {
	return nil, nil
}
func (m *mockLabelServiceForSearch) GetLabel(labelID uint) (*services.LabelDetailResponse, error) {
	return nil, nil
}
func (m *mockLabelServiceForSearch) GetLabelBySlug(slug string) (*services.LabelDetailResponse, error) {
	return nil, nil
}
func (m *mockLabelServiceForSearch) ListLabels(filters map[string]interface{}) ([]*services.LabelListResponse, error) {
	return nil, nil
}
func (m *mockLabelServiceForSearch) SearchLabels(query string) ([]*services.LabelListResponse, error) {
	if m.searchLabelsFn != nil {
		return m.searchLabelsFn(query)
	}
	return nil, nil
}
func (m *mockLabelServiceForSearch) UpdateLabel(labelID uint, req *services.UpdateLabelRequest) (*services.LabelDetailResponse, error) {
	return nil, nil
}
func (m *mockLabelServiceForSearch) DeleteLabel(labelID uint) error { return nil }
func (m *mockLabelServiceForSearch) GetLabelRoster(labelID uint) ([]*services.LabelArtistResponse, error) {
	return nil, nil
}
func (m *mockLabelServiceForSearch) GetLabelCatalog(labelID uint) ([]*services.LabelReleaseResponse, error) {
	return nil, nil
}

// ============================================================================
// Mock: FestivalServiceInterface (minimal for search tests)
// ============================================================================

type mockFestivalServiceForSearch struct {
	searchFestivalsFn func(query string) ([]*services.FestivalListResponse, error)
}

func (m *mockFestivalServiceForSearch) CreateFestival(req *services.CreateFestivalRequest) (*services.FestivalDetailResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) GetFestival(festivalID uint) (*services.FestivalDetailResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) GetFestivalBySlug(slug string) (*services.FestivalDetailResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) ListFestivals(filters map[string]interface{}) ([]*services.FestivalListResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) SearchFestivals(query string) ([]*services.FestivalListResponse, error) {
	if m.searchFestivalsFn != nil {
		return m.searchFestivalsFn(query)
	}
	return nil, nil
}
func (m *mockFestivalServiceForSearch) UpdateFestival(festivalID uint, req *services.UpdateFestivalRequest) (*services.FestivalDetailResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) DeleteFestival(festivalID uint) error { return nil }
func (m *mockFestivalServiceForSearch) GetFestivalArtists(festivalID uint, dayDate *string) ([]*services.FestivalArtistResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) AddFestivalArtist(festivalID uint, req *services.AddFestivalArtistRequest) (*services.FestivalArtistResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) UpdateFestivalArtist(festivalID, artistID uint, req *services.UpdateFestivalArtistRequest) (*services.FestivalArtistResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) RemoveFestivalArtist(festivalID, artistID uint) error {
	return nil
}
func (m *mockFestivalServiceForSearch) GetFestivalVenues(festivalID uint) ([]*services.FestivalVenueResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) AddFestivalVenue(festivalID uint, req *services.AddFestivalVenueRequest) (*services.FestivalVenueResponse, error) {
	return nil, nil
}
func (m *mockFestivalServiceForSearch) RemoveFestivalVenue(festivalID, venueID uint) error {
	return nil
}
func (m *mockFestivalServiceForSearch) GetFestivalsForArtist(artistID uint) ([]*services.ArtistFestivalListResponse, error) {
	return nil, nil
}

// ============================================================================
// Tests: SearchReleasesHandler
// ============================================================================

func TestSearchReleases_Success(t *testing.T) {
	year := 1991
	mock := &mockReleaseServiceForSearch{
		searchReleasesFn: func(query string) ([]*services.ReleaseListResponse, error) {
			if query != "nevermind" {
				t.Errorf("expected query='nevermind', got %q", query)
			}
			return []*services.ReleaseListResponse{
				{ID: 1, Title: "Nevermind", Slug: "nevermind", ReleaseType: "lp", ReleaseYear: &year},
			}, nil
		},
	}
	h := NewReleaseHandler(mock, nil, nil)

	resp, err := h.SearchReleasesHandler(context.Background(), &SearchReleasesRequest{Query: "nevermind"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
	if resp.Body.Releases[0].Title != "Nevermind" {
		t.Errorf("expected title='Nevermind', got %q", resp.Body.Releases[0].Title)
	}
}

func TestSearchReleases_EmptyQuery(t *testing.T) {
	mock := &mockReleaseServiceForSearch{
		searchReleasesFn: func(query string) ([]*services.ReleaseListResponse, error) {
			return []*services.ReleaseListResponse{}, nil
		},
	}
	h := NewReleaseHandler(mock, nil, nil)

	resp, err := h.SearchReleasesHandler(context.Background(), &SearchReleasesRequest{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
}

func TestSearchReleases_ServiceError(t *testing.T) {
	mock := &mockReleaseServiceForSearch{
		searchReleasesFn: func(_ string) ([]*services.ReleaseListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewReleaseHandler(mock, nil, nil)

	_, err := h.SearchReleasesHandler(context.Background(), &SearchReleasesRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Tests: SearchLabelsHandler
// ============================================================================

func TestSearchLabels_Success(t *testing.T) {
	mock := &mockLabelServiceForSearch{
		searchLabelsFn: func(query string) ([]*services.LabelListResponse, error) {
			if query != "sub pop" {
				t.Errorf("expected query='sub pop', got %q", query)
			}
			return []*services.LabelListResponse{
				{ID: 1, Name: "Sub Pop", Slug: "sub-pop", Status: "active"},
			}, nil
		},
	}
	h := NewLabelHandler(mock, nil)

	resp, err := h.SearchLabelsHandler(context.Background(), &SearchLabelsRequest{Query: "sub pop"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
	if resp.Body.Labels[0].Name != "Sub Pop" {
		t.Errorf("expected name='Sub Pop', got %q", resp.Body.Labels[0].Name)
	}
}

func TestSearchLabels_EmptyQuery(t *testing.T) {
	mock := &mockLabelServiceForSearch{
		searchLabelsFn: func(query string) ([]*services.LabelListResponse, error) {
			return []*services.LabelListResponse{}, nil
		},
	}
	h := NewLabelHandler(mock, nil)

	resp, err := h.SearchLabelsHandler(context.Background(), &SearchLabelsRequest{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
}

func TestSearchLabels_ServiceError(t *testing.T) {
	mock := &mockLabelServiceForSearch{
		searchLabelsFn: func(_ string) ([]*services.LabelListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewLabelHandler(mock, nil)

	_, err := h.SearchLabelsHandler(context.Background(), &SearchLabelsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Tests: SearchFestivalsHandler
// ============================================================================

func TestSearchFestivals_Success(t *testing.T) {
	mock := &mockFestivalServiceForSearch{
		searchFestivalsFn: func(query string) ([]*services.FestivalListResponse, error) {
			if query != "m3f" {
				t.Errorf("expected query='m3f', got %q", query)
			}
			return []*services.FestivalListResponse{
				{ID: 1, Name: "M3F Festival", Slug: "m3f-2026", EditionYear: 2026, Status: "confirmed"},
			}, nil
		},
	}
	h := NewFestivalHandler(mock, nil, nil, nil)

	resp, err := h.SearchFestivalsHandler(context.Background(), &SearchFestivalsRequest{Query: "m3f"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
	if resp.Body.Festivals[0].Name != "M3F Festival" {
		t.Errorf("expected name='M3F Festival', got %q", resp.Body.Festivals[0].Name)
	}
}

func TestSearchFestivals_EmptyQuery(t *testing.T) {
	mock := &mockFestivalServiceForSearch{
		searchFestivalsFn: func(query string) ([]*services.FestivalListResponse, error) {
			return []*services.FestivalListResponse{}, nil
		},
	}
	h := NewFestivalHandler(mock, nil, nil, nil)

	resp, err := h.SearchFestivalsHandler(context.Background(), &SearchFestivalsRequest{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
}

func TestSearchFestivals_ServiceError(t *testing.T) {
	mock := &mockFestivalServiceForSearch{
		searchFestivalsFn: func(_ string) ([]*services.FestivalListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewFestivalHandler(mock, nil, nil, nil)

	_, err := h.SearchFestivalsHandler(context.Background(), &SearchFestivalsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
