package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// ============================================================================
// Focused interfaces for RadioHandler
// ============================================================================

// RadioStationReader reads radio stations (public endpoints).
type RadioStationReader interface {
	GetStation(stationID uint) (*contracts.RadioStationDetailResponse, error)
	GetStationBySlug(slug string) (*contracts.RadioStationDetailResponse, error)
	ResolveStationIDBySlug(slug string) (uint, error)
	ListStations(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error)
}

// RadioShowReader reads radio shows (public endpoints).
type RadioShowReader interface {
	GetShow(showID uint) (*contracts.RadioShowDetailResponse, error)
	GetShowBySlug(slug string) (*contracts.RadioShowDetailResponse, error)
	ListShows(stationID uint, sortBy string) ([]*contracts.RadioShowListResponse, error)
}

// RadioEpisodeReader reads radio episodes (public endpoints).
type RadioEpisodeReader interface {
	GetEpisodes(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error)
	GetEpisodeByShowAndDate(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error)
	GetStationEpisodes(stationID uint, limit, offset int) ([]*contracts.RadioStationEpisodeRow, int64, error)
	GetRecentEpisodes(limit, offset int) ([]*contracts.RadioStationEpisodeRow, int64, error)
}

// RadioNowPlayingReader reads a station's live/latest-archive now-playing
// payload (PSY-1022).
type RadioNowPlayingReader interface {
	GetStationNowPlaying(stationID uint) (*contracts.RadioNowPlayingResponse, error)
}

// RadioAggregationReader reads radio aggregation/stats data (public endpoints).
type RadioAggregationReader interface {
	GetTopArtistsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error)
	GetTopLabelsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error)
	GetTopArtistsForStation(stationID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error)
	GetTopLabelsForStation(stationID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error)
	GetAsHeardOnForArtist(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error)
	GetAsHeardOnForRelease(releaseID uint) ([]*contracts.RadioAsHeardOnResponse, error)
	GetNewReleaseRadar(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error)
	GetStationGraph(stationID uint, window string, limit int) (*contracts.RadioStationGraphResponse, error)
	GetRadioStats() (*contracts.RadioStatsResponse, error)
}

