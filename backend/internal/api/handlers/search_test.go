package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Tests: SearchReleasesHandler
// ============================================================================

func TestSearchReleases_Success(t *testing.T) {
	year := 1991
	mock := &mockReleaseService{
		searchReleasesFn: func(query string) ([]*contracts.ReleaseListResponse, error) {
			if query != "nevermind" {
				t.Errorf("expected query='nevermind', got %q", query)
			}
			return []*contracts.ReleaseListResponse{
				{ID: 1, Title: "Nevermind", Slug: "nevermind", ReleaseType: "lp", ReleaseYear: &year},
			}, nil
		},
	}
	h := NewReleaseHandler(mock, nil, nil, nil)

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
	mock := &mockReleaseService{
		searchReleasesFn: func(query string) ([]*contracts.ReleaseListResponse, error) {
			return []*contracts.ReleaseListResponse{}, nil
		},
	}
	h := NewReleaseHandler(mock, nil, nil, nil)

	resp, err := h.SearchReleasesHandler(context.Background(), &SearchReleasesRequest{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
}

func TestSearchReleases_ServiceError(t *testing.T) {
	mock := &mockReleaseService{
		searchReleasesFn: func(_ string) ([]*contracts.ReleaseListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewReleaseHandler(mock, nil, nil, nil)

	_, err := h.SearchReleasesHandler(context.Background(), &SearchReleasesRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Tests: SearchLabelsHandler
// ============================================================================

func TestSearchLabels_Success(t *testing.T) {
	mock := &mockLabelService{
		searchLabelsFn: func(query string) ([]*contracts.LabelListResponse, error) {
			if query != "sub pop" {
				t.Errorf("expected query='sub pop', got %q", query)
			}
			return []*contracts.LabelListResponse{
				{ID: 1, Name: "Sub Pop", Slug: "sub-pop", Status: "active"},
			}, nil
		},
	}
	h := NewLabelHandler(mock, nil, nil)

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
	mock := &mockLabelService{
		searchLabelsFn: func(query string) ([]*contracts.LabelListResponse, error) {
			return []*contracts.LabelListResponse{}, nil
		},
	}
	h := NewLabelHandler(mock, nil, nil)

	resp, err := h.SearchLabelsHandler(context.Background(), &SearchLabelsRequest{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
}

func TestSearchLabels_ServiceError(t *testing.T) {
	mock := &mockLabelService{
		searchLabelsFn: func(_ string) ([]*contracts.LabelListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewLabelHandler(mock, nil, nil)

	_, err := h.SearchLabelsHandler(context.Background(), &SearchLabelsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Tests: SearchFestivalsHandler
// ============================================================================

func TestSearchFestivals_Success(t *testing.T) {
	mock := &mockFestivalService{
		searchFestivalsFn: func(query string) ([]*contracts.FestivalListResponse, error) {
			if query != "m3f" {
				t.Errorf("expected query='m3f', got %q", query)
			}
			return []*contracts.FestivalListResponse{
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
	mock := &mockFestivalService{
		searchFestivalsFn: func(query string) ([]*contracts.FestivalListResponse, error) {
			return []*contracts.FestivalListResponse{}, nil
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
	mock := &mockFestivalService{
		searchFestivalsFn: func(_ string) ([]*contracts.FestivalListResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewFestivalHandler(mock, nil, nil, nil)

	_, err := h.SearchFestivalsHandler(context.Background(), &SearchFestivalsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ============================================================================
// Tests: SearchShowsHandler (PSY-520)
// ============================================================================

// newShowHandlerWithSearchMock builds a ShowHandler with only the mock show
// service wired — every other dependency is nil because the search handler
// only touches showService. Mirrors the minimal setup used in
// TestSearchReleases_* etc.
func newShowHandlerWithSearchMock(mock *mockShowService) *ShowHandler {
	return NewShowHandler(mock, nil, nil, nil, nil, nil, nil)
}

func TestSearchShows_Success(t *testing.T) {
	called := false
	mock := &mockShowService{
		searchShowsFn: func(query string) ([]*contracts.ShowSearchResult, error) {
			called = true
			if query != "valley" {
				t.Errorf("expected query='valley', got %q", query)
			}
			return []*contracts.ShowSearchResult{
				{ID: 1, Slug: "valley-show", Title: "Valley Bar Showcase", HeadlinerName: "Band A", VenueName: "Valley Bar"},
			}, nil
		},
	}
	h := newShowHandlerWithSearchMock(mock)

	resp, err := h.SearchShowsHandler(context.Background(), &SearchShowsRequest{Query: "valley"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected service to be called")
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Body.Count)
	}
	if len(resp.Body.Shows) != 1 || resp.Body.Shows[0].Title != "Valley Bar Showcase" {
		t.Errorf("expected title='Valley Bar Showcase', got %+v", resp.Body.Shows)
	}
}

func TestSearchShows_EmptyQuery(t *testing.T) {
	// Empty query must short-circuit at the handler — service should not
	// be invoked at all (avoids unnecessary DB round-trip).
	mock := &mockShowService{
		searchShowsFn: func(_ string) ([]*contracts.ShowSearchResult, error) {
			t.Error("service should not be called on empty query")
			return nil, nil
		},
	}
	h := newShowHandlerWithSearchMock(mock)

	resp, err := h.SearchShowsHandler(context.Background(), &SearchShowsRequest{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
	// Body.Shows is initialized to a non-nil empty slice so the JSON is `[]`,
	// not `null`. The frontend treats null and [] differently.
	if resp.Body.Shows == nil {
		t.Error("expected Body.Shows to be initialized to []")
	}
}

func TestSearchShows_WhitespaceQuery(t *testing.T) {
	// Whitespace-only queries must short-circuit identically to empty
	// queries: no DB call, [] result.
	mock := &mockShowService{
		searchShowsFn: func(_ string) ([]*contracts.ShowSearchResult, error) {
			t.Error("service should not be called on whitespace-only query")
			return nil, nil
		},
	}
	h := newShowHandlerWithSearchMock(mock)

	resp, err := h.SearchShowsHandler(context.Background(), &SearchShowsRequest{Query: "   \t\n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Body.Count)
	}
}

func TestSearchShows_ServiceError(t *testing.T) {
	mock := &mockShowService{
		searchShowsFn: func(_ string) ([]*contracts.ShowSearchResult, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newShowHandlerWithSearchMock(mock)

	_, err := h.SearchShowsHandler(context.Background(), &SearchShowsRequest{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
