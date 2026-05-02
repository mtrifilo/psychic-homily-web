package catalog

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"

	adminm "psychic-homily-backend/internal/models/admin"

	"github.com/danielgtaylor/huma/v2"
)

type VenueHandler struct {
	venueService    contracts.VenueServiceInterface
	discordService  contracts.DiscordServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
}

func NewVenueHandler(venueService contracts.VenueServiceInterface, discordService contracts.DiscordServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface) *VenueHandler {
	return &VenueHandler{
		venueService:    venueService,
		discordService:  discordService,
		auditLogService: auditLogService,
		revisionService: revisionService,
	}
}

type SearchVenuesRequest struct {
	Query string `query:"q" doc:"Search query for venue autocomplete" example:"empty bottle"`
}

type SearchVenuesResponse struct {
	Body struct {
		Venues []*contracts.VenueDetailResponse `json:"venues" doc:"Matching venues"`
		Count  int                              `json:"count" doc:"Number of results"`
	}
}

func (h *VenueHandler) SearchVenuesHandler(ctx context.Context, req *SearchVenuesRequest) (*SearchVenuesResponse, error) {
	venues, err := h.venueService.SearchVenues(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Count = len(venues)

	return resp, nil
}

// ListVenuesRequest represents the request parameters for listing venues
type ListVenuesRequest struct {
	State    string `query:"state" doc:"Filter by state" example:"AZ"`
	City     string `query:"city" doc:"Filter by city" example:"Phoenix"`
	Cities   string `query:"cities" doc:"Pipe-delimited multi-city filter (max 10): Phoenix,AZ|Tucson,AZ" example:"Phoenix,AZ|Tucson,AZ"`
	Limit    int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Maximum number of venues to return"`
	Offset   int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	Tags     string `query:"tags" doc:"Comma-separated tag slugs. Multi-tag filter (PSY-309): AND by default; set tag_match=any for OR." example:"diy,phoenix"`
	TagMatch string `query:"tag_match" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// ListVenuesResponse represents the response for the list venues endpoint
type ListVenuesResponse struct {
	Body struct {
		Venues []*contracts.VenueWithShowCountResponse `json:"venues" doc:"List of venues with show counts"`
		Total  int64                                   `json:"total" doc:"Total number of venues"`
		Limit  int                                     `json:"limit" doc:"Limit used in query"`
		Offset int                                     `json:"offset" doc:"Offset used in query"`
	}
}

// ListVenuesHandler handles GET /venues - returns verified venues with upcoming show counts
func (h *VenueHandler) ListVenuesHandler(ctx context.Context, req *ListVenuesRequest) (*ListVenuesResponse, error) {
	filters := contracts.VenueListFilters{}

	if req.Cities != "" {
		// Parse pipe-delimited multi-city param: "Phoenix,AZ|Tucson,AZ"
		pairs := strings.Split(req.Cities, "|")
		var cityFilters []contracts.CityStateFilter
		for _, pair := range pairs {
			parts := strings.SplitN(pair, ",", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				cityFilters = append(cityFilters, contracts.CityStateFilter{
					City:  strings.TrimSpace(parts[0]),
					State: strings.TrimSpace(parts[1]),
				})
			}
		}
		// Cap at 10 cities
		if len(cityFilters) > 10 {
			cityFilters = cityFilters[:10]
		}
		filters.Cities = cityFilters
	} else {
		filters.State = req.State
		filters.City = req.City
	}
	if tf := parseTagFilter(req.Tags, req.TagMatch); tf.HasTags() {
		filters.TagSlugs = tf.TagSlugs
		filters.TagMatchAny = tf.MatchAny
	}

	limit := req.Limit
	if limit == 0 {
		limit = 50
	}

	venues, total, err := h.venueService.GetVenuesWithShowCounts(filters, limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch venues", err)
	}

	resp := &ListVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Total = total
	resp.Body.Limit = limit
	resp.Body.Offset = req.Offset

	return resp, nil
}

// GetVenueRequest represents the request parameters for getting a single venue
type GetVenueRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
}

// GetVenueResponse represents the response for the get venue endpoint
type GetVenueResponse struct {
	Body *contracts.VenueDetailResponse
}

// GetVenueHandler handles GET /venues/{venue_id} - returns a single venue by ID or slug
func (h *VenueHandler) GetVenueHandler(ctx context.Context, req *GetVenueRequest) (*GetVenueResponse, error) {
	var venue *contracts.VenueDetailResponse
	var err error

	// Try to parse as numeric ID first
	if id, parseErr := strconv.ParseUint(req.VenueID, 10, 32); parseErr == nil {
		venue, err = h.venueService.GetVenue(uint(id))
	} else {
		// Fall back to slug lookup
		venue, err = h.venueService.GetVenueBySlug(req.VenueID)
	}

	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch venue", err)
	}

	return &GetVenueResponse{Body: venue}, nil
}

// GetVenueShowsRequest represents the request parameters for getting shows at a venue
type GetVenueShowsRequest struct {
	VenueID    string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
	Timezone   string `query:"timezone" doc:"Timezone for date filtering" example:"America/Phoenix"`
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"50" doc:"Maximum number of shows to return"`
	TimeFilter string `query:"time_filter" doc:"Filter shows by time: upcoming, past, or all" example:"upcoming" enum:"upcoming,past,all"`
}

// GetVenueShowsResponse represents the response for the venue shows endpoint
type GetVenueShowsResponse struct {
	Body struct {
		Shows   []*contracts.VenueShowResponse `json:"shows" doc:"List of upcoming shows"`
		VenueID uint                           `json:"venue_id" doc:"Venue ID"`
		Total   int64                          `json:"total" doc:"Total number of upcoming shows"`
	}
}

// GetVenueShowsHandler handles GET /venues/{venue_id}/shows - returns shows at a venue
func (h *VenueHandler) GetVenueShowsHandler(ctx context.Context, req *GetVenueShowsRequest) (*GetVenueShowsResponse, error) {
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

	// Resolve venue ID from ID or slug
	var venueID uint
	if id, parseErr := strconv.ParseUint(req.VenueID, 10, 32); parseErr == nil {
		venueID = uint(id)
	} else {
		// Look up by slug to get the ID
		venue, err := h.venueService.GetVenueBySlug(req.VenueID)
		if err != nil {
			var venueErr *apperrors.VenueError
			if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
				return nil, huma.Error404NotFound("Venue not found")
			}
			return nil, huma.Error500InternalServerError("Failed to fetch venue", err)
		}
		venueID = venue.ID
	}

	shows, total, err := h.venueService.GetShowsForVenue(venueID, timezone, limit, timeFilter)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch shows", err)
	}

	resp := &GetVenueShowsResponse{}
	resp.Body.Shows = shows
	resp.Body.VenueID = venueID
	resp.Body.Total = total

	return resp, nil
}

