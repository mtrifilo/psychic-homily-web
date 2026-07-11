package catalog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
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
				{ShowID: 1, Title: "Big Show", Slug: "big-show", SaveCount: 15, VenueName: "The Venue", City: "Phoenix"},
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
	if resp.Body.Shows[0].SaveCount != 15 {
		t.Errorf("expected save_count=15, got %d", resp.Body.Shows[0].SaveCount)
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
		t.Errorf("expected limit=10 forwarded, got %d", receivedLimit)
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
				TrendingShows:  []contracts.TrendingShow{{ShowID: 1, Title: "Show", SaveCount: 5}},
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

// ============================================================================
// Tests: GetMostActiveArtistsHandler
// ============================================================================

func TestChartsHandler_MostActiveArtists_Success(t *testing.T) {
	lastShow := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostActiveArtistsFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.MostActiveArtist, int, error) {
			if window != contracts.ChartWindowQuarter {
				t.Errorf("expected default window=quarter, got %q", window)
			}
			if limit != 10 {
				t.Errorf("expected limit=10 forwarded, got %d", limit)
			}
			return []contracts.MostActiveArtist{
				{ArtistID: 1, Name: "Busy Band", Slug: "busy-band", City: "Phoenix", State: "AZ",
					ShowCount: 14, HeadlinePct: 50, LastShowDate: &lastShow, LastShowSlug: "a-show", LastShowVenue: "The Venue"},
			}, 1, nil
		},
	})

	resp, err := h.GetMostActiveArtistsHandler(context.Background(), &GetMostActiveArtistsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "quarter" {
		t.Errorf("expected echoed window=quarter, got %q", resp.Body.Window)
	}
	if len(resp.Body.Artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(resp.Body.Artists))
	}
	a := resp.Body.Artists[0]
	if a.Name != "Busy Band" || a.ShowCount != 14 || a.HeadlinePct != 50 || a.LastShowVenue != "The Venue" {
		t.Errorf("unexpected mapping: %+v", a)
	}
}

func TestChartsHandler_MostActiveArtists_WindowPassthrough(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostActiveArtistsFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.MostActiveArtist, int, error) {
			if window != contracts.ChartWindowMonth {
				t.Errorf("expected window=month, got %q", window)
			}
			return []contracts.MostActiveArtist{}, 0, nil
		},
	})

	resp, err := h.GetMostActiveArtistsHandler(context.Background(), &GetMostActiveArtistsRequest{Window: "month"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "month" {
		t.Errorf("expected echoed window=month, got %q", resp.Body.Window)
	}
}

func TestChartsHandler_MostActiveArtists_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostActiveArtistsFn: func(contracts.ChartWindow, int, int) ([]contracts.MostActiveArtist, int, error) {
			return nil, 0, fmt.Errorf("db exploded")
		},
	})

	_, err := h.GetMostActiveArtistsHandler(context.Background(), &GetMostActiveArtistsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_MostActiveArtists_InvalidWindow422 exercises the full huma
// request-validation chain: the `enum` tag on the request struct must 422
// invalid window values BEFORE the handler runs (huma-native validation — the
// project's `validate:` tags are dead, so this asserts the tag actually fires).
func TestChartsHandler_MostActiveArtists_InvalidWindow422(t *testing.T) {
	_, api := humatest.New(t)
	h := testChartsHandler()
	huma.Get(api, "/charts/most-active-artists", h.GetMostActiveArtistsHandler)

	if resp := api.Get("/charts/most-active-artists?window=bogus"); resp.Code != 422 {
		t.Errorf("expected 422 for window=bogus, got %d", resp.Code)
	}
	if resp := api.Get("/charts/most-active-artists?window=all_time"); resp.Code != 200 {
		t.Errorf("expected 200 for window=all_time, got %d", resp.Code)
	}
	if resp := api.Get("/charts/most-active-artists"); resp.Code != 200 {
		t.Errorf("expected 200 for absent window, got %d", resp.Code)
	}
}

// ============================================================================
// Tests: GetBusiestVenuesHandler + GetOpenersToWatchHandler
// ============================================================================

func TestChartsHandler_BusiestVenues_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetBusiestVenuesFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.BusiestVenue, int, error) {
			if window != contracts.ChartWindowQuarter {
				t.Errorf("expected default window=quarter, got %q", window)
			}
			if limit != 10 {
				t.Errorf("expected limit=10 forwarded, got %d", limit)
			}
			return []contracts.BusiestVenue{
				{VenueID: 1, Name: "Empty Bottle", Slug: "empty-bottle", City: "Chicago", State: "IL", ShowCount: 41, Rank: 1},
			}, 9, nil
		},
	})

	resp, err := h.GetBusiestVenuesHandler(context.Background(), &GetBusiestVenuesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "quarter" {
		t.Errorf("expected echoed window=quarter, got %q", resp.Body.Window)
	}
	if len(resp.Body.Venues) != 1 || resp.Body.Venues[0].ShowCount != 41 || resp.Body.Venues[0].Name != "Empty Bottle" || resp.Body.Venues[0].Rank != 1 {
		t.Errorf("unexpected mapping: %+v", resp.Body.Venues)
	}
	if resp.Body.Total != 9 {
		t.Errorf("expected total=9 echoed, got %d", resp.Body.Total)
	}
}

