package admin

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminVenueHandler handles admin venue management
type AdminVenueHandler struct {
	venueService    contracts.VenueServiceInterface
	auditLogService contracts.AuditLogServiceInterface
}

// NewAdminVenueHandler creates a new admin venue handler
func NewAdminVenueHandler(
	venueService contracts.VenueServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *AdminVenueHandler {
	return &AdminVenueHandler{
		venueService:    venueService,
		auditLogService: auditLogService,
	}
}

// VerifyVenueRequest represents the HTTP request for verifying a venue
type VerifyVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// VerifyVenueResponse represents the HTTP response for verifying a venue
type VerifyVenueResponse struct {
	Body contracts.VenueDetailResponse `json:"body"`
}

// GetUnverifiedVenuesRequest represents the HTTP request for listing unverified venues
type GetUnverifiedVenuesRequest struct {
	Limit  int `query:"limit" default:"50" doc:"Number of venues to return (max 100)"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetUnverifiedVenuesResponse represents the HTTP response for listing unverified venues
type GetUnverifiedVenuesResponse struct {
	Body struct {
		Venues []*contracts.UnverifiedVenueResponse `json:"venues"`
		Total  int64                                `json:"total"`
	}
}

// VerifyVenueHandler handles POST /admin/venues/{venue_id}/verify
func (h *AdminVenueHandler) VerifyVenueHandler(ctx context.Context, req *VerifyVenueRequest) (*VerifyVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	logger.FromContext(ctx).Debug("admin_verify_venue_attempt",
		"venue_id", venueID,
		"admin_id", user.ID,
	)

	// Verify the venue
	venue, err := h.venueService.VerifyVenue(uint(venueID))
	if err != nil {
		logger.FromContext(ctx).Error("admin_verify_venue_failed",
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to verify venue (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_verify_venue_success",
		"venue_id", venueID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log
	h.auditLogService.LogAction(user.ID, "verify_venue", "venue", uint(venueID), nil)

	return &VerifyVenueResponse{Body: *venue}, nil
}

// GetUnverifiedVenuesHandler handles GET /admin/venues/unverified
// Returns venues that have not been verified by an admin, for admin review.
func (h *AdminVenueHandler) GetUnverifiedVenuesHandler(ctx context.Context, req *GetUnverifiedVenuesRequest) (*GetUnverifiedVenuesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Validate limit
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("admin_unverified_venues_attempt",
		"limit", limit,
		"offset", offset,
	)

	// Get unverified venues
	venues, total, err := h.venueService.GetUnverifiedVenues(limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("admin_unverified_venues_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get unverified venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_unverified_venues_success",
		"count", len(venues),
		"total", total,
	)

	resp := &GetUnverifiedVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Total = total
	return resp, nil
}
