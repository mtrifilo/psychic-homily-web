package handlers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	showerrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// ShowHandler handles show-related HTTP requests
type ShowHandler struct {
	showService      *services.ShowService
	savedShowService *services.SavedShowService
}

// NewShowHandler creates a new show handler
func NewShowHandler() *ShowHandler {
	return &ShowHandler{
		showService:      services.NewShowService(),
		savedShowService: services.NewSavedShowService(),
	}
}

// Artist represents an artist in a show request
type Artist struct {
	ID          *uint   `json:"id,omitempty"`
	Name        *string `json:"name,omitempty"`
	IsHeadliner *bool   `json:"is_headliner,omitempty"`
}

// Venue represents a venue in a show request
type Venue struct {
	ID      *uint   `json:"id,omitempty"`
	Name    *string `json:"name,omitempty"`
	City    *string `json:"city,omitempty"`
	State   *string `json:"state,omitempty"`
	Address *string `json:"address,omitempty"`
}

// initializeArtist provides sensible defaults for Artist fields
func initializeArtist(a *Artist) {
	// Set default for IsHeadliner if not provided
	if a.IsHeadliner == nil {
		// Default to false for non-headliners
		defaultValue := false
		a.IsHeadliner = &defaultValue
	}

	// Note: ID and Name are left as-is since validation will check
	// that at least one is provided. No need to "initialize" nil values.
}

// CreateShowRequestBody represents the request body with preprocessing
type CreateShowRequestBody struct {
	Title          *string   `json:"title,omitempty" doc:"Show title (optional)"`
	EventDate      time.Time `json:"event_date" validate:"required" doc:"Event date and time"`
	City           string    `json:"city" doc:"City where the show takes place"`
	State          string    `json:"state" doc:"State where the show takes place"`
	Price          *float64  `json:"price,omitempty" doc:"Ticket price"`
	AgeRequirement *string   `json:"age_requirement,omitempty" doc:"Age requirement (e.g., '21+', 'All Ages')"`
	Description    *string   `json:"description,omitempty" doc:"Show description"`
	Venues         []Venue   `json:"venues" validate:"required,min=1" doc:"List of venues for the show"`
	Artists        []Artist  `json:"artists" validate:"required,min=1" doc:"List of artists in the show"`
	IsPrivate      *bool     `json:"is_private,omitempty" doc:"If true, show is private and only visible to submitter"`
}

// Resolve implements preprocessing and validation for the request body
func (r *CreateShowRequestBody) Resolve(ctx huma.Context) []error {
	var errors []error

	// Validate venues - no preprocessing needed currently
	for i := range r.Venues {
		// Validate that either ID or Name is provided
		venue := &r.Venues[i]
		if (venue.ID == nil || *venue.ID == 0) && (venue.Name == nil || *venue.Name == "") {
			errors = append(errors, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.venues[%d]", i),
				Message:  "Either 'id' or 'name' must be provided",
				Value:    venue,
			})
		}
	}

	// Preprocess and validate artists
	for i := range r.Artists {
		initializeArtist(&r.Artists[i])

		// Validate that either ID or Name is provided
		artist := &r.Artists[i]
		if (artist.ID == nil || *artist.ID == 0) && (artist.Name == nil || *artist.Name == "") {
			errors = append(errors, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.artists[%d]", i),
				Message:  "Either 'id' or 'name' must be provided",
				Value:    artist,
			})
		}
	}

	return errors
}

// CreateShowRequest represents the HTTP request for creating a show
type CreateShowRequest struct {
	Body CreateShowRequestBody `json:"body"`
}

// CreateShowResponse represents the HTTP response for creating a show
type CreateShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// GetShowRequest represents the HTTP request for getting a show
type GetShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// GetShowResponse represents the HTTP response for getting a show
type GetShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// GetShowsRequest represents the HTTP request for listing shows
type GetShowsRequest struct {
	City     string    `query:"city" doc:"Filter by city"`
	State    string    `query:"state" doc:"Filter by state"`
	FromDate time.Time `query:"from_date" doc:"Filter shows from this date"`
	ToDate   time.Time `query:"to_date" doc:"Filter shows until this date"`
}

// GetShowsResponse represents the HTTP response for listing shows
type GetShowsResponse struct {
	Body []*services.ShowResponse `json:"body"`
}