func TestChartsHandler_BusiestVenues_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetBusiestVenuesFn: func(contracts.ChartWindow, int, int) ([]contracts.BusiestVenue, int, error) {
			return nil, 0, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetBusiestVenuesHandler(context.Background(), &GetBusiestVenuesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestChartsHandler_OpenersToWatch_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetOpenersToWatchFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.OpenerToWatch, int, error) {
			if window != contracts.ChartWindowMonth {
				t.Errorf("expected window=month, got %q", window)
			}
			return []contracts.OpenerToWatch{
				{ArtistID: 2, Name: "Dizzy Mavis", Slug: "dizzy-mavis", City: "Phoenix", State: "AZ", SupportSlotCount: 11, Rank: 4},
			}, 17, nil
		},
	})

	resp, err := h.GetOpenersToWatchHandler(context.Background(), &GetOpenersToWatchRequest{Window: "month"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "month" {
		t.Errorf("expected echoed window=month, got %q", resp.Body.Window)
	}
	if len(resp.Body.Artists) != 1 || resp.Body.Artists[0].SupportSlotCount != 11 || resp.Body.Artists[0].Rank != 4 {
		t.Errorf("unexpected mapping: %+v", resp.Body.Artists)
	}
	if resp.Body.Total != 17 {
		t.Errorf("expected total=17 echoed, got %d", resp.Body.Total)
	}
}

func TestChartsHandler_OpenersToWatch_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetOpenersToWatchFn: func(contracts.ChartWindow, int, int) ([]contracts.OpenerToWatch, int, error) {
			return nil, 0, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetOpenersToWatchHandler(context.Background(), &GetOpenersToWatchRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_VenueOpenerEndpoints_InvalidWindow422 exercises the huma
// enum-tag validation chain for both new endpoints (per the dead-validate-tag
// gotcha: assert the 422, don't trust the tag).
func TestChartsHandler_VenueOpenerEndpoints_InvalidWindow422(t *testing.T) {
	_, api := humatest.New(t)
	h := testChartsHandler()
	huma.Get(api, "/charts/busiest-venues", h.GetBusiestVenuesHandler)
	huma.Get(api, "/charts/openers-to-watch", h.GetOpenersToWatchHandler)

	for _, path := range []string{"/charts/busiest-venues", "/charts/openers-to-watch"} {
		if resp := api.Get(path + "?window=bogus"); resp.Code != 422 {
			t.Errorf("%s: expected 422 for window=bogus, got %d", path, resp.Code)
		}
		if resp := api.Get(path + "?window=all_time"); resp.Code != 200 {
			t.Errorf("%s: expected 200 for window=all_time, got %d", path, resp.Code)
		}
	}
}

// ============================================================================
// Tests: GetOnTheRadioArtistsHandler
// ============================================================================

func TestChartsHandler_OnTheRadio_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetOnTheRadioArtistsFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.OnTheRadioArtist, int, error) {
			if window != contracts.ChartWindowQuarter {
				t.Errorf("expected default window=quarter, got %q", window)
			}
			if limit != 10 {
				t.Errorf("expected limit=10 forwarded, got %d", limit)
			}
			return []contracts.OnTheRadioArtist{
				{ArtistID: 3, Name: "Airwave Act", Slug: "airwave-act", City: "Seattle", State: "WA", PlayCount: 42, StationCount: 2, IsNew: true, Rank: 2},
			}, 33, nil
		},
	})

	resp, err := h.GetOnTheRadioArtistsHandler(context.Background(), &GetOnTheRadioArtistsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "quarter" {
		t.Errorf("expected echoed window=quarter, got %q", resp.Body.Window)
	}
	if len(resp.Body.Artists) != 1 {
		t.Fatalf("expected 1 artist, got %d", len(resp.Body.Artists))
	}
	a := resp.Body.Artists[0]
	if a.Name != "Airwave Act" || a.PlayCount != 42 || a.StationCount != 2 || !a.IsNew || a.Rank != 2 {
		t.Errorf("unexpected mapping: %+v", a)
	}
	if resp.Body.Total != 33 {
		t.Errorf("expected total=33 echoed, got %d", resp.Body.Total)
	}
}

