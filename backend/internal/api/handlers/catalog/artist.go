package catalog

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

type ArtistHandler struct {
	artistService   contracts.ArtistServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
	// internalSecret is the configured INTERNAL_API_SECRET, loaded once at
	// startup. The artist Bandcamp/Spotify mutation endpoints accept it as an
	// admin bypass for the discovery backfill bot.
	internalSecret string
}

func NewArtistHandler(artistService contracts.ArtistServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface, cfg *config.Config) *ArtistHandler {
	var internalSecret string
	if cfg != nil {
		internalSecret = cfg.MusicDiscovery.InternalAPISecret
	}
	return &ArtistHandler{
		artistService:   artistService,
		auditLogService: auditLogService,
		revisionService: revisionService,
		internalSecret:  internalSecret,
	}
}

// matchesInternalSecret reports whether the request-supplied secret matches the
// configured internal API secret. The comparison is constant-time to avoid a
// timing oracle: these endpoints are mounted on rc.Protected (not rc.Admin)
// because a match grants an admin-equivalent bypass. An empty configured secret
// never matches, so the bypass is disabled when INTERNAL_API_SECRET is unset.
func (h *ArtistHandler) matchesInternalSecret(provided string) bool {
	if h.internalSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(h.internalSecret)) == 1
}

