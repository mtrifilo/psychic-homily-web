package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
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
	artistService   contracts.ArtistServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
}

func NewArtistHandler(artistService contracts.ArtistServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface) *ArtistHandler {
	return &ArtistHandler{
		artistService:   artistService,
		auditLogService: auditLogService,
		revisionService: revisionService,
	}
}

// SearchArtistsRequest represents the autocomplete search request
type SearchArtistsRequest struct {
	Query string `query:"q" doc:"Search query for artist autocomplete" example:"radio"`
}

// SearchArtistsResponse represents the autocomplete search response
type SearchArtistsResponse struct {
	Body struct {
		Artists []*contracts.ArtistDetailResponse `json:"artists" doc:"Matching artists"`
		Count   int                               `json:"count" doc:"Number of results"`
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

// ListArtistsRequest represents the request for listing all artists
type ListArtistsRequest struct {
	State    string `query:"state" doc:"Filter by state" example:"AZ"`
	City     string `query:"city" doc:"Filter by city" example:"Phoenix"`
	Cities   string `query:"cities" doc:"Pipe-delimited multi-city filter (max 10): Phoenix,AZ|Mesa,AZ" example:"Phoenix,AZ|Mesa,AZ"`
	Tags     string `query:"tags" doc:"Comma-separated tag slugs. Multi-tag filter (PSY-309): AND by default (entity must have every tag); set tag_match=any for OR." example:"post-punk,phoenix"`
	TagMatch string `query:"tag_match" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// ListArtistsResponse represents the response for listing artists
type ListArtistsResponse struct {
	Body struct {
		Artists []*contracts.ArtistWithShowCountResponse `json:"artists" doc:"List of artists with upcoming show counts"`
		Count   int                                      `json:"count" doc:"Number of artists"`
	}
}

// ListArtistsHandler handles GET /artists - returns all artists
func (h *ArtistHandler) ListArtistsHandler(ctx context.Context, req *ListArtistsRequest) (*ListArtistsResponse, error) {
	filters := make(map[string]interface{})

	if req.Cities != "" {
		// Parse pipe-delimited multi-city param: "Phoenix,AZ|Mesa,AZ"
		pairs := strings.Split(req.Cities, "|")
		var cityFilters []map[string]string
		for _, pair := range pairs {
			parts := strings.SplitN(pair, ",", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				cityFilters = append(cityFilters, map[string]string{
					"city":  strings.TrimSpace(parts[0]),
					"state": strings.TrimSpace(parts[1]),
				})
			}
		}
		// Cap at 10 cities
		if len(cityFilters) > 10 {
			cityFilters = cityFilters[:10]
		}
		if len(cityFilters) > 0 {
			filters["cities"] = cityFilters
		}
	} else {
		if req.State != "" {
			filters["state"] = req.State
		}
		if req.City != "" {
			filters["city"] = req.City
		}
	}
	if tf := parseTagFilter(req.Tags, req.TagMatch); tf.HasTags() {
		filters["tag_filter"] = tf
		// PSY-495 (Bandcamp model): when a tag filter is engaged, drop the
		// default "has upcoming shows" activity gate so tag pages are
		// evergreen discovery surfaces. A fan filtering by "punk" wants
		// every punk-tagged artist — active or not — so the facet-chip
		// count matches the list result count.
		filters["skip_active_filter"] = true
	}

	artists, err := h.artistService.GetArtistsWithShowCounts(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch artists", err)
	}

	resp := &ListArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Count = len(artists)

	return resp, nil
}

// GetArtistCitiesRequest represents the request for getting artist cities
type GetArtistCitiesRequest struct{}

// GetArtistCitiesResponse represents the response for the artist cities endpoint
type GetArtistCitiesResponse struct {
	Body struct {
		Cities []*contracts.ArtistCityResponse `json:"cities" doc:"List of cities with artist counts"`
	}
}

// GetArtistCitiesHandler handles GET /artists/cities
func (h *ArtistHandler) GetArtistCitiesHandler(ctx context.Context, req *GetArtistCitiesRequest) (*GetArtistCitiesResponse, error) {
	cities, err := h.artistService.GetArtistCities()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch artist cities", err)
	}

	resp := &GetArtistCitiesResponse{}
	resp.Body.Cities = cities

	return resp, nil
}

// GetArtistRequest represents the request for getting a single artist
type GetArtistRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID or slug" example:"the-national"`
}

// GetArtistResponse represents the response for the get artist endpoint
type GetArtistResponse struct {
	Body *contracts.ArtistDetailResponse
}

// GetArtistHandler handles GET /artists/{artist_id} - returns a single artist by ID or slug
func (h *ArtistHandler) GetArtistHandler(ctx context.Context, req *GetArtistRequest) (*GetArtistResponse, error) {
	var artist *contracts.ArtistDetailResponse
	var err error

	// Try to parse as numeric ID first
	if id, parseErr := strconv.ParseUint(req.ArtistID, 10, 32); parseErr == nil {
		artist, err = h.artistService.GetArtist(uint(id))
	} else {
		// Fall back to slug lookup
		artist, err = h.artistService.GetArtistBySlug(req.ArtistID)
	}

	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
	}

	return &GetArtistResponse{Body: artist}, nil
}

// GetArtistShowsRequest represents the request for getting shows for an artist
type GetArtistShowsRequest struct {
	ArtistID   string `path:"artist_id" doc:"Artist ID or slug" example:"the-national"`
	Timezone   string `query:"timezone" doc:"Timezone for date filtering" example:"America/Phoenix"`
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"50" doc:"Maximum number of shows to return"`
	TimeFilter string `query:"time_filter" doc:"Filter shows by time: upcoming, past, or all" example:"upcoming" enum:"upcoming,past,all"`
}

// GetArtistShowsResponse represents the response for the artist shows endpoint
type GetArtistShowsResponse struct {
	Body struct {
		Shows    []*contracts.ArtistShowResponse `json:"shows" doc:"List of shows"`
		ArtistID uint                            `json:"artist_id" doc:"Artist ID (resolved from slug if provided)"`
		Total    int64                           `json:"total" doc:"Total number of shows matching filter"`
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

	// Resolve artist ID from ID or slug
	var artistID uint
	if id, parseErr := strconv.ParseUint(req.ArtistID, 10, 32); parseErr == nil {
		artistID = uint(id)
	} else {
		// Look up by slug to get the ID
		artist, err := h.artistService.GetArtistBySlug(req.ArtistID)
		if err != nil {
			var artistErr *apperrors.ArtistError
			if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
				return nil, huma.Error404NotFound("Artist not found")
			}
			return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
		}
		artistID = artist.ID
	}

	shows, total, err := h.artistService.GetShowsForArtist(artistID, timezone, limit, timeFilter)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch shows", err)
	}

	resp := &GetArtistShowsResponse{}
	resp.Body.Shows = shows
	resp.Body.ArtistID = artistID
	resp.Body.Total = total

	return resp, nil
}

// ============================================================================
// Artist Labels
// ============================================================================

// GetArtistLabelsRequest represents the request for getting labels for an artist
type GetArtistLabelsRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID or slug" example:"nirvana"`
}

// GetArtistLabelsResponse represents the response for the artist labels endpoint
type GetArtistLabelsResponse struct {
	Body struct {
		Labels []*contracts.ArtistLabelResponse `json:"labels" doc:"List of labels"`
		Count  int                              `json:"count" doc:"Number of labels"`
	}
}

// GetArtistLabelsHandler handles GET /artists/{artist_id}/labels
func (h *ArtistHandler) GetArtistLabelsHandler(ctx context.Context, req *GetArtistLabelsRequest) (*GetArtistLabelsResponse, error) {
	// Resolve artist ID from numeric ID or slug
	var artistID uint
	if id, parseErr := strconv.ParseUint(req.ArtistID, 10, 32); parseErr == nil {
		artistID = uint(id)
	} else {
		// Look up by slug to get the ID
		artist, err := h.artistService.GetArtistBySlug(req.ArtistID)
		if err != nil {
			var artistErr *apperrors.ArtistError
			if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
				return nil, huma.Error404NotFound("Artist not found")
			}
			return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
		}
		artistID = artist.ID
	}

	labels, err := h.artistService.GetLabelsForArtist(artistID)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch labels", err)
	}

	resp := &GetArtistLabelsResponse{}
	resp.Body.Labels = labels
	resp.Body.Count = len(labels)

	return resp, nil
}