// RadioUnmatchedManager manages unmatched radio plays (admin endpoints).
type RadioUnmatchedManager interface {
	GetUnmatchedPlays(stationID uint, limit, offset int) ([]*contracts.UnmatchedPlayGroup, int64, error)
	LinkPlay(playID uint, req *contracts.LinkPlayRequest) error
	BulkLinkPlays(req *contracts.BulkLinkRequest) (*contracts.BulkLinkResult, error)
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

// RadioSyncManager triggers and observes unified ingestion runs (admin
// endpoints, PSY-1135). The triggers are async: they open a radio_sync_runs row
// and return its poll handle (status=running) while the run executes in the
// background. Replaces the old discover/fetch/import + import-job interfaces.
type RadioSyncManager interface {
	TriggerStationSync(stationID uint, mode string) (*contracts.RadioSyncRunResponse, error)
	TriggerShowBackfill(showID uint, since, until string) (*contracts.RadioSyncRunResponse, error)
	GetSyncRun(runID uint) (*contracts.RadioSyncRunResponse, error)
	CancelSyncRun(runID uint) error
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
	nowPlayingReader  RadioNowPlayingReader
	aggregationReader RadioAggregationReader
	stationWriter     RadioStationWriter
	showWriter        RadioShowWriter
	unmatchedManager  RadioUnmatchedManager
	syncManager       RadioSyncManager
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
		nowPlayingReader:  radioService,
		aggregationReader: radioService,
		stationWriter:     radioService,
		showWriter:        radioService,
		unmatchedManager:  radioService,
		syncManager:       radioService,
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
	switch req.IsActive {
	case "true":
		filters["is_active"] = true
	case "false":
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
// Public: Station Now Playing (PSY-1022)
// ============================================================================

// GetRadioStationNowPlayingRequest asks for a station's current broadcast.
type GetRadioStationNowPlayingRequest struct {
	Slug string `path:"slug" doc:"Radio station slug or numeric ID" example:"kexp"`
}

// GetRadioStationNowPlayingResponse carries the live (or latest-archive
// fallback) now-playing payload.
type GetRadioStationNowPlayingResponse struct {
	Body *contracts.RadioNowPlayingResponse
}

// GetRadioStationNowPlayingHandler handles GET /radio-stations/{slug}/now-playing.
// Provider failures never surface here — the service degrades to the
// latest-archive payload — so non-404 errors are genuine server errors.
func (h *RadioHandler) GetRadioStationNowPlayingHandler(ctx context.Context, req *GetRadioStationNowPlayingRequest) (*GetRadioStationNowPlayingResponse, error) {
	stationID, err := h.resolveStationID(req.Slug)
	if err != nil {
		return nil, err
	}

	nowPlaying, err := h.nowPlayingReader.GetStationNowPlaying(stationID)
	if err != nil {
		return nil, mapRadioStationErrorMsg(err, "Failed to fetch station now-playing")
	}

	return &GetRadioStationNowPlayingResponse{Body: nowPlaying}, nil
}

// ============================================================================
// Public: Station Latest Playlists (PSY-1048)
// ============================================================================

// GetRadioStationEpisodesRequest lists a station's latest playlists across
// all of its shows — strictly the requested station (PSY-1074).
type GetRadioStationEpisodesRequest struct {
	Slug   string `path:"slug" doc:"Radio station slug or numeric ID" example:"wfmu"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" default:"20" doc:"Max results (default 20)" example:"20"`
	Offset int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination" example:"0"`
}

// GetRadioStationEpisodesResponse is the station latest-playlists feed.
type GetRadioStationEpisodesResponse struct {
	Body struct {
		Episodes []*contracts.RadioStationEpisodeRow `json:"episodes" doc:"Latest episodes across the station's shows, newest first"`
		Total    int64                               `json:"total" doc:"Total number of episodes"`
	}
}

// GetRadioStationEpisodesHandler handles GET /radio-stations/{slug}/episodes
func (h *RadioHandler) GetRadioStationEpisodesHandler(ctx context.Context, req *GetRadioStationEpisodesRequest) (*GetRadioStationEpisodesResponse, error) {
	stationID, err := h.resolveStationID(req.Slug)
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

	episodes, total, err := h.episodeReader.GetStationEpisodes(stationID, limit, offset)
	if err != nil {
		return nil, mapRadioStationErrorMsg(err, "Failed to fetch station episodes")
	}

	resp := &GetRadioStationEpisodesResponse{}
	resp.Body.Episodes = episodes
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Public: Dial-wide Recent Playlists (PSY-1048)
// ============================================================================

// GetRecentRadioEpisodesRequest lists the newest playlists across every
// active station.
type GetRecentRadioEpisodesRequest struct {
	Limit  int `query:"limit" required:"false" minimum:"1" maximum:"100" default:"20" doc:"Max results (default 20)" example:"20"`
	Offset int `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination" example:"0"`
}

// GetRecentRadioEpisodesResponse is the dial-wide latest-playlists feed.
type GetRecentRadioEpisodesResponse struct {
	Body struct {
		Episodes []*contracts.RadioStationEpisodeRow `json:"episodes" doc:"Latest episodes across all active stations, newest first"`
		Total    int64                               `json:"total" doc:"Total number of episodes"`
	}
}

// GetRecentRadioEpisodesHandler handles GET /radio/episodes/recent
func (h *RadioHandler) GetRecentRadioEpisodesHandler(ctx context.Context, req *GetRecentRadioEpisodesRequest) (*GetRecentRadioEpisodesResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	episodes, total, err := h.episodeReader.GetRecentEpisodes(limit, offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch recent episodes", err)
	}

	resp := &GetRecentRadioEpisodesResponse{}
	resp.Body.Episodes = episodes
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Public: Station Top Artists / Top Labels (PSY-1048)
// ============================================================================

// GetRadioStationTopArtistsRequest asks for a station's most-played artists.
type GetRadioStationTopArtistsRequest struct {
	Slug   string `path:"slug" doc:"Radio station slug or numeric ID" example:"wfmu"`
	Period int    `query:"period" required:"false" default:"90" doc:"Period in days (default 90)" example:"90"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" default:"20" doc:"Max results (default 20)" example:"20"`
}

// GetRadioStationTopArtistsResponse carries a station's top artists.
type GetRadioStationTopArtistsResponse struct {
	Body struct {
		Artists []*contracts.RadioTopArtistResponse `json:"artists" doc:"Top artists across the station's shows"`
		Count   int                                 `json:"count" doc:"Number of results"`
	}
}

// GetRadioStationTopArtistsHandler handles GET /radio-stations/{slug}/top-artists
func (h *RadioHandler) GetRadioStationTopArtistsHandler(ctx context.Context, req *GetRadioStationTopArtistsRequest) (*GetRadioStationTopArtistsResponse, error) {
	stationID, err := h.resolveStationID(req.Slug)
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

	artists, err := h.aggregationReader.GetTopArtistsForStation(stationID, period, limit)
	if err != nil {
		return nil, mapRadioStationErrorMsg(err, "Failed to fetch top artists")
	}

	resp := &GetRadioStationTopArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Count = len(artists)
	return resp, nil
}

// GetRadioStationTopLabelsRequest asks for a station's most-featured labels.
type GetRadioStationTopLabelsRequest struct {
	Slug   string `path:"slug" doc:"Radio station slug or numeric ID" example:"wfmu"`
	Period int    `query:"period" required:"false" default:"90" doc:"Period in days (default 90)" example:"90"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" default:"20" doc:"Max results (default 20)" example:"20"`
}

// GetRadioStationTopLabelsResponse carries a station's top labels.
type GetRadioStationTopLabelsResponse struct {
	Body struct {
		Labels []*contracts.RadioTopLabelResponse `json:"labels" doc:"Top labels across the station's shows"`
		Count  int                                `json:"count" doc:"Number of results"`
	}
}

// GetRadioStationTopLabelsHandler handles GET /radio-stations/{slug}/top-labels
func (h *RadioHandler) GetRadioStationTopLabelsHandler(ctx context.Context, req *GetRadioStationTopLabelsRequest) (*GetRadioStationTopLabelsResponse, error) {
	stationID, err := h.resolveStationID(req.Slug)
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

	labels, err := h.aggregationReader.GetTopLabelsForStation(stationID, period, limit)
	if err != nil {
		return nil, mapRadioStationErrorMsg(err, "Failed to fetch top labels")
	}

	resp := &GetRadioStationTopLabelsResponse{}
	resp.Body.Labels = labels
	resp.Body.Count = len(labels)
	return resp, nil
}

// ============================================================================
// Public: Station Graph (PSY-1081)
// ============================================================================

// GetRadioStationGraphRequest asks for a station's co-occurrence subgraph.
//
// `Window` accepts "12m" (rolling last 12 months, the default) or "all".
// Huma's enum validation rejects other values with a 422; the service
// additionally coerces unknown values to the 12m default so non-HTTP callers
// degrade gracefully rather than 500ing.
type GetRadioStationGraphRequest struct {
	Slug   string `path:"slug" doc:"Radio station slug or numeric ID" example:"kexp"`
	Window string `query:"window" required:"false" default:"12m" enum:"all,12m" doc:"Time window: '12m' (rolling last 12 months, default) or 'all'" example:"12m"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"150" default:"75" doc:"Max artists (graph nodes), ranked by station play count (default 75)" example:"75"`
}

// GetRadioStationGraphResponse wraps the contracts payload for huma.
type GetRadioStationGraphResponse struct {
	Body *contracts.RadioStationGraphResponse
}

// GetRadioStationGraphHandler handles GET /radio-stations/{slug}/graph.
//
// PSY-1081 — station-scoped radio co-occurrence subgraph. Mirrors
// GET /scenes/{slug}/graph and GET /venues/{id}/bill-network in shape (the
// shared frontend ForceGraphView renders all three), with nodes = the
// station's top-N artists by play count and edges = within-station episode
// co-occurrence.
func (h *RadioHandler) GetRadioStationGraphHandler(ctx context.Context, req *GetRadioStationGraphRequest) (*GetRadioStationGraphResponse, error) {
	stationID, err := h.resolveStationID(req.Slug)
	if err != nil {
		return nil, err
	}

	graph, err := h.aggregationReader.GetStationGraph(stationID, req.Window, req.Limit)
	if err != nil {
		return nil, mapRadioStationErrorMsg(err, "Failed to fetch station graph")
	}

	return &GetRadioStationGraphResponse{Body: graph}, nil
}

// ============================================================================
// Public: List Radio Shows
// ============================================================================

// ListRadioShowsRequest represents the request for listing radio shows.
type ListRadioShowsRequest struct {
	StationID uint   `query:"station_id" doc:"Station ID (required)" example:"1"`
	Sort      string `query:"sort" required:"false" enum:"name,latest" doc:"Sort order: name (alphabetical, default) or latest (active shows first, most recent playlist first)" example:"latest"`
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
		return nil, huma.Error422UnprocessableEntity("station_id query parameter is required")
	}

	shows, err := h.showReader.ListShows(req.StationID, req.Sort)
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
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20)" example:"20"`
	Offset int    `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination" example:"0"`
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
	if _, err := shared.ParseDate(req.Date); err != nil {
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
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20)" example:"20"`
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
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20)" example:"20"`
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
	Limit     int  `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20)" example:"20"`
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
		BroadcastType    string           `json:"broadcast_type" doc:"Broadcast type (terrestrial, internet, both)" example:"both"`
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

	user := middleware.GetUserFromContext(ctx)

	if req.Body.Name == "" {
		return nil, huma.Error422UnprocessableEntity("Name is required")
	}
	if req.Body.BroadcastType == "" {
		return nil, huma.Error422UnprocessableEntity("Broadcast type is required")
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
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioStationNameConflict {
			return nil, huma.Error409Conflict(radioErr.Message)
		}
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
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "create_radio_station", "radio_station", station.ID, nil)
		})
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

	user := middleware.GetUserFromContext(ctx)

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
		if errors.As(err, &radioErr) {
			switch radioErr.Code {
			case apperrors.CodeRadioStationNotFound:
				return nil, huma.Error404NotFound("Radio station not found")
			case apperrors.CodeRadioStationNameConflict:
				return nil, huma.Error409Conflict(radioErr.Message)
			}
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
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "update_radio_station", "radio_station", req.StationID, nil)
		})
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

	user := middleware.GetUserFromContext(ctx)

	err := h.stationWriter.DeleteStation(req.StationID)
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
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "delete_radio_station", "radio_station", req.StationID, nil)
		})
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

	user := middleware.GetUserFromContext(ctx)

	if req.Body.Name == "" {
		return nil, huma.Error422UnprocessableEntity("Name is required")
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
		if errors.As(err, &radioErr) {
			switch radioErr.Code {
			case apperrors.CodeRadioStationNotFound:
				return nil, huma.Error404NotFound("Radio station not found")
			case apperrors.CodeRadioScheduleInvalid:
				return nil, huma.Error422UnprocessableEntity(radioErr.Message)
			}
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
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "create_radio_show", "radio_show", show.ID, map[string]interface{}{
				"station_id": req.StationID,
			})
		})
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
		LifecycleState  *string          `json:"lifecycle_state,omitempty" required:"false" enum:"active,dormant,retired" doc:"Operational state: active|dormant|retired. 'retired' is the manual-only 'ended forever' signal the janitor never sets/clobbers; active|dormant are advisory and may be re-reconciled nightly."`
		ScheduleLocked  *bool            `json:"schedule_locked,omitempty" required:"false" doc:"Pin schedule provenance (PSY-1186): true protects the schedule from the weekly WFMU scrape; false resumes auto-scrape. Omitted = a structured-schedule edit auto-locks."`
	}
}