// GetVenueCitiesRequest represents the request for getting venue cities (empty, no params needed)
type GetVenueCitiesRequest struct{}

// GetVenueCitiesResponse represents the response for the venue cities endpoint
type GetVenueCitiesResponse struct {
	Body struct {
		Cities []*contracts.VenueCityResponse `json:"cities" doc:"List of cities with venue counts"`
	}
}

// GetVenueCitiesHandler handles GET /venues/cities - returns distinct cities with venue counts
func (h *VenueHandler) GetVenueCitiesHandler(ctx context.Context, req *GetVenueCitiesRequest) (*GetVenueCitiesResponse, error) {
	cities, err := h.venueService.GetVenueCities()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch cities", err)
	}

	resp := &GetVenueCitiesResponse{}
	resp.Body.Cities = cities

	return resp, nil
}

// ============================================================================
// Admin Venue Creation
// ============================================================================

// AdminCreateVenueRequest represents the request for creating a venue directly
type AdminCreateVenueRequest struct {
	Body struct {
		Name       string  `json:"name" required:"true" doc:"Venue name" maxLength:"255"`
		City       string  `json:"city" required:"true" doc:"Venue city" maxLength:"100"`
		State      string  `json:"state" required:"true" doc:"Venue state" maxLength:"100"`
		Address    *string `json:"address" required:"false" doc:"Street address" maxLength:"500"`
		Zipcode    *string `json:"zipcode" required:"false" doc:"ZIP code" maxLength:"20"`
		Instagram  *string `json:"instagram" required:"false" doc:"Instagram URL" maxLength:"255"`
		Facebook   *string `json:"facebook" required:"false" doc:"Facebook URL" maxLength:"500"`
		Twitter    *string `json:"twitter" required:"false" doc:"Twitter URL" maxLength:"255"`
		YouTube    *string `json:"youtube" required:"false" doc:"YouTube URL" maxLength:"500"`
		Spotify    *string `json:"spotify" required:"false" doc:"Spotify URL" maxLength:"500"`
		SoundCloud *string `json:"soundcloud" required:"false" doc:"SoundCloud URL" maxLength:"500"`
		Bandcamp   *string `json:"bandcamp" required:"false" doc:"Bandcamp URL" maxLength:"500"`
		Website    *string `json:"website" required:"false" doc:"Website URL" maxLength:"500"`
		Country    *string `json:"country,omitempty" required:"false" doc:"Venue country" maxLength:"100"`
	}
}

