package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"

	"github.com/danielgtaylor/huma/v2"
)

type VenueHandler struct {
	venueService   services.VenueServiceInterface
	discordService services.DiscordServiceInterface
}

func NewVenueHandler(venueService services.VenueServiceInterface, discordService services.DiscordServiceInterface) *VenueHandler {
	return &VenueHandler{
		venueService:   venueService,
		discordService: discordService,
	}
}

type SearchVenuesRequest struct {
	Query string `query:"q" doc:"Search query for venue autocomplete" example:"empty bottle"`
}

type SearchVenuesResponse struct {
	Body struct {
		Venues []*services.VenueDetailResponse `json:"venues" doc:"Matching venues"`
		Count  int                             `json:"count" doc:"Number of results"`
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
	State  string `query:"state" doc:"Filter by state" example:"AZ"`
	City   string `query:"city" doc:"Filter by city" example:"Phoenix"`
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Maximum number of venues to return"`
	Offset int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
}

// ListVenuesResponse represents the response for the list venues endpoint
type ListVenuesResponse struct {
	Body struct {
		Venues []*services.VenueWithShowCountResponse `json:"venues" doc:"List of venues with show counts"`
		Total  int64                                  `json:"total" doc:"Total number of venues"`
		Limit  int                                    `json:"limit" doc:"Limit used in query"`
		Offset int                                    `json:"offset" doc:"Offset used in query"`
	}
}

// ListVenuesHandler handles GET /venues - returns verified venues with upcoming show counts
func (h *VenueHandler) ListVenuesHandler(ctx context.Context, req *ListVenuesRequest) (*ListVenuesResponse, error) {
	filters := services.VenueListFilters{
		State: req.State,
		City:  req.City,
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
	Body *services.VenueDetailResponse
}

// GetVenueHandler handles GET /venues/{venue_id} - returns a single venue by ID or slug
func (h *VenueHandler) GetVenueHandler(ctx context.Context, req *GetVenueRequest) (*GetVenueResponse, error) {
	var venue *services.VenueDetailResponse
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
		Shows   []*services.VenueShowResponse `json:"shows" doc:"List of upcoming shows"`
		VenueID uint                          `json:"venue_id" doc:"Venue ID"`
		Total   int64                         `json:"total" doc:"Total number of upcoming shows"`
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
		Cities []*services.VenueCityResponse `json:"cities" doc:"List of cities with venue counts"`
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
// Venue Editing Handlers
// ============================================================================

// UpdateVenueRequest represents the HTTP request for updating a venue
// All body fields are optional - only changed fields need to be sent
type UpdateVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
	Body    struct {
		Name       *string `json:"name,omitempty" required:"false" doc:"Venue name"`
		Address    *string `json:"address,omitempty" required:"false" doc:"Venue address"`
		City       *string `json:"city,omitempty" required:"false" doc:"Venue city"`
		State      *string `json:"state,omitempty" required:"false" doc:"Venue state"`
		Zipcode    *string `json:"zipcode,omitempty" required:"false" doc:"Venue zipcode"`
		Instagram  *string `json:"instagram,omitempty" required:"false" doc:"Instagram handle or URL"`
		Facebook   *string `json:"facebook,omitempty" required:"false" doc:"Facebook URL"`
		Twitter    *string `json:"twitter,omitempty" required:"false" doc:"Twitter URL"`
		YouTube    *string `json:"youtube,omitempty" required:"false" doc:"YouTube URL"`
		Spotify    *string `json:"spotify,omitempty" required:"false" doc:"Spotify URL"`
		SoundCloud *string `json:"soundcloud,omitempty" required:"false" doc:"SoundCloud URL"`
		Bandcamp   *string `json:"bandcamp,omitempty" required:"false" doc:"Bandcamp URL"`
		Website    *string `json:"website,omitempty" required:"false" doc:"Website URL"`
	}
}

// UpdateVenueResponse represents the HTTP response for updating a venue
// Can return either updated venue (admin) or pending edit info (non-admin)
type UpdateVenueResponse struct {
	Body struct {
		Venue       *services.VenueDetailResponse       `json:"venue,omitempty" doc:"Updated venue (admin only)"`
		PendingEdit *services.PendingVenueEditResponse  `json:"pending_edit,omitempty" doc:"Pending edit info (non-admin)"`
		Status      string                              `json:"status" doc:"Result status: updated or pending"`
		Message     string                              `json:"message" doc:"Human-readable message"`
	}
}

// UpdateVenueHandler handles PUT /venues/{venue_id}
// Admin: Updates venue directly
// Non-admin: Creates pending edit if user is the venue owner
func (h *VenueHandler) UpdateVenueHandler(ctx context.Context, req *UpdateVenueRequest) (*UpdateVenueResponse, error) {
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

	// Build edit request
	editReq := &services.VenueEditRequest{
		Name:       req.Body.Name,
		Address:    req.Body.Address,
		City:       req.Body.City,
		State:      req.Body.State,
		Zipcode:    req.Body.Zipcode,
		Instagram:  req.Body.Instagram,
		Facebook:   req.Body.Facebook,
		Twitter:    req.Body.Twitter,
		YouTube:    req.Body.YouTube,
		Spotify:    req.Body.Spotify,
		SoundCloud: req.Body.SoundCloud,
		Bandcamp:   req.Body.Bandcamp,
		Website:    req.Body.Website,
	}

	// Validate required fields aren't being set to empty strings
	if editReq.Name != nil && *editReq.Name == "" {
		return nil, huma.Error422UnprocessableEntity("Venue name cannot be empty")
	}
	if editReq.City != nil && *editReq.City == "" {
		return nil, huma.Error422UnprocessableEntity("City cannot be empty")
	}
	if editReq.State != nil && *editReq.State == "" {
		return nil, huma.Error422UnprocessableEntity("State cannot be empty")
	}

	// Admin flow: direct update
	if user.IsAdmin {
		logger.FromContext(ctx).Info("admin_venue_update",
			"venue_id", venueID,
			"admin_id", user.ID,
			"request_id", requestID,
		)

		// Build updates map
		updates := make(map[string]interface{})
		if editReq.Name != nil {
			updates["name"] = *editReq.Name
		}
		if editReq.Address != nil {
			updates["address"] = *editReq.Address
		}
		if editReq.City != nil {
			updates["city"] = *editReq.City
		}
		if editReq.State != nil {
			updates["state"] = *editReq.State
		}
		if editReq.Zipcode != nil {
			updates["zipcode"] = *editReq.Zipcode
		}
		if editReq.Instagram != nil {
			updates["instagram"] = *editReq.Instagram
		}
		if editReq.Facebook != nil {
			updates["facebook"] = *editReq.Facebook
		}
		if editReq.Twitter != nil {
			updates["twitter"] = *editReq.Twitter
		}
		if editReq.YouTube != nil {
			updates["youtube"] = *editReq.YouTube
		}
		if editReq.Spotify != nil {
			updates["spotify"] = *editReq.Spotify
		}
		if editReq.SoundCloud != nil {
			updates["soundcloud"] = *editReq.SoundCloud
		}
		if editReq.Bandcamp != nil {
			updates["bandcamp"] = *editReq.Bandcamp
		}
		if editReq.Website != nil {
			updates["website"] = *editReq.Website
		}

		updatedVenue, err := h.venueService.UpdateVenue(uint(venueID), updates)
		if err != nil {
			logger.FromContext(ctx).Error("admin_venue_update_failed",
				"venue_id", venueID,
				"error", err.Error(),
				"request_id", requestID,
			)
			return nil, huma.Error422UnprocessableEntity(
				fmt.Sprintf("Failed to update venue (request_id: %s)", requestID),
			)
		}

		return &UpdateVenueResponse{
			Body: struct {
				Venue       *services.VenueDetailResponse       `json:"venue,omitempty" doc:"Updated venue (admin only)"`
				PendingEdit *services.PendingVenueEditResponse  `json:"pending_edit,omitempty" doc:"Pending edit info (non-admin)"`
				Status      string                              `json:"status" doc:"Result status: updated or pending"`
				Message     string                              `json:"message" doc:"Human-readable message"`
			}{
				Venue:   updatedVenue,
				Status:  "updated",
				Message: "Venue updated successfully",
			},
		}, nil
	}

	// Non-admin flow: check ownership and create pending edit
	// Only allow edits if user is the venue owner (submitted_by matches)
	if venue.SubmittedBy == nil || *venue.SubmittedBy != user.ID {
		logger.FromContext(ctx).Warn("venue_edit_forbidden",
			"venue_id", venueID,
			"user_id", user.ID,
			"venue_submitted_by", venue.SubmittedBy,
			"request_id", requestID,
		)
		return nil, huma.Error403Forbidden("You can only edit venues you submitted")
	}

	logger.FromContext(ctx).Info("user_venue_edit_request",
		"venue_id", venueID,
		"user_id", user.ID,
		"request_id", requestID,
	)

	// Create pending edit
	pendingEdit, err := h.venueService.CreatePendingVenueEdit(uint(venueID), user.ID, editReq)
	if err != nil {
		logger.FromContext(ctx).Error("user_venue_edit_failed",
			"venue_id", venueID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)

		// Check for conflict error (existing pending edit)
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenuePendingEditExists {
			return nil, huma.Error409Conflict("You already have a pending edit for this venue")
		}

		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to create pending edit (request_id: %s)", requestID),
		)
	}

	// Send Discord notification for pending venue edit
	submitterEmail := ""
	if user.Email != nil {
		submitterEmail = *user.Email
	}
	venueName := ""
	if pendingEdit.Venue != nil {
		venueName = pendingEdit.Venue.Name
	}
	h.discordService.NotifyPendingVenueEdit(pendingEdit.ID, pendingEdit.VenueID, venueName, submitterEmail)

	return &UpdateVenueResponse{
		Body: struct {
			Venue       *services.VenueDetailResponse       `json:"venue,omitempty" doc:"Updated venue (admin only)"`
			PendingEdit *services.PendingVenueEditResponse  `json:"pending_edit,omitempty" doc:"Pending edit info (non-admin)"`
			Status      string                              `json:"status" doc:"Result status: updated or pending"`
			Message     string                              `json:"message" doc:"Human-readable message"`
		}{
			PendingEdit: pendingEdit,
			Status:      "pending",
			Message:     "Your edit has been submitted for review",
		},
	}, nil
}

// GetMyPendingEditRequest represents the request for getting user's pending edit
type GetMyPendingEditRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// GetMyPendingEditResponse represents the response for user's pending edit
type GetMyPendingEditResponse struct {
	Body struct {
		PendingEdit *services.PendingVenueEditResponse `json:"pending_edit" doc:"User's pending edit for this venue, null if none"`
	}
}

// GetMyPendingEditHandler handles GET /venues/{venue_id}/my-pending-edit
func (h *VenueHandler) GetMyPendingEditHandler(ctx context.Context, req *GetMyPendingEditRequest) (*GetMyPendingEditResponse, error) {
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

	pendingEdit, err := h.venueService.GetPendingEditForVenue(uint(venueID), user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_pending_edit_failed",
			"venue_id", venueID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get pending edit (request_id: %s)", requestID),
		)
	}

	return &GetMyPendingEditResponse{
		Body: struct {
			PendingEdit *services.PendingVenueEditResponse `json:"pending_edit" doc:"User's pending edit for this venue, null if none"`
		}{
			PendingEdit: pendingEdit,
		},
	}, nil
}

