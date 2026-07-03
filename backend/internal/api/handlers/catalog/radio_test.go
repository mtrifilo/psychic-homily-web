package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

type mockArtistSlugResolver struct {
	GetArtistBySlugFn func(slug string) (*contracts.ArtistDetailResponse, error)
}

func (m *mockArtistSlugResolver) GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
	if m.GetArtistBySlugFn != nil {
		return m.GetArtistBySlugFn(slug)
	}
	return nil, fmt.Errorf("artist not found")
}

type mockReleaseSlugResolver struct {
	GetReleaseBySlugFn func(slug string) (*contracts.ReleaseDetailResponse, error)
}

func (m *mockReleaseSlugResolver) GetReleaseBySlug(slug string) (*contracts.ReleaseDetailResponse, error) {
	if m.GetReleaseBySlugFn != nil {
		return m.GetReleaseBySlugFn(slug)
	}
	return nil, fmt.Errorf("release not found")
}

func testRadioHandler(radio *testhelpers.MockRadioService) *RadioHandler {
	return NewRadioHandler(radio, &mockArtistSlugResolver{}, &mockReleaseSlugResolver{}, nil)
}

func testRadioHandlerWithResolvers(radio *testhelpers.MockRadioService, artist *mockArtistSlugResolver, release *mockReleaseSlugResolver) *RadioHandler {
	return NewRadioHandler(radio, artist, release, nil)
}

func radioAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true, EmailVerified: true})
}

// ============================================================================
// ListRadioStationsHandler Tests
// ============================================================================