func TestChartsHandler_OnTheRadio_WindowPassthrough(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetOnTheRadioArtistsFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.OnTheRadioArtist, int, error) {
			if window != contracts.ChartWindowMonth {
				t.Errorf("expected window=month, got %q", window)
			}
			return []contracts.OnTheRadioArtist{}, 0, nil
		},
	})

	resp, err := h.GetOnTheRadioArtistsHandler(context.Background(), &GetOnTheRadioArtistsRequest{Window: "month"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "month" {
		t.Errorf("expected echoed window=month, got %q", resp.Body.Window)
	}
}

func TestChartsHandler_OnTheRadio_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetOnTheRadioArtistsFn: func(contracts.ChartWindow, int, int) ([]contracts.OnTheRadioArtist, int, error) {
			return nil, 0, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetOnTheRadioArtistsHandler(context.Background(), &GetOnTheRadioArtistsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_OnTheRadio_InvalidWindow422 exercises the huma enum-tag
// validation chain (per the dead-validate-tag gotcha: assert the 422, don't
// trust the tag).
func TestChartsHandler_OnTheRadio_InvalidWindow422(t *testing.T) {
	_, api := humatest.New(t)
	h := testChartsHandler()
	huma.Get(api, "/charts/on-the-radio", h.GetOnTheRadioArtistsHandler)

	if resp := api.Get("/charts/on-the-radio?window=bogus"); resp.Code != 422 {
		t.Errorf("expected 422 for window=bogus, got %d", resp.Code)
	}
	if resp := api.Get("/charts/on-the-radio?window=all_time"); resp.Code != 200 {
		t.Errorf("expected 200 for window=all_time, got %d", resp.Code)
	}
	if resp := api.Get("/charts/on-the-radio"); resp.Code != 200 {
		t.Errorf("expected 200 for absent window, got %d", resp.Code)
	}
}

// ============================================================================
// Tests: GetMostAnticipatedShowsHandler
// ============================================================================

func TestChartsHandler_MostAnticipated_RankedMapping(t *testing.T) {
	three := 3
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostAnticipatedShowsFn: func(limit, offset int) (*contracts.MostAnticipatedShows, error) {
			if limit != 10 {
				t.Errorf("expected limit=10 forwarded, got %d", limit)
			}
			return &contracts.MostAnticipatedShows{
				Mode: contracts.MostAnticipatedModeRanked,
				Shows: []contracts.MostAnticipatedShow{
					{ShowID: 1, Title: "Hot Show", Slug: "hot-show", VenueName: "The Spot", City: "Phoenix", ArtistNames: []string{"Band"}, SaveCount: &three},
				},
			}, nil
		},
	})

	resp, err := h.GetMostAnticipatedShowsHandler(context.Background(), &GetMostAnticipatedShowsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Mode != "ranked" {
		t.Errorf("expected mode=ranked, got %q", resp.Body.Mode)
	}
	if len(resp.Body.Shows) != 1 || resp.Body.Shows[0].SaveCount == nil || *resp.Body.Shows[0].SaveCount != 3 {
		t.Errorf("unexpected mapping: %+v", resp.Body.Shows)
	}
}

func TestChartsHandler_MostAnticipated_FallbackMapping(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostAnticipatedShowsFn: func(limit, offset int) (*contracts.MostAnticipatedShows, error) {
			return &contracts.MostAnticipatedShows{
				Mode: contracts.MostAnticipatedModeSoonestUpcoming,
				Shows: []contracts.MostAnticipatedShow{
					{ShowID: 2, Title: "Soon Show", Slug: "soon-show", ArtistNames: []string{}},
				},
			}, nil
		},
	})

	resp, err := h.GetMostAnticipatedShowsHandler(context.Background(), &GetMostAnticipatedShowsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Mode != "soonest_upcoming" {
		t.Errorf("expected mode=soonest_upcoming, got %q", resp.Body.Mode)
	}
	if len(resp.Body.Shows) != 1 || resp.Body.Shows[0].SaveCount != nil {
		t.Errorf("fallback rows must carry no save count: %+v", resp.Body.Shows)
	}
}

func TestChartsHandler_MostAnticipated_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostAnticipatedShowsFn: func(int, int) (*contracts.MostAnticipatedShows, error) {
			return nil, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetMostAnticipatedShowsHandler(context.Background(), &GetMostAnticipatedShowsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_MostAnticipated_WireShape asserts through the full huma
// serialization chain that save_count is PRESENT on ranked rows and ABSENT —
// not null, absent — on fallback rows (the fail-closed dual-shape contract
// the frontend discriminates on).
func TestChartsHandler_MostAnticipated_WireShape(t *testing.T) {
	seven := 7
	payloads := map[string]*contracts.MostAnticipatedShows{
		"ranked": {
			Mode:  contracts.MostAnticipatedModeRanked,
			Shows: []contracts.MostAnticipatedShow{{ShowID: 1, Title: "Ranked", ArtistNames: []string{}, SaveCount: &seven}},
		},
		"fallback": {
			Mode:  contracts.MostAnticipatedModeSoonestUpcoming,
			Shows: []contracts.MostAnticipatedShow{{ShowID: 2, Title: "Fallback", ArtistNames: []string{}}},
		},
	}
	current := "ranked"
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostAnticipatedShowsFn: func(int, int) (*contracts.MostAnticipatedShows, error) {
			return payloads[current], nil
		},
	})
	_, api := humatest.New(t)
	huma.Get(api, "/charts/most-anticipated", h.GetMostAnticipatedShowsHandler)

	resp := api.Get("/charts/most-anticipated")
	if resp.Code != 200 {
		t.Fatalf("ranked: expected 200, got %d", resp.Code)
	}
	if body := resp.Body.String(); !strings.Contains(body, `"save_count":7`) {
		t.Errorf("ranked wire payload must include save_count: %s", body)
	}

	current = "fallback"
	resp = api.Get("/charts/most-anticipated")
	if resp.Code != 200 {
		t.Fatalf("fallback: expected 200, got %d", resp.Code)
	}
	if body := resp.Body.String(); strings.Contains(body, "save_count") {
		t.Errorf("fallback wire payload must omit save_count entirely: %s", body)
	}
}

func TestChartsHandler_MostAnticipated_ExplicitLimitForwarded(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostAnticipatedShowsFn: func(limit, offset int) (*contracts.MostAnticipatedShows, error) {
			receivedLimit = limit
			return &contracts.MostAnticipatedShows{Mode: contracts.MostAnticipatedModeRanked, Shows: []contracts.MostAnticipatedShow{}}, nil
		},
	})
	if _, err := h.GetMostAnticipatedShowsHandler(context.Background(), &GetMostAnticipatedShowsRequest{Limit: 5}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 5 {
		t.Errorf("expected limit=5 forwarded, got %d", receivedLimit)
	}
}