// ============================================================================
// Admin Artist Handlers
// ============================================================================

// AdminCreateArtistRequest represents the request for creating a new artist (admin only)
type AdminCreateArtistRequest struct {
	Body struct {
		Name        string  `json:"name" required:"true" doc:"Artist name" maxLength:"255"`
		City        *string `json:"city" required:"false" doc:"Artist city" maxLength:"100"`
		State       *string `json:"state" required:"false" doc:"Artist state" maxLength:"100"`
		Country     *string `json:"country" required:"false" doc:"Artist country" maxLength:"100"`
		Instagram   *string `json:"instagram" required:"false" doc:"Instagram handle" maxLength:"255"`
		Facebook    *string `json:"facebook" required:"false" doc:"Facebook URL" maxLength:"500"`
		Twitter     *string `json:"twitter" required:"false" doc:"Twitter handle" maxLength:"255"`
		YouTube     *string `json:"youtube" required:"false" doc:"YouTube URL" maxLength:"500"`
		Spotify     *string `json:"spotify" required:"false" doc:"Spotify URL" maxLength:"500"`
		SoundCloud  *string `json:"soundcloud" required:"false" doc:"SoundCloud URL" maxLength:"500"`
		Bandcamp    *string `json:"bandcamp" required:"false" doc:"Bandcamp URL" maxLength:"500"`
		Website     *string `json:"website" required:"false" doc:"Website URL" maxLength:"500"`
		Description *string `json:"description" required:"false" doc:"Markdown description (max 5000 chars)" maxLength:"5000"`
	}
}