// GetUpcomingShowsRequest represents the HTTP request for listing upcoming shows
type GetUpcomingShowsRequest struct {
	Timezone string `query:"timezone" default:"UTC" doc:"IANA timezone (e.g., 'America/Phoenix', 'America/New_York'). Defaults to UTC."`
	Cursor   string `query:"cursor" doc:"Pagination cursor from previous response. Omit for first page."`
	Limit    int    `query:"limit" default:"50" doc:"Number of shows per page (max 200). Defaults to 50."`
}

// CursorPaginationMeta contains cursor-based pagination metadata
type CursorPaginationMeta struct {
	NextCursor *string `json:"next_cursor" doc:"Cursor for the next page (null if no more results)"`
	HasMore    bool    `json:"has_more" doc:"Whether there are more results"`
	Limit      int     `json:"limit" doc:"Number of items per page"`
}

// GetUpcomingShowsResponse represents the HTTP response for listing upcoming shows
type GetUpcomingShowsResponse struct {
	Body struct {
		Shows      []*services.ShowResponse `json:"shows"`
		Timezone   string                   `json:"timezone" doc:"The timezone used for filtering"`
		Pagination CursorPaginationMeta     `json:"pagination"`
	}
}

// UpdateShowRequest represents the HTTP request for updating a show
// All body fields are optional for partial updates
type UpdateShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
	Body   struct {
		Title          *string    `json:"title,omitempty" doc:"Show title"`
		EventDate      *time.Time `json:"event_date,omitempty" doc:"Event date and time"`
		City           *string    `json:"city,omitempty" doc:"City where the show takes place"`
		State          *string    `json:"state,omitempty" doc:"State where the show takes place"`
		Price          *float64   `json:"price,omitempty" doc:"Ticket price"`
		AgeRequirement *string    `json:"age_requirement,omitempty" doc:"Age requirement"`
		Description    *string    `json:"description,omitempty" doc:"Show description"`
		Venues         []Venue    `json:"venues,omitempty" doc:"List of venues for the show"`
		Artists        []Artist   `json:"artists,omitempty" doc:"List of artists for the show"`
	}
}

// UpdateShowResponse represents the HTTP response for updating a show
type UpdateShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// DeleteShowRequest represents the HTTP request for deleting a show
type DeleteShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// AIProcessShowRequest represents the HTTP request for AI show processing (future)
type AIProcessShowRequest struct {
	Body struct {
		Text string `json:"text" validate:"required" doc:"Unstructured text to process"`
	}
}

// AIProcessShowResponse represents the HTTP response for AI show processing
type AIProcessShowResponse struct {
	Body struct {
		Message string `json:"message" doc:"Response message"`
		Status  string `json:"status" doc:"Processing status"`
	}
}

