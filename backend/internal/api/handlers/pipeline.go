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
	enrichmentService  services.EnrichmentServiceInterface
}

// NewPipelineHandler creates a new pipeline handler.
func NewPipelineHandler(
	pipelineService services.PipelineServiceInterface,
	venueConfigService services.VenueSourceConfigServiceInterface,
	enrichmentService services.EnrichmentServiceInterface,
) *PipelineHandler {
	return &PipelineHandler{
		pipelineService:    pipelineService,
		venueConfigService: venueConfigService,
		enrichmentService:  enrichmentService,
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

// --- Update Venue Config ---

// UpdateVenueConfigRequest is the Huma request for PUT /admin/pipeline/venues/{venue_id}/config
type UpdateVenueConfigRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
	Body    struct {
		CalendarURL     *string `json:"calendar_url" doc:"URL to the venue's event calendar page"`
		PreferredSource string  `json:"preferred_source" doc:"Extraction source preference (ai, ical, rss)"`
		RenderMethod    *string `json:"render_method" doc:"Rendering method (static, dynamic, screenshot) or null for auto-detect"`
		FeedURL         *string `json:"feed_url" doc:"URL to iCal/RSS feed if available"`
		AutoApprove     bool    `json:"auto_approve" doc:"Whether to auto-approve extracted shows"`
		StrategyLocked  bool    `json:"strategy_locked" doc:"Lock strategy to prevent automatic re-evaluation"`
		ExtractionNotes *string `json:"extraction_notes" doc:"Notes included in AI extraction prompt"`
	}
}

// UpdateVenueConfigResponse is the Huma response for PUT /admin/pipeline/venues/{venue_id}/config
type UpdateVenueConfigResponse struct {
	Body PipelineVenueInfo
}

// UpdateVenueConfigHandler handles PUT /admin/pipeline/venues/{venue_id}/config
func (h *PipelineHandler) UpdateVenueConfigHandler(ctx context.Context, req *UpdateVenueConfigRequest) (*UpdateVenueConfigResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	config := &models.VenueSourceConfig{
		VenueID:         uint(venueID),
		CalendarURL:     req.Body.CalendarURL,
		PreferredSource: req.Body.PreferredSource,
		RenderMethod:    req.Body.RenderMethod,
		FeedURL:         req.Body.FeedURL,
		AutoApprove:     req.Body.AutoApprove,
		StrategyLocked:  req.Body.StrategyLocked,
		ExtractionNotes: req.Body.ExtractionNotes,
	}

	updated, err := h.venueConfigService.CreateOrUpdate(config)
	if err != nil {
		logger.FromContext(ctx).Error("pipeline_update_config_failed",
			"venue_id", venueID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("pipeline_venue_config_updated",
		"venue_id", venueID,
		"admin_id", user.ID,
	)

	info := PipelineVenueInfo{
		VenueID:             updated.VenueID,
		VenueName:           updated.Venue.Name,
		CalendarURL:         updated.CalendarURL,
		PreferredSource:     updated.PreferredSource,
		RenderMethod:        updated.RenderMethod,
		FeedURL:             updated.FeedURL,
		LastExtractedAt:     updated.LastExtractedAt,
		EventsExpected:      updated.EventsExpected,
		ConsecutiveFailures: updated.ConsecutiveFailures,
		StrategyLocked:      updated.StrategyLocked,
		AutoApprove:         updated.AutoApprove,
		ExtractionNotes:     updated.ExtractionNotes,
	}
	if updated.Venue.Slug != nil {
		info.VenueSlug = *updated.Venue.Slug
	}

	return &UpdateVenueConfigResponse{Body: info}, nil
}

// --- Get Venue Runs ---

// GetVenueRunsRequest is the Huma request for GET /admin/pipeline/venues/{venue_id}/runs
type GetVenueRunsRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
	Limit   int    `query:"limit" doc:"Max runs to return (default 10, max 100)"`
}

// GetVenueRunsResponse is the Huma response for GET /admin/pipeline/venues/{venue_id}/runs
type GetVenueRunsResponse struct {
	Body struct {
		Runs  []models.VenueExtractionRun `json:"runs"`
		Total int                         `json:"total"`
	}
}

// GetVenueRunsHandler handles GET /admin/pipeline/venues/{venue_id}/runs
func (h *PipelineHandler) GetVenueRunsHandler(ctx context.Context, req *GetVenueRunsRequest) (*GetVenueRunsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	runs, err := h.venueConfigService.GetRecentRuns(uint(venueID), limit)
	if err != nil {
		logger.FromContext(ctx).Error("pipeline_get_runs_failed",
			"venue_id", venueID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	resp := &GetVenueRunsResponse{}
	resp.Body.Runs = runs
	resp.Body.Total = len(runs)
	return resp, nil
}

// --- Import History (cross-venue) ---

// GetImportHistoryRequest is the Huma request for GET /admin/pipeline/imports
type GetImportHistoryRequest struct {
	Limit  int `query:"limit" doc:"Max entries to return (default 20, max 100)"`
	Offset int `query:"offset" doc:"Number of entries to skip for pagination"`
}

// GetImportHistoryResponse is the Huma response for GET /admin/pipeline/imports
type GetImportHistoryResponse struct {
	Body struct {
		Imports []services.ImportHistoryEntry `json:"imports"`
		Total   int64                        `json:"total"`
	}
}

// GetImportHistoryHandler handles GET /admin/pipeline/imports
func (h *PipelineHandler) GetImportHistoryHandler(ctx context.Context, req *GetImportHistoryRequest) (*GetImportHistoryResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	imports, total, err := h.venueConfigService.GetAllRecentRuns(req.Limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("pipeline_get_import_history_failed",
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to get import history")
	}

	resp := &GetImportHistoryResponse{}
	resp.Body.Imports = imports
	resp.Body.Total = total
	return resp, nil
}

// --- Reset Render Method ---

// ResetRenderMethodRequest is the Huma request for POST /admin/pipeline/venues/{venue_id}/reset-render-method
type ResetRenderMethodRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// ResetRenderMethodResponse is the Huma response for POST /admin/pipeline/venues/{venue_id}/reset-render-method
type ResetRenderMethodResponse struct {
	Body struct {
		Success bool `json:"success"`
	}
}

// ResetRenderMethodHandler handles POST /admin/pipeline/venues/{venue_id}/reset-render-method
func (h *PipelineHandler) ResetRenderMethodHandler(ctx context.Context, req *ResetRenderMethodRequest) (*ResetRenderMethodResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 64)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	if err := h.venueConfigService.ResetRenderMethod(uint(venueID)); err != nil {
		logger.FromContext(ctx).Error("pipeline_reset_render_method_failed",
			"venue_id", venueID,
			"error", err.Error(),
		)
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	logger.FromContext(ctx).Info("pipeline_render_method_reset",
		"venue_id", venueID,
		"admin_id", user.ID,
	)

	resp := &ResetRenderMethodResponse{}
	resp.Body.Success = true
	return resp, nil
}

// --- Enrichment Status ---

// EnrichmentStatusRequest is the Huma request for GET /admin/pipeline/enrichment/status
type EnrichmentStatusRequest struct{}

// EnrichmentStatusResponse is the Huma response for GET /admin/pipeline/enrichment/status
type EnrichmentStatusResponse struct {
	Body services.EnrichmentQueueStats
}

// EnrichmentStatusHandler handles GET /admin/pipeline/enrichment/status
func (h *PipelineHandler) EnrichmentStatusHandler(ctx context.Context, req *EnrichmentStatusRequest) (*EnrichmentStatusResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

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
	if user == nil || !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

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
