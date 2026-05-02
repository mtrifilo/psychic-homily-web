package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testChartsHandler() *ChartsHandler {
	return NewChartsHandler(&testhelpers.MockChartsService{})
}

// ============================================================================
// Tests: normalizeChartsLimit
// ============================================================================

func TestNormalizeChartsLimit(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 20},
		{-1, 20},
		{1, 1},
		{20, 20},
		{50, 50},
		{51, 50},
		{100, 50},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("input_%d", tc.input), func(t *testing.T) {
			result := normalizeChartsLimit(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeChartsLimit(%d) = %d, want %d", tc.input, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// Tests: GetTrendingShowsHandler
// ============================================================================

func TestChartsHandler_TrendingShows_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetTrendingShowsFn: func(limit int) ([]contracts.TrendingShow, error) {
			if limit != 20 {
				t.Errorf("expected limit=20, got %d", limit)
			}
			return []contracts.TrendingShow{
				{ShowID: 1, Title: "Big Show", Slug: "big-show", GoingCount: 10, InterestedCount: 5, TotalAttendance: 15, VenueName: "The Venue", City: "Phoenix"},
			}, nil
		},
	})

	resp, err := h.GetTrendingShowsHandler(context.Background(), &GetTrendingShowsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Shows) != 1 {
		t.Fatalf("expected 1 show, got %d", len(resp.Body.Shows))
	}
	if resp.Body.Shows[0].TotalAttendance != 15 {
		t.Errorf("expected total_attendance=15, got %d", resp.Body.Shows[0].TotalAttendance)
	}
	if resp.Body.Shows[0].Title != "Big Show" {
		t.Errorf("expected title='Big Show', got %s", resp.Body.Shows[0].Title)
	}
}

func TestChartsHandler_TrendingShows_CustomLimit(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetTrendingShowsFn: func(limit int) ([]contracts.TrendingShow, error) {
			receivedLimit = limit
			return []contracts.TrendingShow{}, nil
		},
	})

	_, err := h.GetTrendingShowsHandler(context.Background(), &GetTrendingShowsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 10 {
		t.Errorf("expected limit=10, got %d", receivedLimit)
	}
}

func TestChartsHandler_TrendingShows_LimitCapped(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetTrendingShowsFn: func(limit int) ([]contracts.TrendingShow, error) {
			receivedLimit = limit
			return []contracts.TrendingShow{}, nil
		},
	})

	_, err := h.GetTrendingShowsHandler(context.Background(), &GetTrendingShowsRequest{Limit: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 50 {
		t.Errorf("expected limit capped to 50, got %d", receivedLimit)
	}
}

func TestChartsHandler_TrendingShows_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetTrendingShowsFn: func(limit int) ([]contracts.TrendingShow, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetTrendingShowsHandler(context.Background(), &GetTrendingShowsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetPopularArtistsHandler
// ============================================================================

func TestChartsHandler_PopularArtists_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetPopularArtistsFn: func(limit int) ([]contracts.PopularArtist, error) {
			return []contracts.PopularArtist{
				{ArtistID: 1, Name: "Great Band", Slug: "great-band", FollowCount: 100, UpcomingShowCount: 5, Score: 205},
				{ArtistID: 2, Name: "Good Band", Slug: "good-band", FollowCount: 50, UpcomingShowCount: 2, Score: 102},
			}, nil
		},
	})

	resp, err := h.GetPopularArtistsHandler(context.Background(), &GetPopularArtistsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Artists) != 2 {
		t.Fatalf("expected 2 artists, got %d", len(resp.Body.Artists))
	}
	if resp.Body.Artists[0].Score != 205 {
		t.Errorf("expected score=205, got %d", resp.Body.Artists[0].Score)
	}
}

func TestChartsHandler_PopularArtists_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetPopularArtistsFn: func(limit int) ([]contracts.PopularArtist, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetPopularArtistsHandler(context.Background(), &GetPopularArtistsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestChartsHandler_PopularArtists_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetPopularArtistsFn: func(limit int) ([]contracts.PopularArtist, error) {
			receivedLimit = limit
			return []contracts.PopularArtist{}, nil
		},
	})

	_, err := h.GetPopularArtistsHandler(context.Background(), &GetPopularArtistsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

// ============================================================================
// Tests: GetActiveVenuesHandler
// ============================================================================

func TestChartsHandler_ActiveVenues_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetActiveVenuesFn: func(limit int) ([]contracts.ActiveVenue, error) {
			return []contracts.ActiveVenue{
				{VenueID: 1, Name: "Best Venue", Slug: "best-venue", City: "Phoenix", State: "AZ", UpcomingShowCount: 10, FollowCount: 50, Score: 70},
			}, nil
		},
	})

	resp, err := h.GetActiveVenuesHandler(context.Background(), &GetActiveVenuesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Venues) != 1 {
		t.Fatalf("expected 1 venue, got %d", len(resp.Body.Venues))
	}
	if resp.Body.Venues[0].Name != "Best Venue" {
		t.Errorf("expected name='Best Venue', got %s", resp.Body.Venues[0].Name)
	}
	if resp.Body.Venues[0].Score != 70 {
		t.Errorf("expected score=70, got %d", resp.Body.Venues[0].Score)
	}
}