// AdminCreateVenueResponse represents the response for creating a venue
type AdminCreateVenueResponse struct {
	Body *contracts.VenueDetailResponse
}

// AdminCreateVenueHandler handles POST /admin/venues - creates a venue directly (admin only)
func (h *VenueHandler) AdminCreateVenueHandler(ctx context.Context, req *AdminCreateVenueRequest) (*AdminCreateVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	// PSY-525: URL scheme validation (http/https only) for social URL fields.
	if err := validateSocialURLs(req.Body.Instagram, req.Body.Facebook, req.Body.Twitter,
		req.Body.YouTube, req.Body.Spotify, req.Body.SoundCloud, req.Body.Bandcamp, req.Body.Website); err != nil {
		return nil, err
	}

	// Build service request
	serviceReq := &contracts.CreateVenueRequest{
		Name:        req.Body.Name,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		Address:     req.Body.Address,
		Zipcode:     req.Body.Zipcode,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.YouTube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.SoundCloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
		SubmittedBy: &user.ID,
	}

	venue, err := h.venueService.CreateVenue(serviceReq, true)
	if err != nil {
		logger.FromContext(ctx).Error("admin_create_venue_failed",
			"error", err.Error(),
			"admin_id", user.ID,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to create venue: %s", err.Error()),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_venue", "venue", venue.ID, map[string]interface{}{
				"name":  venue.Name,
				"city":  venue.City,
				"state": venue.State,
			})
		}()
	}

	logger.FromContext(ctx).Info("admin_venue_created",
		"venue_id", venue.ID,
		"venue_slug", venue.Slug,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminCreateVenueResponse{Body: venue}, nil
}

// ============================================================================
// Venue Editing Handlers
// ============================================================================

// UpdateVenueRequest represents the HTTP request for updating a venue
// All body fields are optional - only changed fields need to be sent
type UpdateVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
	Body    struct {
		Name        *string `json:"name,omitempty" required:"false" doc:"Venue name"`
		Address     *string `json:"address,omitempty" required:"false" doc:"Venue address"`
		City        *string `json:"city,omitempty" required:"false" doc:"Venue city"`
		State       *string `json:"state,omitempty" required:"false" doc:"Venue state"`
		Country     *string `json:"country,omitempty" required:"false" doc:"Venue country"`
		Zipcode     *string `json:"zipcode,omitempty" required:"false" doc:"Venue zipcode"`
		Instagram   *string `json:"instagram,omitempty" required:"false" doc:"Instagram URL"`
		Facebook    *string `json:"facebook,omitempty" required:"false" doc:"Facebook URL"`
		Twitter     *string `json:"twitter,omitempty" required:"false" doc:"Twitter URL"`
		YouTube     *string `json:"youtube,omitempty" required:"false" doc:"YouTube URL"`
		Spotify     *string `json:"spotify,omitempty" required:"false" doc:"Spotify URL"`
		SoundCloud  *string `json:"soundcloud,omitempty" required:"false" doc:"SoundCloud URL"`
		Bandcamp    *string `json:"bandcamp,omitempty" required:"false" doc:"Bandcamp URL"`
		Website     *string `json:"website,omitempty" required:"false" doc:"Website URL"`
		Description *string `json:"description,omitempty" required:"false" doc:"Markdown description (max 5000 chars)"`
		ImageURL    *string `json:"image_url,omitempty" required:"false" doc:"Venue photo URL (max 2048 chars)"`
		Summary     *string `json:"summary,omitempty" required:"false" doc:"Revision summary describing the change"`
	}
}

