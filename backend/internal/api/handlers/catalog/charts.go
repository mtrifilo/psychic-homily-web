package catalog

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
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
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// TrendingShowResponse is a single trending show in the response.
type TrendingShowResponse struct {
	ShowID      uint      `json:"show_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Date        time.Time `json:"date"`
	VenueName   string    `json:"venue_name"`
	VenueSlug   string    `json:"venue_slug"`
	City        string    `json:"city"`
	ArtistNames []string  `json:"artist_names"`
	SaveCount   int       `json:"save_count"`
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
			ShowID:      s.ShowID,
			Title:       s.Title,
			Slug:        s.Slug,
			Date:        s.Date,
			VenueName:   s.VenueName,
			VenueSlug:   s.VenueSlug,
			City:        s.City,
			ArtistNames: s.ArtistNames,
			SaveCount:   s.SaveCount,
		}
	}
	return resp, nil
}

// --- GetMostAnticipatedShows ---

// GetMostAnticipatedShowsRequest is the Huma request for GET /charts/most-anticipated
type GetMostAnticipatedShowsRequest struct {
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// MostAnticipatedShowResponse is a single show in the response. SaveCount is
// omitted entirely in soonest_upcoming fallback mode — the fallback exists
// because counts are too sparse to render, so they never appear in it.
type MostAnticipatedShowResponse struct {
	ShowID      uint      `json:"show_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Date        time.Time `json:"date"`
	VenueName   string    `json:"venue_name"`
	VenueSlug   string    `json:"venue_slug"`
	City        string    `json:"city"`
	ArtistNames []string  `json:"artist_names"`
	SaveCount   *int      `json:"save_count,omitempty"`
}

// GetMostAnticipatedShowsResponse is the Huma response for GET /charts/most-anticipated
type GetMostAnticipatedShowsResponse struct {
	Body struct {
		Mode  string                        `json:"mode" enum:"ranked,soonest_upcoming" doc:"ranked = save-floor chart with counts; soonest_upcoming = date-ordered fallback, counts omitted"`
		Shows []MostAnticipatedShowResponse `json:"shows"`
	}
}

// GetMostAnticipatedShowsHandler handles GET /charts/most-anticipated —
// upcoming shows over the save floor ranked by saves, or the date-ordered
// soonest_upcoming fallback when too few qualify. Replaces
// /charts/trending-shows; the legacy route stays until the redesigned charts
// frontend migrates off it.
func (h *ChartsHandler) GetMostAnticipatedShowsHandler(ctx context.Context, req *GetMostAnticipatedShowsRequest) (*GetMostAnticipatedShowsResponse, error) {
	limit := normalizeChartsLimit(req.Limit)

	data, err := h.chartsService.GetMostAnticipatedShows(limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_most_anticipated_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get most-anticipated shows")
	}

	resp := &GetMostAnticipatedShowsResponse{}
	resp.Body.Mode = string(data.Mode)
	resp.Body.Shows = make([]MostAnticipatedShowResponse, len(data.Shows))
	for i, s := range data.Shows {
		// Direct conversion (the structs are field-identical): a field added
		// to one side without the other breaks the build instead of silently
		// shipping a zero value.
		resp.Body.Shows[i] = MostAnticipatedShowResponse(s)
	}
	return resp, nil
}

// --- GetMostActiveArtists ---