// AdminUpdateRadioShowResponse represents the response for updating a radio show.
type AdminUpdateRadioShowResponse struct {
	Body *contracts.RadioShowDetailResponse
}

// AdminUpdateRadioShowHandler handles PUT /admin/radio-shows/{id}
func (h *RadioHandler) AdminUpdateRadioShowHandler(ctx context.Context, req *AdminUpdateRadioShowRequest) (*AdminUpdateRadioShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

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
		LifecycleState:  req.Body.LifecycleState,
		ScheduleLocked:  req.Body.ScheduleLocked,
	}

	show, err := h.showWriter.UpdateShow(req.ShowID, serviceReq)
	if err != nil {
		var radioErr *apperrors.RadioError
		if errors.As(err, &radioErr) {
			switch radioErr.Code {
			case apperrors.CodeRadioShowNotFound:
				return nil, huma.Error404NotFound("Radio show not found")
			case apperrors.CodeRadioScheduleInvalid, apperrors.CodeRadioLifecycleInvalid:
				return nil, huma.Error422UnprocessableEntity(radioErr.Message)
			}
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
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "update_radio_show", "radio_show", req.ShowID, nil)
		})
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

	user := middleware.GetUserFromContext(ctx)

	err := h.showWriter.DeleteShow(req.ShowID)
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
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "delete_radio_show", "radio_show", req.ShowID, nil)
		})
	}

	logger.FromContext(ctx).Info("radio_show_deleted",
		"show_id", req.ShowID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Admin: Trigger Station Sync (discover | fetch) — PSY-1135
// ============================================================================

// AdminSyncRunResponse wraps a sync-run DTO; shared by the trigger + poll
// endpoints so the frontend renders one shape across all radio ingestion runs.
type AdminSyncRunResponse struct {
	Body *contracts.RadioSyncRunResponse
}

// AdminTriggerStationSyncRequest triggers a manual station-scoped sync.
type AdminTriggerStationSyncRequest struct {
	StationID uint `path:"id" doc:"Radio station ID" example:"1"`
	Body      struct {
		Mode string `json:"mode" enum:"discover,fetch" doc:"Sync mode: 'discover' (find new shows) or 'fetch' (pull new episodes)" example:"fetch"`
	}
}

// AdminTriggerStationSyncHandler handles POST /admin/radio-stations/{id}/sync.
// Async: opens a radio_sync_runs row and returns it (status=running) while the
// run executes in the background. A manual trigger bypasses the circuit breaker.
func (h *RadioHandler) AdminTriggerStationSyncHandler(ctx context.Context, req *AdminTriggerStationSyncRequest) (*AdminSyncRunResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	run, err := h.syncManager.TriggerStationSync(req.StationID, req.Body.Mode)
	if err != nil {
		return nil, mapRadioSyncError(ctx, err, "Failed to trigger station sync")
	}

	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "trigger_radio_station_sync", "radio_station", req.StationID, map[string]interface{}{
				"mode":   req.Body.Mode,
				"run_id": run.ID,
			})
		})
	}

	logger.FromContext(ctx).Info("radio_station_sync_triggered",
		"station_id", req.StationID,
		"mode", req.Body.Mode,
		"run_id", run.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminSyncRunResponse{Body: run}, nil
}

