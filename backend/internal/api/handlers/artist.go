package handlers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// isInternalServiceRequest checks if the request has a valid internal service secret
func isInternalServiceRequest(ctx huma.Context) bool {
	internalSecret := os.Getenv("INTERNAL_API_SECRET")
	if internalSecret == "" {
		return false
	}
	requestSecret := ctx.Header("X-Internal-Secret")
	return requestSecret == internalSecret
}

type ArtistHandler struct {
	artistService *services.ArtistService
}

func NewArtistHandler() *ArtistHandler {
	return &ArtistHandler{
		artistService: services.NewArtistService(),
	}
}

// SearchArtistsRequest represents the autocomplete search request
type SearchArtistsRequest struct {
	Query string `query:"q" doc:"Search query for artist autocomplete" example:"radio"`
}

// SearchArtistsResponse represents the autocomplete search response
type SearchArtistsResponse struct {
	Body struct {
		Artists []*services.ArtistDetailResponse `json:"artists" doc:"Matching artists"`
		Count   int                              `json:"count" doc:"Number of results"`
	}
}

// SearchArtistsHandler handles GET /artists/search?q=query
func (h *ArtistHandler) SearchArtistsHandler(ctx context.Context, req *SearchArtistsRequest) (*SearchArtistsResponse, error) {
	artists, err := h.artistService.SearchArtists(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Count = len(artists)

	return resp, nil
}

// GetArtistRequest represents the request for getting a single artist
type GetArtistRequest struct {
	ArtistID uint `path:"artist_id" doc:"Artist ID" example:"1"`
}

// GetArtistResponse represents the response for the get artist endpoint
type GetArtistResponse struct {
	Body *services.ArtistDetailResponse
}

// GetArtistHandler handles GET /artists/{artist_id} - returns a single artist by ID
func (h *ArtistHandler) GetArtistHandler(ctx context.Context, req *GetArtistRequest) (*GetArtistResponse, error) {
	artist, err := h.artistService.GetArtist(req.ArtistID)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
	}

	return &GetArtistResponse{Body: artist}, nil
}