// CancelMyPendingEditRequest represents the request for canceling user's pending edit
type CancelMyPendingEditRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// CancelMyPendingEditResponse represents the response for canceling pending edit
type CancelMyPendingEditResponse struct {
	Body struct {
		Message string `json:"message" doc:"Success message"`
	}
}

// CancelMyPendingEditHandler handles DELETE /venues/{venue_id}/my-pending-edit
func (h *VenueHandler) CancelMyPendingEditHandler(ctx context.Context, req *CancelMyPendingEditRequest) (*CancelMyPendingEditResponse, error) {
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

	// First get the pending edit to get its ID
	pendingEdit, err := h.venueService.GetPendingEditForVenue(uint(venueID), user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_pending_edit_for_cancel_failed",
			"venue_id", venueID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get pending edit (request_id: %s)", requestID),
		)
	}

	if pendingEdit == nil {
		return nil, huma.Error404NotFound("No pending edit found for this venue")
	}

	// Cancel the pending edit
	if err := h.venueService.CancelPendingVenueEdit(pendingEdit.ID, user.ID); err != nil {
		logger.FromContext(ctx).Error("cancel_pending_edit_failed",
			"venue_id", venueID,
			"edit_id", pendingEdit.ID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to cancel pending edit (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("pending_edit_cancelled",
		"venue_id", venueID,
		"edit_id", pendingEdit.ID,
		"user_id", user.ID,
		"request_id", requestID,
	)

	return &CancelMyPendingEditResponse{
		Body: struct {
			Message string `json:"message" doc:"Success message"`
		}{
			Message: "Pending edit cancelled successfully",
		},
	}, nil
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