// ============================================================================
// Admin: Trigger Show Backfill — PSY-1135
// ============================================================================

// AdminTriggerShowBackfillRequest triggers a manual historic backfill of one show.
type AdminTriggerShowBackfillRequest struct {
	ShowID uint `path:"id" doc:"Radio show ID" example:"1"`
	Body   struct {
		Since string `json:"since" doc:"Start date (YYYY-MM-DD)" example:"2025-01-01"`
		Until string `json:"until" doc:"End date (YYYY-MM-DD)" example:"2025-12-31"`
	}
}

// AdminTriggerShowBackfillHandler handles POST /admin/radio-shows/{id}/backfill.
// Async historic re-ingestion of one show over [since, until]; replaces the old
// import-job create+start.
func (h *RadioHandler) AdminTriggerShowBackfillHandler(ctx context.Context, req *AdminTriggerShowBackfillRequest) (*AdminSyncRunResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	if req.Body.Since == "" || req.Body.Until == "" {
		return nil, huma.Error422UnprocessableEntity("since and until dates are required")
	}
	since, err := shared.ParseDate(req.Body.Since)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("Invalid since date, expected YYYY-MM-DD")
	}
	until, err := shared.ParseDate(req.Body.Until)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("Invalid until date, expected YYYY-MM-DD")
	}
	// Reject a reversed window with a clean 422 rather than letting it open a run
	// row that the DB CHECK (window_end >= window_start) rejects as a 500, or that
	// silently imports zero episodes.
	if until.Before(since) {
		return nil, huma.Error422UnprocessableEntity("until must be on or after since")
	}

	run, err := h.syncManager.TriggerShowBackfill(req.ShowID, req.Body.Since, req.Body.Until)
	if err != nil {
		return nil, mapRadioSyncError(ctx, err, "Failed to trigger backfill")
	}

	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "trigger_radio_show_backfill", "radio_show", req.ShowID, map[string]interface{}{
				"since":  req.Body.Since,
				"until":  req.Body.Until,
				"run_id": run.ID,
			})
		})
	}

	logger.FromContext(ctx).Info("radio_show_backfill_triggered",
		"show_id", req.ShowID,
		"run_id", run.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminSyncRunResponse{Body: run}, nil
}

