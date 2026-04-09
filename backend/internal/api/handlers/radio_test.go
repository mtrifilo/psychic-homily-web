package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

type mockArtistSlugResolver struct {
	getArtistBySlugFn func(slug string) (*contracts.ArtistDetailResponse, error)
}

func (m *mockArtistSlugResolver) GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error) {
	if m.getArtistBySlugFn != nil {
		return m.getArtistBySlugFn(slug)
	}
	return nil, fmt.Errorf("artist not found")
}

type mockReleaseSlugResolver struct {
	getReleaseBySlugFn func(slug string) (*contracts.ReleaseDetailResponse, error)
}

func (m *mockReleaseSlugResolver) GetReleaseBySlug(slug string) (*contracts.ReleaseDetailResponse, error) {
	if m.getReleaseBySlugFn != nil {
		return m.getReleaseBySlugFn(slug)
	}
	return nil, fmt.Errorf("release not found")
}

func testRadioHandler(radio *mockRadioService) *RadioHandler {
	return NewRadioHandler(radio, &mockArtistSlugResolver{}, &mockReleaseSlugResolver{}, nil)
}

func testRadioHandlerWithResolvers(radio *mockRadioService, artist *mockArtistSlugResolver, release *mockReleaseSlugResolver) *RadioHandler {
	return NewRadioHandler(radio, artist, release, nil)
}

func radioAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true, EmailVerified: true})
}

// ============================================================================
// ListRadioStationsHandler Tests
// ============================================================================