// ============================================================================
// Tests: GetNewReleasesHandler
// ============================================================================

func TestChartsHandler_NewReleases_Success(t *testing.T) {
	released := "2026-07-03"
	addedAt := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetNewReleasesFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.NewRelease, int, error) {
			if window != contracts.ChartWindowQuarter {
				t.Errorf("expected default window=quarter, got %q", window)
			}
			if limit != 10 {
				t.Errorf("expected limit=10 forwarded, got %d", limit)
			}
			return []contracts.NewRelease{
				{ReleaseID: 9, Title: "Fresh Wax", Slug: "fresh-wax", ReleaseType: "lp", ReleaseDate: &released, AddedAt: addedAt, ArtistNames: []string{"Band"}, LabelNames: []string{"Sub Rosa"}},
			}, 1, nil
		},
	})

	resp, err := h.GetNewReleasesHandler(context.Background(), &GetNewReleasesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "quarter" {
		t.Errorf("expected echoed window=quarter, got %q", resp.Body.Window)
	}
	if len(resp.Body.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(resp.Body.Releases))
	}
	r := resp.Body.Releases[0]
	if r.Title != "Fresh Wax" || r.ReleaseType != "lp" || r.ReleaseDate == nil || len(r.LabelNames) != 1 {
		t.Errorf("unexpected mapping: %+v", r)
	}
}