// UpdateVenueResponse represents the HTTP response for updating a venue.
type UpdateVenueResponse struct {
	Body *contracts.VenueDetailResponse
}

// UpdateVenueHandler handles PUT /venues/{venue_id} — admin-only direct update.
// Non-admin users (including venue submitters) go through PUT /venues/{id}/suggest-edit,
// which routes through the unified pending_entity_edits queue.
func (h *VenueHandler) UpdateVenueHandler(ctx context.Context, req *UpdateVenueRequest) (*UpdateVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	// Validate required fields aren't being set to empty strings
	if req.Body.Name != nil && *req.Body.Name == "" {
		return nil, huma.Error422UnprocessableEntity("Venue name cannot be empty")
	}
	if req.Body.City != nil && *req.Body.City == "" {
		return nil, huma.Error422UnprocessableEntity("City cannot be empty")
	}
	if req.Body.State != nil && *req.Body.State == "" {
		return nil, huma.Error422UnprocessableEntity("State cannot be empty")
	}
	if req.Body.Description != nil && len(*req.Body.Description) > 5000 {
		return nil, huma.Error422UnprocessableEntity("Description must be 5000 characters or fewer")
	}

	// PSY-525: URL scheme validation (http/https only) for image_url and social URL fields.
	// Length check first (cheaper, reports bytes); URL scheme check second.
	if req.Body.ImageURL != nil && len(*req.Body.ImageURL) > 2048 {
		return nil, huma.Error422UnprocessableEntity("Image URL must be 2048 characters or fewer")
	}
	if err := validateImageURL(req.Body.ImageURL); err != nil {
		return nil, err
	}
	if err := validateSocialURLs(req.Body.Instagram, req.Body.Facebook, req.Body.Twitter,
		req.Body.YouTube, req.Body.Spotify, req.Body.SoundCloud, req.Body.Bandcamp, req.Body.Website); err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info("admin_venue_update",
		"venue_id", venueID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Capture old values for revision diff (fire-and-forget safe)
	var oldVenue *contracts.VenueDetailResponse
	if h.revisionService != nil {
		oldVenue, _ = h.venueService.GetVenue(uint(venueID))
	}

	updates := make(map[string]interface{})
	if req.Body.Name != nil {
		updates["name"] = *req.Body.Name
	}
	if req.Body.Address != nil {
		updates["address"] = *req.Body.Address
	}
	if req.Body.City != nil {
		updates["city"] = *req.Body.City
	}
	if req.Body.State != nil {
		updates["state"] = *req.Body.State
	}
	if req.Body.Country != nil {
		updates["country"] = *req.Body.Country
	}
	if req.Body.Zipcode != nil {
		updates["zipcode"] = *req.Body.Zipcode
	}
	if req.Body.Instagram != nil {
		updates["instagram"] = *req.Body.Instagram
	}
	if req.Body.Facebook != nil {
		updates["facebook"] = *req.Body.Facebook
	}
	if req.Body.Twitter != nil {
		updates["twitter"] = *req.Body.Twitter
	}
	if req.Body.YouTube != nil {
		updates["youtube"] = *req.Body.YouTube
	}
	if req.Body.Spotify != nil {
		updates["spotify"] = *req.Body.Spotify
	}
	if req.Body.SoundCloud != nil {
		updates["soundcloud"] = *req.Body.SoundCloud
	}
	if req.Body.Bandcamp != nil {
		updates["bandcamp"] = *req.Body.Bandcamp
	}
	if req.Body.Website != nil {
		updates["website"] = *req.Body.Website
	}
	if req.Body.Description != nil {
		updates["description"] = utils.NilIfEmpty(*req.Body.Description)
	}
	if req.Body.ImageURL != nil {
		if len(*req.Body.ImageURL) > 2048 {
			return nil, huma.Error422UnprocessableEntity("Image URL must be 2048 characters or fewer")
		}
		updates["image_url"] = utils.NilIfEmpty(*req.Body.ImageURL)
	}

	updatedVenue, err := h.venueService.UpdateVenue(uint(venueID), updates)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		logger.FromContext(ctx).Error("admin_venue_update_failed",
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to update venue (request_id: %s)", requestID),
		)
	}

	// Record revision (fire and forget)
	if h.revisionService != nil && oldVenue != nil {
		go func() {
			changes := computeVenueChanges(oldVenue, updatedVenue)
			if len(changes) > 0 {
				summary := ""
				if req.Body.Summary != nil {
					summary = *req.Body.Summary
				}
				if err := h.revisionService.RecordRevision("venue", uint(venueID), user.ID, changes, summary); err != nil {
					logger.Default().Error("record_venue_revision_failed",
						"venue_id", venueID,
						"error", err.Error(),
					)
				}
			}
		}()
	}

	return &UpdateVenueResponse{Body: updatedVenue}, nil
}