func TestListRadioStations_Success(t *testing.T) {
	mock := &mockRadioService{
		listStationsFn: func(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
			return []*contracts.RadioStationListResponse{
				{ID: 1, Name: "KEXP", Slug: "kexp", BroadcastType: "fm", IsActive: true, ShowCount: 5},
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
	mock := &mockRadioService{
		listStationsFn: func(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
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
	mock := &mockRadioService{
		listStationsFn: func(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.ListRadioStationsHandler(context.Background(), &ListRadioStationsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// GetRadioStationHandler Tests
// ============================================================================

func TestGetRadioStation_BySlug(t *testing.T) {
	mock := &mockRadioService{
		getStationBySlugFn: func(slug string) (*contracts.RadioStationDetailResponse, error) {
			return &contracts.RadioStationDetailResponse{ID: 1, Name: "KEXP", Slug: "kexp", BroadcastType: "fm"}, nil
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
	mock := &mockRadioService{
		getStationFn: func(stationID uint) (*contracts.RadioStationDetailResponse, error) {
			return &contracts.RadioStationDetailResponse{ID: stationID, Name: "KEXP", Slug: "kexp", BroadcastType: "fm"}, nil
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
	mock := &mockRadioService{
		getStationBySlugFn: func(slug string) (*contracts.RadioStationDetailResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioStationHandler(context.Background(), &GetRadioStationRequest{Slug: "nonexistent"})
	assertHumaError(t, err, 404)
}

// ============================================================================
// ListRadioShowsHandler Tests
// ============================================================================

func TestListRadioShows_Success(t *testing.T) {
	mock := &mockRadioService{
		listShowsFn: func(stationID uint) ([]*contracts.RadioShowListResponse, error) {
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
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	_, err := h.ListRadioShowsHandler(context.Background(), &ListRadioShowsRequest{StationID: 0})
	assertHumaError(t, err, 400)
}

// ============================================================================
// GetRadioShowHandler Tests
// ============================================================================

func TestGetRadioShow_BySlug(t *testing.T) {
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowHandler(context.Background(), &GetRadioShowRequest{Slug: "nonexistent"})
	assertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioShowEpisodesHandler Tests
// ============================================================================

func TestGetRadioShowEpisodes_Success(t *testing.T) {
	now := time.Now()
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getEpisodesFn: func(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getEpisodesFn: func(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowEpisodesHandler(context.Background(), &GetRadioShowEpisodesRequest{Slug: "nonexistent"})
	assertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioEpisodeByDateHandler Tests
// ============================================================================

func TestGetRadioEpisodeByDate_Success(t *testing.T) {
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getEpisodeByShowAndDateFn: func(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioEpisodeByDateHandler(context.Background(), &GetRadioEpisodeByDateRequest{Slug: "morning-show", Date: "not-a-date"})
	assertHumaError(t, err, 400)
}

func TestGetRadioEpisodeByDate_NotFound(t *testing.T) {
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getEpisodeByShowAndDateFn: func(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error) {
			return nil, apperrors.ErrRadioEpisodeNotFound(0)
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioEpisodeByDateHandler(context.Background(), &GetRadioEpisodeByDateRequest{Slug: "morning-show", Date: "2026-03-15"})
	assertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioShowTopArtistsHandler Tests
// ============================================================================

func TestGetRadioShowTopArtists_Success(t *testing.T) {
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getTopArtistsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getTopArtistsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getTopLabelsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
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
	mock := &mockRadioService{
		getShowBySlugFn: func(slug string) (*contracts.RadioShowDetailResponse, error) {
			return &contracts.RadioShowDetailResponse{ID: 1, Name: "Morning Show", Slug: "morning-show"}, nil
		},
		getTopLabelsForShowFn: func(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioShowTopLabelsHandler(context.Background(), &GetRadioShowTopLabelsRequest{Slug: "morning-show"})
	assertHumaError(t, err, 500)
}

// ============================================================================
// GetArtistRadioPlaysHandler Tests
// ============================================================================

func TestGetArtistRadioPlays_BySlug(t *testing.T) {
	artistMock := &mockArtistSlugResolver{
		getArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			return &contracts.ArtistDetailResponse{ID: 42, Name: "Radiohead"}, nil
		},
	}
	radioMock := &mockRadioService{
		getAsHeardOnForArtistFn: func(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
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
	radioMock := &mockRadioService{
		getAsHeardOnForArtistFn: func(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
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
		getArtistBySlugFn: func(slug string) (*contracts.ArtistDetailResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	radioMock := &mockRadioService{}
	h := testRadioHandlerWithResolvers(radioMock, artistMock, &mockReleaseSlugResolver{})
	_, err := h.GetArtistRadioPlaysHandler(context.Background(), &GetArtistRadioPlaysRequest{Slug: "nonexistent"})
	assertHumaError(t, err, 404)
}

// ============================================================================
// GetReleaseRadioPlaysHandler Tests
// ============================================================================

func TestGetReleaseRadioPlays_BySlug(t *testing.T) {
	releaseMock := &mockReleaseSlugResolver{
		getReleaseBySlugFn: func(slug string) (*contracts.ReleaseDetailResponse, error) {
			return &contracts.ReleaseDetailResponse{ID: 10, Title: "OK Computer"}, nil
		},
	}
	radioMock := &mockRadioService{
		getAsHeardOnForReleaseFn: func(releaseID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
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
		getReleaseBySlugFn: func(slug string) (*contracts.ReleaseDetailResponse, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	radioMock := &mockRadioService{}
	h := testRadioHandlerWithResolvers(radioMock, &mockArtistSlugResolver{}, releaseMock)
	_, err := h.GetReleaseRadioPlaysHandler(context.Background(), &GetReleaseRadioPlaysRequest{Slug: "nonexistent"})
	assertHumaError(t, err, 404)
}

// ============================================================================
// GetRadioNewReleaseRadarHandler Tests
// ============================================================================

func TestGetRadioNewReleaseRadar_Success(t *testing.T) {
	mock := &mockRadioService{
		getNewReleaseRadarFn: func(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
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
	mock := &mockRadioService{
		getNewReleaseRadarFn: func(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
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
	mock := &mockRadioService{
		getNewReleaseRadarFn: func(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioNewReleaseRadarHandler(context.Background(), &GetRadioNewReleaseRadarRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// GetRadioStatsHandler Tests
// ============================================================================

func TestGetRadioStats_Success(t *testing.T) {
	mock := &mockRadioService{
		getRadioStatsFn: func() (*contracts.RadioStatsResponse, error) {
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
	mock := &mockRadioService{
		getRadioStatsFn: func() (*contracts.RadioStatsResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.GetRadioStatsHandler(context.Background(), &GetRadioStatsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// AdminCreateRadioStationHandler Tests
// ============================================================================

func TestAdminCreateRadioStation_Success(t *testing.T) {
	mock := &mockRadioService{
		createStationFn: func(req *contracts.CreateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
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
	req.Body.BroadcastType = "fm"

	resp, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "KEXP" {
		t.Errorf("expected KEXP, got %s", resp.Body.Name)
	}
}

func TestAdminCreateRadioStation_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.Name = "KEXP"
	req.Body.BroadcastType = "fm"

	_, err := h.AdminCreateRadioStationHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminCreateRadioStation_MissingName(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.BroadcastType = "fm"

	_, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminCreateRadioStation_MissingBroadcastType(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.Name = "KEXP"

	_, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminCreateRadioStation_ServiceError(t *testing.T) {
	mock := &mockRadioService{
		createStationFn: func(req *contracts.CreateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioStationRequest{}
	req.Body.Name = "KEXP"
	req.Body.BroadcastType = "fm"

	_, err := h.AdminCreateRadioStationHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// AdminUpdateRadioStationHandler Tests
// ============================================================================

func TestAdminUpdateRadioStation_Success(t *testing.T) {
	newName := "KEXP 2.0"
	mock := &mockRadioService{
		updateStationFn: func(stationID uint, req *contracts.UpdateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
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

func TestAdminUpdateRadioStation_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioStationRequest{StationID: 1}

	_, err := h.AdminUpdateRadioStationHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminUpdateRadioStation_NotFound(t *testing.T) {
	mock := &mockRadioService{
		updateStationFn: func(stationID uint, req *contracts.UpdateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioStationRequest{StationID: 999}

	_, err := h.AdminUpdateRadioStationHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 404)
}

// ============================================================================
// AdminDeleteRadioStationHandler Tests
// ============================================================================

func TestAdminDeleteRadioStation_Success(t *testing.T) {
	mock := &mockRadioService{
		deleteStationFn: func(stationID uint) error {
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

func TestAdminDeleteRadioStation_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioStationRequest{StationID: 1}

	_, err := h.AdminDeleteRadioStationHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminDeleteRadioStation_NotFound(t *testing.T) {
	mock := &mockRadioService{
		deleteStationFn: func(stationID uint) error {
			return apperrors.ErrRadioStationNotFound(stationID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioStationRequest{StationID: 999}

	_, err := h.AdminDeleteRadioStationHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 404)
}

// ============================================================================
// AdminCreateRadioShowHandler Tests
// ============================================================================

func TestAdminCreateRadioShow_Success(t *testing.T) {
	mock := &mockRadioService{
		createShowFn: func(stationID uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
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

func TestAdminCreateRadioShow_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioShowRequest{StationID: 1}
	req.Body.Name = "Morning Show"

	_, err := h.AdminCreateRadioShowHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminCreateRadioShow_MissingName(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioShowRequest{StationID: 1}

	_, err := h.AdminCreateRadioShowHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminCreateRadioShow_StationNotFound(t *testing.T) {
	mock := &mockRadioService{
		createShowFn: func(stationID uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateRadioShowRequest{StationID: 999}
	req.Body.Name = "Morning Show"

	_, err := h.AdminCreateRadioShowHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 404)
}

// ============================================================================
// AdminUpdateRadioShowHandler Tests
// ============================================================================

func TestAdminUpdateRadioShow_Success(t *testing.T) {
	newName := "Evening Show"
	mock := &mockRadioService{
		updateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
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

func TestAdminUpdateRadioShow_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioShowRequest{ShowID: 1}

	_, err := h.AdminUpdateRadioShowHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminUpdateRadioShow_NotFound(t *testing.T) {
	mock := &mockRadioService{
		updateShowFn: func(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
			return nil, apperrors.ErrRadioShowNotFound(showID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminUpdateRadioShowRequest{ShowID: 999}

	_, err := h.AdminUpdateRadioShowHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 404)
}

// ============================================================================
// AdminDeleteRadioShowHandler Tests
// ============================================================================

func TestAdminDeleteRadioShow_Success(t *testing.T) {
	mock := &mockRadioService{
		deleteShowFn: func(showID uint) error {
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

func TestAdminDeleteRadioShow_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioShowRequest{ShowID: 1}

	_, err := h.AdminDeleteRadioShowHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminDeleteRadioShow_NotFound(t *testing.T) {
	mock := &mockRadioService{
		deleteShowFn: func(showID uint) error {
			return apperrors.ErrRadioShowNotFound(showID)
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDeleteRadioShowRequest{ShowID: 999}

	_, err := h.AdminDeleteRadioShowHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 404)
}

// ============================================================================
// AdminTriggerFetchHandler Tests
// ============================================================================

func TestAdminTriggerFetch_Success(t *testing.T) {
	mock := &mockRadioService{
		discoverStationShowsFn: func(stationID uint) (*contracts.RadioDiscoverResult, error) {
			return &contracts.RadioDiscoverResult{ShowsDiscovered: 3, ShowNames: []string{"Show A", "Show B", "Show C"}}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminTriggerFetchRequest{StationID: 1}

	resp, err := h.AdminTriggerFetchHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ShowsDiscovered != 3 {
		t.Fatalf("expected 3 shows discovered, got %d", resp.Body.ShowsDiscovered)
	}
}

func TestAdminTriggerFetch_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminTriggerFetchRequest{StationID: 1}

	_, err := h.AdminTriggerFetchHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

// ============================================================================
// AdminDiscoverShowsHandler Tests
// ============================================================================

func TestAdminDiscoverShows_Success(t *testing.T) {
	mock := &mockRadioService{
		discoverStationShowsFn: func(stationID uint) (*contracts.RadioDiscoverResult, error) {
			return &contracts.RadioDiscoverResult{ShowsDiscovered: 2, ShowNames: []string{"Show X", "Show Y"}}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDiscoverShowsRequest{StationID: 1}

	resp, err := h.AdminDiscoverShowsHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ShowsDiscovered != 2 {
		t.Fatalf("expected 2 shows discovered, got %d", resp.Body.ShowsDiscovered)
	}
	if len(resp.Body.ShowNames) != 2 {
		t.Fatalf("expected 2 show names, got %d", len(resp.Body.ShowNames))
	}
}

func TestAdminDiscoverShows_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminDiscoverShowsRequest{StationID: 1}

	_, err := h.AdminDiscoverShowsHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminDiscoverShows_ServiceError(t *testing.T) {
	mock := &mockRadioService{
		discoverStationShowsFn: func(stationID uint) (*contracts.RadioDiscoverResult, error) {
			return nil, fmt.Errorf("station not found")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminDiscoverShowsRequest{StationID: 999}

	_, err := h.AdminDiscoverShowsHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// AdminImportShowEpisodesHandler Tests
// ============================================================================

func TestAdminImportShowEpisodes_Success(t *testing.T) {
	mock := &mockRadioService{
		importShowEpisodesFn: func(showID uint, since string, until string) (*contracts.RadioImportResult, error) {
			return &contracts.RadioImportResult{
				EpisodesImported: 5,
				PlaysImported:    50,
				PlaysMatched:     30,
			}, nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminImportShowEpisodesRequest{ShowID: 1}
	req.Body.Since = "2024-01-01"
	req.Body.Until = "2024-12-31"

	resp, err := h.AdminImportShowEpisodesHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.EpisodesImported != 5 {
		t.Fatalf("expected 5 episodes imported, got %d", resp.Body.EpisodesImported)
	}
	if resp.Body.PlaysImported != 50 {
		t.Fatalf("expected 50 plays imported, got %d", resp.Body.PlaysImported)
	}
	if resp.Body.PlaysMatched != 30 {
		t.Fatalf("expected 30 plays matched, got %d", resp.Body.PlaysMatched)
	}
}

func TestAdminImportShowEpisodes_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminImportShowEpisodesRequest{ShowID: 1}
	req.Body.Since = "2024-01-01"
	req.Body.Until = "2024-12-31"

	_, err := h.AdminImportShowEpisodesHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminImportShowEpisodes_ServiceError(t *testing.T) {
	mock := &mockRadioService{
		importShowEpisodesFn: func(showID uint, since string, until string) (*contracts.RadioImportResult, error) {
			return nil, fmt.Errorf("show not found")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminImportShowEpisodesRequest{ShowID: 999}
	req.Body.Since = "2024-01-01"
	req.Body.Until = "2024-12-31"

	_, err := h.AdminImportShowEpisodesHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 500)
}
// ============================================================================
// AdminCreateImportJobHandler Tests
// ============================================================================

func TestAdminCreateImportJob_Success(t *testing.T) {
	now := time.Now()
	mock := &mockRadioService{
		createImportJobFn: func(showID uint, since, until string) (*contracts.RadioImportJobResponse, error) {
			return &contracts.RadioImportJobResponse{
				ID:          1,
				ShowID:      showID,
				ShowName:    "Test Show",
				StationID:   1,
				StationName: "Test Station",
				Since:       since,
				Until:       until,
				Status:      "pending",
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
		startImportJobFn: func(jobID uint) error {
			return nil
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateImportJobRequest{ShowID: 1}
	req.Body.Since = "2025-01-01"
	req.Body.Until = "2025-12-31"

	resp, err := h.AdminCreateImportJobHandler(radioAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected job ID 1, got %d", resp.Body.ID)
	}
	if resp.Body.ShowName != "Test Show" {
		t.Errorf("expected show name 'Test Show', got %s", resp.Body.ShowName)
	}
	if resp.Body.Status != "pending" {
		t.Errorf("expected status 'pending', got %s", resp.Body.Status)
	}
}

func TestAdminCreateImportJob_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateImportJobRequest{ShowID: 1}
	req.Body.Since = "2025-01-01"
	req.Body.Until = "2025-12-31"

	_, err := h.AdminCreateImportJobHandler(context.Background(), req)
	assertHumaError(t, err, 403)
}

func TestAdminCreateImportJob_MissingSince(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateImportJobRequest{ShowID: 1}
	req.Body.Since = ""
	req.Body.Until = "2025-12-31"

	_, err := h.AdminCreateImportJobHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminCreateImportJob_MissingUntil(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	req := &AdminCreateImportJobRequest{ShowID: 1}
	req.Body.Since = "2025-01-01"
	req.Body.Until = ""

	_, err := h.AdminCreateImportJobHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminCreateImportJob_ServiceError(t *testing.T) {
	mock := &mockRadioService{
		createImportJobFn: func(showID uint, since, until string) (*contracts.RadioImportJobResponse, error) {
			return nil, fmt.Errorf("an import job is already running")
		},
	}
	h := testRadioHandler(mock)
	req := &AdminCreateImportJobRequest{ShowID: 1}
	req.Body.Since = "2025-01-01"
	req.Body.Until = "2025-12-31"

	_, err := h.AdminCreateImportJobHandler(radioAdminCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// AdminGetImportJobHandler Tests
// ============================================================================

func TestAdminGetImportJob_Success(t *testing.T) {
	now := time.Now()
	mock := &mockRadioService{
		getImportJobFn: func(jobID uint) (*contracts.RadioImportJobResponse, error) {
			return &contracts.RadioImportJobResponse{
				ID:          jobID,
				ShowID:      1,
				ShowName:    "Test Show",
				StationID:   1,
				StationName: "Test Station",
				Status:      "running",
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminGetImportJobHandler(radioAdminCtx(), &AdminGetImportJobRequest{JobID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected job ID 1, got %d", resp.Body.ID)
	}
	if resp.Body.Status != "running" {
		t.Errorf("expected status 'running', got %s", resp.Body.Status)
	}
}

func TestAdminGetImportJob_NotFound(t *testing.T) {
	mock := &mockRadioService{
		getImportJobFn: func(jobID uint) (*contracts.RadioImportJobResponse, error) {
			return nil, fmt.Errorf("job not found")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminGetImportJobHandler(radioAdminCtx(), &AdminGetImportJobRequest{JobID: 999})
	assertHumaError(t, err, 404)
}

func TestAdminGetImportJob_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	_, err := h.AdminGetImportJobHandler(context.Background(), &AdminGetImportJobRequest{JobID: 1})
	assertHumaError(t, err, 403)
}

// ============================================================================
// AdminCancelImportJobHandler Tests
// ============================================================================

func TestAdminCancelImportJob_Success(t *testing.T) {
	mock := &mockRadioService{
		cancelImportJobFn: func(jobID uint) error {
			return nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminCancelImportJobHandler(radioAdminCtx(), &AdminCancelImportJobRequest{JobID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestAdminCancelImportJob_ServiceError(t *testing.T) {
	mock := &mockRadioService{
		cancelImportJobFn: func(jobID uint) error {
			return fmt.Errorf("job cannot be cancelled")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminCancelImportJobHandler(radioAdminCtx(), &AdminCancelImportJobRequest{JobID: 1})
	assertHumaError(t, err, 500)
}

func TestAdminCancelImportJob_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	_, err := h.AdminCancelImportJobHandler(context.Background(), &AdminCancelImportJobRequest{JobID: 1})
	assertHumaError(t, err, 403)
}

// ============================================================================
// AdminListImportJobsHandler Tests
// ============================================================================

func TestAdminListImportJobs_Success(t *testing.T) {
	now := time.Now()
	mock := &mockRadioService{
		listImportJobsFn: func(showID uint) ([]*contracts.RadioImportJobResponse, error) {
			return []*contracts.RadioImportJobResponse{
				{
					ID:          1,
					ShowID:      showID,
					ShowName:    "Test Show",
					StationID:   1,
					StationName: "Test Station",
					Status:      "completed",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminListImportJobsHandler(radioAdminCtx(), &AdminListImportJobsRequest{ShowID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.Body.Count)
	}
	if resp.Body.Jobs[0].Status != "completed" {
		t.Errorf("expected status 'completed', got %s", resp.Body.Jobs[0].Status)
	}
}

func TestAdminListImportJobs_Empty(t *testing.T) {
	mock := &mockRadioService{
		listImportJobsFn: func(showID uint) ([]*contracts.RadioImportJobResponse, error) {
			return []*contracts.RadioImportJobResponse{}, nil
		},
	}
	h := testRadioHandler(mock)
	resp, err := h.AdminListImportJobsHandler(radioAdminCtx(), &AdminListImportJobsRequest{ShowID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Count != 0 {
		t.Errorf("expected count 0, got %d", resp.Body.Count)
	}
}

func TestAdminListImportJobs_ServiceError(t *testing.T) {
	mock := &mockRadioService{
		listImportJobsFn: func(showID uint) ([]*contracts.RadioImportJobResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := testRadioHandler(mock)
	_, err := h.AdminListImportJobsHandler(radioAdminCtx(), &AdminListImportJobsRequest{ShowID: 1})
	assertHumaError(t, err, 500)
}

func TestAdminListImportJobs_NotAdmin(t *testing.T) {
	mock := &mockRadioService{}
	h := testRadioHandler(mock)
	_, err := h.AdminListImportJobsHandler(context.Background(), &AdminListImportJobsRequest{ShowID: 1})
	assertHumaError(t, err, 403)
}
