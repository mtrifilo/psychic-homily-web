package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Focused interfaces for RadioHandler
// ============================================================================

// RadioStationReader reads radio stations (public endpoints).
type RadioStationReader interface {
	GetStation(stationID uint) (*contracts.RadioStationDetailResponse, error)
	GetStationBySlug(slug string) (*contracts.RadioStationDetailResponse, error)
	ListStations(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error)
}

// RadioShowReader reads radio shows (public endpoints).
type RadioShowReader interface {
	GetShow(showID uint) (*contracts.RadioShowDetailResponse, error)
	GetShowBySlug(slug string) (*contracts.RadioShowDetailResponse, error)
	ListShows(stationID uint) ([]*contracts.RadioShowListResponse, error)
}

// RadioEpisodeReader reads radio episodes (public endpoints).
type RadioEpisodeReader interface {
	GetEpisodes(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error)
	GetEpisodeByShowAndDate(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error)
}

// RadioAggregationReader reads radio aggregation/stats data (public endpoints).
type RadioAggregationReader interface {
	GetTopArtistsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error)
	GetTopLabelsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error)
	GetAsHeardOnForArtist(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error)
	GetAsHeardOnForRelease(releaseID uint) ([]*contracts.RadioAsHeardOnResponse, error)
	GetNewReleaseRadar(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error)
	GetRadioStats() (*contracts.RadioStatsResponse, error)
}

// RadioStationWriter writes radio stations (admin endpoints).
type RadioStationWriter interface {
	CreateStation(req *contracts.CreateRadioStationRequest) (*contracts.RadioStationDetailResponse, error)
	UpdateStation(stationID uint, req *contracts.UpdateRadioStationRequest) (*contracts.RadioStationDetailResponse, error)
	DeleteStation(stationID uint) error
}

// RadioShowWriter writes radio shows (admin endpoints).
type RadioShowWriter interface {
	CreateShow(stationID uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error)
	UpdateShow(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error)
	DeleteShow(showID uint) error
}

// ArtistSlugResolver resolves artist slugs to IDs.
type ArtistSlugResolver interface {
	GetArtistBySlug(slug string) (*contracts.ArtistDetailResponse, error)
}

// ReleaseSlugResolver resolves release slugs to IDs.
type ReleaseSlugResolver interface {
	GetReleaseBySlug(slug string) (*contracts.ReleaseDetailResponse, error)
}

// ============================================================================
// Handler
// ============================================================================

// RadioHandler handles all radio entity HTTP endpoints.
type RadioHandler struct {
	stationReader     RadioStationReader
	showReader        RadioShowReader
	episodeReader     RadioEpisodeReader
	aggregationReader RadioAggregationReader
	stationWriter     RadioStationWriter
	showWriter        RadioShowWriter
	artistResolver    ArtistSlugResolver
	releaseResolver   ReleaseSlugResolver
	auditLogService   contracts.AuditLogServiceInterface
}

// NewRadioHandler creates a new RadioHandler.
func NewRadioHandler(
	radioService contracts.RadioServiceInterface,
	artistResolver ArtistSlugResolver,
	releaseResolver ReleaseSlugResolver,
	auditLogService contracts.AuditLogServiceInterface,
) *RadioHandler {
	return &RadioHandler{
		stationReader:     radioService,
		showReader:        radioService,
		episodeReader:     radioService,
		aggregationReader: radioService,
		stationWriter:     radioService,
		showWriter:        radioService,
		artistResolver:    artistResolver,
		releaseResolver:   releaseResolver,
		auditLogService:   auditLogService,
	}
}

// ============================================================================
// Public: List Radio Stations
// ============================================================================

// ListRadioStationsRequest represents the request for listing radio stations.
type ListRadioStationsRequest struct {
	IsActive string `query:"is_active" required:"false" doc:"Filter by active status (true/false)" example:"true"`
}

// ListRadioStationsResponse represents the response for listing radio stations.
type ListRadioStationsResponse struct {
	Body struct {
		Stations []*contracts.RadioStationListResponse `json:"stations" doc:"List of radio stations"`
		Count    int                                   `json:"count" doc:"Number of stations"`
	}
}

// ListRadioStationsHandler handles GET /radio-stations
func (h *RadioHandler) ListRadioStationsHandler(ctx context.Context, req *ListRadioStationsRequest) (*ListRadioStationsResponse, error) {
	filters := make(map[string]interface{})
	if req.IsActive == "true" {
		filters["is_active"] = true
	} else if req.IsActive == "false" {
		filters["is_active"] = false
	}

	stations, err := h.stationReader.ListStations(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch radio stations", err)
	}

	resp := &ListRadioStationsResponse{}
	resp.Body.Stations = stations
	if stations != nil {
		resp.Body.Count = len(stations)
	}
	return resp, nil
}