func TestListRadioStations_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ListStationsFn: func(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
			return []*contracts.RadioStationListResponse{
				{ID: 1, Name: "KEXP", Slug: "kexp", BroadcastType: "both", IsActive: true, ShowCount: 5},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.ListRadioStationsHandler(context.Background(), &ListRadioStationsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
	if resp.Body.Stations[0].Name != "KEXP" {
		t.Errorf("expected KEXP, got %s", resp.Body.Stations[0].Name)
	}
}

func TestListRadioStations_FilterActive(t *testing.T) {
	var capturedFilters map[string]interface{}
	mock := &testhelpers.MockRadioService{
		ListStationsFn: func(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
			capturedFilters = filters
			return []*contracts.RadioStationListResponse{}, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.ListRadioStationsHandler(context.Background(), &ListRadioStationsRequest{IsActive: "true"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilters["is_active"] != true {
		t.Errorf("expected is_active=true filter, got %v", capturedFilters)
	}
}

func TestListRadioStations_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ListStationsFn: func(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.ListRadioStationsHandler(context.Background(), &ListRadioStationsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetRadioStationHandler Tests
// ============================================================================

func TestGetRadioStation_BySlug(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetStationBySlugFn: func(slug string) (*contracts.RadioStationDetailResponse, error) {
			return &contracts.RadioStationDetailResponse{ID: 1, Name: "KEXP", Slug: "kexp", BroadcastType: "both"}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioStationHandler(context.Background(), &GetRadioStationRequest{Slug: "kexp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "KEXP" {
		t.Errorf("expected KEXP, got %s", resp.Body.Name)
	}
}

func TestGetRadioStation_ByID(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetStationFn: func(stationID uint) (*contracts.RadioStationDetailResponse, error) {
			return &contracts.RadioStationDetailResponse{ID: stationID, Name: "KEXP", Slug: "kexp", BroadcastType: "both"}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioStationHandler(context.Background(), &GetRadioStationRequest{Slug: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID 1, got %d", resp.Body.ID)
	}
}

func TestGetRadioStation_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetStationBySlugFn: func(slug string) (*contracts.RadioStationDetailResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioStationHandler(context.Background(), &GetRadioStationRequest{Slug: "nonexistent"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// ListRadioShowsHandler Tests
// ============================================================================

func TestListRadioShows_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ListShowsFn: func(stationID uint, sortBy string) ([]*contracts.RadioShowListResponse, error) {
			return []*contracts.RadioShowListResponse{
				{ID: 1, StationID: stationID, Name: "Morning Show", Slug: "morning-show"},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.ListRadioShowsHandler(context.Background(), &ListRadioShowsRequest{StationID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
}

func TestListRadioShows_MissingStationID(t *testing.T) {
	mock := &testhelpers.MockRadioService{}
	h := testRadioHandler(mock)
	_, err := h.ListRadioShowsHandler(context.Background(), &ListRadioShowsRequest{StationID: 0})
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// GetRadioShowHandler Tests
// ============================================================================

func TestGetRadioShow_BySlug(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show", StationID: 1}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioShowHandler(context.Background(), &GetRadioShowRequest{Slug: "morning-show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Morning Show" {
		t.Errorf("expected Morning Show, got %s", resp.Body.Name)
	}
}

func TestGetRadioShow_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowHandler(context.Background(), &GetRadioShowRequest{Slug: "nonexistent"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioShowEpisodesHandler Tests
// ============================================================================

func TestGetRadioShowEpisodes_Success(t *testing.T) {
	now := time.Now()
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetEpisodesFn: func(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error) {
			return []*contracts.RadioEpisodeResponse{
				{ID: 1, ShowID: showID, AirDate: "2026-03-15", PlayCount: 25, CreatedAt: now},
			}, 1, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioShowEpisodesHandler(context.Background(), &GetRadioShowEpisodesRequest{Slug: "morning-show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Body.Total)
	}
}

func TestGetRadioShowEpisodes_DefaultLimit(t *testing.T) {
	var capturedLimit int
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetEpisodesFn: func(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error) {
			capturedLimit = limit
			return []*contracts.RadioEpisodeResponse{}, 0, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowEpisodesHandler(context.Background(), &GetRadioShowEpisodesRequest{Slug: "morning-show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 20 {
		t.Errorf("expected default limit 20, got %d", capturedLimit)
	}
}

func TestGetRadioShowEpisodes_ShowNotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowEpisodesHandler(context.Background(), &GetRadioShowEpisodesRequest{Slug: "nonexistent"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioStationNowPlayingHandler Tests (PSY-1022)
// ============================================================================

func TestGetRadioStationNowPlaying_Success(t *testing.T) {
	showName := "The Morning Show"
	var capturedStationID uint
	mock := &testhelpers.MockRadioService{
		ResolveStationIDBySlugFn: func(slug string) (uint, error) { return 7, nil },
		GetStationNowPlayingFn: func(stationID uint) (*contracts.RadioNowPlayingResponse, error) {
			capturedStationID = stationID
			return &contracts.RadioNowPlayingResponse{
				Source:        contracts.NowPlayingSourceLive,
				OnAir:         true,
				ShowName:      &showName,
				Show:          &contracts.RadioNowPlayingShowRef{ID: 3, Name: showName, Slug: "the-morning-show"},
				RecentArtists: []contracts.RadioEpisodePreviewArtist{},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioStationNowPlayingHandler(context.Background(), &GetRadioStationNowPlayingRequest{Slug: "kexp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStationID != 7 {
		t.Errorf("expected resolved station id 7, got %d", capturedStationID)
	}
	if resp.Body.Source != contracts.NowPlayingSourceLive || !resp.Body.OnAir {
		t.Errorf("expected live/on-air payload, got %+v", resp.Body)
	}
	if resp.Body.Show == nil || resp.Body.Show.Slug != "the-morning-show" {
		t.Errorf("expected matched show, got %+v", resp.Body.Show)
	}
}

func TestGetRadioStationNowPlaying_StationNotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ResolveStationIDBySlugFn: func(slug string) (uint, error) {
			return 0, apperrors.ErrRadioStationNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioStationNowPlayingHandler(context.Background(), &GetRadioStationNowPlayingRequest{Slug: "nonexistent"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetRadioStationNowPlaying_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ResolveStationIDBySlugFn: func(slug string) (uint, error) { return 7, nil },
		GetStationNowPlayingFn: func(stationID uint) (*contracts.RadioNowPlayingResponse, error) {
			return nil, fmt.Errorf("db down")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioStationNowPlayingHandler(context.Background(), &GetRadioStationNowPlayingRequest{Slug: "kexp"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetRadioEpisodeByDateHandler Tests
// ============================================================================

func TestGetRadioEpisodeByDate_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetEpisodeByShowAndDateFn: func(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error) {
			return &contracts.RadioEpisodeDetailResponse{
				ID:          1,
				ShowID:      showID,
				AirDate:     airDate,
				ShowName:    "Morning Show",
				StationName: "KEXP",
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioEpisodeByDateHandler(context.Background(), &GetRadioEpisodeByDateRequest{Slug: "morning-show", Date: "2026-03-15"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.AirDate != "2026-03-15" {
		t.Errorf("expected air date 2026-03-15, got %s", resp.Body.AirDate)
	}
}

func TestGetRadioEpisodeByDate_InvalidDate(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioEpisodeByDateHandler(context.Background(), &GetRadioEpisodeByDateRequest{Slug: "morning-show", Date: "not-a-date"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetRadioEpisodeByDate_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetEpisodeByShowAndDateFn: func(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error) {
			return nil, apperrors.ErrRadioEpisodeNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioEpisodeByDateHandler(context.Background(), &GetRadioEpisodeByDateRequest{Slug: "morning-show", Date: "2026-03-15"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioShowTopArtistsHandler Tests
// ============================================================================

func TestGetRadioShowTopArtists_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetTopArtistsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
			return []*contracts.RadioTopArtistResponse{
				{ArtistName: "Radiohead", PlayCount: 15, EpisodeCount: 10},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioShowTopArtistsHandler(context.Background(), &GetRadioShowTopArtistsRequest{Slug: "morning-show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
	if resp.Body.Artists[0].ArtistName != "Radiohead" {
		t.Errorf("expected Radiohead, got %s", resp.Body.Artists[0].ArtistName)
	}
}

func TestGetRadioShowTopArtists_DefaultPeriodAndLimit(t *testing.T) {
	var capturedPeriod, capturedLimit int
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetTopArtistsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
			capturedPeriod = periodDays
			capturedLimit = limit
			return []*contracts.RadioTopArtistResponse{}, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowTopArtistsHandler(context.Background(), &GetRadioShowTopArtistsRequest{Slug: "morning-show"})
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

// ============================================================================
// GetRadioShowTopLabelsHandler Tests
// ============================================================================

func TestGetRadioShowTopLabels_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetTopLabelsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
			return []*contracts.RadioTopLabelResponse{
				{LabelName: "Sub Pop", PlayCount: 30},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioShowTopLabelsHandler(context.Background(), &GetRadioShowTopLabelsRequest{Slug: "morning-show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
}

func TestGetRadioShowTopLabels_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		GetTopLabelsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowTopLabelsHandler(context.Background(), &GetRadioShowTopLabelsRequest{Slug: "morning-show"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetArtistRadioPlaysHandler Tests
// ============================================================================

func TestGetArtistRadioPlays_BySlug(t *testing.T) {
	artistMock := &mockArtistSlugResolver{
		GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			return &contracts.ArtistDetailResponse{ID: 42, Name: "Radiohead"}, nil
		},
	}
	radioMock := &testhelpers.MockRadioService{
		GetAsHeardOnForArtistFn: func(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
			return []*contracts.RadioAsHeardOnResponse{
				{StationID: 1, StationName: "KEXP", ShowID: 1, ShowName: "Morning Show", PlayCount: 5},
			}, nil
		},
	}
	h := testRadioHandlerWithResolvers(radioMock, artistMock, &mockReleaseSlugResolver{})
	resp, err := h.GetArtistRadioPlaysHandler(context.Background(), &GetArtistRadioPlaysRequest{Slug: "radiohead"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
}

func TestGetArtistRadioPlays_ByNumericID(t *testing.T) {
	var capturedID uint
	radioMock := &testhelpers.MockRadioService{
		GetAsHeardOnForArtistFn: func(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
			capturedID = artistID
			return []*contracts.RadioAsHeardOnResponse{}, nil
		},
	}
	h := testRadioHandler(radioMock)
	_, err := h.GetArtistRadioPlaysHandler(context.Background(), &GetArtistRadioPlaysRequest{Slug: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedID != 42 {
		t.Errorf("expected artist ID 42, got %d", capturedID)
	}
}

func TestGetArtistRadioPlays_ArtistNotFound(t *testing.T) {
	artistMock := &mockArtistSlugResolver{
		GetArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	radioMock := &testhelpers.MockRadioService{}
	h := testRadioHandlerWithResolvers(radioMock, artistMock, &mockReleaseSlugResolver{})
	_, err := h.GetArtistRadioPlaysHandler(context.Background(), &GetArtistRadioPlaysRequest{Slug: "nonexistent"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// GetReleaseRadioPlaysHandler Tests
// ============================================================================

func TestGetReleaseRadioPlays_BySlug(t *testing.T) {
	releaseMock := &mockReleaseSlugResolver{
		GetReleaseBySlugFn: func(slug string) (*contracts.ReleaseDetailResponse, error) {
			return &contracts.ReleaseDetailResponse{ID: 10, Title: "OK Computer"}, nil
		},
	}
	radioMock := &testhelpers.MockRadioService{
		GetAsHeardOnForReleaseFn: func(releaseID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
			return []*contracts.RadioAsHeardOnResponse{
				{StationID: 1, StationName: "KEXP", ShowID: 1, ShowName: "Morning Show", PlayCount: 3},
			}, nil
		},
	}
	h := testRadioHandlerWithResolvers(radioMock, &mockArtistSlugResolver{}, releaseMock)
	resp, err := h.GetReleaseRadioPlaysHandler(context.Background(), &GetReleaseRadioPlaysRequest{Slug: "ok-computer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
}

func TestGetReleaseRadioPlays_ReleaseNotFound(t *testing.T) {
	releaseMock := &mockReleaseSlugResolver{
		GetReleaseBySlugFn: func(slug string) (*contracts.ReleaseDetailResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	radioMock := &testhelpers.MockRadioService{}
	h := testRadioHandlerWithResolvers(radioMock, &mockArtistSlugResolver{}, releaseMock)
	_, err := h.GetReleaseRadioPlaysHandler(context.Background(), &GetReleaseRadioPlaysRequest{Slug: "nonexistent"})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioNewReleaseRadarHandler Tests
// ============================================================================

func TestGetRadioNewReleaseRadar_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetNewReleaseRadarFn: func(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
			return []*contracts.RadioNewReleaseRadarEntry{
				{ArtistName: "Radiohead", PlayCount: 5, StationCount: 2},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioNewReleaseRadarHandler(context.Background(), &GetRadioNewReleaseRadarRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
}

func TestGetRadioNewReleaseRadar_DefaultLimit(t *testing.T) {
	var capturedLimit int
	mock := &testhelpers.MockRadioService{
		GetNewReleaseRadarFn: func(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
			capturedLimit = limit
			return []*contracts.RadioNewReleaseRadarEntry{}, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioNewReleaseRadarHandler(context.Background(), &GetRadioNewReleaseRadarRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 20 {
		t.Errorf("expected default limit 20, got %d", capturedLimit)
	}
}

func TestGetRadioNewReleaseRadar_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetNewReleaseRadarFn: func(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioNewReleaseRadarHandler(context.Background(), &GetRadioNewReleaseRadarRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// GetRadioStatsHandler Tests
// ============================================================================

func TestGetRadioStats_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetRadioStatsFn: func() (*contracts.RadioStatsResponse, error) {
			return &contracts.RadioStatsResponse{
				TotalStations: 3,
				TotalShows:    15,
				TotalEpisodes: 500,
				TotalPlays:    12000,
				MatchedPlays:  8000,
				UniqueArtists: 2000,
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.GetRadioStatsHandler(context.Background(), &GetRadioStatsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.TotalStations != 3 {
		t.Errorf("expected 3 stations, got %d", resp.Body.TotalStations)
	}
	if resp.Body.TotalPlays != 12000 {
		t.Errorf("expected 12000 plays, got %d", resp.Body.TotalPlays)
	}
}

func TestGetRadioStats_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetRadioStatsFn: func() (*contracts.RadioStatsResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioStatsHandler(context.Background(), &GetRadioStatsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// AdminCreateRadioStationHandler Tests
// ============================================================================

func TestAdminCreateRadioStation_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CreateStationFn: func(req *contracts.CreateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
			return &contracts.RadioStationDetailResponse{
				ID:            1,
				Name:          req.Name,
				Slug:          "kexp",
				BroadcastType: req.BroadcastType,
				IsActive:      true,
			}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.Name = "KEXP"
	req.Body.BroadcastType = "both"

	resp, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "KEXP" {
		t.Errorf("expected KEXP, got %s", resp.Body.Name)
	}
}

func TestAdminCreateRadioStation_MissingName(t *testing.T) {
	mock := &testhelpers.MockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.BroadcastType = "both"

	_, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminCreateRadioStation_MissingBroadcastType(t *testing.T) {
	mock := &testhelpers.MockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.Name = "KEXP"

	_, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminCreateRadioStation_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CreateStationFn: func(req *contracts.CreateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.Name = "KEXP"
	req.Body.BroadcastType = "both"

	_, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// AdminUpdateRadioStationHandler Tests
// ============================================================================

func TestAdminUpdateRadioStation_Success(t *testing.T) {
	newName := "KEXP 2.0"
	mock := &testhelpers.MockRadioService{
		UpdateStationFn: func(stationID uint, req *contracts.UpdateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
			return &contracts.RadioStationDetailResponse{
				ID:   stationID,
				Name: *req.Name,
				Slug: "kexp",
			}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioStationRequest{StationID: 1}
	req.Body.Name = &newName

	resp, err := h.AdminUpdateRadioStationHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "KEXP 2.0" {
		t.Errorf("expected KEXP 2.0, got %s", resp.Body.Name)
	}
}

func TestAdminUpdateRadioStation_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		UpdateStationFn: func(stationID uint, req *contracts.UpdateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioStationRequest{StationID: 999}

	_, err := h.AdminUpdateRadioStationHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminDeleteRadioStationHandler Tests
// ============================================================================

func TestAdminDeleteRadioStation_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		DeleteStationFn: func(stationID uint) error {
			return nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioStationRequest{StationID: 1}

	_, err := h.AdminDeleteRadioStationHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminDeleteRadioStation_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		DeleteStationFn: func(stationID uint) error {
			return apperrors.ErrRadioStationNotFound(stationID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioStationRequest{StationID: 999}

	_, err := h.AdminDeleteRadioStationHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminCreateRadioShowHandler Tests
// ============================================================================

func TestAdminCreateRadioShow_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CreateShowFn: func(stationID uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{
				ID:        1,
				StationID: stationID,
				Name:      req.Name,
				Slug:      "morning-show",
			}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioShowRequest{StationID: 1}
	req.Body.Name = "Morning Show"

	resp, err := h.AdminCreateRadioShowHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Morning Show" {
		t.Errorf("expected Morning Show, got %s", resp.Body.Name)
	}
}

func TestAdminCreateRadioShow_MissingName(t *testing.T) {
	mock := &testhelpers.MockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioShowRequest{StationID: 1}

	_, err := h.AdminCreateRadioShowHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminCreateRadioShow_StationNotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CreateShowFn: func(stationID uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioShowRequest{StationID: 999}
	req.Body.Name = "Morning Show"

	_, err := h.AdminCreateRadioShowHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminUpdateRadioShowHandler Tests
// ============================================================================

func TestAdminUpdateRadioShow_Success(t *testing.T) {
	newName := "Evening Show"
	mock := &testhelpers.MockRadioService{
		UpdateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{
				ID:   showID,
				Name: *req.Name,
				Slug: "evening-show",
			}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioShowRequest{ShowID: 1}
	req.Body.Name = &newName

	resp, err := h.AdminUpdateRadioShowHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Evening Show" {
		t.Errorf("expected Evening Show, got %s", resp.Body.Name)
	}
}

func TestAdminUpdateRadioShow_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		UpdateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(showID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioShowRequest{ShowID: 999}

	_, err := h.AdminUpdateRadioShowHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// PSY-1172: the admin handler forwards lifecycle_state to the service (the field the
// admin form previously could not reach).
func TestAdminUpdateRadioShow_PassesLifecycleStateToService(t *testing.T) {
	var gotState *string
	mock := &testhelpers.MockRadioService{
		UpdateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			gotState = req.LifecycleState
			return &contracts.RadioShowDetailResponse{ID: showID, LifecycleState: *req.LifecycleState}, nil
		},
	}
	h := testRadioHandler(mock)
	retired := catalogm.RadioLifecycleRetired
	req := &AdminUpdateRadioShowRequest{ShowID: 1}
	req.Body.LifecycleState = &retired

	resp, err := h.AdminUpdateRadioShowHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotState == nil || *gotState != catalogm.RadioLifecycleRetired {
		t.Errorf("expected service to receive lifecycle_state=retired, got %v", gotState)
	}
	if resp.Body.LifecycleState != catalogm.RadioLifecycleRetired {
		t.Errorf("expected response lifecycle_state=retired, got %s", resp.Body.LifecycleState)
	}
}

// PSY-1172: a service-level invalid-lifecycle error maps to HTTP 422.
func TestAdminUpdateRadioShow_LifecycleInvalid_Returns422(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		UpdateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioLifecycleInvalid("archived")
		},
	}
	h := testRadioHandler(mock)
	bogus := "archived"
	req := &AdminUpdateRadioShowRequest{ShowID: 1}
	req.Body.LifecycleState = &bogus

	_, err := h.AdminUpdateRadioShowHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// PSY-1193: the admin handler forwards schedule_locked to the service so the admin UI
// can lock/unlock a show's schedule (the field the form previously could not reach).
func TestAdminUpdateRadioShow_PassesScheduleLockedToService(t *testing.T) {
	var gotLocked *bool
	mock := &testhelpers.MockRadioService{
		UpdateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			gotLocked = req.ScheduleLocked
			return &contracts.RadioShowDetailResponse{ID: showID, ScheduleLocked: req.ScheduleLocked != nil && *req.ScheduleLocked}, nil
		},
	}
	h := testRadioHandler(mock)
	locked := true
	req := &AdminUpdateRadioShowRequest{ShowID: 1}
	req.Body.ScheduleLocked = &locked

	resp, err := h.AdminUpdateRadioShowHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLocked == nil || *gotLocked != true {
		t.Errorf("expected service to receive schedule_locked=true, got %v", gotLocked)
	}
	if !resp.Body.ScheduleLocked {
		t.Errorf("expected response schedule_locked=true, got %v", resp.Body.ScheduleLocked)
	}
}

// ============================================================================
// AdminDeleteRadioShowHandler Tests
// ============================================================================

func TestAdminDeleteRadioShow_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		DeleteShowFn: func(showID uint) error {
			return nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioShowRequest{ShowID: 1}

	_, err := h.AdminDeleteRadioShowHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdminDeleteRadioShow_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		DeleteShowFn: func(showID uint) error {
			return apperrors.ErrRadioShowNotFound(showID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioShowRequest{ShowID: 999}

	_, err := h.AdminDeleteRadioShowHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminTriggerStationSyncHandler Tests (PSY-1135)
// ============================================================================

func TestAdminTriggerStationSync_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		TriggerStationSyncFn: func(stationID uint, mode string) (*contracts.RadioSyncRunResponse, error) {
			if stationID != 1 || mode != "fetch" {
				t.Errorf("unexpected args: station=%d mode=%q", stationID, mode)
			}
			return &contracts.RadioSyncRunResponse{ID: 42, Status: "running", RunType: "fetch", Trigger: "manual"}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminTriggerStationSyncRequest{StationID: 1}
	req.Body.Mode = "fetch"

	resp, err := h.AdminTriggerStationSyncHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 || resp.Body.Status != "running" {
		t.Fatalf("unexpected run: %+v", resp.Body)
	}
}

func TestAdminTriggerStationSync_AlreadyRunning(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		TriggerStationSyncFn: func(_ uint, _ string) (*contracts.RadioSyncRunResponse, error) {
			return nil, apperrors.ErrRadioSyncAlreadyRunning(1)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminTriggerStationSyncRequest{StationID: 1}
	req.Body.Mode = "discover"

	_, err := h.AdminTriggerStationSyncHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAdminTriggerStationSync_StationNotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		TriggerStationSyncFn: func(_ uint, _ string) (*contracts.RadioSyncRunResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(999)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminTriggerStationSyncRequest{StationID: 999}
	req.Body.Mode = "fetch"

	_, err := h.AdminTriggerStationSyncHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminTriggerShowBackfillHandler Tests (PSY-1135)
// ============================================================================

func TestAdminTriggerShowBackfill_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		TriggerShowBackfillFn: func(showID uint, since, until string) (*contracts.RadioSyncRunResponse, error) {
			if showID != 5 || since != "2025-01-01" || until != "2025-12-31" {
				t.Errorf("unexpected args: show=%d since=%q until=%q", showID, since, until)
			}
			return &contracts.RadioSyncRunResponse{ID: 7, Status: "running", RunType: "backfill", Trigger: "manual"}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminTriggerShowBackfillRequest{ShowID: 5}
	req.Body.Since = "2025-01-01"
	req.Body.Until = "2025-12-31"

	resp, err := h.AdminTriggerShowBackfillHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 7 {
		t.Fatalf("unexpected run id %d", resp.Body.ID)
	}
}

func TestAdminTriggerShowBackfill_MissingDates(t *testing.T) {
	h := testRadioHandler(&testhelpers.MockRadioService{})
	req := &AdminTriggerShowBackfillRequest{ShowID: 5}
	req.Body.Since = "2025-01-01" // until omitted

	_, err := h.AdminTriggerShowBackfillHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminTriggerShowBackfill_InvalidDate(t *testing.T) {
	h := testRadioHandler(&testhelpers.MockRadioService{})
	req := &AdminTriggerShowBackfillRequest{ShowID: 5}
	req.Body.Since = "01-01-2025" // wrong format
	req.Body.Until = "2025-12-31"

	_, err := h.AdminTriggerShowBackfillHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminTriggerShowBackfill_InvalidWindow(t *testing.T) {
	h := testRadioHandler(&testhelpers.MockRadioService{})
	req := &AdminTriggerShowBackfillRequest{ShowID: 5}
	req.Body.Since = "2025-12-31"
	req.Body.Until = "2025-01-01" // until before since — reversed window

	_, err := h.AdminTriggerShowBackfillHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminTriggerShowBackfill_ShowNotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		TriggerShowBackfillFn: func(_ uint, _, _ string) (*contracts.RadioSyncRunResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(999)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminTriggerShowBackfillRequest{ShowID: 999}
	req.Body.Since = "2025-01-01"
	req.Body.Until = "2025-12-31"

	_, err := h.AdminTriggerShowBackfillHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminGetSyncRunHandler Tests (PSY-1135)
// ============================================================================

func TestAdminGetSyncRun_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetSyncRunFn: func(runID uint) (*contracts.RadioSyncRunResponse, error) {
			if runID != 42 {
				t.Errorf("unexpected runID=%d", runID)
			}
			return &contracts.RadioSyncRunResponse{ID: 42, Status: "success"}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminGetSyncRunHandler(radioAdminCtx(), &AdminGetSyncRunRequest{RunID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != "success" {
		t.Fatalf("unexpected status %q", resp.Body.Status)
	}
}

func TestAdminGetSyncRun_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetSyncRunFn: func(_ uint) (*contracts.RadioSyncRunResponse, error) {
			return nil, apperrors.ErrRadioSyncRunNotFound(999)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminGetSyncRunHandler(radioAdminCtx(), &AdminGetSyncRunRequest{RunID: 999})
	testhelpers.AssertHumaError(t, err, 404)
}

// ============================================================================
// AdminCancelSyncRunHandler Tests (PSY-1135)
// ============================================================================

func TestAdminCancelSyncRun_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CancelSyncRunFn: func(runID uint) error {
			if runID != 42 {
				t.Errorf("unexpected runID=%d", runID)
			}
			return nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminCancelSyncRunHandler(radioAdminCtx(), &AdminCancelSyncRunRequest{RunID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestAdminCancelSyncRun_NotFound(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CancelSyncRunFn: func(_ uint) error {
			return apperrors.ErrRadioSyncRunNotFound(999)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminCancelSyncRunHandler(radioAdminCtx(), &AdminCancelSyncRunRequest{RunID: 999})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminCancelSyncRun_NotCancellable(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		CancelSyncRunFn: func(_ uint) error {
			return apperrors.ErrRadioSyncNotCancellable(42, "success")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminCancelSyncRunHandler(radioAdminCtx(), &AdminCancelSyncRunRequest{RunID: 42})
	testhelpers.AssertHumaError(t, err, 409)
}


// ============================================================================
// AdminGetUnmatchedPlaysHandler Tests
// ============================================================================

func TestAdminGetUnmatchedPlays_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetUnmatchedPlaysFn: func(stationID uint, limit, offset int) ([]*contracts.UnmatchedPlayGroup, int64, error) {
			// Defaults are applied by the handler when limit<=0 / offset<0.
			if limit != 50 || offset != 0 {
				t.Errorf("expected default limit=50 offset=0, got limit=%d offset=%d", limit, offset)
			}
			return []*contracts.UnmatchedPlayGroup{
				{ArtistName: "Unknown Band", PlayCount: 3, StationNames: []string{"KEXP"}},
			}, 1, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminGetUnmatchedPlaysHandler(radioAdminCtx(), &AdminGetUnmatchedPlaysRequest{Limit: 0, Offset: -5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 || len(resp.Body.Groups) != 1 {
		t.Errorf("unexpected response body: %+v", resp.Body)
	}
}

func TestAdminGetUnmatchedPlays_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetUnmatchedPlaysFn: func(_ uint, _, _ int) ([]*contracts.UnmatchedPlayGroup, int64, error) {
			return nil, 0, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminGetUnmatchedPlaysHandler(radioAdminCtx(), &AdminGetUnmatchedPlaysRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// AdminLinkPlayHandler Tests
// ============================================================================

func TestAdminLinkPlay_Success(t *testing.T) {
	artistID := uint(123)
	mock := &testhelpers.MockRadioService{
		LinkPlayFn: func(playID uint, req *contracts.LinkPlayRequest) error {
			if playID != 7 {
				t.Errorf("unexpected playID=%d", playID)
			}
			if req.ArtistID == nil || *req.ArtistID != 123 {
				t.Errorf("unexpected link request: %+v", req)
			}
			return nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminLinkPlayRequest{PlayID: 7}
	req.Body.ArtistID = &artistID

	resp, err := h.AdminLinkPlayHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestAdminLinkPlay_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		LinkPlayFn: func(_ uint, _ *contracts.LinkPlayRequest) error {
			return fmt.Errorf("constraint violation")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminLinkPlayRequest{PlayID: 7}

	_, err := h.AdminLinkPlayHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// AdminBulkLinkPlaysHandler Tests
// ============================================================================

func TestAdminBulkLinkPlays_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		BulkLinkPlaysFn: func(req *contracts.BulkLinkRequest) (*contracts.BulkLinkResult, error) {
			if req.ArtistName != "Radiohead" || req.ArtistID != 123 {
				t.Errorf("unexpected bulk request: %+v", req)
			}
			return &contracts.BulkLinkResult{Updated: 9}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminBulkLinkPlaysRequest{}
	req.Body.ArtistName = "Radiohead"
	req.Body.ArtistID = 123

	resp, err := h.AdminBulkLinkPlaysHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Updated != 9 {
		t.Errorf("expected 9 updated, got %d", resp.Body.Updated)
	}
}

func TestAdminBulkLinkPlays_MissingArtistName(t *testing.T) {
	h := testRadioHandler(&testhelpers.MockRadioService{})
	req := &AdminBulkLinkPlaysRequest{}
	req.Body.ArtistID = 123 // name omitted

	_, err := h.AdminBulkLinkPlaysHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminBulkLinkPlays_MissingArtistID(t *testing.T) {
	h := testRadioHandler(&testhelpers.MockRadioService{})
	req := &AdminBulkLinkPlaysRequest{}
	req.Body.ArtistName = "Radiohead" // id omitted (zero)

	_, err := h.AdminBulkLinkPlaysHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminBulkLinkPlays_ServiceError(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		BulkLinkPlaysFn: func(_ *contracts.BulkLinkRequest) (*contracts.BulkLinkResult, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminBulkLinkPlaysRequest{}
	req.Body.ArtistName = "Radiohead"
	req.Body.ArtistID = 123

	_, err := h.AdminBulkLinkPlaysHandler(radioAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Admin observability feeds (PSY-1129/P5)
// ============================================================================

func TestAdminListSyncRuns_GlobalForwardsArgsAndEnvelope(t *testing.T) {
	var gotStation *uint
	var gotStatus, gotScope string
	var gotLimit, gotOffset int
	mock := &testhelpers.MockRadioService{
		ListSyncRunsFn: func(stationID *uint, status, scope string, limit, offset int) ([]*contracts.RadioSyncRunResponse, int64, error) {
			gotStation, gotStatus, gotScope, gotLimit, gotOffset = stationID, status, scope, limit, offset
			return []*contracts.RadioSyncRunResponse{{ID: 1}, {ID: 2}}, 7, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminListSyncRunsHandler(radioAdminCtx(), &AdminListSyncRunsRequest{Status: "failed", Scope: "sweep", Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStation != nil {
		t.Errorf("global feed must pass nil stationID, got %v", gotStation)
	}
	if gotStatus != "failed" || gotScope != "sweep" || gotLimit != 2 || gotOffset != 4 {
		t.Errorf("args not forwarded: status=%q scope=%q limit=%d offset=%d", gotStatus, gotScope, gotLimit, gotOffset)
	}
	if resp.Body.Total != 7 || resp.Body.Count != 2 {
		t.Errorf("envelope wrong: total=%d count=%d", resp.Body.Total, resp.Body.Count)
	}
}

func TestAdminListStationSyncRuns_ScopesToStation(t *testing.T) {
	var gotStation *uint
	mock := &testhelpers.MockRadioService{
		ListSyncRunsFn: func(stationID *uint, _, _ string, _, _ int) ([]*contracts.RadioSyncRunResponse, int64, error) {
			gotStation = stationID
			return []*contracts.RadioSyncRunResponse{{ID: 1}}, 1, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminListStationSyncRunsHandler(radioAdminCtx(), &AdminListStationSyncRunsRequest{StationID: 5, Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStation == nil || *gotStation != 5 {
		t.Errorf("per-station feed must scope to station 5, got %v", gotStation)
	}
}

func TestAdminListStationSyncRuns_StationNotFound404(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ListSyncRunsFn: func(_ *uint, _, _ string, _, _ int) ([]*contracts.RadioSyncRunResponse, int64, error) {
			return nil, 0, apperrors.ErrRadioStationNotFound(999)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminListStationSyncRunsHandler(radioAdminCtx(), &AdminListStationSyncRunsRequest{StationID: 999})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminGetStationHealth_Success(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetStationHealthFn: func(stationID uint) (*contracts.RadioStationHealthResponse, error) {
			return &contracts.RadioStationHealthResponse{StationID: stationID, BreakerState: "closed"}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminGetStationHealthHandler(radioAdminCtx(), &AdminGetStationHealthRequest{StationID: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body == nil || resp.Body.StationID != 3 {
		t.Errorf("expected station 3 health, got %+v", resp.Body)
	}
}

func TestAdminGetStationHealth_NotFound404(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		GetStationHealthFn: func(_ uint) (*contracts.RadioStationHealthResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(999)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminGetStationHealthHandler(radioAdminCtx(), &AdminGetStationHealthRequest{StationID: 999})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminListStationHealth_Envelope(t *testing.T) {
	mock := &testhelpers.MockRadioService{
		ListStationHealthFn: func() ([]*contracts.RadioStationHealthResponse, error) {
			return []*contracts.RadioStationHealthResponse{{StationID: 1}, {StationID: 2}, {StationID: 3}}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminListStationHealthHandler(radioAdminCtx(), &AdminListStationHealthRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 3 || len(resp.Body.Stations) != 3 {
		t.Errorf("envelope wrong: count=%d len=%d", resp.Body.Count, len(resp.Body.Stations))
	}
}