// ============================================================================
// Admin: List Unmatched Plays
// ============================================================================

// AdminGetUnmatchedPlaysRequest represents the request for listing unmatched plays.
type AdminGetUnmatchedPlaysRequest struct {
	StationID uint `query:"station_id" required:"false" doc:"Filter by station ID (0 for all)" example:"0"`
	Limit     int  `query:"limit" required:"false" default:"50" minimum:"1" maximum:"100" doc:"Max results (default 50)" example:"50"`
	Offset    int  `query:"offset" required:"false" default:"0" minimum:"0" doc:"Offset for pagination" example:"0"`
}

// AdminGetUnmatchedPlaysResponse represents the response for listing unmatched plays.
type AdminGetUnmatchedPlaysResponse struct {
	Body struct {
		Groups []*contracts.UnmatchedPlayGroup `json:"groups" doc:"Unmatched play groups"`
		Total  int64                           `json:"total" doc:"Total distinct artist names"`
	}
}

// AdminGetUnmatchedPlaysHandler handles GET /admin/radio/unmatched
func (h *RadioHandler) AdminGetUnmatchedPlaysHandler(ctx context.Context, req *AdminGetUnmatchedPlaysRequest) (*AdminGetUnmatchedPlaysResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	groups, total, err := h.unmatchedManager.GetUnmatchedPlays(req.StationID, limit, offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch unmatched plays", err)
	}

	resp := &AdminGetUnmatchedPlaysResponse{}
	resp.Body.Groups = groups
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Admin: Link Single Play
// ============================================================================

// AdminLinkPlayRequest represents the request for linking a single play to an artist.
type AdminLinkPlayRequest struct {
	PlayID uint `path:"id" doc:"Radio play ID" example:"1"`
	Body   struct {
		ArtistID  *uint `json:"artist_id,omitempty" required:"false" doc:"Artist ID to link"`
		ReleaseID *uint `json:"release_id,omitempty" required:"false" doc:"Release ID to link"`
		LabelID   *uint `json:"label_id,omitempty" required:"false" doc:"Label ID to link"`
	}
}

// AdminLinkPlayResponse represents the response for linking a single play.
type AdminLinkPlayResponse struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// AdminLinkPlayHandler handles POST /admin/radio/plays/{id}/link
func (h *RadioHandler) AdminLinkPlayHandler(ctx context.Context, req *AdminLinkPlayRequest) (*AdminLinkPlayResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	linkReq := &contracts.LinkPlayRequest{
		ArtistID:  req.Body.ArtistID,
		ReleaseID: req.Body.ReleaseID,
		LabelID:   req.Body.LabelID,
	}

	if err := h.unmatchedManager.LinkPlay(req.PlayID, linkReq); err != nil {
		logger.FromContext(ctx).Error("link_play_failed",
			"play_id", req.PlayID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to link play (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "link_radio_play", "radio_play", req.PlayID, map[string]interface{}{
				"artist_id":  req.Body.ArtistID,
				"release_id": req.Body.ReleaseID,
				"label_id":   req.Body.LabelID,
			})
		})
	}

	resp := &AdminLinkPlayResponse{}
	resp.Body.Success = true
	return resp, nil
}