// CreateShowHandler handles POST /shows
func (h *ShowHandler) CreateShowHandler(ctx context.Context, req *CreateShowRequest) (*CreateShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user from context
	user := middleware.GetUserFromContext(ctx)
	var submittedByUserID *uint
	var submitterIsAdmin bool
	if user != nil {
		submittedByUserID = &user.ID
		submitterIsAdmin = user.IsAdmin
	}

	logger.FromContext(ctx).Debug("show_create_attempt",
		"venue_count", len(req.Body.Venues),
		"artist_count", len(req.Body.Artists),
		"event_date", req.Body.EventDate,
		"city", req.Body.City,
		"state", req.Body.State,
		"submitted_by", submittedByUserID,
		"is_admin", submitterIsAdmin,
	)

	// Validation is now handled by Huma's custom resolvers

	// Convert Venues to service format
	serviceVenues := make([]services.CreateShowVenue, len(req.Body.Venues))
	for i, venue := range req.Body.Venues {
		var name, city, state, address string
		if venue.Name != nil {
			name = *venue.Name
		}
		if venue.City != nil {
			city = *venue.City
		}
		if venue.State != nil {
			state = *venue.State
		}
		if venue.Address != nil {
			address = *venue.Address
		}
		serviceVenues[i] = services.CreateShowVenue{
			ID:      venue.ID,
			Name:    name,
			City:    city,
			State:   state,
			Address: address,
		}
	}

	// Convert Artists to service format
	serviceArtists := make([]services.CreateShowArtist, len(req.Body.Artists))
	for i, artist := range req.Body.Artists {
		var name string
		if artist.Name != nil {
			name = *artist.Name
		}
		serviceArtists[i] = services.CreateShowArtist{
			ID:          artist.ID,
			Name:        name,
			IsHeadliner: artist.IsHeadliner,
		}
	}

	title := ""
	if req.Body.Title != nil {
		title = *req.Body.Title
	}

	description := ""
	if req.Body.Description != nil {
		description = *req.Body.Description
	}

	ageRequirement := ""
	if req.Body.AgeRequirement != nil {
		ageRequirement = *req.Body.AgeRequirement
	}

	// Check if show should be private
	isPrivate := false
	if req.Body.IsPrivate != nil && *req.Body.IsPrivate {
		isPrivate = true
	}

	// Convert request to service request with user context
	serviceReq := &services.CreateShowRequest{
		Title:             title,
		EventDate:         req.Body.EventDate,
		City:              req.Body.City,
		State:             req.Body.State,
		Price:             req.Body.Price,
		AgeRequirement:    ageRequirement,
		Description:       description,
		Venues:            serviceVenues,
		Artists:           serviceArtists,
		SubmittedByUserID: submittedByUserID,
		SubmitterIsAdmin:  submitterIsAdmin,
		IsPrivate:         isPrivate,
	}

	// Create show using service
	show, err := h.showService.CreateShow(serviceReq)
	if err != nil {
		showErr := showerrors.ErrShowCreateFailed(err)
		logger.FromContext(ctx).Error("show_create_failed",
			"error", err.Error(),
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s [%s] (request_id: %s)", showErr.Message, showErr.Code, requestID),
		)
	}

	logger.FromContext(ctx).Info("show_created",
		"show_id", show.ID,
		"title", show.Title,
		"status", show.Status,
		"request_id", requestID,
	)

	// Auto-save the show to the submitter's personal list
	if submittedByUserID != nil {
		if err := h.savedShowService.SaveShow(*submittedByUserID, show.ID); err != nil {
			// Log but don't fail the request - show was created successfully
			logger.FromContext(ctx).Warn("show_auto_save_failed",
				"show_id", show.ID,
				"user_id", *submittedByUserID,
				"error", err.Error(),
				"request_id", requestID,
			)
		} else {
			logger.FromContext(ctx).Debug("show_auto_saved",
				"show_id", show.ID,
				"user_id", *submittedByUserID,
			)
		}
	}

	return &CreateShowResponse{Body: *show}, nil
}