func TestChartsHandler_NewReleases_WindowAndLimitPassthrough(t *testing.T) {
	var gotWindow contracts.ChartWindow
	var gotLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetNewReleasesFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.NewRelease, int, error) {
			gotWindow, gotLimit = window, limit
			return []contracts.NewRelease{}, 0, nil
		},
	})

	resp, err := h.GetNewReleasesHandler(context.Background(), &GetNewReleasesRequest{Window: "month", Limit: 7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotWindow != contracts.ChartWindowMonth || gotLimit != 7 {
		t.Errorf("expected month/7 forwarded, got %q/%d", gotWindow, gotLimit)
	}
	if resp.Body.Window != "month" {
		t.Errorf("expected echoed window=month, got %q", resp.Body.Window)
	}
}

func TestChartsHandler_NewReleases_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetNewReleasesFn: func(contracts.ChartWindow, int, int) ([]contracts.NewRelease, int, error) {
			return nil, 0, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetNewReleasesHandler(context.Background(), &GetNewReleasesRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_NewReleases_InvalidWindow422 exercises the huma enum-tag
// validation chain (dead-validate-tag gotcha: assert the 422, don't trust the
// tag).
func TestChartsHandler_NewReleases_InvalidWindow422(t *testing.T) {
	_, api := humatest.New(t)
	h := testChartsHandler()
	huma.Get(api, "/charts/new-releases", h.GetNewReleasesHandler)

	if resp := api.Get("/charts/new-releases?window=bogus"); resp.Code != 422 {
		t.Errorf("expected 422 for window=bogus, got %d", resp.Code)
	}
	if resp := api.Get("/charts/new-releases?window=all_time"); resp.Code != 200 {
		t.Errorf("expected 200 for window=all_time, got %d", resp.Code)
	}
	if resp := api.Get("/charts/new-releases"); resp.Code != 200 {
		t.Errorf("expected 200 for absent window, got %d", resp.Code)
	}
}

// ============================================================================
// Tests: GetChartsSummaryHandler + GetFreshlyAddedHandler
// ============================================================================

func TestChartsHandler_Summary_Success(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetChartsSummaryFn: func(window contracts.ChartWindow) (*contracts.ChartsSummary, error) {
			if window != contracts.ChartWindowQuarter {
				t.Errorf("expected default window=quarter, got %q", window)
			}
			return &contracts.ChartsSummary{ShowsAdded: 142, NewArtists: 87, NewReleases: 31, RadioPlays: 418, ActiveScenes: 9}, nil
		},
	})

	resp, err := h.GetChartsSummaryHandler(context.Background(), &GetChartsSummaryRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "quarter" {
		t.Errorf("expected echoed window=quarter, got %q", resp.Body.Window)
	}
	b := resp.Body
	if b.ShowsAdded != 142 || b.NewArtists != 87 || b.NewReleases != 31 || b.RadioPlays != 418 || b.ActiveScenes != 9 {
		t.Errorf("unexpected mapping: %+v", b)
	}
}

func TestChartsHandler_Summary_WindowPassthrough(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetChartsSummaryFn: func(window contracts.ChartWindow) (*contracts.ChartsSummary, error) {
			if window != contracts.ChartWindowMonth {
				t.Errorf("expected window=month, got %q", window)
			}
			return &contracts.ChartsSummary{}, nil
		},
	})
	resp, err := h.GetChartsSummaryHandler(context.Background(), &GetChartsSummaryRequest{Window: "month"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Window != "month" {
		t.Errorf("expected echoed window=month, got %q", resp.Body.Window)
	}
}

func TestChartsHandler_Summary_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetChartsSummaryFn: func(contracts.ChartWindow) (*contracts.ChartsSummary, error) {
			return nil, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetChartsSummaryHandler(context.Background(), &GetChartsSummaryRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_Summary_InvalidWindow422 exercises the huma enum-tag
// validation chain (dead-validate-tag gotcha: assert the 422).
func TestChartsHandler_Summary_InvalidWindow422(t *testing.T) {
	_, api := humatest.New(t)
	h := testChartsHandler()
	huma.Get(api, "/charts/summary", h.GetChartsSummaryHandler)

	if resp := api.Get("/charts/summary?window=bogus"); resp.Code != 422 {
		t.Errorf("expected 422 for window=bogus, got %d", resp.Code)
	}
	if resp := api.Get("/charts/summary?window=all_time"); resp.Code != 200 {
		t.Errorf("expected 200 for window=all_time, got %d", resp.Code)
	}
}

func TestChartsHandler_FreshlyAdded_Success(t *testing.T) {
	added := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetFreshlyAddedFn: func(limit int) ([]contracts.FreshlyAddedItem, error) {
			if limit != 20 {
				t.Errorf("expected default limit=20, got %d", limit)
			}
			return []contracts.FreshlyAddedItem{
				{EntityType: "release", EntityID: 4, Name: "Fresh Wax", Slug: "fresh-wax", AddedAt: added},
				{EntityType: "artist", EntityID: 2, Name: "New Band", Slug: "new-band", AddedAt: added.Add(-time.Hour)},
			}, nil
		},
	})

	resp, err := h.GetFreshlyAddedHandler(context.Background(), &GetFreshlyAddedRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Items) != 2 || resp.Body.Items[0].EntityType != "release" || resp.Body.Items[1].Name != "New Band" {
		t.Errorf("unexpected mapping: %+v", resp.Body.Items)
	}
}

func TestChartsHandler_FreshlyAdded_LimitForwarded(t *testing.T) {
	var receivedLimit int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetFreshlyAddedFn: func(limit int) ([]contracts.FreshlyAddedItem, error) {
			receivedLimit = limit
			return []contracts.FreshlyAddedItem{}, nil
		},
	})
	if _, err := h.GetFreshlyAddedHandler(context.Background(), &GetFreshlyAddedRequest{Limit: 5}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 5 {
		t.Errorf("expected limit=5 forwarded, got %d", receivedLimit)
	}
}

func TestChartsHandler_FreshlyAdded_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetFreshlyAddedFn: func(int) ([]contracts.FreshlyAddedItem, error) {
			return nil, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetFreshlyAddedHandler(context.Background(), &GetFreshlyAddedRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetPersonalChartsStatsHandler
// ============================================================================

// personalStatsCtx returns a context carrying an authenticated user, the way
// the Protected group's JWT middleware would have populated it.
func personalStatsCtx(userID uint) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, &authm.User{ID: userID})
}

func TestChartsHandler_PersonalStats_Success(t *testing.T) {
	first := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	var receivedUserID uint
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetPersonalChartsStatsFn: func(userID uint) (*contracts.PersonalChartsStats, error) {
			receivedUserID = userID
			return &contracts.PersonalChartsStats{
				SavedShows:      17,
				ArtistsFollowed: 5,
				TopVenue:        &contracts.PersonalTopVenue{VenueID: 3, Name: "Valley Bar", Slug: "valley-bar", SavedShowCount: 6},
				FirstActivityAt: &first,
			}, nil
		},
	})

	resp, err := h.GetPersonalChartsStatsHandler(personalStatsCtx(42), &GetPersonalChartsStatsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedUserID != 42 {
		t.Errorf("expected the context user's id (42) forwarded, got %d", receivedUserID)
	}
	b := resp.Body
	if b.SavedShows != 17 || b.ArtistsFollowed != 5 {
		t.Errorf("unexpected counts: %+v", b)
	}
	if b.TopVenue == nil || b.TopVenue.Name != "Valley Bar" || b.TopVenue.SavedShowCount != 6 {
		t.Errorf("unexpected top venue mapping: %+v", b.TopVenue)
	}
	if b.FirstActivityAt == nil || !b.FirstActivityAt.Equal(first) {
		t.Errorf("unexpected first activity: %v", b.FirstActivityAt)
	}
}