// SearchArtistsRequest represents the autocomplete search request
type SearchArtistsRequest struct {
	Query string `query:"q" maxLength:"200" doc:"Search query for artist autocomplete" example:"radio"`
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
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"200" doc:"Maximum number of shows to return (max 200)"`
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
		Instagram   *string `json:"instagram" required:"false" doc:"Instagram URL" maxLength:"255"`
		Facebook    *string `json:"facebook" required:"false" doc:"Facebook URL" maxLength:"500"`
		Twitter     *string `json:"twitter" required:"false" doc:"Twitter URL" maxLength:"255"`
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

	user := middleware.GetUserFromContext(ctx)

	// Validate name
	name := strings.TrimSpace(req.Body.Name)
	if name == "" {
		return nil, huma.Error422UnprocessableEntity("Artist name cannot be empty")
	}

	// Validate description length if provided
	if req.Body.Description != nil && len(*req.Body.Description) > 5000 {
		return nil, huma.Error422UnprocessableEntity("Description must be 5000 characters or fewer")
	}

	// PSY-525: URL scheme validation (http/https only) for social URL fields.
	if err := shared.ValidateSocialURLs(req.Body.Instagram, req.Body.Facebook, req.Body.Twitter,
		req.Body.YouTube, req.Body.Spotify, req.Body.SoundCloud, req.Body.Bandcamp, req.Body.Website); err != nil {
		return nil, err
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
		if mapped := shared.MapArtistError(err); mapped != nil {
			return nil, mapped
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
	if h.matchesInternalSecret(req.InternalSecret) {
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
			return nil, huma.Error422UnprocessableEntity("Invalid Bandcamp URL format. URL must be a bandcamp.com album or track URL.")
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

	// Always write the embed column; an empty string clears it (the service
	// normalizes empty input to SQL NULL).
	embedValue := shared.Deref(req.Body.BandcampEmbedURL)
	serviceReq := &contracts.UpdateArtistRequest{
		BandcampEmbedURL: &embedValue,
	}

	// Mirror the embed URL into the social bandcamp profile URL.
	if embedValue != "" {
		// Extract profile URL: https://artist.bandcamp.com/album/name → https://artist.bandcamp.com
		if idx := strings.Index(embedValue, ".bandcamp.com"); idx != -1 {
			profileURL := embedValue[:idx+len(".bandcamp.com")]
			serviceReq.Bandcamp = &profileURL
		}
		// If the URL lacks a bandcamp.com host the social link is left
		// unchanged (Bandcamp stays nil — no write).
	} else {
		// Clear the social bandcamp link when clearing the embed URL.
		empty := ""
		serviceReq.Bandcamp = &empty
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), serviceReq)
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

// isValidBandcampURL validates that the URL is a proper Bandcamp album/track URL.
// It parses the URL and anchors on the host (not a substring match), so a
// hostile value like "http://169.254.169.254/album/x?bandcamp.com" — which the
// old strings.Contains check accepted — is rejected. This handler STORES the
// value (later rendered in an iframe src); it does not fetch it, so the win here
// is preventing a hostile/foreign host from being persisted, not SSRF.
//
// NOTE: the general social.bandcamp/social.spotify fields are host-anchored to
// *.bandcamp.com / *.spotify.com by ValidateSocialURLs (PSY-1113). This
// dedicated validator is the STRICTER gate the embed endpoint needs — it also
// requires the /album|/track path, since the value drives an iframe embed.
func isValidBandcampURL(rawURL string) bool {
	u, ok := parseHTTPURL(rawURL)
	if !ok {
		return false
	}
	// Real album/track pages always live on an artist subdomain
	// (<artist>.bandcamp.com); the bare apex is not a release URL, and the
	// social-link mirror (above) keys off the ".bandcamp.com" subdomain too.
	if !strings.HasSuffix(strings.ToLower(u.Hostname()), ".bandcamp.com") {
		return false
	}
	// Album or track page, not a bare profile.
	return strings.HasPrefix(u.Path, "/album/") || strings.HasPrefix(u.Path, "/track/")
}

// parseHTTPURL parses rawURL and confirms it has an http/https scheme and a
// host, returning the parsed URL. Shared by the two validators below so the
// parse + scheme check lives in one place. http is accepted (not just https) to
// match the codebase's URL convention (utils.ValidateHTTPURL); the host anchor,
// not the scheme, is what rejects hostile values.
func parseHTTPURL(rawURL string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	if u.Hostname() == "" {
		return nil, false
	}
	return u, true
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
	if h.matchesInternalSecret(req.InternalSecret) {
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
			return nil, huma.Error422UnprocessableEntity("Invalid Spotify URL format. URL must be in format: open.spotify.com/artist/{id}")
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

	// Always write the spotify column; an empty string clears it (the service
	// normalizes empty input to SQL NULL).
	spotifyValue := shared.Deref(req.Body.SpotifyURL)
	serviceReq := &contracts.UpdateArtistRequest{
		Spotify: &spotifyValue,
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), serviceReq)
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

// spotifyArtistPath matches an /artist/<id> segment ANYWHERE in the path, so a
// locale prefix (/intl-de/artist/...) or a trailing sub-tab (/artist/<id>/about)
// — both shapes Spotify's web player serves and admins paste verbatim — still
// validate. Id length is intentionally not pinned to 22 here: the security win
// is the open.spotify.com host anchor (below); the canonical 22-char form is a
// frontend normalization concern, and rejecting other lengths server-side would
// hard-422 real pasted URLs the BFF forwards unchanged.
var spotifyArtistPath = regexp.MustCompile(`/artist/[A-Za-z0-9]+(?:/|$)`)

// isValidSpotifyURL validates that the URL is a proper Spotify artist page URL.
// Parses and anchors on the open.spotify.com host, replacing the old substring
// check that accepted any URL merely containing "open.spotify.com/artist/" (e.g.
// on an attacker-controlled host like "https://evil.test/artist/<id>").
func isValidSpotifyURL(rawURL string) bool {
	u, ok := parseHTTPURL(rawURL)
	if !ok {
		return false
	}
	if strings.ToLower(u.Hostname()) != "open.spotify.com" {
		return false
	}
	return spotifyArtistPath.MatchString(u.Path)
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
		if mapped := shared.MapArtistError(err); mapped != nil {
			return nil, mapped
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

// hasArtistUpdateFields reports whether the request carries at least one field
// to update. The admin endpoint rejects an empty PATCH with 422 rather than
// issuing a no-op write.
func hasArtistUpdateFields(req *contracts.UpdateArtistRequest) bool {
	return req.Name != nil ||
		req.State != nil ||
		req.City != nil ||
		req.Country != nil ||
		req.Description != nil ||
		req.BandcampEmbedURL != nil ||
		req.Instagram != nil ||
		req.Facebook != nil ||
		req.Twitter != nil ||
		req.YouTube != nil ||
		req.Spotify != nil ||
		req.SoundCloud != nil ||
		req.Bandcamp != nil ||
		req.Website != nil
}

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

	user := middleware.GetUserFromContext(ctx)

	// Parse artist ID
	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Validate name if provided
	if req.Body.Name != nil && strings.TrimSpace(*req.Body.Name) == "" {
		return nil, huma.Error422UnprocessableEntity("Artist name cannot be empty")
	}

	// Validate description length if provided
	if req.Body.Description != nil && len(*req.Body.Description) > 5000 {
		return nil, huma.Error422UnprocessableEntity("Description must be 5000 characters or fewer")
	}

	// PSY-525: URL scheme validation (http/https only) for social URL fields.
	// (artist.go AdminUpdate uses lowercase Youtube/Soundcloud field names.)
	if err := shared.ValidateSocialURLs(req.Body.Instagram, req.Body.Facebook, req.Body.Twitter,
		req.Body.Youtube, req.Body.Spotify, req.Body.Soundcloud, req.Body.Bandcamp, req.Body.Website); err != nil {
		return nil, err
	}

	// Capture old values for revision diff (fire-and-forget safe)
	var oldArtist *contracts.ArtistDetailResponse
	if h.revisionService != nil {
		oldArtist, _ = h.artistService.GetArtist(uint(artistID))
	}

	// Trim the name at the boundary; the service applies the uniqueness guard
	// and slug regeneration, and normalizes the remaining nullable fields'
	// empty values to SQL NULL.
	var trimmedName *string
	if req.Body.Name != nil {
		trimmed := strings.TrimSpace(*req.Body.Name)
		trimmedName = &trimmed
	}
	serviceReq := &contracts.UpdateArtistRequest{
		Name:        trimmedName,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		Description: req.Body.Description,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.Youtube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.Soundcloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
	}

	if !hasArtistUpdateFields(serviceReq) {
		return nil, huma.Error422UnprocessableEntity("No fields to update")
	}

	artist, err := h.artistService.UpdateArtist(uint(artistID), serviceReq)
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

	// Audit log (fire and forget) — PSY-618: edits go to entity_edit_audit_logs
	if h.auditLogService != nil {
		h.auditLogService.LogEntityEdit(user.ID, "artist", uint(artistID), nil)
	}

	// Record revision (fire and forget)
	if h.revisionService != nil && oldArtist != nil {
		servicesshared.GoSafe(ctx, "record_revision", func() {
			changes := revisiondiff.Compare(oldArtist, artist, revisiondiff.ArtistFields)
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
		})
	}

	logger.FromContext(ctx).Info("admin_update_artist_success",
		"artist_id", artistID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminUpdateArtistResponse{Body: artist}, nil
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

	user := middleware.GetUserFromContext(ctx)

	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	if strings.TrimSpace(req.Body.Alias) == "" {
		return nil, huma.Error422UnprocessableEntity("Alias cannot be empty")
	}

	alias, err := h.artistService.AddArtistAlias(uint(artistID), req.Body.Alias)
	if err != nil {
		if mapped := shared.MapArtistError(err); mapped != nil {
			return nil, mapped
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

	user := middleware.GetUserFromContext(ctx)

	aliasID, err := strconv.ParseUint(req.AliasID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid alias ID")
	}

	err = h.artistService.RemoveArtistAlias(uint(aliasID))
	if err != nil {
		if mapped := shared.MapArtistError(err); mapped != nil {
			return nil, mapped
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

	user := middleware.GetUserFromContext(ctx)

	if req.Body.CanonicalArtistID == 0 || req.Body.MergeFromArtistID == 0 {
		return nil, huma.Error422UnprocessableEntity("Both canonical_artist_id and merge_from_artist_id are required")
	}

	result, err := h.artistService.MergeArtists(req.Body.CanonicalArtistID, req.Body.MergeFromArtistID)
	if err != nil {
		if mapped := shared.MapArtistError(err); mapped != nil {
			return nil, mapped
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
