package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ChartsHandler handles public top charts endpoints.
type ChartsHandler struct {
	chartsService contracts.ChartsServiceInterface
}

// NewChartsHandler creates a new charts handler.
func NewChartsHandler(
	chartsService contracts.ChartsServiceInterface,
) *ChartsHandler {
	return &ChartsHandler{
		chartsService: chartsService,
	}
}

// --- GetTrendingShows ---

// GetTrendingShowsRequest is the Huma request for GET /charts/trending-shows
type GetTrendingShowsRequest struct {
	Limit int `query:"limit" required:"false" doc:"Number of results (default 20, max 50)"`
}

// TrendingShowResponse is a single trending show in the response.
type TrendingShowResponse struct {
	ShowID          uint      `json:"show_id"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	Date            time.Time `json:"date"`
	VenueName       string    `json:"venue_name"`
	VenueSlug       string    `json:"venue_slug"`
	City            string    `json:"city"`
	GoingCount      int       `json:"going_count"`
	InterestedCount int       `json:"interested_count"`
	TotalAttendance int       `json:"total_attendance"`
}

// GetTrendingShowsResponse is the Huma response for GET /charts/trending-shows
type GetTrendingShowsResponse struct {
	Body struct {
		Shows []TrendingShowResponse `json:"shows"`
	}
}

// GetTrendingShowsHandler handles GET /charts/trending-shows
func (h *ChartsHandler) GetTrendingShowsHandler(ctx context.Context, req *GetTrendingShowsRequest) (*GetTrendingShowsResponse, error) {
	limit := normalizeChartsLimit(req.Limit)

	data, err := h.chartsService.GetTrendingShows(limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_trending_shows_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get trending shows")
	}

	resp := &GetTrendingShowsResponse{}
	resp.Body.Shows = make([]TrendingShowResponse, len(data))
	for i, s := range data {
		resp.Body.Shows[i] = TrendingShowResponse{
			ShowID:          s.ShowID,
			Title:           s.Title,
			Slug:            s.Slug,
			Date:            s.Date,
			VenueName:       s.VenueName,
			VenueSlug:       s.VenueSlug,
			City:            s.City,
			GoingCount:      s.GoingCount,
			InterestedCount: s.InterestedCount,
			TotalAttendance: s.TotalAttendance,
		}
	}
	return resp, nil
}

// --- GetPopularArtists ---

// GetPopularArtistsRequest is the Huma request for GET /charts/popular-artists
type GetPopularArtistsRequest struct {
	Limit int `query:"limit" required:"false" doc:"Number of results (default 20, max 50)"`
}

// PopularArtistResponse is a single popular artist in the response.
type PopularArtistResponse struct {
	ArtistID          uint   `json:"artist_id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	ImageURL          string `json:"image_url"`
	FollowCount       int    `json:"follow_count"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	Score             int    `json:"score"`
}

// GetPopularArtistsResponse is the Huma response for GET /charts/popular-artists
type GetPopularArtistsResponse struct {
	Body struct {
		Artists []PopularArtistResponse `json:"artists"`
	}
}

// GetPopularArtistsHandler handles GET /charts/popular-artists
func (h *ChartsHandler) GetPopularArtistsHandler(ctx context.Context, req *GetPopularArtistsRequest) (*GetPopularArtistsResponse, error) {
	limit := normalizeChartsLimit(req.Limit)

	data, err := h.chartsService.GetPopularArtists(limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_popular_artists_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get popular artists")
	}

	resp := &GetPopularArtistsResponse{}
	resp.Body.Artists = make([]PopularArtistResponse, len(data))
	for i, a := range data {
		resp.Body.Artists[i] = PopularArtistResponse{
			ArtistID:          a.ArtistID,
			Name:              a.Name,
			Slug:              a.Slug,
			ImageURL:          a.ImageURL,
			FollowCount:       a.FollowCount,
			UpcomingShowCount: a.UpcomingShowCount,
			Score:             a.Score,
		}
	}
	return resp, nil
}

// --- GetActiveVenues ---

// GetActiveVenuesRequest is the Huma request for GET /charts/active-venues
type GetActiveVenuesRequest struct {
	Limit int `query:"limit" required:"false" doc:"Number of results (default 20, max 50)"`
}

// ActiveVenueResponse is a single active venue in the response.
type ActiveVenueResponse struct {
	VenueID           uint   `json:"venue_id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city"`
	State             string `json:"state"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	FollowCount       int    `json:"follow_count"`
	Score             int    `json:"score"`
}

// GetActiveVenuesResponse is the Huma response for GET /charts/active-venues
type GetActiveVenuesResponse struct {
	Body struct {
		Venues []ActiveVenueResponse `json:"venues"`
	}
}

// GetActiveVenuesHandler handles GET /charts/active-venues
func (h *ChartsHandler) GetActiveVenuesHandler(ctx context.Context, req *GetActiveVenuesRequest) (*GetActiveVenuesResponse, error) {
	limit := normalizeChartsLimit(req.Limit)

	data, err := h.chartsService.GetActiveVenues(limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_active_venues_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get active venues")
	}

	resp := &GetActiveVenuesResponse{}
	resp.Body.Venues = make([]ActiveVenueResponse, len(data))
	for i, v := range data {
		resp.Body.Venues[i] = ActiveVenueResponse{
			VenueID:           v.VenueID,
			Name:              v.Name,
			Slug:              v.Slug,
			City:              v.City,
			State:             v.State,
			UpcomingShowCount: v.UpcomingShowCount,
			FollowCount:       v.FollowCount,
			Score:             v.Score,
		}
	}
	return resp, nil
}

