package pipeline

import (
	"context"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// PipelineHandler handles the enrichment admin endpoints. (The legacy
// venue-extraction handlers — extract/venues/imports — were removed with the
// extraction pipeline in PSY-1165; the enrichment + discovery surface stays.)
type PipelineHandler struct {
	enrichmentService contracts.EnrichmentServiceInterface
}

// NewPipelineHandler creates a new pipeline handler.
func NewPipelineHandler(
	enrichmentService contracts.EnrichmentServiceInterface,
) *PipelineHandler {
	return &PipelineHandler{
		enrichmentService: enrichmentService,
	}
}

// --- Enrichment Status ---

// EnrichmentStatusRequest is the Huma request for GET /admin/pipeline/enrichment/status
type EnrichmentStatusRequest struct{}

// EnrichmentStatusResponse is the Huma response for GET /admin/pipeline/enrichment/status
type EnrichmentStatusResponse struct {
	Body contracts.EnrichmentQueueStats
}

// EnrichmentStatusHandler handles GET /admin/pipeline/enrichment/status
func (h *PipelineHandler) EnrichmentStatusHandler(ctx context.Context, req *EnrichmentStatusRequest) (*EnrichmentStatusResponse, error) {
	stats, err := h.enrichmentService.GetQueueStats()
	if err != nil {
		logger.FromContext(ctx).Error("enrichment_status_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get enrichment status")
	}

	return &EnrichmentStatusResponse{Body: *stats}, nil
}

// --- Trigger Enrichment ---

// TriggerEnrichmentRequest is the Huma request for POST /admin/pipeline/enrichment/trigger/{show_id}
type TriggerEnrichmentRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID to enrich"`
}

// TriggerEnrichmentResponse is the Huma response for POST /admin/pipeline/enrichment/trigger/{show_id}
type TriggerEnrichmentResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// TriggerEnrichmentHandler handles POST /admin/pipeline/enrichment/trigger/{show_id}
func (h *PipelineHandler) TriggerEnrichmentHandler(ctx context.Context, req *TriggerEnrichmentRequest) (*TriggerEnrichmentResponse, error) {
	user := middleware.GetUserFromContext(ctx)

	showID, err := strconv.ParseUint(req.ShowID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	if err := h.enrichmentService.QueueShowForEnrichment(uint(showID), "all"); err != nil {
		logger.FromContext(ctx).Error("enrichment_trigger_failed",
			"show_id", showID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("enrichment_triggered",
		"show_id", showID,
		"admin_id", user.ID,
	)

	resp := &TriggerEnrichmentResponse{}
	resp.Body.Success = true
	resp.Body.Message = "Show queued for enrichment"
	return resp, nil
}