// AdminCreateArtistResponse represents the response for creating an artist
type AdminCreateArtistResponse struct {
	Body *contracts.ArtistDetailResponse
}

// AdminCreateArtistHandler handles POST /admin/artists
// Admin-only endpoint to create a new artist directly
func (h *ArtistHandler) AdminCreateArtistHandler(ctx context.Context, req *AdminCreateArtistRequest) (*AdminCreateArtistResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate name
	name := strings.TrimSpace(req.Body.Name)
	if name == "" {
		return nil, huma.Error400BadRequest("Artist name cannot be empty")
	}

	// Validate description length if provided
	if req.Body.Description != nil && len(*req.Body.Description) > 5000 {
		return nil, huma.Error400BadRequest("Description must be 5000 characters or fewer")
	}

	// Build the create request
	createReq := &contracts.CreateArtistRequest{
		Name:        name,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.YouTube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.SoundCloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
		Description: req.Body.Description,
	}

	artist, err := h.artistService.CreateArtist(createReq)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil, huma.Error409Conflict(err.Error())
		}
		logger.FromContext(ctx).Error("admin_create_artist_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create artist (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		h.auditLogService.LogAction(user.ID, "create_artist", "artist", artist.ID, map[string]interface{}{
			"name": name,
		})
	}

	logger.FromContext(ctx).Info("admin_create_artist_success",
		"artist_id", artist.ID,
		"artist_name", name,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminCreateArtistResponse{Body: artist}, nil
}

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
	Body *contracts.ArtistDetailResponse
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

	// Also set the social bandcamp profile URL from the embed URL
	if bandcampURL != nil {
		// Extract profile URL: https://artist.bandcamp.com/album/name → https://artist.bandcamp.com
		if idx := strings.Index(*bandcampURL, ".bandcamp.com"); idx != -1 {
			profileURL := (*bandcampURL)[:idx+len(".bandcamp.com")]
			updates["bandcamp"] = profileURL
		}
	} else {
		// Clear the social bandcamp link when clearing the embed URL
		updates["bandcamp"] = nil
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), updates)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
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
	Body *contracts.ArtistDetailResponse
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
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
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