// --- GetHotReleases ---

// GetHotReleasesRequest is the Huma request for GET /charts/hot-releases
type GetHotReleasesRequest struct {
	Limit int `query:"limit" required:"false" doc:"Number of results (default 20, max 50)"`
}

// HotReleaseResponse is a single hot release in the response.
type HotReleaseResponse struct {
	ReleaseID     uint       `json:"release_id"`
	Title         string     `json:"title"`
	Slug          string     `json:"slug"`
	ReleaseDate   *time.Time `json:"release_date"`
	ArtistNames   []string   `json:"artist_names"`
	BookmarkCount int        `json:"bookmark_count"`
}

// GetHotReleasesResponse is the Huma response for GET /charts/hot-releases
type GetHotReleasesResponse struct {
	Body struct {
		Releases []HotReleaseResponse `json:"releases"`
	}
}

// GetHotReleasesHandler handles GET /charts/hot-releases
func (h *ChartsHandler) GetHotReleasesHandler(ctx context.Context, req *GetHotReleasesRequest) (*GetHotReleasesResponse, error) {
	limit := normalizeChartsLimit(req.Limit)

	data, err := h.chartsService.GetHotReleases(limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_hot_releases_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get hot releases")
	}

	resp := &GetHotReleasesResponse{}
	resp.Body.Releases = make([]HotReleaseResponse, len(data))
	for i, r := range data {
		resp.Body.Releases[i] = HotReleaseResponse{
			ReleaseID:     r.ReleaseID,
			Title:         r.Title,
			Slug:          r.Slug,
			ReleaseDate:   r.ReleaseDate,
			ArtistNames:   r.ArtistNames,
			BookmarkCount: r.BookmarkCount,
		}
	}
	return resp, nil
}

// --- GetChartsOverview ---

// GetChartsOverviewRequest is the Huma request for GET /charts/overview
type GetChartsOverviewRequest struct{}

// GetChartsOverviewResponse is the Huma response for GET /charts/overview
type GetChartsOverviewResponse struct {
	Body struct {
		TrendingShows  []TrendingShowResponse  `json:"trending_shows"`
		PopularArtists []PopularArtistResponse `json:"popular_artists"`
		ActiveVenues   []ActiveVenueResponse   `json:"active_venues"`
		HotReleases    []HotReleaseResponse    `json:"hot_releases"`
	}
}

// GetChartsOverviewHandler handles GET /charts/overview
func (h *ChartsHandler) GetChartsOverviewHandler(ctx context.Context, _ *GetChartsOverviewRequest) (*GetChartsOverviewResponse, error) {
	data, err := h.chartsService.GetChartsOverview()
	if err != nil {
		logger.FromContext(ctx).Error("charts_overview_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get charts overview")
	}

	resp := &GetChartsOverviewResponse{}

	resp.Body.TrendingShows = make([]TrendingShowResponse, len(data.TrendingShows))
	for i, s := range data.TrendingShows {
		resp.Body.TrendingShows[i] = TrendingShowResponse{
			ShowID:          s.ShowID,
			Title:           s.Title,
			Slug:            s.Slug,
			Date:            s.Date,
			VenueName:       s.VenueName,
			VenueSlug:       s.VenueSlug,
			City:            s.City,
			GoingCount:      s.GoingCount,
			InterestedCount: s.InterestedCount,
			TotalAttendance: s.TotalAttendance,
		}
	}

	resp.Body.PopularArtists = make([]PopularArtistResponse, len(data.PopularArtists))
	for i, a := range data.PopularArtists {
		resp.Body.PopularArtists[i] = PopularArtistResponse{
			ArtistID:          a.ArtistID,
			Name:              a.Name,
			Slug:              a.Slug,
			ImageURL:          a.ImageURL,
			FollowCount:       a.FollowCount,
			UpcomingShowCount: a.UpcomingShowCount,
			Score:             a.Score,
		}
	}

	resp.Body.ActiveVenues = make([]ActiveVenueResponse, len(data.ActiveVenues))
	for i, v := range data.ActiveVenues {
		resp.Body.ActiveVenues[i] = ActiveVenueResponse{
			VenueID:           v.VenueID,
			Name:              v.Name,
			Slug:              v.Slug,
			City:              v.City,
			State:             v.State,
			UpcomingShowCount: v.UpcomingShowCount,
			FollowCount:       v.FollowCount,
			Score:             v.Score,
		}
	}

	resp.Body.HotReleases = make([]HotReleaseResponse, len(data.HotReleases))
	for i, r := range data.HotReleases {
		resp.Body.HotReleases[i] = HotReleaseResponse{
			ReleaseID:     r.ReleaseID,
			Title:         r.Title,
			Slug:          r.Slug,
			ReleaseDate:   r.ReleaseDate,
			ArtistNames:   r.ArtistNames,
			BookmarkCount: r.BookmarkCount,
		}
	}

	return resp, nil
}

// --- Helpers ---

// normalizeChartsLimit clamps the limit param to a valid range [1, 50], defaulting to 20.
func normalizeChartsLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}