// GetMostActiveArtistsRequest is the Huma request for GET /charts/most-active-artists
type GetMostActiveArtistsRequest struct {
	Window string `query:"window" required:"false" enum:"month,quarter,all_time" doc:"Rolling time window (default quarter)"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// MostActiveArtistResponse is a single ranked artist in the response.
type MostActiveArtistResponse struct {
	ArtistID      uint       `json:"artist_id"`
	Name          string     `json:"name"`
	Slug          string     `json:"slug"`
	City          string     `json:"city"`
	State         string     `json:"state"`
	ShowCount     int        `json:"show_count"`
	HeadlinePct   int        `json:"headline_pct"`
	LastShowDate  *time.Time `json:"last_show_date"`
	LastShowSlug  string     `json:"last_show_slug"`
	LastShowVenue string     `json:"last_show_venue"`
}

// GetMostActiveArtistsResponse is the Huma response for GET /charts/most-active-artists
type GetMostActiveArtistsResponse struct {
	Body struct {
		Window  string                     `json:"window"`
		Artists []MostActiveArtistResponse `json:"artists"`
	}
}

// GetMostActiveArtistsHandler handles GET /charts/most-active-artists
func (h *ChartsHandler) GetMostActiveArtistsHandler(ctx context.Context, req *GetMostActiveArtistsRequest) (*GetMostActiveArtistsResponse, error) {
	limit := normalizeChartsLimit(req.Limit)
	window := normalizeChartWindow(req.Window)

	data, err := h.chartsService.GetMostActiveArtists(window, limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_most_active_artists_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get most-active artists")
	}

	resp := &GetMostActiveArtistsResponse{}
	resp.Body.Window = string(window)
	resp.Body.Artists = make([]MostActiveArtistResponse, len(data))
	for i, a := range data {
		resp.Body.Artists[i] = MostActiveArtistResponse{
			ArtistID:      a.ArtistID,
			Name:          a.Name,
			Slug:          a.Slug,
			City:          a.City,
			State:         a.State,
			ShowCount:     a.ShowCount,
			HeadlinePct:   a.HeadlinePct,
			LastShowDate:  a.LastShowDate,
			LastShowSlug:  a.LastShowSlug,
			LastShowVenue: a.LastShowVenue,
		}
	}
	return resp, nil
}

// --- GetBusiestVenues ---

// GetBusiestVenuesRequest is the Huma request for GET /charts/busiest-venues
type GetBusiestVenuesRequest struct {
	Window string `query:"window" required:"false" enum:"month,quarter,all_time" doc:"Rolling time window (default quarter)"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// BusiestVenueResponse is a single ranked venue in the response.
type BusiestVenueResponse struct {
	VenueID   uint   `json:"venue_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	City      string `json:"city"`
	State     string `json:"state"`
	ShowCount int    `json:"show_count"`
}

// GetBusiestVenuesResponse is the Huma response for GET /charts/busiest-venues
type GetBusiestVenuesResponse struct {
	Body struct {
		Window string                 `json:"window"`
		Venues []BusiestVenueResponse `json:"venues"`
	}
}

// GetBusiestVenuesHandler handles GET /charts/busiest-venues — venues by
// shows HOSTED in the window (past tense). Contrast /charts/active-venues,
// which scores venues by upcoming shows + follows.
func (h *ChartsHandler) GetBusiestVenuesHandler(ctx context.Context, req *GetBusiestVenuesRequest) (*GetBusiestVenuesResponse, error) {
	limit := normalizeChartsLimit(req.Limit)
	window := normalizeChartWindow(req.Window)

	data, err := h.chartsService.GetBusiestVenues(window, limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_busiest_venues_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get busiest venues")
	}

	resp := &GetBusiestVenuesResponse{}
	resp.Body.Window = string(window)
	resp.Body.Venues = make([]BusiestVenueResponse, len(data))
	for i, v := range data {
		resp.Body.Venues[i] = BusiestVenueResponse{
			VenueID:   v.VenueID,
			Name:      v.Name,
			Slug:      v.Slug,
			City:      v.City,
			State:     v.State,
			ShowCount: v.ShowCount,
		}
	}
	return resp, nil
}

// --- GetOpenersToWatch ---

// GetOpenersToWatchRequest is the Huma request for GET /charts/openers-to-watch
type GetOpenersToWatchRequest struct {
	Window string `query:"window" required:"false" enum:"month,quarter,all_time" doc:"Rolling time window (default quarter)"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// OpenerToWatchResponse is a single ranked support artist in the response.
type OpenerToWatchResponse struct {
	ArtistID         uint   `json:"artist_id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	City             string `json:"city"`
	State            string `json:"state"`
	SupportSlotCount int    `json:"support_slot_count"`
}

// GetOpenersToWatchResponse is the Huma response for GET /charts/openers-to-watch
type GetOpenersToWatchResponse struct {
	Body struct {
		Window  string                  `json:"window"`
		Artists []OpenerToWatchResponse `json:"artists"`
	}
}

// GetOpenersToWatchHandler handles GET /charts/openers-to-watch
func (h *ChartsHandler) GetOpenersToWatchHandler(ctx context.Context, req *GetOpenersToWatchRequest) (*GetOpenersToWatchResponse, error) {
	limit := normalizeChartsLimit(req.Limit)
	window := normalizeChartWindow(req.Window)

	data, err := h.chartsService.GetOpenersToWatch(window, limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_openers_to_watch_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get openers to watch")
	}

	resp := &GetOpenersToWatchResponse{}
	resp.Body.Window = string(window)
	resp.Body.Artists = make([]OpenerToWatchResponse, len(data))
	for i, a := range data {
		resp.Body.Artists[i] = OpenerToWatchResponse{
			ArtistID:         a.ArtistID,
			Name:             a.Name,
			Slug:             a.Slug,
			City:             a.City,
			State:            a.State,
			SupportSlotCount: a.SupportSlotCount,
		}
	}
	return resp, nil
}

// --- GetOnTheRadioArtists ---

// GetOnTheRadioArtistsRequest is the Huma request for GET /charts/on-the-radio
type GetOnTheRadioArtistsRequest struct {
	Window string `query:"window" required:"false" enum:"month,quarter,all_time" doc:"Rolling time window (default quarter)"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// OnTheRadioArtistResponse is a single ranked artist in the response.
type OnTheRadioArtistResponse struct {
	ArtistID     uint   `json:"artist_id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	City         string `json:"city"`
	State        string `json:"state"`
	PlayCount    int    `json:"play_count"`
	StationCount int    `json:"station_count"`
	IsNew        bool   `json:"is_new"`
}

// GetOnTheRadioArtistsResponse is the Huma response for GET /charts/on-the-radio
type GetOnTheRadioArtistsResponse struct {
	Body struct {
		Window  string                     `json:"window"`
		Artists []OnTheRadioArtistResponse `json:"artists"`
	}
}

// GetOnTheRadioArtistsHandler handles GET /charts/on-the-radio — artists by
// resolved radio plays in the window. station_count counts broadcasters
// (network-grouped stations collapse to one); is_new means any in-window play
// was flagged new rotation.
func (h *ChartsHandler) GetOnTheRadioArtistsHandler(ctx context.Context, req *GetOnTheRadioArtistsRequest) (*GetOnTheRadioArtistsResponse, error) {
	limit := normalizeChartsLimit(req.Limit)
	window := normalizeChartWindow(req.Window)

	data, err := h.chartsService.GetOnTheRadioArtists(window, limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_on_the_radio_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get on-the-radio artists")
	}

	resp := &GetOnTheRadioArtistsResponse{}
	resp.Body.Window = string(window)
	resp.Body.Artists = make([]OnTheRadioArtistResponse, len(data))
	for i, a := range data {
		resp.Body.Artists[i] = OnTheRadioArtistResponse{
			ArtistID:     a.ArtistID,
			Name:         a.Name,
			Slug:         a.Slug,
			City:         a.City,
			State:        a.State,
			PlayCount:    a.PlayCount,
			StationCount: a.StationCount,
			IsNew:        a.IsNew,
		}
	}
	return resp, nil
}

// --- GetPopularArtists ---

// GetPopularArtistsRequest is the Huma request for GET /charts/popular-artists
type GetPopularArtistsRequest struct {
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
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
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
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

// GetActiveVenuesHandler handles GET /charts/active-venues — venues scored by
// UPCOMING shows + follows. Contrast /charts/busiest-venues, which counts
// past shows hosted in a window.
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
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
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

// --- GetNewReleases ---

// GetNewReleasesRequest is the Huma request for GET /charts/new-releases
type GetNewReleasesRequest struct {
	Window string `query:"window" required:"false" enum:"month,quarter,all_time" doc:"Rolling time window (default quarter)"`
	Limit  int    `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// NewReleaseResponse is a single release in the response. release_date is
// the world release date as a day-grain YYYY-MM-DD string (matching the
// release contracts); added_at is when the release entered the graph. A null
// release_date means the row surfaced by its graph-added day (the graph-new
// tell); rows with a known world date always order and window by it.
type NewReleaseResponse struct {
	ReleaseID   uint      `json:"release_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	ReleaseType string    `json:"release_type"`
	ReleaseDate *string   `json:"release_date"`
	AddedAt     time.Time `json:"added_at"`
	ArtistNames []string  `json:"artist_names"`
	LabelNames  []string  `json:"label_names"`
}

// GetNewReleasesResponse is the Huma response for GET /charts/new-releases
type GetNewReleasesResponse struct {
	Body struct {
		Window   string               `json:"window"`
		Releases []NewReleaseResponse `json:"releases"`
	}
}

// GetNewReleasesHandler handles GET /charts/new-releases — releases in the
// window ordered by date (release_date, else graph-added date), newest first,
// no engagement inputs. Replaces /charts/hot-releases; the legacy route stays
// until the redesigned charts frontend migrates off it.
func (h *ChartsHandler) GetNewReleasesHandler(ctx context.Context, req *GetNewReleasesRequest) (*GetNewReleasesResponse, error) {
	limit := normalizeChartsLimit(req.Limit)
	window := normalizeChartWindow(req.Window)

	data, err := h.chartsService.GetNewReleases(window, limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_new_releases_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get new releases")
	}

	resp := &GetNewReleasesResponse{}
	resp.Body.Window = string(window)
	resp.Body.Releases = make([]NewReleaseResponse, len(data))
	for i, r := range data {
		// Direct conversion (field-identical structs): a one-sided field add
		// breaks the build instead of silently shipping a zero value.
		resp.Body.Releases[i] = NewReleaseResponse(r)
	}
	return resp, nil
}

// --- GetChartsSummary ---

// GetChartsSummaryRequest is the Huma request for GET /charts/summary
type GetChartsSummaryRequest struct {
	Window string `query:"window" required:"false" enum:"month,quarter,all_time" doc:"Rolling time window (default quarter)"`
}

// GetChartsSummaryResponse is the Huma response for GET /charts/summary —
// the masthead proof-of-life stat strip.
type GetChartsSummaryResponse struct {
	Body struct {
		Window       string `json:"window"`
		ShowsAdded   int    `json:"shows_added"`
		NewArtists   int    `json:"new_artists"`
		NewReleases  int    `json:"new_releases"`
		RadioPlays   int    `json:"radio_plays"`
		ActiveScenes int    `json:"active_scenes"`
	}
}

// GetChartsSummaryHandler handles GET /charts/summary
func (h *ChartsHandler) GetChartsSummaryHandler(ctx context.Context, req *GetChartsSummaryRequest) (*GetChartsSummaryResponse, error) {
	window := normalizeChartWindow(req.Window)

	data, err := h.chartsService.GetChartsSummary(window)
	if err != nil {
		logger.FromContext(ctx).Error("charts_summary_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get charts summary")
	}

	resp := &GetChartsSummaryResponse{}
	resp.Body.Window = string(window)
	resp.Body.ShowsAdded = data.ShowsAdded
	resp.Body.NewArtists = data.NewArtists
	resp.Body.NewReleases = data.NewReleases
	resp.Body.RadioPlays = data.RadioPlays
	resp.Body.ActiveScenes = data.ActiveScenes
	return resp, nil
}

// --- GetFreshlyAdded ---

// GetFreshlyAddedRequest is the Huma request for GET /charts/freshly-added
type GetFreshlyAddedRequest struct {
	Limit int `query:"limit" required:"false" minimum:"1" maximum:"50" doc:"Number of results (default 20, max 50)"`
}

// FreshlyAddedItemResponse is a single ticker row in the response.
type FreshlyAddedItemResponse struct {
	EntityType string    `json:"entity_type" enum:"artist,venue,release,station" doc:"Entity type discriminator"`
	EntityID   uint      `json:"entity_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	AddedAt    time.Time `json:"added_at"`
}

// GetFreshlyAddedResponse is the Huma response for GET /charts/freshly-added
type GetFreshlyAddedResponse struct {
	Body struct {
		Items []FreshlyAddedItemResponse `json:"items"`
	}
}

// GetFreshlyAddedHandler handles GET /charts/freshly-added — the most
// recently added entities across types, newest first (the footer ticker).
func (h *ChartsHandler) GetFreshlyAddedHandler(ctx context.Context, req *GetFreshlyAddedRequest) (*GetFreshlyAddedResponse, error) {
	limit := normalizeChartsLimit(req.Limit)

	data, err := h.chartsService.GetFreshlyAdded(limit)
	if err != nil {
		logger.FromContext(ctx).Error("charts_freshly_added_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get freshly added items")
	}

	resp := &GetFreshlyAddedResponse{}
	resp.Body.Items = make([]FreshlyAddedItemResponse, len(data))
	for i, item := range data {
		// Direct conversion (field-identical structs): a one-sided field add
		// breaks the build instead of silently shipping a zero value.
		resp.Body.Items[i] = FreshlyAddedItemResponse(item)
	}
	return resp, nil
}

// --- GetPersonalChartsStats ---

// GetPersonalChartsStatsRequest is the Huma request for GET /charts/me
type GetPersonalChartsStatsRequest struct{}

// PersonalTopVenueResponse is the user's top venue in the response — the
// venue holding the most of their saved shows (each show attributed to its
// primary venue, so counts never sum past saved_shows).
type PersonalTopVenueResponse struct {
	VenueID        uint   `json:"venue_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	SavedShowCount int    `json:"saved_show_count"`
}

// GetPersonalChartsStatsResponse is the Huma response for GET /charts/me —
// the authed personal stats strip. Zeros are a valid shape (new user; the
// frontend renders a nudge instead of the strip); top_venue and
// first_activity_at are explicit nulls until the user has the underlying
// activity.
type GetPersonalChartsStatsResponse struct {
	// CacheControl is no-store: auth is cookie-based, so nothing else marks
	// this per-user response uncacheable — without it a browser's heuristic
	// cache (or any future proxy in front of the otherwise-public /charts/*)
	// could replay one user's private stats to another. Same intent as the
	// calendar/unsubscribe handlers (per-user responses marked uncacheable;
	// those are chi handlers setting the header imperatively).
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		SavedShows      int                       `json:"saved_shows"`
		ArtistsFollowed int                       `json:"artists_followed"`
		TopVenue        *PersonalTopVenueResponse `json:"top_venue"`
		FirstActivityAt *time.Time                `json:"first_activity_at"`
	}
}

// GetPersonalChartsStatsHandler handles GET /charts/me — all-time aggregates
// over the requesting user's own engagement rows. Registered on rc.Protected;
// the in-handler nil check is the same belt-and-suspenders 401 every
// protected handler carries.
func (h *ChartsHandler) GetPersonalChartsStatsHandler(ctx context.Context, _ *GetPersonalChartsStatsRequest) (*GetPersonalChartsStatsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	data, err := h.chartsService.GetPersonalChartsStats(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("charts_personal_stats_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get personal charts stats")
	}

	resp := &GetPersonalChartsStatsResponse{CacheControl: "no-store"}
	resp.Body.SavedShows = data.SavedShows
	resp.Body.ArtistsFollowed = data.ArtistsFollowed
	resp.Body.FirstActivityAt = data.FirstActivityAt
	if data.TopVenue != nil {
		// Direct conversion (field-identical structs): a one-sided field add
		// breaks the build instead of silently shipping a zero value.
		tv := PersonalTopVenueResponse(*data.TopVenue)
		resp.Body.TopVenue = &tv
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
			ShowID:      s.ShowID,
			Title:       s.Title,
			Slug:        s.Slug,
			Date:        s.Date,
			VenueName:   s.VenueName,
			VenueSlug:   s.VenueSlug,
			City:        s.City,
			ArtistNames: s.ArtistNames,
			SaveCount:   s.SaveCount,
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

// normalizeChartWindow maps the optional window query param to a ChartWindow.
// Invalid values never reach here — the enum tag on the request struct 422s
// them; the absent-param default is owned by ChartWindow.OrDefault.
func normalizeChartWindow(window string) contracts.ChartWindow {
	return contracts.ChartWindow(window).OrDefault()
}

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