// ============================================================================
// Delete Artist
// ============================================================================

// DeleteArtistRequest represents the request for deleting an artist
type DeleteArtistRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
}

// DeleteArtistHandler handles DELETE /artists/{artist_id}
// Only deletes artists with 0 show associations. Returns 409 Conflict if artist still has shows.
func (h *ArtistHandler) DeleteArtistHandler(ctx context.Context, req *DeleteArtistRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user from context
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	logger.FromContext(ctx).Debug("delete_artist_attempt",
		"artist_id", artistID,
		"user_id", user.ID,
		"request_id", requestID,
	)

	err = h.artistService.DeleteArtist(uint(artistID))
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) {
			switch artistErr.Code {
			case apperrors.CodeArtistNotFound:
				return nil, huma.Error404NotFound("Artist not found")
			case apperrors.CodeArtistHasShows:
				return nil, huma.Error409Conflict(artistErr.Message)
			}
		}
		logger.FromContext(ctx).Error("delete_artist_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete artist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("artist_deleted",
		"artist_id", artistID,
		"user_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Admin Update Artist
// ============================================================================

// AdminUpdateArtistRequest represents the request for updating an artist (admin only)
type AdminUpdateArtistRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
	Body     struct {
		Name        *string `json:"name,omitempty" required:"false" doc:"Artist name"`
		City        *string `json:"city,omitempty" required:"false" doc:"City"`
		State       *string `json:"state,omitempty" required:"false" doc:"State"`
		Country     *string `json:"country,omitempty" required:"false" doc:"Country"`
		Instagram   *string `json:"instagram,omitempty" required:"false" doc:"Instagram URL"`
		Facebook    *string `json:"facebook,omitempty" required:"false" doc:"Facebook URL"`
		Twitter     *string `json:"twitter,omitempty" required:"false" doc:"Twitter/X URL"`
		Youtube     *string `json:"youtube,omitempty" required:"false" doc:"YouTube URL"`
		Spotify     *string `json:"spotify,omitempty" required:"false" doc:"Spotify URL"`
		Soundcloud  *string `json:"soundcloud,omitempty" required:"false" doc:"SoundCloud URL"`
		Bandcamp    *string `json:"bandcamp,omitempty" required:"false" doc:"Bandcamp URL"`
		Website     *string `json:"website,omitempty" required:"false" doc:"Website URL"`
		Description *string `json:"description,omitempty" required:"false" doc:"Markdown description (max 5000 chars)"`
		Summary     *string `json:"summary,omitempty" required:"false" doc:"Revision summary describing the change"`
	}
}

// AdminUpdateArtistResponse represents the response for updating an artist
type AdminUpdateArtistResponse struct {
	Body *contracts.ArtistDetailResponse
}

