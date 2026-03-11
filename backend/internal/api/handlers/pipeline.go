package handlers

import (
	"context"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// PipelineHandler handles AI extraction pipeline admin endpoints.
type PipelineHandler struct {
	pipelineService    services.PipelineServiceInterface
	venueConfigService services.VenueSourceConfigServiceInterface
}

// NewPipelineHandler creates a new pipeline handler.
func NewPipelineHandler(
	pipelineService services.PipelineServiceInterface,
	venueConfigService services.VenueSourceConfigServiceInterface,
) *PipelineHandler {
	return &PipelineHandler{
		pipelineService:    pipelineService,
		venueConfigService: venueConfigService,
	}
}

// --- Extract Venue ---

// ExtractVenueRequest is the Huma request for POST /admin/pipeline/extract/{venue_id}
type ExtractVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID to extract"`
	DryRun  bool   `query:"dry_run" doc:"If true, extract but do not import events"`
}

// ExtractVenueResponse is the Huma response for POST /admin/pipeline/extract/{venue_id}
type ExtractVenueResponse struct {
	Body services.PipelineResult
}

// ExtractVenueHandler handles POST /admin/pipeline/extract/{venue_id}
func (h *PipelineHandler) ExtractVenueHandler(ctx context.Context, req *ExtractVenueRequest) (*ExtractVenueResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	logger.FromContext(ctx).Info("pipeline_extract_venue",
		"venue_id", venueID,
		"dry_run", req.DryRun,
		"admin_id", user.ID,
	)

	result, err := h.pipelineService.ExtractVenue(uint(venueID), req.DryRun)
	if err != nil {
		logger.FromContext(ctx).Error("pipeline_extract_failed",
			"venue_id", venueID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	return &ExtractVenueResponse{Body: *result}, nil
}

// --- List Configured Venues ---

// ListPipelineVenuesRequest is the Huma request for GET /admin/pipeline/venues
type ListPipelineVenuesRequest struct{}

// PipelineVenueInfo represents a venue with its source config and last run info.
type PipelineVenueInfo struct {
	VenueID             uint                       `json:"venue_id"`
	VenueName           string                     `json:"venue_name"`
	VenueSlug           string                     `json:"venue_slug"`
	CalendarURL         *string                    `json:"calendar_url"`
	PreferredSource     string                     `json:"preferred_source"`
	RenderMethod        *string                    `json:"render_method"`
	FeedURL             *string                    `json:"feed_url"`
	LastExtractedAt     *time.Time                 `json:"last_extracted_at"`
	EventsExpected      int                        `json:"events_expected"`
	ConsecutiveFailures int                        `json:"consecutive_failures"`
	StrategyLocked      bool                       `json:"strategy_locked"`
	AutoApprove         bool                       `json:"auto_approve"`
	ExtractionNotes     *string                    `json:"extraction_notes,omitempty"`
	ApprovalRate        *float64                   `json:"approval_rate,omitempty"`
	TotalRuns           int                        `json:"total_runs"`
	LastRun             *models.VenueExtractionRun `json:"last_run,omitempty"`
}

// ListPipelineVenuesResponse is the Huma response for GET /admin/pipeline/venues
type ListPipelineVenuesResponse struct {
	Body struct {
		Venues []PipelineVenueInfo `json:"venues"`
		Total  int                 `json:"total"`
	}
}

// ListPipelineVenuesHandler handles GET /admin/pipeline/venues
func (h *PipelineHandler) ListPipelineVenuesHandler(ctx context.Context, req *ListPipelineVenuesRequest) (*ListPipelineVenuesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	configs, err := h.venueConfigService.ListConfigured()
	if err != nil {
		logger.FromContext(ctx).Error("pipeline_list_venues_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to list configured venues")
	}

	venues := make([]PipelineVenueInfo, 0, len(configs))
	for _, cfg := range configs {
		info := PipelineVenueInfo{
			VenueID:             cfg.VenueID,
			VenueName:           cfg.Venue.Name,
			CalendarURL:         cfg.CalendarURL,
			PreferredSource:     cfg.PreferredSource,
			RenderMethod:        cfg.RenderMethod,
			FeedURL:             cfg.FeedURL,
			LastExtractedAt:     cfg.LastExtractedAt,
			EventsExpected:      cfg.EventsExpected,
			ConsecutiveFailures: cfg.ConsecutiveFailures,
			StrategyLocked:      cfg.StrategyLocked,
			AutoApprove:         cfg.AutoApprove,
			ExtractionNotes:     cfg.ExtractionNotes,
		}
		if cfg.Venue.Slug != nil {
			info.VenueSlug = *cfg.Venue.Slug
		}

		// Get recent runs for count + most recent
		runs, runErr := h.venueConfigService.GetRecentRuns(cfg.VenueID, 1)
		if runErr == nil && len(runs) > 0 {
			info.LastRun = &runs[0]
		}

		// Get rejection stats for approval rate (fire-and-forget on error)
		stats, statsErr := h.venueConfigService.GetRejectionStats(cfg.VenueID)
		if statsErr == nil && stats.TotalExtracted > 0 {
			info.ApprovalRate = &stats.ApprovalRate
		}

		venues = append(venues, info)
	}

	resp := &ListPipelineVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Total = len(venues)
	return resp, nil
}

// --- Venue Rejection Stats ---

// VenueRejectionStatsRequest is the Huma request for GET /admin/pipeline/venues/{venue_id}/stats
type VenueRejectionStatsRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// VenueRejectionStatsResponse is the Huma response for GET /admin/pipeline/venues/{venue_id}/stats
type VenueRejectionStatsResponse struct {
	Body services.VenueRejectionStats
}

// VenueRejectionStatsHandler handles GET /admin/pipeline/venues/{venue_id}/stats
func (h *PipelineHandler) VenueRejectionStatsHandler(ctx context.Context, req *VenueRejectionStatsRequest) (*VenueRejectionStatsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	stats, err := h.venueConfigService.GetRejectionStats(uint(venueID))
	if err != nil {
		logger.FromContext(ctx).Error("pipeline_rejection_stats_failed",
			"venue_id", venueID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	return &VenueRejectionStatsResponse{Body: *stats}, nil
}

// --- Update Extraction Notes ---

// UpdateExtractionNotesRequest is the Huma request for PATCH /admin/pipeline/venues/{venue_id}/notes
type UpdateExtractionNotesRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
	Body    struct {
		ExtractionNotes *string `json:"extraction_notes" doc:"Per-venue notes appended to AI extraction prompt"`
	}
}

// UpdateExtractionNotesResponse is the Huma response for PATCH /admin/pipeline/venues/{venue_id}/notes
type UpdateExtractionNotesResponse struct {
	Body struct {
		Success         bool    `json:"success"`
		ExtractionNotes *string `json:"extraction_notes"`
	}
}

// UpdateExtractionNotesHandler handles PATCH /admin/pipeline/venues/{venue_id}/notes
func (h *PipelineHandler) UpdateExtractionNotesHandler(ctx context.Context, req *UpdateExtractionNotesRequest) (*UpdateExtractionNotesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	if err := h.venueConfigService.UpdateExtractionNotes(uint(venueID), req.Body.ExtractionNotes); err != nil {
		logger.FromContext(ctx).Error("pipeline_update_notes_failed",
			"venue_id", venueID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("pipeline_extraction_notes_updated",
		"venue_id", venueID,
		"admin_id", user.ID,
	)

	resp := &UpdateExtractionNotesResponse{}
	resp.Body.Success = true
	resp.Body.ExtractionNotes = req.Body.ExtractionNotes
	return resp, nil
}