// TestChartsHandler_PersonalStats_Unauthenticated401 asserts the anonymous
// path is a 401 — not a 500 and not an empty 200 (the ticket's explicit AC).
func TestChartsHandler_PersonalStats_Unauthenticated401(t *testing.T) {
	h := testChartsHandler()
	_, err := h.GetPersonalChartsStatsHandler(context.Background(), &GetPersonalChartsStatsRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestChartsHandler_PersonalStats_ServiceError(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetPersonalChartsStatsFn: func(uint) (*contracts.PersonalChartsStats, error) {
			return nil, fmt.Errorf("db exploded")
		},
	})
	_, err := h.GetPersonalChartsStatsHandler(personalStatsCtx(42), &GetPersonalChartsStatsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// TestChartsHandler_PersonalStats_WireShape asserts the JSON contract through
// the full huma chain: snake_case field names, and the zeros shape carrying
// EXPLICIT nulls for top_venue / first_activity_at (the frontend keys its
// "start marking shows" nudge off this shape, so absent-vs-null matters).
func TestChartsHandler_PersonalStats_WireShape(t *testing.T) {
	stats := &contracts.PersonalChartsStats{} // zeros shape (new user)
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetPersonalChartsStatsFn: func(uint) (*contracts.PersonalChartsStats, error) {
			return stats, nil
		},
	})

	_, api := humatest.New(t)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithValue(ctx, middleware.UserContextKey, &authm.User{ID: 42}))
	})
	huma.Get(api, "/charts/me", h.GetPersonalChartsStatsHandler)

	resp := api.Get("/charts/me")
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if cc := resp.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("per-user response must be Cache-Control: no-store, got %q", cc)
	}
	body := resp.Body.String()
	for _, want := range []string{`"saved_shows":0`, `"artists_followed":0`, `"top_venue":null`, `"first_activity_at":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("zeros wire payload must include %s: %s", want, body)
		}
	}

	first := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	stats = &contracts.PersonalChartsStats{
		SavedShows:      3,
		ArtistsFollowed: 1,
		TopVenue:        &contracts.PersonalTopVenue{VenueID: 7, Name: "Trunk Space", Slug: "trunk-space", SavedShowCount: 2},
		FirstActivityAt: &first,
	}
	resp = api.Get("/charts/me")
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body = resp.Body.String()
	for _, want := range []string{`"saved_shows":3`, `"saved_show_count":2`, `"slug":"trunk-space"`, `"first_activity_at":"2026-05-01T12:00:00Z"`} {
		if !strings.Contains(body, want) {
			t.Errorf("populated wire payload must include %s: %s", want, body)
		}
	}
}

// ============================================================================
// Tests: module pagination wiring (PSY-1405)
// ============================================================================

func TestChartsHandler_MostActiveArtists_PaginationForwardingAndTotal(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostActiveArtistsFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.MostActiveArtist, int, error) {
			if limit != 50 || offset != 100 {
				t.Errorf("expected limit=50 offset=100 forwarded, got %d/%d", limit, offset)
			}
			return []contracts.MostActiveArtist{
				{ArtistID: 9, Name: "Deep Page Band", Rank: 101, ShowCount: 4},
			}, 123, nil
		},
	})

	resp, err := h.GetMostActiveArtistsHandler(context.Background(), &GetMostActiveArtistsRequest{Limit: 50, Offset: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 123 {
		t.Errorf("expected total=123 echoed, got %d", resp.Body.Total)
	}
	if len(resp.Body.Artists) != 1 || resp.Body.Artists[0].Rank != 101 {
		t.Errorf("expected rank mapped through, got %+v", resp.Body.Artists)
	}
}

// TestChartsHandler_ModuleOffsetLimit_Validation422 exercises the huma
// constraint tags through the full chain (dead-validate-tag gotcha: assert
// the 422, don't trust the tag): offset [0,10000], limit [1,100].
func TestChartsHandler_ModuleOffsetLimit_Validation422(t *testing.T) {
	_, api := humatest.New(t)
	h := testChartsHandler()
	huma.Get(api, "/charts/most-active-artists", h.GetMostActiveArtistsHandler)

	for _, tc := range []struct {
		query string
		want  int
	}{
		{"?offset=-1", 422},
		{"?offset=20000", 422},
		{"?offset=10000", 200},
		{"?limit=100", 200},
		{"?limit=101", 422},
		{"?limit=100&offset=9999", 200},
	} {
		if resp := api.Get("/charts/most-active-artists" + tc.query); resp.Code != tc.want {
			t.Errorf("%s: expected %d, got %d", tc.query, tc.want, resp.Code)
		}
	}
}

// TestChartsHandler_NewReleases_WireTotalAndRank asserts total and rank reach
// the wire through the full huma chain (NewReleaseResponse is a direct struct
// conversion — this pins the new fields' snake_case names).
func TestChartsHandler_NewReleases_WireTotalAndRank(t *testing.T) {
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetNewReleasesFn: func(window contracts.ChartWindow, limit, offset int) ([]contracts.NewRelease, int, error) {
			return []contracts.NewRelease{
				{ReleaseID: 5, Title: "Ranked Wax", ArtistNames: []string{}, LabelNames: []string{}, Rank: 11},
			}, 42, nil
		},
	})
	_, api := humatest.New(t)
	huma.Get(api, "/charts/new-releases", h.GetNewReleasesHandler)

	resp := api.Get("/charts/new-releases?offset=10")
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	for _, want := range []string{`"total":42`, `"rank":11`} {
		if !strings.Contains(body, want) {
			t.Errorf("wire payload must include %s: %s", want, body)
		}
	}
}

// TestChartsHandler_MostAnticipated_WireRankOmittedInFallback pins the
// rank-omission rule on the wire: ranked rows carry rank, fallback rows omit
// the key entirely (like save_count).
func TestChartsHandler_MostAnticipated_WireRankOmittedInFallback(t *testing.T) {
	three, one := 3, 1
	payloads := map[string]*contracts.MostAnticipatedShows{
		"ranked": {
			Mode:  contracts.MostAnticipatedModeRanked,
			Total: 6,
			Shows: []contracts.MostAnticipatedShow{{ShowID: 1, Title: "Ranked", ArtistNames: []string{}, SaveCount: &three, Rank: &one}},
		},
		"fallback": {
			Mode:  contracts.MostAnticipatedModeSoonestUpcoming,
			Total: 2,
			Shows: []contracts.MostAnticipatedShow{{ShowID: 2, Title: "Fallback", ArtistNames: []string{}}},
		},
	}
	current := "ranked"
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostAnticipatedShowsFn: func(int, int) (*contracts.MostAnticipatedShows, error) {
			return payloads[current], nil
		},
	})
	_, api := humatest.New(t)
	huma.Get(api, "/charts/most-anticipated", h.GetMostAnticipatedShowsHandler)

	resp := api.Get("/charts/most-anticipated")
	if body := resp.Body.String(); !strings.Contains(body, `"rank":1`) || !strings.Contains(body, `"total":6`) {
		t.Errorf("ranked wire payload must include rank + total: %s", body)
	}

	current = "fallback"
	resp = api.Get("/charts/most-anticipated")
	if body := resp.Body.String(); strings.Contains(body, `"rank"`) {
		t.Errorf("fallback wire payload must omit rank entirely: %s", body)
	}
}

// TestChartsHandler_ModuleTagDefaults proves the huma default tags own the
// module defaults (limit 10, offset 0) through the full request chain —
// direct handler calls bypass tag defaults, so this is the one place the
// contract-layer default is pinned (dead-validate-tag gotcha: assert it).
func TestChartsHandler_ModuleTagDefaults(t *testing.T) {
	var gotLimit, gotOffset int
	h := NewChartsHandler(&testhelpers.MockChartsService{
		GetMostActiveArtistsFn: func(_ contracts.ChartWindow, limit, offset int) ([]contracts.MostActiveArtist, int, error) {
			gotLimit, gotOffset = limit, offset
			return []contracts.MostActiveArtist{}, 0, nil
		},
	})
	_, api := humatest.New(t)
	huma.Get(api, "/charts/most-active-artists", h.GetMostActiveArtistsHandler)

	if resp := api.Get("/charts/most-active-artists"); resp.Code != 200 {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if gotLimit != 10 || gotOffset != 0 {
		t.Errorf("expected tag defaults limit=10 offset=0, got %d/%d", gotLimit, gotOffset)
	}
}

// TestChartsHandler_PublicCacheControl pins the public max-age headers —
// deliberately a fraction of the server TTLs since the two layers stack.
func TestChartsHandler_PublicCacheControl(t *testing.T) {
	h := testChartsHandler()
	_, api := humatest.New(t)
	huma.Get(api, "/charts/most-active-artists", h.GetMostActiveArtistsHandler)
	huma.Get(api, "/charts/summary", h.GetChartsSummaryHandler)
	huma.Get(api, "/charts/freshly-added", h.GetFreshlyAddedHandler)

	if cc := api.Get("/charts/most-active-artists").Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("module endpoints must be public max-age=60 (a fraction of the server TTL), got %q", cc)
	}
	if cc := api.Get("/charts/summary").Header().Get("Cache-Control"); cc != "public, max-age=30" {
		t.Errorf("summary must be public max-age=30, got %q", cc)
	}
	if cc := api.Get("/charts/freshly-added").Header().Get("Cache-Control"); cc != "public, max-age=30" {
		t.Errorf("ticker must be public max-age=30, got %q", cc)
	}
}