// AdminUpdateArtistHandler handles PATCH /admin/artists/{artist_id}
// Admin-only endpoint to update any artist fields
func (h *ArtistHandler) AdminUpdateArtistHandler(ctx context.Context, req *AdminUpdateArtistRequest) (*AdminUpdateArtistResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Validate name if provided
	if req.Body.Name != nil && strings.TrimSpace(*req.Body.Name) == "" {
		return nil, huma.Error400BadRequest("Artist name cannot be empty")
	}

	// Validate description length if provided
	if req.Body.Description != nil && len(*req.Body.Description) > 5000 {
		return nil, huma.Error400BadRequest("Description must be 5000 characters or fewer")
	}

	// Capture old values for revision diff (fire-and-forget safe)
	var oldArtist *contracts.ArtistDetailResponse
	if h.revisionService != nil {
		oldArtist, _ = h.artistService.GetArtist(uint(artistID))
	}

	// Build updates map with only provided fields
	updates := map[string]interface{}{}

	if req.Body.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Body.Name)
	}
	if req.Body.City != nil {
		updates["city"] = nilIfEmpty(*req.Body.City)
	}
	if req.Body.State != nil {
		updates["state"] = nilIfEmpty(*req.Body.State)
	}
	if req.Body.Country != nil {
		updates["country"] = nilIfEmpty(*req.Body.Country)
	}
	if req.Body.Instagram != nil {
		updates["instagram"] = nilIfEmpty(*req.Body.Instagram)
	}
	if req.Body.Facebook != nil {
		updates["facebook"] = nilIfEmpty(*req.Body.Facebook)
	}
	if req.Body.Twitter != nil {
		updates["twitter"] = nilIfEmpty(*req.Body.Twitter)
	}
	if req.Body.Youtube != nil {
		updates["youtube"] = nilIfEmpty(*req.Body.Youtube)
	}
	if req.Body.Spotify != nil {
		updates["spotify"] = nilIfEmpty(*req.Body.Spotify)
	}
	if req.Body.Soundcloud != nil {
		updates["soundcloud"] = nilIfEmpty(*req.Body.Soundcloud)
	}
	if req.Body.Bandcamp != nil {
		updates["bandcamp"] = nilIfEmpty(*req.Body.Bandcamp)
	}
	if req.Body.Website != nil {
		updates["website"] = nilIfEmpty(*req.Body.Website)
	}
	if req.Body.Description != nil {
		updates["description"] = nilIfEmpty(*req.Body.Description)
	}

	if len(updates) == 0 {
		return nil, huma.Error400BadRequest("No fields to update")
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), updates)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		logger.FromContext(ctx).Error("admin_update_artist_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update artist (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		h.auditLogService.LogAction(user.ID, "edit_artist", "artist", uint(artistID), nil)
	}

	// Record revision (fire and forget)
	if h.revisionService != nil && oldArtist != nil {
		go func() {
			changes := computeArtistChanges(oldArtist, artist)
			if len(changes) > 0 {
				summary := ""
				if req.Body.Summary != nil {
					summary = *req.Body.Summary
				}
				if err := h.revisionService.RecordRevision("artist", uint(artistID), user.ID, changes, summary); err != nil {
					logger.Default().Error("record_artist_revision_failed",
						"artist_id", artistID,
						"error", err.Error(),
					)
				}
			}
		}()
	}

	logger.FromContext(ctx).Info("admin_update_artist_success",
		"artist_id", artistID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminUpdateArtistResponse{Body: artist}, nil
}

