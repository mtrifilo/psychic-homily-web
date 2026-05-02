package admin

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminDataHandler handles admin data sync export/import
type AdminDataHandler struct {
	dataSyncService contracts.DataSyncServiceInterface
}

// NewAdminDataHandler creates a new admin data handler
func NewAdminDataHandler(
	dataSyncService contracts.DataSyncServiceInterface,
) *AdminDataHandler {
	return &AdminDataHandler{
		dataSyncService: dataSyncService,
	}
}

// ============================================================================
// Data Export/Import Handlers (for syncing local data to Stage/Production)
// ============================================================================

// ExportShowsRequest represents the HTTP request for exporting shows
type ExportShowsRequest struct {
	Limit    int    `query:"limit" default:"50" doc:"Number of shows to return (max 200)"`
	Offset   int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Status   string `query:"status" doc:"Filter by status: approved, pending, rejected, all"`
	FromDate string `query:"from_date" doc:"Filter shows from this date (YYYY-MM-DD)"`
	City     string `query:"city" doc:"Filter by city"`
	State    string `query:"state" doc:"Filter by state"`
}

// ExportShowsResponse represents the HTTP response for exporting shows
type ExportShowsResponse struct {
	Body contracts.ExportShowsResult `json:"body"`
}

// ExportShowsHandler handles GET /admin/export/shows
func (h *AdminDataHandler) ExportShowsHandler(ctx context.Context, req *ExportShowsRequest) (*ExportShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Build params
	params := contracts.ExportShowsParams{
		Limit:  req.Limit,
		Offset: offset,
		Status: req.Status,
		City:   req.City,
		State:  req.State,
	}

	// Parse date filter
	if req.FromDate != "" {
		fromDate, err := shared.ParseDate(req.FromDate)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid from_date format, expected YYYY-MM-DD")
		}
		params.FromDate = &fromDate
	}

	logger.FromContext(ctx).Debug("admin_export_shows_attempt",
		"limit", params.Limit,
		"offset", params.Offset,
		"status", params.Status,
	)

	result, err := h.dataSyncService.ExportShows(params)
	if err != nil {
		logger.FromContext(ctx).Error("admin_export_shows_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to export shows (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_export_shows_success",
		"count", len(result.Shows),
		"total", result.Total,
	)

	return &ExportShowsResponse{Body: *result}, nil
}

// ExportArtistsRequest represents the HTTP request for exporting artists
type ExportArtistsRequest struct {
	Limit  int    `query:"limit" default:"50" doc:"Number of artists to return (max 200)"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search string `query:"search" doc:"Search by name"`
}

// ExportArtistsResponse represents the HTTP response for exporting artists
type ExportArtistsResponse struct {
	Body contracts.ExportArtistsResult `json:"body"`
}

// ExportArtistsHandler handles GET /admin/export/artists
func (h *AdminDataHandler) ExportArtistsHandler(ctx context.Context, req *ExportArtistsRequest) (*ExportArtistsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	params := contracts.ExportArtistsParams{
		Limit:  req.Limit,
		Offset: offset,
		Search: req.Search,
	}

	logger.FromContext(ctx).Debug("admin_export_artists_attempt",
		"limit", params.Limit,
		"offset", params.Offset,
		"search", params.Search,
	)

	result, err := h.dataSyncService.ExportArtists(params)
	if err != nil {
		logger.FromContext(ctx).Error("admin_export_artists_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to export artists (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_export_artists_success",
		"count", len(result.Artists),
		"total", result.Total,
	)

	return &ExportArtistsResponse{Body: *result}, nil
}

// ExportVenuesRequest represents the HTTP request for exporting venues
type ExportVenuesRequest struct {
	Limit    int    `query:"limit" default:"50" doc:"Number of venues to return (max 200)"`
	Offset   int    `query:"offset" default:"0" doc:"Offset for pagination"`
	Search   string `query:"search" doc:"Search by name"`
	Verified string `query:"verified" doc:"Filter by verified status: true, false, or empty for all"`
	City     string `query:"city" doc:"Filter by city"`
	State    string `query:"state" doc:"Filter by state"`
}

// ExportVenuesResponse represents the HTTP response for exporting venues
type ExportVenuesResponse struct {
	Body contracts.ExportVenuesResult `json:"body"`
}

// ExportVenuesHandler handles GET /admin/export/venues
func (h *AdminDataHandler) ExportVenuesHandler(ctx context.Context, req *ExportVenuesRequest) (*ExportVenuesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	params := contracts.ExportVenuesParams{
		Limit:  req.Limit,
		Offset: offset,
		Search: req.Search,
		City:   req.City,
		State:  req.State,
	}

	// Parse verified filter
	if req.Verified == "true" {
		verified := true
		params.Verified = &verified
	} else if req.Verified == "false" {
		verified := false
		params.Verified = &verified
	}

	logger.FromContext(ctx).Debug("admin_export_venues_attempt",
		"limit", params.Limit,
		"offset", params.Offset,
		"search", params.Search,
	)

	result, err := h.dataSyncService.ExportVenues(params)
	if err != nil {
		logger.FromContext(ctx).Error("admin_export_venues_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to export venues (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_export_venues_success",
		"count", len(result.Venues),
		"total", result.Total,
	)

	return &ExportVenuesResponse{Body: *result}, nil
}

// DataImportRequest represents the HTTP request for importing data
type DataImportRequest struct {
	Body contracts.DataImportRequest `json:"body"`
}

// DataImportResponse represents the HTTP response for importing data
type DataImportResponse struct {
	Body contracts.DataImportResult `json:"body"`
}

// DataImportHandler handles POST /admin/data/import
func (h *AdminDataHandler) DataImportHandler(ctx context.Context, req *DataImportRequest) (*DataImportResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	// Validate limits
	totalItems := len(req.Body.Shows) + len(req.Body.Artists) + len(req.Body.Venues)
	if totalItems == 0 {
		return nil, huma.Error422UnprocessableEntity("At least one show, artist, or venue is required")
	}
	if totalItems > 500 {
		return nil, huma.Error422UnprocessableEntity("Maximum 500 total items can be imported at once")
	}

	logger.FromContext(ctx).Debug("admin_data_import_attempt",
		"shows", len(req.Body.Shows),
		"artists", len(req.Body.Artists),
		"venues", len(req.Body.Venues),
		"dry_run", req.Body.DryRun,
		"admin_id", user.ID,
	)

	result, err := h.dataSyncService.ImportData(req.Body)
	if err != nil {
		logger.FromContext(ctx).Error("admin_data_import_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to import data (request_id: %s)", requestID),
		)
	}

	action := "imported"
	if req.Body.DryRun {
		action = "previewed"
	}

	logger.FromContext(ctx).Info("admin_data_import_success",
		"action", action,
		"shows_imported", result.Shows.Imported,
		"artists_imported", result.Artists.Imported,
		"venues_imported", result.Venues.Imported,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &DataImportResponse{Body: *result}, nil
}