// ============================================================================
// Admin: Bulk Link Plays
// ============================================================================

// AdminBulkLinkPlaysRequest represents the request for bulk-linking plays.
type AdminBulkLinkPlaysRequest struct {
	Body struct {
		ArtistName string `json:"artist_name" doc:"Artist name to match" example:"Radiohead"`
		ArtistID   uint   `json:"artist_id" doc:"Artist ID to link to" example:"123"`
	}
}

// AdminBulkLinkPlaysResponse represents the response for bulk-linking plays.
type AdminBulkLinkPlaysResponse struct {
	Body *contracts.BulkLinkResult
}

// AdminBulkLinkPlaysHandler handles POST /admin/radio/plays/bulk-link
func (h *RadioHandler) AdminBulkLinkPlaysHandler(ctx context.Context, req *AdminBulkLinkPlaysRequest) (*AdminBulkLinkPlaysResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	if req.Body.ArtistName == "" {
		return nil, huma.Error422UnprocessableEntity("artist_name is required")
	}
	if req.Body.ArtistID == 0 {
		return nil, huma.Error422UnprocessableEntity("artist_id is required")
	}

	bulkReq := &contracts.BulkLinkRequest{
		ArtistName: req.Body.ArtistName,
		ArtistID:   req.Body.ArtistID,
	}

	result, err := h.unmatchedManager.BulkLinkPlays(bulkReq)
	if err != nil {
		logger.FromContext(ctx).Error("bulk_link_plays_failed",
			"artist_name", req.Body.ArtistName,
			"artist_id", req.Body.ArtistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to bulk link plays (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "bulk_link_radio_plays", "radio_play", 0, map[string]interface{}{
				"artist_name": req.Body.ArtistName,
				"artist_id":   req.Body.ArtistID,
				"updated":     result.Updated,
			})
		})
	}

	logger.FromContext(ctx).Info("bulk_link_plays_complete",
		"artist_name", req.Body.ArtistName,
		"artist_id", req.Body.ArtistID,
		"updated", result.Updated,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminBulkLinkPlaysResponse{Body: result}, nil
}