// computeArtistChanges compares old and new artist detail responses and returns field-level diffs.
func computeArtistChanges(old, new *contracts.ArtistDetailResponse) []adminm.FieldChange {
	var changes []adminm.FieldChange

	if old.Name != new.Name {
		changes = append(changes, adminm.FieldChange{Field: "name", OldValue: old.Name, NewValue: new.Name})
	}
	if ptrToStr(old.City) != ptrToStr(new.City) {
		changes = append(changes, adminm.FieldChange{Field: "city", OldValue: ptrToStr(old.City), NewValue: ptrToStr(new.City)})
	}
	if ptrToStr(old.State) != ptrToStr(new.State) {
		changes = append(changes, adminm.FieldChange{Field: "state", OldValue: ptrToStr(old.State), NewValue: ptrToStr(new.State)})
	}
	if ptrToStr(old.Social.Instagram) != ptrToStr(new.Social.Instagram) {
		changes = append(changes, adminm.FieldChange{Field: "instagram", OldValue: ptrToStr(old.Social.Instagram), NewValue: ptrToStr(new.Social.Instagram)})
	}
	if ptrToStr(old.Social.Facebook) != ptrToStr(new.Social.Facebook) {
		changes = append(changes, adminm.FieldChange{Field: "facebook", OldValue: ptrToStr(old.Social.Facebook), NewValue: ptrToStr(new.Social.Facebook)})
	}
	if ptrToStr(old.Social.Twitter) != ptrToStr(new.Social.Twitter) {
		changes = append(changes, adminm.FieldChange{Field: "twitter", OldValue: ptrToStr(old.Social.Twitter), NewValue: ptrToStr(new.Social.Twitter)})
	}
	if ptrToStr(old.Social.YouTube) != ptrToStr(new.Social.YouTube) {
		changes = append(changes, adminm.FieldChange{Field: "youtube", OldValue: ptrToStr(old.Social.YouTube), NewValue: ptrToStr(new.Social.YouTube)})
	}
	if ptrToStr(old.Social.Spotify) != ptrToStr(new.Social.Spotify) {
		changes = append(changes, adminm.FieldChange{Field: "spotify", OldValue: ptrToStr(old.Social.Spotify), NewValue: ptrToStr(new.Social.Spotify)})
	}
	if ptrToStr(old.Social.SoundCloud) != ptrToStr(new.Social.SoundCloud) {
		changes = append(changes, adminm.FieldChange{Field: "soundcloud", OldValue: ptrToStr(old.Social.SoundCloud), NewValue: ptrToStr(new.Social.SoundCloud)})
	}
	if ptrToStr(old.Social.Bandcamp) != ptrToStr(new.Social.Bandcamp) {
		changes = append(changes, adminm.FieldChange{Field: "bandcamp", OldValue: ptrToStr(old.Social.Bandcamp), NewValue: ptrToStr(new.Social.Bandcamp)})
	}
	if ptrToStr(old.Social.Website) != ptrToStr(new.Social.Website) {
		changes = append(changes, adminm.FieldChange{Field: "website", OldValue: ptrToStr(old.Social.Website), NewValue: ptrToStr(new.Social.Website)})
	}
	if ptrToStr(old.Description) != ptrToStr(new.Description) {
		changes = append(changes, adminm.FieldChange{Field: "description", OldValue: ptrToStr(old.Description), NewValue: ptrToStr(new.Description)})
	}

	return changes
}

// ptrToStr safely dereferences a *string, returning "" if nil.
func ptrToStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nilIfEmpty returns nil if the string is empty, otherwise returns a pointer to the string
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ============================================================================
// Artist Aliases
// ============================================================================

// GetArtistAliasesRequest represents the request for getting an artist's aliases
type GetArtistAliasesRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID" example:"42"`
}

// GetArtistAliasesResponse represents the response for the artist aliases endpoint
type GetArtistAliasesResponse struct {
	Body struct {
		Aliases []*contracts.ArtistAliasResponse `json:"aliases" doc:"List of aliases"`
		Count   int                              `json:"count" doc:"Number of aliases"`
	}
}

// GetArtistAliasesHandler handles GET /artists/{artist_id}/aliases
func (h *ArtistHandler) GetArtistAliasesHandler(ctx context.Context, req *GetArtistAliasesRequest) (*GetArtistAliasesResponse, error) {
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	aliases, err := h.artistService.GetArtistAliases(uint(artistID))
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch aliases", err)
	}

	resp := &GetArtistAliasesResponse{}
	resp.Body.Aliases = aliases
	resp.Body.Count = len(aliases)

	return resp, nil
}

// AddArtistAliasRequest represents the request for adding an alias
type AddArtistAliasRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID" example:"42"`
	Body     struct {
		Alias string `json:"alias" doc:"Alias name" example:"The Artist Formerly Known As"`
	}
}

// AddArtistAliasResponse represents the response for adding an alias
type AddArtistAliasResponse struct {
	Body *contracts.ArtistAliasResponse
}

