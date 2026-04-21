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