// ============================================================================
// Public: Get Radio Station
// ============================================================================

// GetRadioStationRequest represents the request for getting a single radio station.
type GetRadioStationRequest struct {
	Slug string `path:"slug" doc:"Radio station slug or numeric ID" example:"kexp"`
}

// GetRadioStationResponse represents the response for the get radio station endpoint.
type GetRadioStationResponse struct {
	Body *contracts.RadioStationDetailResponse
}

// GetRadioStationHandler handles GET /radio-stations/{slug}
func (h *RadioHandler) GetRadioStationHandler(ctx context.Context, req *GetRadioStationRequest) (*GetRadioStationResponse, error) {
	station, err := h.resolveStation(req.Slug)
	if err != nil {
		return nil, err
	}
	return &GetRadioStationResponse{Body: station}, nil
}

// ============================================================================
// Public: List Radio Shows
// ============================================================================

// ListRadioShowsRequest represents the request for listing radio shows.
type ListRadioShowsRequest struct {
	StationID uint `query:"station_id" doc:"Station ID (required)" example:"1"`
}

// ListRadioShowsResponse represents the response for listing radio shows.
type ListRadioShowsResponse struct {
	Body struct {
		Shows []*contracts.RadioShowListResponse `json:"shows" doc:"List of radio shows"`
		Count int                                `json:"count" doc:"Number of shows"`
	}
}

// ListRadioShowsHandler handles GET /radio-shows
func (h *RadioHandler) ListRadioShowsHandler(ctx context.Context, req *ListRadioShowsRequest) (*ListRadioShowsResponse, error) {
	if req.StationID == 0 {
		return nil, huma.Error400BadRequest("station_id query parameter is required")
	}

	shows, err := h.showReader.ListShows(req.StationID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch radio shows", err)
	}

	resp := &ListRadioShowsResponse{}
	resp.Body.Shows = shows
	if shows != nil {
		resp.Body.Count = len(shows)
	}
	return resp, nil
}

// ============================================================================
// Public: Get Radio Show
// ============================================================================

// GetRadioShowRequest represents the request for getting a single radio show.
type GetRadioShowRequest struct {
	Slug string `path:"slug" doc:"Radio show slug or numeric ID" example:"morning-show"`
}

// GetRadioShowResponse represents the response for the get radio show endpoint.
type GetRadioShowResponse struct {
	Body *contracts.RadioShowDetailResponse
}

// GetRadioShowHandler handles GET /radio-shows/{slug}
func (h *RadioHandler) GetRadioShowHandler(ctx context.Context, req *GetRadioShowRequest) (*GetRadioShowResponse, error) {
	show, err := h.resolveShow(req.Slug)
	if err != nil {
		return nil, err
	}
	return &GetRadioShowResponse{Body: show}, nil
}

// ============================================================================
// Public: Get Radio Show Episodes
// ============================================================================