// GetArtistShowsRequest represents the request for getting shows for an artist
type GetArtistShowsRequest struct {
	ArtistID   uint   `path:"artist_id" doc:"Artist ID" example:"1"`
	Timezone   string `query:"timezone" doc:"Timezone for date filtering" example:"America/Phoenix"`
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"50" doc:"Maximum number of shows to return"`
	TimeFilter string `query:"time_filter" doc:"Filter shows by time: upcoming, past, or all" example:"upcoming" enum:"upcoming,past,all"`
}

// GetArtistShowsResponse represents the response for the artist shows endpoint
type GetArtistShowsResponse struct {
	Body struct {
		Shows    []*services.ArtistShowResponse `json:"shows" doc:"List of shows"`
		ArtistID uint                           `json:"artist_id" doc:"Artist ID"`
		Total    int64                          `json:"total" doc:"Total number of shows matching filter"`
	}
}

// GetArtistShowsHandler handles GET /artists/{artist_id}/shows - returns shows for an artist
func (h *ArtistHandler) GetArtistShowsHandler(ctx context.Context, req *GetArtistShowsRequest) (*GetArtistShowsResponse, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}

	timeFilter := req.TimeFilter
	if timeFilter == "" {
		timeFilter = "upcoming"
	}

	shows, total, err := h.artistService.GetShowsForArtist(req.ArtistID, timezone, limit, timeFilter)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch shows", err)
	}

	resp := &GetArtistShowsResponse{}
	resp.Body.Shows = shows
	resp.Body.ArtistID = req.ArtistID
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Admin Artist Handlers
// ============================================================================

// UpdateArtistBandcampRequest represents the request for updating bandcamp URL
type UpdateArtistBandcampRequest struct {
	ArtistID       string `path:"artist_id" validate:"required" doc:"Artist ID"`
	InternalSecret string `header:"X-Internal-Secret" doc:"Internal service secret for automated discovery"`
	Body           struct {
		BandcampEmbedURL *string `json:"bandcamp_embed_url" doc:"Bandcamp album or track URL for embedding. Set to null or empty string to clear."`
	}
}

// UpdateArtistBandcampResponse represents the response for updating bandcamp URL
type UpdateArtistBandcampResponse struct {
	Body *services.ArtistDetailResponse
}

// UpdateArtistBandcampHandler handles PATCH /admin/artists/{artist_id}/bandcamp
// Admin-only endpoint to update an artist's Bandcamp embed URL
// Also accepts internal service secret for automated discovery
func (h *ArtistHandler) UpdateArtistBandcampHandler(ctx context.Context, req *UpdateArtistBandcampRequest) (*UpdateArtistBandcampResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Check for internal service request (automated discovery)
	isInternalRequest := false
	internalSecret := os.Getenv("INTERNAL_API_SECRET")
	if internalSecret != "" && req.InternalSecret == internalSecret {
		isInternalRequest = true
		logger.FromContext(ctx).Info("internal_service_request",
			"endpoint", "update_artist_bandcamp",
			"request_id", requestID,
		)
	}

	// Get user for admin check (will be nil for internal requests)
	user := middleware.GetUserFromContext(ctx)

	// Verify admin access (unless internal request)
	if !isInternalRequest {
		if user == nil || !user.IsAdmin {
			var userID uint
			if user != nil {
				userID = user.ID
			}
			logger.FromContext(ctx).Warn("admin_access_denied",
				"user_id", userID,
				"request_id", requestID,
			)
			return nil, huma.Error403Forbidden("Admin access required")
		}
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Validate URL format if provided and not empty
	if req.Body.BandcampEmbedURL != nil && *req.Body.BandcampEmbedURL != "" {
		if !isValidBandcampURL(*req.Body.BandcampEmbedURL) {
			return nil, huma.Error400BadRequest("Invalid Bandcamp URL format. URL must be a bandcamp.com album or track URL.")
		}
	}

	var adminID uint
	if user != nil {
		adminID = user.ID
	}
	logger.FromContext(ctx).Debug("admin_update_artist_bandcamp_attempt",
		"artist_id", artistID,
		"admin_id", adminID,
		"is_internal", isInternalRequest,
	)

	// Prepare updates - if URL is empty string, set to nil to clear the field
	var bandcampURL *string
	if req.Body.BandcampEmbedURL != nil && *req.Body.BandcampEmbedURL != "" {
		bandcampURL = req.Body.BandcampEmbedURL
	}

	updates := map[string]interface{}{
		"bandcamp_embed_url": bandcampURL,
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), updates)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		logger.FromContext(ctx).Error("admin_update_artist_bandcamp_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update artist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_update_artist_bandcamp_success",
		"artist_id", artistID,
		"admin_id", adminID,
		"is_internal", isInternalRequest,
		"request_id", requestID,
	)

	return &UpdateArtistBandcampResponse{Body: artist}, nil
}

// isValidBandcampURL validates that the URL is a proper Bandcamp album/track URL
func isValidBandcampURL(url string) bool {
	// Must contain bandcamp.com
	if !strings.Contains(url, "bandcamp.com") {
		return false
	}
	// Must be an album or track URL (not just profile)
	if !strings.Contains(url, "/album/") && !strings.Contains(url, "/track/") {
		return false
	}
	return true
}

// ============================================================================
// Spotify URL Management
// ============================================================================

// UpdateArtistSpotifyRequest represents the request for updating Spotify URL
type UpdateArtistSpotifyRequest struct {
	ArtistID       string `path:"artist_id" validate:"required" doc:"Artist ID"`
	InternalSecret string `header:"X-Internal-Secret" doc:"Internal service secret for automated discovery"`
	Body           struct {
		SpotifyURL *string `json:"spotify_url" doc:"Spotify artist page URL. Set to null or empty string to clear."`
	}
}

// UpdateArtistSpotifyResponse represents the response for updating Spotify URL
type UpdateArtistSpotifyResponse struct {
	Body *services.ArtistDetailResponse
}

// UpdateArtistSpotifyHandler handles PATCH /admin/artists/{artist_id}/spotify
// Admin-only endpoint to update an artist's Spotify URL
// Also accepts internal service secret for automated discovery
func (h *ArtistHandler) UpdateArtistSpotifyHandler(ctx context.Context, req *UpdateArtistSpotifyRequest) (*UpdateArtistSpotifyResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Check for internal service request (automated discovery)
	isInternalRequest := false
	internalSecret := os.Getenv("INTERNAL_API_SECRET")
	if internalSecret != "" && req.InternalSecret == internalSecret {
		isInternalRequest = true
		logger.FromContext(ctx).Info("internal_service_request",
			"endpoint", "update_artist_spotify",
			"request_id", requestID,
		)
	}

	// Get user for admin check (will be nil for internal requests)
	user := middleware.GetUserFromContext(ctx)

	// Verify admin access (unless internal request)
	if !isInternalRequest {
		if user == nil || !user.IsAdmin {
			var userID uint
			if user != nil {
				userID = user.ID
			}
			logger.FromContext(ctx).Warn("admin_access_denied",
				"user_id", userID,
				"request_id", requestID,
			)
			return nil, huma.Error403Forbidden("Admin access required")
		}
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Validate URL format if provided and not empty
	if req.Body.SpotifyURL != nil && *req.Body.SpotifyURL != "" {
		if !isValidSpotifyURL(*req.Body.SpotifyURL) {
			return nil, huma.Error400BadRequest("Invalid Spotify URL format. URL must be in format: open.spotify.com/artist/{id}")
		}
	}

	var adminID uint
	if user != nil {
		adminID = user.ID
	}
	logger.FromContext(ctx).Debug("admin_update_artist_spotify_attempt",
		"artist_id", artistID,
		"admin_id", adminID,
		"is_internal", isInternalRequest,
	)

	// Prepare updates - if URL is empty string, set to nil to clear the field
	var spotifyURL *string
	if req.Body.SpotifyURL != nil && *req.Body.SpotifyURL != "" {
		spotifyURL = req.Body.SpotifyURL
	}

	updates := map[string]interface{}{
		"spotify": spotifyURL,
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), updates)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		logger.FromContext(ctx).Error("admin_update_artist_spotify_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update artist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_update_artist_spotify_success",
		"artist_id", artistID,
		"admin_id", adminID,
		"is_internal", isInternalRequest,
		"request_id", requestID,
	)

	return &UpdateArtistSpotifyResponse{Body: artist}, nil
}

// isValidSpotifyURL validates that the URL is a proper Spotify artist page URL
func isValidSpotifyURL(url string) bool {
	// Must contain open.spotify.com/artist/
	if !strings.Contains(url, "open.spotify.com/artist/") {
		return false
	}
	return true
}