// ============================================================================
// Venue Deletion Handlers
// ============================================================================

// DeleteVenueRequest represents the request for deleting a venue
type DeleteVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// DeleteVenueResponse represents the response for deleting a venue
type DeleteVenueResponse struct {
	Body struct {
		Message string `json:"message" doc:"Success message"`
	}
}

// DeleteVenueHandler handles DELETE /venues/{venue_id}
// Admin: Can delete any venue
// Non-admin: Can delete venues they submitted (via submitted_by field)
// Constraint: Venues with associated shows cannot be deleted
func (h *VenueHandler) DeleteVenueHandler(ctx context.Context, req *DeleteVenueRequest) (*DeleteVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	// Get the venue to check ownership
	venue, err := h.venueService.GetVenueModel(uint(venueID))
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get venue (request_id: %s)", requestID),
		)
	}

	// Check permissions: admin can delete any venue, non-admin can only delete their own
	if !user.IsAdmin {
		if venue.SubmittedBy == nil || *venue.SubmittedBy != user.ID {
			logger.FromContext(ctx).Warn("venue_delete_forbidden",
				"venue_id", venueID,
				"user_id", user.ID,
				"venue_submitted_by", venue.SubmittedBy,
				"request_id", requestID,
			)
			return nil, huma.Error403Forbidden("You can only delete venues you submitted")
		}
	}

	logger.FromContext(ctx).Info("venue_delete_attempt",
		"venue_id", venueID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
		"request_id", requestID,
	)

	// Delete the venue (service checks for associated shows)
	if err := h.venueService.DeleteVenue(uint(venueID)); err != nil {
		logger.FromContext(ctx).Error("venue_delete_failed",
			"venue_id", venueID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)

		// Check if the error is due to associated shows
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueHasShows {
			return nil, huma.Error422UnprocessableEntity(venueErr.Message)
		}

		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete venue (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("venue_deleted",
		"venue_id", venueID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
		"request_id", requestID,
	)

	return &DeleteVenueResponse{
		Body: struct {
			Message string `json:"message" doc:"Success message"`
		}{
			Message: "Venue deleted successfully",
		},
	}, nil
}

// ============================================================================
// Get Venue Genres
// ============================================================================

// GetVenueGenresRequest represents the request for getting a venue's genre profile.
type GetVenueGenresRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"the-rebel-lounge-phoenix-az"`
}

// GetVenueGenresResponse represents the response for venue genre profile.
type GetVenueGenresResponse struct {
	Body *contracts.VenueGenreResponse
}

// GetVenueGenresHandler handles GET /venues/{venue_id}/genres — returns top genre tags for a venue.
func (h *VenueHandler) GetVenueGenresHandler(ctx context.Context, req *GetVenueGenresRequest) (*GetVenueGenresResponse, error) {
	// Resolve venue by ID or slug
	var venueID uint
	if id, err := strconv.ParseUint(req.VenueID, 10, 32); err == nil {
		venueID = uint(id)
	} else {
		// Try slug lookup
		venue, err := h.venueService.GetVenueBySlug(req.VenueID)
		if err != nil {
			return nil, huma.Error404NotFound("Venue not found")
		}
		venueID = venue.ID
	}

	genres, err := h.venueService.GetVenueGenreProfile(venueID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get venue genre profile", err)
	}
	if genres == nil {
		genres = []contracts.GenreCount{}
	}

	return &GetVenueGenresResponse{
		Body: &contracts.VenueGenreResponse{
			Genres: genres,
		},
	}, nil
}

// ============================================================================
// Get Venue Bill Network (PSY-365)
// ============================================================================

// GetVenueBillNetworkRequest is the request shape for the venue co-bill graph.
//
// `Window` accepts "all" (default), "12m" (rolling last 12 months), or "year"
// (paired with `Year`). Unknown values are coerced to "all" by the service so
// a client mistake degrades gracefully rather than 500ing.
//
// `Year` is required when Window=="year". Huma forbids pointer query params,
// so we use an int with the zero-value sentinel; the handler validates
// presence before passing to the service.
type GetVenueBillNetworkRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
	Window  string `query:"window" doc:"Time window: 'all' (default), '12m' (rolling), or 'year' (with year=YYYY)" example:"all" enum:"all,12m,year"`
	Year    int    `query:"year" doc:"Calendar year for window=year (required when window=year)" example:"2025" minimum:"2000" maximum:"2100"`
}

// GetVenueBillNetworkResponse wraps the contracts payload for huma.
type GetVenueBillNetworkResponse struct {
	Body *contracts.VenueBillNetworkResponse
}

// GetVenueBillNetworkHandler handles GET /venues/{venue_id}/bill-network.
//
// PSY-365 — venue-rooted co-bill graph. Mirrors GET /scenes/{slug}/graph in
// shape and intent (same shared frontend component renders both), with the
// scope narrowed to a single venue and edges weighted by AT-VENUE shared
// shows rather than global ones.
func (h *VenueHandler) GetVenueBillNetworkHandler(ctx context.Context, req *GetVenueBillNetworkRequest) (*GetVenueBillNetworkResponse, error) {
	// Resolve venue by ID or slug — same pattern as /venues/{id}/genres.
	var venueID uint
	if id, err := strconv.ParseUint(req.VenueID, 10, 32); err == nil {
		venueID = uint(id)
	} else {
		venue, err := h.venueService.GetVenueBySlug(req.VenueID)
		if err != nil {
			return nil, huma.Error404NotFound("Venue not found")
		}
		venueID = venue.ID
	}

	// Validate window=year requires year. Empty / "all" / "12m" pass through.
	var yearPtr *int
	window := strings.ToLower(strings.TrimSpace(req.Window))
	if window == "year" {
		if req.Year == 0 {
			return nil, huma.Error400BadRequest("Year is required when window=year")
		}
		y := req.Year
		yearPtr = &y
	}

	graph, err := h.venueService.GetVenueBillNetwork(venueID, window, yearPtr)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to get venue bill network", err)
	}

	return &GetVenueBillNetworkResponse{Body: graph}, nil
}

// computeVenueChanges compares old and new venue detail responses and returns field-level diffs.
func computeVenueChanges(old, new *contracts.VenueDetailResponse) []adminm.FieldChange {
	var changes []adminm.FieldChange

	if old.Name != new.Name {
		changes = append(changes, adminm.FieldChange{Field: "name", OldValue: old.Name, NewValue: new.Name})
	}
	if ptrToStr(old.Address) != ptrToStr(new.Address) {
		changes = append(changes, adminm.FieldChange{Field: "address", OldValue: ptrToStr(old.Address), NewValue: ptrToStr(new.Address)})
	}
	if old.City != new.City {
		changes = append(changes, adminm.FieldChange{Field: "city", OldValue: old.City, NewValue: new.City})
	}
	if old.State != new.State {
		changes = append(changes, adminm.FieldChange{Field: "state", OldValue: old.State, NewValue: new.State})
	}
	if ptrToStr(old.Zipcode) != ptrToStr(new.Zipcode) {
		changes = append(changes, adminm.FieldChange{Field: "zipcode", OldValue: ptrToStr(old.Zipcode), NewValue: ptrToStr(new.Zipcode)})
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
	if ptrToStr(old.ImageURL) != ptrToStr(new.ImageURL) {
		changes = append(changes, adminm.FieldChange{Field: "image_url", OldValue: ptrToStr(old.ImageURL), NewValue: ptrToStr(new.ImageURL)})
	}

	return changes
}