func TestChartsHandler_ActiveVenues_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetActiveVenuesFn: func(limit int) ([]contracts.ActiveVenue, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetActiveVenuesHandler(context.Background(), &GetActiveVenuesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestChartsHandler_ActiveVenues_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetActiveVenuesFn: func(limit int) ([]contracts.ActiveVenue, error) {
			receivedLimit = limit
			return []contracts.ActiveVenue{}, nil
		},
	})

	_, err := h.GetActiveVenuesHandler(context.Background(), &GetActiveVenuesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

// ============================================================================
// Tests: GetHotReleasesHandler
// ============================================================================

func TestChartsHandler_HotReleases_Success(t *testing.T) {
	now := time.Now()
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetHotReleasesFn: func(limit int) ([]contracts.HotRelease, error) {
			return []contracts.HotRelease{
				{ReleaseID: 1, Title: "Hot Album", Slug: "hot-album", ReleaseDate: &now, ArtistNames: []string{"Artist A", "Artist B"}, BookmarkCount: 42},
			}, nil
		},
	})

	resp, err := h.GetHotReleasesHandler(context.Background(), &GetHotReleasesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(resp.Body.Releases))
	}
	if resp.Body.Releases[0].BookmarkCount != 42 {
		t.Errorf("expected bookmark_count=42, got %d", resp.Body.Releases[0].BookmarkCount)
	}
	if len(resp.Body.Releases[0].ArtistNames) != 2 {
		t.Errorf("expected 2 artist names, got %d", len(resp.Body.Releases[0].ArtistNames))
	}
}

func TestChartsHandler_HotReleases_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetHotReleasesFn: func(limit int) ([]contracts.HotRelease, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetHotReleasesHandler(context.Background(), &GetHotReleasesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestChartsHandler_HotReleases_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetHotReleasesFn: func(limit int) ([]contracts.HotRelease, error) {
			receivedLimit = limit
			return []contracts.HotRelease{}, nil
		},
	})

	_, err := h.GetHotReleasesHandler(context.Background(), &GetHotReleasesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

// ============================================================================
// Tests: GetChartsOverviewHandler
// ============================================================================

func TestChartsHandler_Overview_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetChartsOverviewFn: func() (*contracts.ChartsOverview, error) {
			return &contracts.ChartsOverview{
				TrendingShows:  []contracts.TrendingShow{{ShowID: 1, Title: "Show", TotalAttendance: 5}},
				PopularArtists: []contracts.PopularArtist{{ArtistID: 1, Name: "Artist", Score: 10}},
				ActiveVenues:   []contracts.ActiveVenue{{VenueID: 1, Name: "Venue", Score: 7}},
				HotReleases:    []contracts.HotRelease{{ReleaseID: 1, Title: "Release", ArtistNames: []string{"A"}, BookmarkCount: 3}},
			}, nil
		},
	})

	resp, err := h.GetChartsOverviewHandler(context.Background(), &GetChartsOverviewRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.TrendingShows) != 1 {
		t.Errorf("expected 1 trending show, got %d", len(resp.Body.TrendingShows))
	}
	if len(resp.Body.PopularArtists) != 1 {
		t.Errorf("expected 1 popular artist, got %d", len(resp.Body.PopularArtists))
	}
	if len(resp.Body.ActiveVenues) != 1 {
		t.Errorf("expected 1 active venue, got %d", len(resp.Body.ActiveVenues))
	}
	if len(resp.Body.HotReleases) != 1 {
		t.Errorf("expected 1 hot release, got %d", len(resp.Body.HotReleases))
	}
}

func TestChartsHandler_Overview_Empty(t *testing.T) {
	h := testChartsHandler()

	resp, err := h.GetChartsOverviewHandler(context.Background(), &GetChartsOverviewRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.TrendingShows) != 0 {
		t.Errorf("expected 0 trending shows, got %d", len(resp.Body.TrendingShows))
	}
	if len(resp.Body.PopularArtists) != 0 {
		t.Errorf("expected 0 popular artists, got %d", len(resp.Body.PopularArtists))
	}
}

func TestChartsHandler_Overview_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetChartsOverviewFn: func() (*contracts.ChartsOverview, error) {
			return nil, fmt.Errorf("database error")
		},
	})

	_, err := h.GetChartsOverviewHandler(context.Background(), &GetChartsOverviewRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}