// GetRadioShowEpisodesRequest represents the request for listing episodes of a show.
type GetRadioShowEpisodesRequest struct {
	Slug   string `path:"slug" doc:"Radio show slug or numeric ID" example:"morning-show"`
	Limit  int    `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
	Offset int    `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

// GetRadioShowEpisodesResponse represents the response for listing episodes.
type GetRadioShowEpisodesResponse struct {
	Body struct {
		Episodes []*contracts.RadioEpisodeResponse `json:"episodes" doc:"List of episodes"`
		Total    int64                             `json:"total" doc:"Total number of episodes"`
	}
}

// GetRadioShowEpisodesHandler handles GET /radio-shows/{slug}/episodes
func (h *RadioHandler) GetRadioShowEpisodesHandler(ctx context.Context, req *GetRadioShowEpisodesRequest) (*GetRadioShowEpisodesResponse, error) {
	show, err := h.resolveShow(req.Slug)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	episodes, total, err := h.episodeReader.GetEpisodes(show.ID, limit, offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch episodes", err)
	}

	resp := &GetRadioShowEpisodesResponse{}
	resp.Body.Episodes = episodes
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Public: Get Radio Episode by Date
// ============================================================================

// GetRadioEpisodeByDateRequest represents the request for getting an episode by date.
type GetRadioEpisodeByDateRequest struct {
	Slug string `path:"slug" doc:"Radio show slug or numeric ID" example:"morning-show"`
	Date string `path:"date" doc:"Air date in YYYY-MM-DD format" example:"2026-03-15"`
}

// GetRadioEpisodeByDateResponse represents the response for the episode detail endpoint.
type GetRadioEpisodeByDateResponse struct {
	Body *contracts.RadioEpisodeDetailResponse
}

// GetRadioEpisodeByDateHandler handles GET /radio-shows/{slug}/episodes/{date}
func (h *RadioHandler) GetRadioEpisodeByDateHandler(ctx context.Context, req *GetRadioEpisodeByDateRequest) (*GetRadioEpisodeByDateResponse, error) {
	show, err := h.resolveShow(req.Slug)
	if err != nil {
		return nil, err
	}

	// Validate date format
	if _, err := parseDate(req.Date); err != nil {
		return nil, huma.Error400BadRequest("Invalid date format, expected YYYY-MM-DD")
	}

	episode, err := h.episodeReader.GetEpisodeByShowAndDate(show.ID, req.Date)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioEpisodeNotFound {
			return nil, huma.Error404NotFound("Episode not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch episode", err)
	}

	return &GetRadioEpisodeByDateResponse{Body: episode}, nil
}

// ============================================================================
// Public: Top Artists for Show
// ============================================================================

// GetRadioShowTopArtistsRequest represents the request for top artists.
type GetRadioShowTopArtistsRequest struct {
	Slug   string `path:"slug" doc:"Radio show slug or numeric ID" example:"morning-show"`
	Period int    `query:"period" required:"false" doc:"Period in days (default 90)" example:"90"`
	Limit  int    `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
}

// GetRadioShowTopArtistsResponse represents the response for top artists.
type GetRadioShowTopArtistsResponse struct {
	Body struct {
		Artists []*contracts.RadioTopArtistResponse `json:"artists" doc:"Top artists"`
		Count   int                                 `json:"count" doc:"Number of results"`
	}
}

// GetRadioShowTopArtistsHandler handles GET /radio-shows/{slug}/top-artists
func (h *RadioHandler) GetRadioShowTopArtistsHandler(ctx context.Context, req *GetRadioShowTopArtistsRequest) (*GetRadioShowTopArtistsResponse, error) {
	show, err := h.resolveShow(req.Slug)
	if err != nil {
		return nil, err
	}

	period := req.Period
	if period <= 0 {
		period = 90
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	artists, err := h.aggregationReader.GetTopArtistsForShow(show.ID, period, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch top artists", err)
	}

	resp := &GetRadioShowTopArtistsResponse{}
	resp.Body.Artists = artists
	if artists != nil {
		resp.Body.Count = len(artists)
	}
	return resp, nil
}

// ============================================================================
// Public: Top Labels for Show
// ============================================================================

// GetRadioShowTopLabelsRequest represents the request for top labels.
type GetRadioShowTopLabelsRequest struct {
	Slug   string `path:"slug" doc:"Radio show slug or numeric ID" example:"morning-show"`
	Period int    `query:"period" required:"false" doc:"Period in days (default 90)" example:"90"`
	Limit  int    `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
}

// GetRadioShowTopLabelsResponse represents the response for top labels.
type GetRadioShowTopLabelsResponse struct {
	Body struct {
		Labels []*contracts.RadioTopLabelResponse `json:"labels" doc:"Top labels"`
		Count  int                                `json:"count" doc:"Number of results"`
	}
}

// GetRadioShowTopLabelsHandler handles GET /radio-shows/{slug}/top-labels
func (h *RadioHandler) GetRadioShowTopLabelsHandler(ctx context.Context, req *GetRadioShowTopLabelsRequest) (*GetRadioShowTopLabelsResponse, error) {
	show, err := h.resolveShow(req.Slug)
	if err != nil {
		return nil, err
	}

	period := req.Period
	if period <= 0 {
		period = 90
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	labels, err := h.aggregationReader.GetTopLabelsForShow(show.ID, period, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch top labels", err)
	}

	resp := &GetRadioShowTopLabelsResponse{}
	resp.Body.Labels = labels
	if labels != nil {
		resp.Body.Count = len(labels)
	}
	return resp, nil
}

// ============================================================================
// Public: Artist Radio Plays ("As Heard On")
// ============================================================================

// GetArtistRadioPlaysRequest represents the request for an artist's radio plays.
type GetArtistRadioPlaysRequest struct {
	Slug string `path:"slug" doc:"Artist slug or numeric ID" example:"radiohead"`
}

// GetArtistRadioPlaysResponse represents the response for an artist's radio plays.
type GetArtistRadioPlaysResponse struct {
	Body struct {
		Stations []*contracts.RadioAsHeardOnResponse `json:"stations" doc:"Stations/shows where artist was played"`
		Count    int                                 `json:"count" doc:"Number of results"`
	}
}

// GetArtistRadioPlaysHandler handles GET /artists/{slug}/radio-plays
func (h *RadioHandler) GetArtistRadioPlaysHandler(ctx context.Context, req *GetArtistRadioPlaysRequest) (*GetArtistRadioPlaysResponse, error) {
	artistID, err := h.resolveArtistID(req.Slug)
	if err != nil {
		return nil, err
	}

	stations, err := h.aggregationReader.GetAsHeardOnForArtist(artistID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch radio plays for artist", err)
	}

	resp := &GetArtistRadioPlaysResponse{}
	resp.Body.Stations = stations
	if stations != nil {
		resp.Body.Count = len(stations)
	}
	return resp, nil
}

// ============================================================================
// Public: Release Radio Plays ("As Heard On")
// ============================================================================

// GetReleaseRadioPlaysRequest represents the request for a release's radio plays.
type GetReleaseRadioPlaysRequest struct {
	Slug string `path:"slug" doc:"Release slug or numeric ID" example:"ok-computer"`
}

// GetReleaseRadioPlaysResponse represents the response for a release's radio plays.
type GetReleaseRadioPlaysResponse struct {
	Body struct {
		Stations []*contracts.RadioAsHeardOnResponse `json:"stations" doc:"Stations/shows where release was played"`
		Count    int                                 `json:"count" doc:"Number of results"`
	}
}

// GetReleaseRadioPlaysHandler handles GET /releases/{slug}/radio-plays
func (h *RadioHandler) GetReleaseRadioPlaysHandler(ctx context.Context, req *GetReleaseRadioPlaysRequest) (*GetReleaseRadioPlaysResponse, error) {
	releaseID, err := h.resolveReleaseID(req.Slug)
	if err != nil {
		return nil, err
	}

	stations, err := h.aggregationReader.GetAsHeardOnForRelease(releaseID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch radio plays for release", err)
	}

	resp := &GetReleaseRadioPlaysResponse{}
	resp.Body.Stations = stations
	if stations != nil {
		resp.Body.Count = len(stations)
	}
	return resp, nil
}

// ============================================================================
// Public: New Release Radar
// ============================================================================

// GetRadioNewReleaseRadarRequest represents the request for new release radar.
type GetRadioNewReleaseRadarRequest struct {
	StationID uint `query:"station_id" required:"false" doc:"Filter by station ID (0 for all)" example:"1"`
	Limit     int  `query:"limit" required:"false" doc:"Max results (default 20)" example:"20"`
}

// GetRadioNewReleaseRadarResponse represents the response for new release radar.
type GetRadioNewReleaseRadarResponse struct {
	Body struct {
		Releases []*contracts.RadioNewReleaseRadarEntry `json:"releases" doc:"New releases discovered via radio"`
		Count    int                                    `json:"count" doc:"Number of results"`
	}
}

// GetRadioNewReleaseRadarHandler handles GET /radio/new-releases
func (h *RadioHandler) GetRadioNewReleaseRadarHandler(ctx context.Context, req *GetRadioNewReleaseRadarRequest) (*GetRadioNewReleaseRadarResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	releases, err := h.aggregationReader.GetNewReleaseRadar(req.StationID, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch new release radar", err)
	}

	resp := &GetRadioNewReleaseRadarResponse{}
	resp.Body.Releases = releases
	if releases != nil {
		resp.Body.Count = len(releases)
	}
	return resp, nil
}

// ============================================================================
// Public: Radio Stats
// ============================================================================

// GetRadioStatsRequest represents the request for overall radio stats.
type GetRadioStatsRequest struct{}

// GetRadioStatsResponse represents the response for overall radio stats.
type GetRadioStatsResponse struct {
	Body *contracts.RadioStatsResponse
}

// GetRadioStatsHandler handles GET /radio/stats
func (h *RadioHandler) GetRadioStatsHandler(ctx context.Context, req *GetRadioStatsRequest) (*GetRadioStatsResponse, error) {
	stats, err := h.aggregationReader.GetRadioStats()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch radio stats", err)
	}
	return &GetRadioStatsResponse{Body: stats}, nil
}

// ============================================================================
// Admin: Create Radio Station
// ============================================================================

// AdminCreateRadioStationRequest represents the request for creating a radio station.
type AdminCreateRadioStationRequest struct {
	Body struct {
		Name             string           `json:"name" doc:"Station name" example:"KEXP"`
		Slug             string           `json:"slug,omitempty" required:"false" doc:"Custom slug (auto-generated if empty)"`
		Description      *string          `json:"description,omitempty" required:"false" doc:"Station description"`
		City             *string          `json:"city,omitempty" required:"false" doc:"City" example:"Seattle"`
		State            *string          `json:"state,omitempty" required:"false" doc:"State" example:"WA"`
		Country          *string          `json:"country,omitempty" required:"false" doc:"Country" example:"US"`
		Timezone         *string          `json:"timezone,omitempty" required:"false" doc:"Timezone" example:"America/Los_Angeles"`
		StreamURL        *string          `json:"stream_url,omitempty" required:"false" doc:"Primary stream URL"`
		StreamURLs       *json.RawMessage `json:"stream_urls,omitempty" required:"false" doc:"Additional stream URLs (JSONB)"`
		Website          *string          `json:"website,omitempty" required:"false" doc:"Website URL"`
		DonationURL      *string          `json:"donation_url,omitempty" required:"false" doc:"Donation page URL"`
		DonationEmbedURL *string          `json:"donation_embed_url,omitempty" required:"false" doc:"Embeddable donation URL"`
		LogoURL          *string          `json:"logo_url,omitempty" required:"false" doc:"Logo image URL"`
		Social           *json.RawMessage `json:"social,omitempty" required:"false" doc:"Social media links (JSONB)"`
		BroadcastType    string           `json:"broadcast_type" doc:"Broadcast type" example:"fm"`
		FrequencyMHz     *float64         `json:"frequency_mhz,omitempty" required:"false" doc:"FM frequency" example:"90.3"`
		PlaylistSource   *string          `json:"playlist_source,omitempty" required:"false" doc:"Playlist source"`
		PlaylistConfig   *json.RawMessage `json:"playlist_config,omitempty" required:"false" doc:"Playlist config (JSONB)"`
	}
}

// AdminCreateRadioStationResponse represents the response for creating a radio station.
type AdminCreateRadioStationResponse struct {
	Body *contracts.RadioStationDetailResponse
}

// AdminCreateRadioStationHandler handles POST /admin/radio-stations
func (h *RadioHandler) AdminCreateRadioStationHandler(ctx context.Context, req *AdminCreateRadioStationRequest) (*AdminCreateRadioStationResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}
	if req.Body.BroadcastType == "" {
		return nil, huma.Error400BadRequest("Broadcast type is required")
	}

	serviceReq := &contracts.CreateRadioStationRequest{
		Name:             req.Body.Name,
		Slug:             req.Body.Slug,
		Description:      req.Body.Description,
		City:             req.Body.City,
		State:            req.Body.State,
		Country:          req.Body.Country,
		Timezone:         req.Body.Timezone,
		StreamURL:        req.Body.StreamURL,
		StreamURLs:       req.Body.StreamURLs,
		Website:          req.Body.Website,
		DonationURL:      req.Body.DonationURL,
		DonationEmbedURL: req.Body.DonationEmbedURL,
		LogoURL:          req.Body.LogoURL,
		Social:           req.Body.Social,
		BroadcastType:    req.Body.BroadcastType,
		FrequencyMHz:     req.Body.FrequencyMHz,
		PlaylistSource:   req.Body.PlaylistSource,
		PlaylistConfig:   req.Body.PlaylistConfig,
	}

	station, err := h.stationWriter.CreateStation(serviceReq)
	if err != nil {
		logger.FromContext(ctx).Error("create_radio_station_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create radio station (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_radio_station", "radio_station", station.ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("radio_station_created",
		"station_id", station.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminCreateRadioStationResponse{Body: station}, nil
}

// ============================================================================
// Admin: Update Radio Station
// ============================================================================

// AdminUpdateRadioStationRequest represents the request for updating a radio station.
type AdminUpdateRadioStationRequest struct {
	StationID uint `path:"id" doc:"Radio station ID" example:"1"`
	Body      struct {
		Name             *string          `json:"name,omitempty" required:"false" doc:"Station name"`
		Description      *string          `json:"description,omitempty" required:"false" doc:"Station description"`
		City             *string          `json:"city,omitempty" required:"false" doc:"City"`
		State            *string          `json:"state,omitempty" required:"false" doc:"State"`
		Country          *string          `json:"country,omitempty" required:"false" doc:"Country"`
		Timezone         *string          `json:"timezone,omitempty" required:"false" doc:"Timezone"`
		StreamURL        *string          `json:"stream_url,omitempty" required:"false" doc:"Primary stream URL"`
		StreamURLs       *json.RawMessage `json:"stream_urls,omitempty" required:"false" doc:"Additional stream URLs"`
		Website          *string          `json:"website,omitempty" required:"false" doc:"Website URL"`
		DonationURL      *string          `json:"donation_url,omitempty" required:"false" doc:"Donation page URL"`
		DonationEmbedURL *string          `json:"donation_embed_url,omitempty" required:"false" doc:"Embeddable donation URL"`
		LogoURL          *string          `json:"logo_url,omitempty" required:"false" doc:"Logo image URL"`
		Social           *json.RawMessage `json:"social,omitempty" required:"false" doc:"Social media links"`
		BroadcastType    *string          `json:"broadcast_type,omitempty" required:"false" doc:"Broadcast type"`
		FrequencyMHz     *float64         `json:"frequency_mhz,omitempty" required:"false" doc:"FM frequency"`
		PlaylistSource   *string          `json:"playlist_source,omitempty" required:"false" doc:"Playlist source"`
		PlaylistConfig   *json.RawMessage `json:"playlist_config,omitempty" required:"false" doc:"Playlist config"`
		IsActive         *bool            `json:"is_active,omitempty" required:"false" doc:"Whether station is active"`
	}
}

// AdminUpdateRadioStationResponse represents the response for updating a radio station.
type AdminUpdateRadioStationResponse struct {
	Body *contracts.RadioStationDetailResponse
}

// AdminUpdateRadioStationHandler handles PUT /admin/radio-stations/{id}
func (h *RadioHandler) AdminUpdateRadioStationHandler(ctx context.Context, req *AdminUpdateRadioStationRequest) (*AdminUpdateRadioStationResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	serviceReq := &contracts.UpdateRadioStationRequest{
		Name:             req.Body.Name,
		Description:      req.Body.Description,
		City:             req.Body.City,
		State:            req.Body.State,
		Country:          req.Body.Country,
		Timezone:         req.Body.Timezone,
		StreamURL:        req.Body.StreamURL,
		StreamURLs:       req.Body.StreamURLs,
		Website:          req.Body.Website,
		DonationURL:      req.Body.DonationURL,
		DonationEmbedURL: req.Body.DonationEmbedURL,
		LogoURL:          req.Body.LogoURL,
		Social:           req.Body.Social,
		BroadcastType:    req.Body.BroadcastType,
		FrequencyMHz:     req.Body.FrequencyMHz,
		PlaylistSource:   req.Body.PlaylistSource,
		PlaylistConfig:   req.Body.PlaylistConfig,
		IsActive:         req.Body.IsActive,
	}

	station, err := h.stationWriter.UpdateStation(req.StationID, serviceReq)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioStationNotFound {
			return nil, huma.Error404NotFound("Radio station not found")
		}
		logger.FromContext(ctx).Error("update_radio_station_failed",
			"station_id", req.StationID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update radio station (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "update_radio_station", "radio_station", req.StationID, nil)
		}()
	}

	logger.FromContext(ctx).Info("radio_station_updated",
		"station_id", req.StationID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminUpdateRadioStationResponse{Body: station}, nil
}

// ============================================================================
// Admin: Delete Radio Station
// ============================================================================

// AdminDeleteRadioStationRequest represents the request for deleting a radio station.
type AdminDeleteRadioStationRequest struct {
	StationID uint `path:"id" doc:"Radio station ID" example:"1"`
}

// AdminDeleteRadioStationHandler handles DELETE /admin/radio-stations/{id}
func (h *RadioHandler) AdminDeleteRadioStationHandler(ctx context.Context, req *AdminDeleteRadioStationRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	err = h.stationWriter.DeleteStation(req.StationID)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioStationNotFound {
			return nil, huma.Error404NotFound("Radio station not found")
		}
		logger.FromContext(ctx).Error("delete_radio_station_failed",
			"station_id", req.StationID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete radio station (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_radio_station", "radio_station", req.StationID, nil)
		}()
	}

	logger.FromContext(ctx).Info("radio_station_deleted",
		"station_id", req.StationID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Admin: Create Radio Show
// ============================================================================

// AdminCreateRadioShowRequest represents the request for creating a radio show.
type AdminCreateRadioShowRequest struct {
	StationID uint `path:"id" doc:"Radio station ID" example:"1"`
	Body      struct {
		Name            string           `json:"name" doc:"Show name" example:"Morning Show"`
		Slug            string           `json:"slug,omitempty" required:"false" doc:"Custom slug (auto-generated if empty)"`
		HostName        *string          `json:"host_name,omitempty" required:"false" doc:"Host name" example:"DJ Cool"`
		Description     *string          `json:"description,omitempty" required:"false" doc:"Show description"`
		ScheduleDisplay *string          `json:"schedule_display,omitempty" required:"false" doc:"Human-readable schedule" example:"Mon-Fri 6-10am"`
		Schedule        *json.RawMessage `json:"schedule,omitempty" required:"false" doc:"Machine-readable schedule (JSONB)"`
		GenreTags       *json.RawMessage `json:"genre_tags,omitempty" required:"false" doc:"Genre tags (JSONB)"`
		ArchiveURL      *string          `json:"archive_url,omitempty" required:"false" doc:"Archive URL"`
		ImageURL        *string          `json:"image_url,omitempty" required:"false" doc:"Show image URL"`
		ExternalID      *string          `json:"external_id,omitempty" required:"false" doc:"External ID from source"`
	}
}

// AdminCreateRadioShowResponse represents the response for creating a radio show.
type AdminCreateRadioShowResponse struct {
	Body *contracts.RadioShowDetailResponse
}

// AdminCreateRadioShowHandler handles POST /admin/radio-stations/{id}/shows
func (h *RadioHandler) AdminCreateRadioShowHandler(ctx context.Context, req *AdminCreateRadioShowRequest) (*AdminCreateRadioShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}

	serviceReq := &contracts.CreateRadioShowRequest{
		Name:            req.Body.Name,
		Slug:            req.Body.Slug,
		HostName:        req.Body.HostName,
		Description:     req.Body.Description,
		ScheduleDisplay: req.Body.ScheduleDisplay,
		Schedule:        req.Body.Schedule,
		GenreTags:       req.Body.GenreTags,
		ArchiveURL:      req.Body.ArchiveURL,
		ImageURL:        req.Body.ImageURL,
		ExternalID:      req.Body.ExternalID,
	}

	show, err := h.showWriter.CreateShow(req.StationID, serviceReq)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioStationNotFound {
			return nil, huma.Error404NotFound("Radio station not found")
		}
		logger.FromContext(ctx).Error("create_radio_show_failed",
			"station_id", req.StationID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create radio show (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_radio_show", "radio_show", show.ID, map[string]interface{}{
				"station_id": req.StationID,
			})
		}()
	}

	logger.FromContext(ctx).Info("radio_show_created",
		"show_id", show.ID,
		"station_id", req.StationID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminCreateRadioShowResponse{Body: show}, nil
}

// ============================================================================
// Admin: Update Radio Show
// ============================================================================

// AdminUpdateRadioShowRequest represents the request for updating a radio show.
type AdminUpdateRadioShowRequest struct {
	ShowID uint `path:"id" doc:"Radio show ID" example:"1"`
	Body   struct {
		Name            *string          `json:"name,omitempty" required:"false" doc:"Show name"`
		HostName        *string          `json:"host_name,omitempty" required:"false" doc:"Host name"`
		Description     *string          `json:"description,omitempty" required:"false" doc:"Show description"`
		ScheduleDisplay *string          `json:"schedule_display,omitempty" required:"false" doc:"Human-readable schedule"`
		Schedule        *json.RawMessage `json:"schedule,omitempty" required:"false" doc:"Machine-readable schedule"`
		GenreTags       *json.RawMessage `json:"genre_tags,omitempty" required:"false" doc:"Genre tags"`
		ArchiveURL      *string          `json:"archive_url,omitempty" required:"false" doc:"Archive URL"`
		ImageURL        *string          `json:"image_url,omitempty" required:"false" doc:"Show image URL"`
		IsActive        *bool            `json:"is_active,omitempty" required:"false" doc:"Whether show is active"`
	}
}

// AdminUpdateRadioShowResponse represents the response for updating a radio show.
type AdminUpdateRadioShowResponse struct {
	Body *contracts.RadioShowDetailResponse
}

// AdminUpdateRadioShowHandler handles PUT /admin/radio-shows/{id}
func (h *RadioHandler) AdminUpdateRadioShowHandler(ctx context.Context, req *AdminUpdateRadioShowRequest) (*AdminUpdateRadioShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	serviceReq := &contracts.UpdateRadioShowRequest{
		Name:            req.Body.Name,
		HostName:        req.Body.HostName,
		Description:     req.Body.Description,
		ScheduleDisplay: req.Body.ScheduleDisplay,
		Schedule:        req.Body.Schedule,
		GenreTags:       req.Body.GenreTags,
		ArchiveURL:      req.Body.ArchiveURL,
		ImageURL:        req.Body.ImageURL,
		IsActive:        req.Body.IsActive,
	}

	show, err := h.showWriter.UpdateShow(req.ShowID, serviceReq)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioShowNotFound {
			return nil, huma.Error404NotFound("Radio show not found")
		}
		logger.FromContext(ctx).Error("update_radio_show_failed",
			"show_id", req.ShowID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update radio show (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "update_radio_show", "radio_show", req.ShowID, nil)
		}()
	}

	logger.FromContext(ctx).Info("radio_show_updated",
		"show_id", req.ShowID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminUpdateRadioShowResponse{Body: show}, nil
}

// ============================================================================
// Admin: Delete Radio Show
// ============================================================================

// AdminDeleteRadioShowRequest represents the request for deleting a radio show.
type AdminDeleteRadioShowRequest struct {
	ShowID uint `path:"id" doc:"Radio show ID" example:"1"`
}

// AdminDeleteRadioShowHandler handles DELETE /admin/radio-shows/{id}
func (h *RadioHandler) AdminDeleteRadioShowHandler(ctx context.Context, req *AdminDeleteRadioShowRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	err = h.showWriter.DeleteShow(req.ShowID)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioShowNotFound {
			return nil, huma.Error404NotFound("Radio show not found")
		}
		logger.FromContext(ctx).Error("delete_radio_show_failed",
			"show_id", req.ShowID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete radio show (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_radio_show", "radio_show", req.ShowID, nil)
		}()
	}

	logger.FromContext(ctx).Info("radio_show_deleted",
		"show_id", req.ShowID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Admin: Trigger Playlist Fetch (stub)
// ============================================================================

// AdminTriggerFetchRequest represents the request for triggering a playlist fetch.
type AdminTriggerFetchRequest struct {
	StationID uint `path:"id" doc:"Radio station ID" example:"1"`
}

// AdminTriggerFetchHandler handles POST /admin/radio-stations/{id}/fetch
// This is a stub that returns 501 Not Implemented until the KEXP provider is built.
func (h *RadioHandler) AdminTriggerFetchHandler(ctx context.Context, req *AdminTriggerFetchRequest) (*struct{}, error) {
	_, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	return nil, huma.Error501NotImplemented("Playlist fetch not yet implemented")
}

// ============================================================================
// Helpers
// ============================================================================

// resolveStation resolves a station by slug or numeric ID.
func (h *RadioHandler) resolveStation(slugOrID string) (*contracts.RadioStationDetailResponse, error) {
	if id, parseErr := strconv.ParseUint(slugOrID, 10, 32); parseErr == nil {
		station, err := h.stationReader.GetStation(uint(id))
		if err != nil {
			return nil, mapRadioStationError(err)
		}
		return station, nil
	}

	station, err := h.stationReader.GetStationBySlug(slugOrID)
	if err != nil {
		return nil, mapRadioStationError(err)
	}
	return station, nil
}

// resolveShow resolves a radio show by slug or numeric ID.
func (h *RadioHandler) resolveShow(slugOrID string) (*contracts.RadioShowDetailResponse, error) {
	if id, parseErr := strconv.ParseUint(slugOrID, 10, 32); parseErr == nil {
		show, err := h.showReader.GetShow(uint(id))
		if err != nil {
			return nil, mapRadioShowError(err)
		}
		return show, nil
	}

	show, err := h.showReader.GetShowBySlug(slugOrID)
	if err != nil {
		return nil, mapRadioShowError(err)
	}
	return show, nil
}

// resolveArtistID resolves an artist slug or numeric ID to its numeric ID.
func (h *RadioHandler) resolveArtistID(slugOrID string) (uint, error) {
	if id, parseErr := strconv.ParseUint(slugOrID, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	artist, err := h.artistResolver.GetArtistBySlug(slugOrID)
	if err != nil {
		return 0, huma.Error404NotFound("Artist not found")
	}
	return artist.ID, nil
}

// resolveReleaseID resolves a release slug or numeric ID to its numeric ID.
func (h *RadioHandler) resolveReleaseID(slugOrID string) (uint, error) {
	if id, parseErr := strconv.ParseUint(slugOrID, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	release, err := h.releaseResolver.GetReleaseBySlug(slugOrID)
	if err != nil {
		return 0, huma.Error404NotFound("Release not found")
	}
	return release.ID, nil
}

// mapRadioStationError maps a radio service error to a Huma HTTP error.
func mapRadioStationError(err error) error {
	var radioErr *apperrors.RadioError
	if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioStationNotFound {
		return huma.Error404NotFound("Radio station not found")
	}
	return huma.Error500InternalServerError("Failed to fetch radio station", err)
}

// mapRadioShowError maps a radio service error to a Huma HTTP error.
func mapRadioShowError(err error) error {
	var radioErr *apperrors.RadioError
	if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioShowNotFound {
		return huma.Error404NotFound("Radio show not found")
	}
	return huma.Error500InternalServerError("Failed to fetch radio show", err)
}