// AddArtistAliasHandler handles POST /admin/artists/{artist_id}/aliases
func (h *ArtistHandler) AddArtistAliasHandler(ctx context.Context, req *AddArtistAliasRequest) (*AddArtistAliasResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	if strings.TrimSpace(req.Body.Alias) == "" {
		return nil, huma.Error400BadRequest("Alias cannot be empty")
	}

	alias, err := h.artistService.AddArtistAlias(uint(artistID), req.Body.Alias)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "conflicts with") {
			return nil, huma.Error409Conflict(err.Error())
		}
		logger.FromContext(ctx).Error("add_artist_alias_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to add alias (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		h.auditLogService.LogAction(user.ID, "add_artist_alias", "artist", uint(artistID), map[string]interface{}{
			"alias": req.Body.Alias,
		})
	}

	return &AddArtistAliasResponse{Body: alias}, nil
}

// DeleteArtistAliasRequest represents the request for deleting an alias
type DeleteArtistAliasRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID" example:"42"`
	AliasID  string `path:"alias_id" doc:"Alias ID" example:"1"`
}

// DeleteArtistAliasHandler handles DELETE /admin/artists/{artist_id}/aliases/{alias_id}
func (h *ArtistHandler) DeleteArtistAliasHandler(ctx context.Context, req *DeleteArtistAliasRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	aliasID, err := strconv.ParseUint(req.AliasID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid alias ID")
	}

	err = h.artistService.RemoveArtistAlias(uint(aliasID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Alias not found")
		}
		logger.FromContext(ctx).Error("delete_artist_alias_failed",
			"alias_id", aliasID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete alias (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		artistID, _ := strconv.ParseUint(req.ArtistID, 10, 32)
		h.auditLogService.LogAction(user.ID, "delete_artist_alias", "artist", uint(artistID), map[string]interface{}{
			"alias_id": aliasID,
		})
	}

	return nil, nil
}

// ============================================================================
// Artist Merge
// ============================================================================

// MergeArtistsRequest represents the request for merging two artists
type MergeArtistsRequest struct {
	Body struct {
		CanonicalArtistID uint `json:"canonical_artist_id" doc:"ID of the artist to keep"`
		MergeFromArtistID uint `json:"merge_from_artist_id" doc:"ID of the artist to merge and delete"`
	}
}

// MergeArtistsResponse represents the response for merging two artists
type MergeArtistsResponse struct {
	Body *contracts.MergeArtistResult
}

// MergeArtistsHandler handles POST /admin/artists/merge
func (h *ArtistHandler) MergeArtistsHandler(ctx context.Context, req *MergeArtistsRequest) (*MergeArtistsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if req.Body.CanonicalArtistID == 0 || req.Body.MergeFromArtistID == 0 {
		return nil, huma.Error400BadRequest("Both canonical_artist_id and merge_from_artist_id are required")
	}

	result, err := h.artistService.MergeArtists(req.Body.CanonicalArtistID, req.Body.MergeFromArtistID)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		if strings.Contains(err.Error(), "cannot merge an artist with itself") {
			return nil, huma.Error400BadRequest("Cannot merge an artist with itself")
		}
		logger.FromContext(ctx).Error("merge_artists_failed",
			"canonical_id", req.Body.CanonicalArtistID,
			"merge_from_id", req.Body.MergeFromArtistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to merge artists (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		h.auditLogService.LogAction(user.ID, "merge_artists", "artist", req.Body.CanonicalArtistID, map[string]interface{}{
			"merged_artist_id":   req.Body.MergeFromArtistID,
			"merged_artist_name": result.MergedArtistName,
			"shows_moved":        result.ShowsMoved,
		})
	}

	logger.FromContext(ctx).Info("artists_merged",
		"canonical_id", req.Body.CanonicalArtistID,
		"merged_id", req.Body.MergeFromArtistID,
		"merged_name", result.MergedArtistName,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &MergeArtistsResponse{Body: result}, nil
}