// GetShowHandler handles GET /shows/{show_id}
func (h *ShowHandler) GetShowHandler(ctx context.Context, req *GetShowRequest) (*GetShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		showErr := showerrors.ErrShowInvalidID(req.ShowID)
		logger.FromContext(ctx).Warn("show_get_invalid_id",
			"show_id_str", req.ShowID,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	logger.FromContext(ctx).Debug("show_get_attempt",
		"show_id", showID,
	)

	// Get show using service
	show, err := h.showService.GetShow(uint(showID))
	if err != nil {
		showErr := showerrors.ErrShowNotFound(uint(showID))
		logger.FromContext(ctx).Warn("show_not_found",
			"show_id", showID,
			"error", err.Error(),
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error404NotFound(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	logger.FromContext(ctx).Debug("show_get_success",
		"show_id", showID,
	)

	return &GetShowResponse{Body: *show}, nil
}

// GetShowsHandler handles GET /shows
func (h *ShowHandler) GetShowsHandler(ctx context.Context, req *GetShowsRequest) (*GetShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Build filters
	filters := make(map[string]interface{})
	if req.City != "" {
		filters["city"] = req.City
	}
	if req.State != "" {
		filters["state"] = req.State
	}
	if !req.FromDate.IsZero() {
		filters["from_date"] = req.FromDate
	}
	if !req.ToDate.IsZero() {
		filters["to_date"] = req.ToDate
	}

	logger.FromContext(ctx).Debug("shows_list_attempt",
		"filter_count", len(filters),
	)

	// Get shows using service
	shows, err := h.showService.GetShows(filters)
	if err != nil {
		logger.FromContext(ctx).Error("shows_list_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get shows [SERVICE_UNAVAILABLE] (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("shows_list_success",
		"count", len(shows),
	)

	return &GetShowsResponse{Body: shows}, nil
}

// GetUpcomingShowsHandler handles GET /shows/upcoming
func (h *ShowHandler) GetUpcomingShowsHandler(ctx context.Context, req *GetUpcomingShowsRequest) (*GetUpcomingShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Check if user is admin (for including non-approved shows)
	user := middleware.GetUserFromContext(ctx)
	includeNonApproved := user != nil && user.IsAdmin

	// Default timezone to UTC if not provided
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200 // Cap at 200 to prevent excessive queries
	}

	logger.FromContext(ctx).Debug("shows_upcoming_attempt",
		"timezone", timezone,
		"limit", limit,
		"has_cursor", req.Cursor != "",
		"include_non_approved", includeNonApproved,
	)

	// Get upcoming shows using service (admins see all, others see only approved)
	shows, nextCursor, err := h.showService.GetUpcomingShows(timezone, req.Cursor, limit, includeNonApproved)
	if err != nil {
		logger.FromContext(ctx).Error("shows_upcoming_failed",
			"error", err.Error(),
			"timezone", timezone,
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get upcoming shows [SERVICE_UNAVAILABLE] (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("shows_upcoming_success",
		"count", len(shows),
		"has_more", nextCursor != nil,
	)

	return &GetUpcomingShowsResponse{
		Body: struct {
			Shows      []*services.ShowResponse `json:"shows"`
			Timezone   string                   `json:"timezone" doc:"The timezone used for filtering"`
			Pagination CursorPaginationMeta     `json:"pagination"`
		}{
			Shows:    shows,
			Timezone: timezone,
			Pagination: CursorPaginationMeta{
				NextCursor: nextCursor,
				HasMore:    nextCursor != nil,
				Limit:      limit,
			},
		},
	}, nil
}

// UpdateShowHandler handles PUT /shows/{show_id}
func (h *ShowHandler) UpdateShowHandler(ctx context.Context, req *UpdateShowRequest) (*UpdateShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user from context for admin status
	user := middleware.GetUserFromContext(ctx)
	isAdmin := user != nil && user.IsAdmin

	// Early debug log to confirm handler is called
	logger.FromContext(ctx).Debug("show_update_handler_start",
		"show_id_path", req.ShowID,
		"has_venues", len(req.Body.Venues) > 0,
		"venue_count", len(req.Body.Venues),
		"has_artists", len(req.Body.Artists) > 0,
		"artist_count", len(req.Body.Artists),
		"is_admin", isAdmin,
		"request_id", requestID,
	)

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		showErr := showerrors.ErrShowInvalidID(req.ShowID)
		logger.FromContext(ctx).Warn("show_update_invalid_id",
			"show_id_str", req.ShowID,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	// Build updates map for basic show fields
	updates := make(map[string]interface{})
	if req.Body.Title != nil {
		updates["title"] = *req.Body.Title
	}
	if req.Body.EventDate != nil {
		updates["event_date"] = *req.Body.EventDate
	}
	if req.Body.City != nil {
		updates["city"] = *req.Body.City
	}
	if req.Body.State != nil {
		updates["state"] = *req.Body.State
	}
	if req.Body.Price != nil {
		updates["price"] = *req.Body.Price
	}
	if req.Body.AgeRequirement != nil {
		updates["age_requirement"] = *req.Body.AgeRequirement
	}
	if req.Body.Description != nil {
		updates["description"] = *req.Body.Description
	}

	// Convert venues to service format (nil if not provided)
	var serviceVenues []services.CreateShowVenue
	if len(req.Body.Venues) > 0 {
		serviceVenues = make([]services.CreateShowVenue, len(req.Body.Venues))
		for i, venue := range req.Body.Venues {
			var name, city, state, address string
			if venue.Name != nil {
				name = *venue.Name
			}
			if venue.City != nil {
				city = *venue.City
			}
			if venue.State != nil {
				state = *venue.State
			}
			if venue.Address != nil {
				address = *venue.Address
			}
			serviceVenues[i] = services.CreateShowVenue{
				ID:      venue.ID,
				Name:    name,
				City:    city,
				State:   state,
				Address: address,
			}
		}
	}

	// Convert artists to service format (nil if not provided)
	var serviceArtists []services.CreateShowArtist
	if len(req.Body.Artists) > 0 {
		serviceArtists = make([]services.CreateShowArtist, len(req.Body.Artists))
		for i, artist := range req.Body.Artists {
			var name string
			if artist.Name != nil {
				name = *artist.Name
			}
			serviceArtists[i] = services.CreateShowArtist{
				ID:          artist.ID,
				Name:        name,
				IsHeadliner: artist.IsHeadliner,
			}
		}
	}

	logger.FromContext(ctx).Debug("show_update_attempt",
		"show_id", showID,
		"update_fields", len(updates),
		"has_venues", serviceVenues != nil,
		"has_artists", serviceArtists != nil,
	)

	// Update show using service with relations support (pass admin status for venue verification)
	show, err := h.showService.UpdateShowWithRelations(uint(showID), updates, serviceVenues, serviceArtists, isAdmin)
	if err != nil {
		showErr := showerrors.ErrShowUpdateFailed(uint(showID), err)
		logger.FromContext(ctx).Error("show_update_failed",
			"show_id", showID,
			"error", err.Error(),
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s [%s] (request_id: %s)", showErr.Message, showErr.Code, requestID),
		)
	}

	logger.FromContext(ctx).Info("show_updated",
		"show_id", showID,
		"title", show.Title,
		"venue_count", len(show.Venues),
		"artist_count", len(show.Artists),
		"request_id", requestID,
	)

	return &UpdateShowResponse{Body: *show}, nil
}

// DeleteShowHandler handles DELETE /shows/{show_id}
func (h *ShowHandler) DeleteShowHandler(ctx context.Context, req *DeleteShowRequest) (*huma.Response, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user from context
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		showErr := showerrors.ErrShowInvalidID(req.ShowID)
		logger.FromContext(ctx).Warn("show_delete_invalid_id",
			"show_id_str", req.ShowID,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	logger.FromContext(ctx).Debug("show_delete_attempt",
		"show_id", showID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
	)

	// Fetch show to check ownership
	show, err := h.showService.GetShow(uint(showID))
	if err != nil {
		showErr := showerrors.ErrShowNotFound(uint(showID))
		logger.FromContext(ctx).Warn("show_delete_not_found",
			"show_id", showID,
			"error", err.Error(),
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error404NotFound(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	// Check authorization: user must be admin OR the show submitter
	isOwner := show.SubmittedBy != nil && *show.SubmittedBy == user.ID
	if !user.IsAdmin && !isOwner {
		showErr := showerrors.ErrShowDeleteUnauthorized(uint(showID))
		logger.FromContext(ctx).Warn("show_delete_unauthorized",
			"show_id", showID,
			"user_id", user.ID,
			"submitted_by", show.SubmittedBy,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	// Delete show using service
	err = h.showService.DeleteShow(uint(showID))
	if err != nil {
		showErr := showerrors.ErrShowDeleteFailed(uint(showID), err)
		logger.FromContext(ctx).Error("show_delete_failed",
			"show_id", showID,
			"error", err.Error(),
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s [%s] (request_id: %s)", showErr.Message, showErr.Code, requestID),
		)
	}

	logger.FromContext(ctx).Info("show_deleted",
		"show_id", showID,
		"deleted_by_user_id", user.ID,
		"request_id", requestID,
	)

	// Return 204 No Content
	return &huma.Response{}, nil
}

// UnpublishShowRequest represents the HTTP request for unpublishing a show
type UnpublishShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID to unpublish"`
}

// UnpublishShowResponse represents the HTTP response for unpublishing a show
type UnpublishShowResponse struct {
	Body services.ShowResponse
}

// UnpublishShowHandler handles POST /shows/{show_id}/unpublish
// Changes an approved show's status back to pending.
// Only the submitter or an admin can unpublish a show.
func (h *ShowHandler) UnpublishShowHandler(ctx context.Context, req *UnpublishShowRequest) (*UnpublishShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Require authentication
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		showErr := showerrors.ErrShowInvalidID(req.ShowID)
		logger.FromContext(ctx).Warn("show_unpublish_invalid_id",
			"show_id_str", req.ShowID,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	logger.FromContext(ctx).Debug("show_unpublish_attempt",
		"show_id", showID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
	)

	// Unpublish show using service (service handles authorization check)
	show, err := h.showService.UnpublishShow(uint(showID), user.ID, user.IsAdmin)
	if err != nil {
		logger.FromContext(ctx).Warn("show_unpublish_failed",
			"show_id", showID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		// Check for specific error types
		if err.Error() == "show not found" {
			return nil, huma.Error404NotFound(
				fmt.Sprintf("Show not found (request_id: %s)", requestID),
			)
		}
		if err.Error() == "only the show submitter or an admin can unpublish this show" {
			return nil, huma.Error403Forbidden(
				fmt.Sprintf("Not authorized to unpublish this show (request_id: %s)", requestID),
			)
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to unpublish show: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("show_unpublished",
		"show_id", showID,
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &UnpublishShowResponse{Body: *show}, nil
}

// MakePrivateShowRequest represents the HTTP request for making a show private
type MakePrivateShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID to make private"`
}

// MakePrivateShowResponse represents the HTTP response for making a show private
type MakePrivateShowResponse struct {
	Body services.ShowResponse
}

// MakePrivateShowHandler handles POST /shows/{show_id}/make-private
// Changes a pending show's status to private.
// Only the submitter or an admin can make a show private.
func (h *ShowHandler) MakePrivateShowHandler(ctx context.Context, req *MakePrivateShowRequest) (*MakePrivateShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Require authentication
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		showErr := showerrors.ErrShowInvalidID(req.ShowID)
		logger.FromContext(ctx).Warn("show_make_private_invalid_id",
			"show_id_str", req.ShowID,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	logger.FromContext(ctx).Debug("show_make_private_attempt",
		"show_id", showID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
	)

	// Make show private using service (service handles authorization check)
	show, err := h.showService.MakePrivateShow(uint(showID), user.ID, user.IsAdmin)
	if err != nil {
		logger.FromContext(ctx).Warn("show_make_private_failed",
			"show_id", showID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		// Check for specific error types
		if err.Error() == "show not found" {
			return nil, huma.Error404NotFound(
				fmt.Sprintf("Show not found (request_id: %s)", requestID),
			)
		}
		if err.Error() == "only the show submitter or an admin can make this show private" {
			return nil, huma.Error403Forbidden(
				fmt.Sprintf("Not authorized to make this show private (request_id: %s)", requestID),
			)
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to make show private: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("show_made_private",
		"show_id", showID,
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &MakePrivateShowResponse{Body: *show}, nil
}

// PublishShowRequest represents the HTTP request for publishing a show
type PublishShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID to publish"`
}

// PublishShowResponse represents the HTTP response for publishing a show
type PublishShowResponse struct {
	Body services.ShowResponse
}

// PublishShowHandler handles POST /shows/{show_id}/publish
// Changes a private show's status to approved or pending.
// If all venues are verified, status becomes approved.
// If any venue is unverified, status becomes pending.
// Only the submitter or an admin can publish a show.
func (h *ShowHandler) PublishShowHandler(ctx context.Context, req *PublishShowRequest) (*PublishShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Require authentication
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		showErr := showerrors.ErrShowInvalidID(req.ShowID)
		logger.FromContext(ctx).Warn("show_publish_invalid_id",
			"show_id_str", req.ShowID,
			"error_code", showErr.Code,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("%s [%s]", showErr.Message, showErr.Code),
		)
	}

	logger.FromContext(ctx).Debug("show_publish_attempt",
		"show_id", showID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
	)

	// Publish show using service (service handles authorization check)
	show, err := h.showService.PublishShow(uint(showID), user.ID, user.IsAdmin)
	if err != nil {
		logger.FromContext(ctx).Warn("show_publish_failed",
			"show_id", showID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		// Check for specific error types
		if err.Error() == "show not found" {
			return nil, huma.Error404NotFound(
				fmt.Sprintf("Show not found (request_id: %s)", requestID),
			)
		}
		if err.Error() == "only the show submitter or an admin can publish this show" {
			return nil, huma.Error403Forbidden(
				fmt.Sprintf("Not authorized to publish this show (request_id: %s)", requestID),
			)
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to publish show: %s (request_id: %s)", err.Error(), requestID),
		)
	}

	logger.FromContext(ctx).Info("show_published",
		"show_id", showID,
		"new_status", show.Status,
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &PublishShowResponse{Body: *show}, nil
}

// AIProcessShowHandler handles POST /shows/ai-process (future implementation)
func (h *ShowHandler) AIProcessShowHandler(ctx context.Context, req *AIProcessShowRequest) (*AIProcessShowResponse, error) {
	// TODO: Implement AI processing logic
	// For now, return "Not Implemented" response
	return &AIProcessShowResponse{
		Body: struct {
			Message string `json:"message" doc:"Response message"`
			Status  string `json:"status" doc:"Processing status"`
		}{
			Message: "AI show processing is not yet implemented",
			Status:  "not_implemented",
		},
	}, nil
}