// ============================================================================
// Admin: Get Sync Run (poll) — PSY-1135
// ============================================================================

// AdminGetSyncRunRequest polls a single sync run by id.
type AdminGetSyncRunRequest struct {
	RunID uint `path:"id" doc:"Sync run ID" example:"1"`
}

// AdminGetSyncRunHandler handles GET /admin/radio/sync-runs/{id}
func (h *RadioHandler) AdminGetSyncRunHandler(ctx context.Context, req *AdminGetSyncRunRequest) (*AdminSyncRunResponse, error) {
	run, err := h.syncManager.GetSyncRun(req.RunID)
	if err != nil {
		return nil, mapRadioSyncError(ctx, err, "Failed to fetch sync run")
	}
	return &AdminSyncRunResponse{Body: run}, nil
}

// ============================================================================
// Admin: Cancel Sync Run — PSY-1135
// ============================================================================

// AdminCancelSyncRunRequest cancels a running sync run by id.
type AdminCancelSyncRunRequest struct {
	RunID uint `path:"id" doc:"Sync run ID" example:"1"`
}

// AdminCancelSyncRunResponse reports the cancel outcome.
type AdminCancelSyncRunResponse struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// AdminCancelSyncRunHandler handles POST /admin/radio/sync-runs/{id}/cancel
func (h *RadioHandler) AdminCancelSyncRunHandler(ctx context.Context, req *AdminCancelSyncRunRequest) (*AdminCancelSyncRunResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	if err := h.syncManager.CancelSyncRun(req.RunID); err != nil {
		return nil, mapRadioSyncError(ctx, err, "Failed to cancel sync run")
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "cancel_radio_sync_run", "radio_sync_run", req.RunID, nil)
		})
	}

	logger.FromContext(ctx).Info("radio_sync_run_cancelled",
		"run_id", req.RunID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	resp := &AdminCancelSyncRunResponse{}
	resp.Body.Success = true
	return resp, nil
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

// resolveStationID resolves a station slug or numeric ID to just the ID,
// skipping the full detail build (network preload, show count, siblings)
// that resolveStation pays for — for station-scoped reads that only need
// the ID (PSY-1048). Numeric IDs pass through unverified; the service 404s
// on a missing station and mapRadioStationError surfaces it.
func (h *RadioHandler) resolveStationID(slugOrID string) (uint, error) {
	if id, parseErr := strconv.ParseUint(slugOrID, 10, 32); parseErr == nil {
		return uint(id), nil
	}
	id, err := h.stationReader.ResolveStationIDBySlug(slugOrID)
	if err != nil {
		return 0, mapRadioStationError(err)
	}
	return id, nil
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
	return mapRadioStationErrorMsg(err, "Failed to fetch radio station")
}

// mapRadioStationErrorMsg is mapRadioStationError with an operation-specific
// 500 message, for station-scoped endpoints whose failures are not station
// fetches (episode feeds, aggregations).
func mapRadioStationErrorMsg(err error, msg500 string) error {
	var radioErr *apperrors.RadioError
	if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioStationNotFound {
		return huma.Error404NotFound("Radio station not found")
	}
	return huma.Error500InternalServerError(msg500, err)
}

// mapRadioShowError maps a radio service error to a Huma HTTP error.
func mapRadioShowError(err error) error {
	var radioErr *apperrors.RadioError
	if errors.As(err, &radioErr) && radioErr.Code == apperrors.CodeRadioShowNotFound {
		return huma.Error404NotFound("Radio show not found")
	}
	return huma.Error500InternalServerError("Failed to fetch radio show", err)
}

// mapRadioSyncError maps a sync trigger/poll/cancel service error to a Huma HTTP
// error (PSY-1135). The typed RadioError codes carry the intended status
// (404 not-found, 409 already-running / not-cancellable); an unrecognized error
// is a 500 with a request-id-tagged log line and message.
func mapRadioSyncError(ctx context.Context, err error, msg500 string) error {
	var radioErr *apperrors.RadioError
	if errors.As(err, &radioErr) {
		switch radioErr.Code {
		case apperrors.CodeRadioStationNotFound:
			return huma.Error404NotFound("Radio station not found")
		case apperrors.CodeRadioShowNotFound:
			return huma.Error404NotFound("Radio show not found")
		case apperrors.CodeRadioSyncRunNotFound:
			return huma.Error404NotFound(radioErr.Message)
		case apperrors.CodeRadioSyncAlreadyRunning, apperrors.CodeRadioSyncNotCancellable:
			return huma.Error409Conflict(radioErr.Message)
		}
	}
	requestID := logger.GetRequestID(ctx)
	logger.FromContext(ctx).Error("radio_sync_operation_failed",
		"error", err.Error(),
		"request_id", requestID,
	)
	return huma.Error500InternalServerError(fmt.Sprintf("%s (request_id: %s)", msg500, requestID))
}
