package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// StreamingWorklistHandler owns the admin streaming-discovery worklist
// surface. Sits alongside AdminDiscoveryHandler — both handlers belong
// to the discovery-flow admin surface.
type StreamingWorklistHandler struct {
	worklistService contracts.StreamingWorklistServiceInterface
	auditLogService contracts.AuditLogServiceInterface
}

// NewStreamingWorklistHandler creates the handler with its service +
// audit-log dependencies.
func NewStreamingWorklistHandler(
	worklistService contracts.StreamingWorklistServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *StreamingWorklistHandler {
	return &StreamingWorklistHandler{
		worklistService: worklistService,
		auditLogService: auditLogService,
	}
}

// ──────────────────────────────────────────────
// GET /admin/streaming-worklist
// ──────────────────────────────────────────────

// GetStreamingWorklistRequest is the Huma request shape.
//
// Status: optional filter; one of {unreviewed, candidates_pending}. Any
// other value returns 400 (terminal-state filters are nonsense — those
// rows are excluded by definition).
type GetStreamingWorklistRequest struct {
	Status string `query:"status" doc:"Optional non-terminal status filter: 'unreviewed' or 'candidates_pending'. Empty means both."`
	Limit  int    `query:"limit" default:"50" doc:"Number of artists to return (1–200; default 50)"`
	Offset int    `query:"offset" default:"0" doc:"Pagination offset"`
}

// GetStreamingWorklistResponse wraps the service result for OpenAPI.
type GetStreamingWorklistResponse struct {
	Body contracts.StreamingWorklistResult `json:"body"`
}

// GetStreamingWorklistHandler handles GET /admin/streaming-worklist.
// Returns artists with non-terminal streaming-discovery status who have
// at least one upcoming show, ordered by soonest show ASC.
func (h *StreamingWorklistHandler) GetStreamingWorklistHandler(ctx context.Context, req *GetStreamingWorklistRequest) (*GetStreamingWorklistResponse, error) {
	requestID := logger.GetRequestID(ctx)

	logger.FromContext(ctx).Debug("admin_streaming_worklist_attempt",
		"status", req.Status,
		"limit", req.Limit,
		"offset", req.Offset,
	)

	result, err := h.worklistService.ListStreamingWorklist(req.Status, req.Limit, req.Offset)
	if err != nil {
		// Invalid status filter — surface the validation error as 400 so
		// callers can correct the query string.
		if errors.Is(err, contracts.ErrInvalidStreamingStatusTransition) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		logger.FromContext(ctx).Error("admin_streaming_worklist_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list streaming worklist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_streaming_worklist_success",
		"count", len(result.Entries),
		"total", result.Total,
	)

	return &GetStreamingWorklistResponse{Body: *result}, nil
}

// ──────────────────────────────────────────────
// POST /admin/artists/{artist_id}/streaming-discovery-status
// ──────────────────────────────────────────────

// UpdateStreamingStatusRequest is the Huma request shape for the
// mutation. Status is required; Reason is optional and only persisted
// for no_links_found / skipped — re-opens to `unreviewed` clear any
// prior reason inside the service.
type UpdateStreamingStatusRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
	Body     struct {
		Status string  `json:"status" doc:"Target status: 'unreviewed', 'linked', 'no_links_found', or 'skipped'. 'candidates_pending' is engine-set and rejected here."`
		Reason *string `json:"reason,omitempty" doc:"Optional admin note. Persisted for no_links_found / skipped. Cleared on re-open."`
	}
}

// UpdateStreamingStatusResponse wraps the updated artist row for
// OpenAPI.
type UpdateStreamingStatusResponse struct {
	Body contracts.StreamingDiscoveryArtistResponse `json:"body"`
}

// UpdateStreamingDiscoveryStatusHandler handles
// POST /admin/artists/{artist_id}/streaming-discovery-status. Validates
// the requested transition, writes the new status (and reason where
// applicable), audits the change, and returns the updated row.
func (h *StreamingWorklistHandler) UpdateStreamingDiscoveryStatusHandler(ctx context.Context, req *UpdateStreamingStatusRequest) (*UpdateStreamingStatusResponse, error) {
	requestID := logger.GetRequestID(ctx)
	user := middleware.GetUserFromContext(ctx)

	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	if req.Body.Status == "" {
		return nil, huma.Error400BadRequest("status is required")
	}

	logger.FromContext(ctx).Debug("admin_streaming_status_update_attempt",
		"artist_id", artistID,
		"target_status", req.Body.Status,
		"has_reason", req.Body.Reason != nil && *req.Body.Reason != "",
		"admin_id", user.ID,
	)

	updated, err := h.worklistService.UpdateStreamingDiscoveryStatus(contracts.UpdateStreamingDiscoveryStatusInput{
		ArtistID: uint(artistID),
		Status:   req.Body.Status,
		Reason:   req.Body.Reason,
	})
	if err != nil {
		// Match-order matters: NotFound is the more specific error.
		if errors.Is(err, contracts.ErrStreamingArtistNotFound) {
			return nil, huma.Error404NotFound("artist not found")
		}
		if errors.Is(err, contracts.ErrInvalidStreamingStatusTransition) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		logger.FromContext(ctx).Error("admin_streaming_status_update_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update streaming-discovery status (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_streaming_status_update_success",
		"artist_id", artistID,
		"new_status", updated.StreamingDiscoveryStatus,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Audit log — captures the decision for the contributor-history
	// surfaces and any future "who triaged X" reporting.
	if h.auditLogService != nil {
		metadata := map[string]interface{}{
			"new_status": updated.StreamingDiscoveryStatus,
		}
		if updated.StreamingDiscoveryReason != nil {
			metadata["reason"] = *updated.StreamingDiscoveryReason
		}
		h.auditLogService.LogAction(user.ID, "update_streaming_discovery_status", "artist", updated.ID, metadata)
	}

	return &UpdateStreamingStatusResponse{Body: *updated}, nil
}
