package admin

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminVenueHandler handles admin venue management
type AdminVenueHandler struct {
	venueService      contracts.VenueServiceInterface
	venueMergeService contracts.VenueMergeServiceInterface
	auditLogService   contracts.AuditLogServiceInterface
}

// NewAdminVenueHandler creates a new admin venue handler
func NewAdminVenueHandler(
	venueService contracts.VenueServiceInterface,
	venueMergeService contracts.VenueMergeServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *AdminVenueHandler {
	return &AdminVenueHandler{
		venueService:      venueService,
		venueMergeService: venueMergeService,
		auditLogService:   auditLogService,
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
	Limit  int `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of venues to return (max 100)"`
	Offset int `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
}

// GetUnverifiedVenuesResponse represents the HTTP response for listing unverified venues
type GetUnverifiedVenuesResponse struct {
	Body struct {
		Venues []*contracts.UnverifiedVenueResponse `json:"venues"`
		Total  int64                                `json:"total"`
	}
}

// MergeVenuesBody is the request body shared by the merge and merge-preview
// endpoints. Both take the same pair of ids so an admin can preview and then
// commit with an identical payload.
type MergeVenuesBody struct {
	CanonicalVenueID uint `json:"canonical_venue_id" doc:"Venue that survives the merge"`
	MergeFromVenueID uint `json:"merge_from_venue_id" doc:"Venue that is folded in and deleted"`
}

// MergeVenuesRequest represents the HTTP request for merging two venues.
type MergeVenuesRequest struct {
	Body MergeVenuesBody
}

// MergeVenuesResponse represents the HTTP response for a venue merge or
// merge preview.
type MergeVenuesResponse struct {
	Body *contracts.MergeVenueResult
}

// PreviewMergeVenuesHandler handles POST /admin/venues/merge/preview.
//
// Read-only despite being a POST: it reports what the merge WOULD do and
// commits nothing. POST rather than GET because it takes the same body as the
// merge itself, so previewing and committing differ only in the URL.
func (h *AdminVenueHandler) PreviewMergeVenuesHandler(ctx context.Context, req *MergeVenuesRequest) (*MergeVenuesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	result, err := h.venueMergeService.PreviewMergeVenues(req.Body.CanonicalVenueID, req.Body.MergeFromVenueID)
	if err != nil {
		if mapped := shared.MapVenueError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("preview_merge_venues_failed",
			"canonical_id", req.Body.CanonicalVenueID,
			"merge_from_id", req.Body.MergeFromVenueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to preview venue merge (request_id: %s)", requestID),
		)
	}

	return &MergeVenuesResponse{Body: result}, nil
}

// MergeVenuesHandler handles POST /admin/venues/merge.
//
// Destructive: it deletes duplicate shows and the losing venue. The service
// writes its own audit-log entry with the full count breakdown.
func (h *AdminVenueHandler) MergeVenuesHandler(ctx context.Context, req *MergeVenuesRequest) (*MergeVenuesResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	result, err := h.venueMergeService.MergeVenues(req.Body.CanonicalVenueID, req.Body.MergeFromVenueID, user.ID)
	if err != nil {
		if mapped := shared.MapVenueError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("merge_venues_failed",
			"canonical_id", req.Body.CanonicalVenueID,
			"merge_from_id", req.Body.MergeFromVenueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to merge venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("venues_merged",
		"canonical_id", result.CanonicalVenueID,
		"merged_id", result.MergedVenueID,
		"merged_name", result.MergedVenueName,
		"duplicate_shows", result.DuplicateShows,
		"support_acts_rescued", result.SupportActsRescued,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &MergeVenuesResponse{Body: result}, nil
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
