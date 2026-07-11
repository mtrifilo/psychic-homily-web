package engagement

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// SavedShowHandler handles saved show HTTP requests
type SavedShowHandler struct {
	savedShowService contracts.SavedShowServiceInterface
}

// NewSavedShowHandler creates a new saved show handler
func NewSavedShowHandler(savedShowService contracts.SavedShowServiceInterface) *SavedShowHandler {
	return &SavedShowHandler{
		savedShowService: savedShowService,
	}
}

// SaveShowRequest represents the HTTP request for saving a show
type SaveShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// SaveShowResponse represents the HTTP response for saving a show
type SaveShowResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnsaveShowRequest represents the HTTP request for unsaving a show
type UnsaveShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// UnsaveShowResponse represents the HTTP response for unsaving a show
type UnsaveShowResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// GetSavedShowsRequest represents the HTTP request for listing saved shows
type GetSavedShowsRequest struct {
	Limit      int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of shows per page"`
	Offset     int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	TimeFilter string `query:"time_filter" required:"false" enum:"upcoming,past" doc:"Partition by show date in the venue's local timezone: 'upcoming' (event date today or later, soonest first) or 'past' (event date before today, most recent first). Omitted: all saved shows, most recently saved first."`
}

// GetSavedShowsResponse represents the HTTP response for listing saved shows
type GetSavedShowsResponse struct {
	Body struct {
		Shows  []*contracts.SavedShowResponse `json:"shows"`
		Total  int64                          `json:"total"`
		Limit  int                            `json:"limit"`
		Offset int                            `json:"offset"`
	}
}

// CheckSavedRequest represents the HTTP request for checking if a show is saved
type CheckSavedRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// CheckSavedResponse represents the HTTP response for checking if a show is saved
type CheckSavedResponse struct {
	Body struct {
		IsSaved bool `json:"is_saved"`
	}
}

// SaveShowHandler handles POST /saved-shows/{show_id}
func (h *SavedShowHandler) SaveShowHandler(ctx context.Context, req *SaveShowRequest) (*SaveShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("saved_show_invalid_id",
			"show_id_str", req.ShowID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	logger.FromContext(ctx).Debug("save_show_attempt",
		"user_id", user.ID,
		"show_id", showID,
	)

	// Save the show
	err = h.savedShowService.SaveShow(user.ID, uint(showID))
	if err != nil {
		logger.FromContext(ctx).Error("save_show_failed",
			"user_id", user.ID,
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to save show (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("save_show_success",
		"user_id", user.ID,
		"show_id", showID,
		"request_id", requestID,
	)

	return &SaveShowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Show saved successfully",
		},
	}, nil
}

// UnsaveShowHandler handles DELETE /saved-shows/{show_id}
func (h *SavedShowHandler) UnsaveShowHandler(ctx context.Context, req *UnsaveShowRequest) (*UnsaveShowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("unsave_show_invalid_id",
			"show_id_str", req.ShowID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	logger.FromContext(ctx).Debug("unsave_show_attempt",
		"user_id", user.ID,
		"show_id", showID,
	)

	// Unsave the show
	err = h.savedShowService.UnsaveShow(user.ID, uint(showID))
	if err != nil {
		logger.FromContext(ctx).Error("unsave_show_failed",
			"user_id", user.ID,
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to unsave show (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("unsave_show_success",
		"user_id", user.ID,
		"show_id", showID,
		"request_id", requestID,
	)

	return &UnsaveShowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Show unsaved successfully",
		},
	}, nil
}

// GetSavedShowsHandler handles GET /saved-shows
func (h *SavedShowHandler) GetSavedShowsHandler(ctx context.Context, req *GetSavedShowsRequest) (*GetSavedShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Validate pagination
	limit := req.Limit
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("get_saved_shows_attempt",
		"user_id", user.ID,
		"limit", limit,
		"offset", offset,
		"time_filter", req.TimeFilter,
	)

	// Get saved shows (huma's enum validation guarantees TimeFilter is
	// "", "upcoming", or "past" here)
	shows, total, err := h.savedShowService.GetUserSavedShows(user.ID, limit, offset, req.TimeFilter)
	if err != nil {
		logger.FromContext(ctx).Error("get_saved_shows_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get saved shows (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("get_saved_shows_success",
		"user_id", user.ID,
		"count", len(shows),
		"total", total,
	)

	return &GetSavedShowsResponse{
		Body: struct {
			Shows  []*contracts.SavedShowResponse `json:"shows"`
			Total  int64                          `json:"total"`
			Limit  int                            `json:"limit"`
			Offset int                            `json:"offset"`
		}{
			Shows:  shows,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}, nil
}

// GetSaveCountRequest represents the HTTP request for a show's public save count
type GetSaveCountRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// GetSaveCountResponse carries a show's public save count, plus — for an
// authenticated caller only — whether that caller saved the show themselves.
type GetSaveCountResponse struct {
	Body struct {
		ShowID    uint `json:"show_id"`
		SaveCount int  `json:"save_count"`
		IsSaved   bool `json:"is_saved"`
	}
}

// GetSaveCountHandler handles GET /shows/{show_id}/saves
// Uses optional auth: the count is public; is_saved is false for anonymous callers.
func (h *SavedShowHandler) GetSaveCountHandler(ctx context.Context, req *GetSaveCountRequest) (*GetSaveCountResponse, error) {
	requestID := logger.GetRequestID(ctx)

	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	count, err := h.savedShowService.GetSaveCount(uint(showID))
	if err != nil {
		logger.FromContext(ctx).Error("get_save_count_failed",
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get save count (request_id: %s)", requestID),
		)
	}

	resp := &GetSaveCountResponse{}
	resp.Body.ShowID = uint(showID)
	resp.Body.SaveCount = count

	if user := middleware.GetUserFromContext(ctx); user != nil {
		isSaved, err := h.savedShowService.IsShowSaved(user.ID, uint(showID))
		if err != nil {
			// Non-fatal: the public count is the primary payload.
			logger.FromContext(ctx).Warn("get_save_count_is_saved_failed",
				"user_id", user.ID,
				"show_id", showID,
				"error", err.Error(),
			)
		} else {
			resp.Body.IsSaved = isSaved
		}
	}

	return resp, nil
}

// BatchSaveCountsRequest represents the HTTP request for batch save counts.
//
// The `validate:` struct tag is not enforced by huma in this codebase, so the
// cap below is checked explicitly in the handler.
type BatchSaveCountsRequest struct {
	Body struct {
		ShowIDs []int `json:"show_ids" doc:"List of show IDs (max 200)"`
	}
}

// BatchSaveCountsEntry is the per-show payload of the batch save-count response.
type BatchSaveCountsEntry struct {
	SaveCount int  `json:"save_count"`
	IsSaved   bool `json:"is_saved"`
}

// BatchSaveCountsResponse maps show ID (as a string key) to its save data.
type BatchSaveCountsResponse struct {
	Body struct {
		Saves map[string]*BatchSaveCountsEntry `json:"saves"`
	}
}

// BatchSaveCountsHandler handles POST /shows/saves/batch
//
// Uses optional auth. This single call replaces what used to be two round-trips
// from the shows list — one for public counts, one for the viewer's own saved
// state — so is_saved is populated here rather than via /saved-shows/check-batch.
func (h *SavedShowHandler) BatchSaveCountsHandler(ctx context.Context, req *BatchSaveCountsRequest) (*BatchSaveCountsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	resp := &BatchSaveCountsResponse{}
	resp.Body.Saves = make(map[string]*BatchSaveCountsEntry)

	if len(req.Body.ShowIDs) == 0 {
		return resp, nil
	}

	if len(req.Body.ShowIDs) > 200 {
		return nil, huma.Error400BadRequest("Maximum 200 show IDs allowed")
	}

	showIDs := make([]uint, len(req.Body.ShowIDs))
	for i, id := range req.Body.ShowIDs {
		if id <= 0 {
			return nil, huma.Error400BadRequest("Invalid show ID")
		}
		showIDs[i] = uint(id)
	}

	countsMap, err := h.savedShowService.GetBatchSaveCounts(showIDs)
	if err != nil {
		logger.FromContext(ctx).Error("batch_save_counts_failed",
			"count", len(showIDs),
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get save counts (request_id: %s)", requestID),
		)
	}

	for showID, count := range countsMap {
		resp.Body.Saves[strconv.FormatUint(uint64(showID), 10)] = &BatchSaveCountsEntry{SaveCount: count}
	}

	if user := middleware.GetUserFromContext(ctx); user != nil {
		savedMap, err := h.savedShowService.GetSavedShowIDs(user.ID, showIDs)
		if err != nil {
			// Non-fatal: the public counts are the primary payload.
			logger.FromContext(ctx).Warn("batch_save_counts_is_saved_failed",
				"user_id", user.ID,
				"count", len(showIDs),
				"error", err.Error(),
			)
		} else {
			for showID, saved := range savedMap {
				if entry, ok := resp.Body.Saves[strconv.FormatUint(uint64(showID), 10)]; ok {
					entry.IsSaved = saved
				}
			}
		}
	}

	return resp, nil
}

// CheckSavedHandler handles GET /saved-shows/{show_id}/check
func (h *SavedShowHandler) CheckSavedHandler(ctx context.Context, req *CheckSavedRequest) (*CheckSavedResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		logger.FromContext(ctx).Warn("check_saved_invalid_id",
			"show_id_str", req.ShowID,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	// Check if saved
	isSaved, err := h.savedShowService.IsShowSaved(user.ID, uint(showID))
	if err != nil {
		logger.FromContext(ctx).Error("check_saved_failed",
			"user_id", user.ID,
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to check if show is saved (request_id: %s)", requestID),
		)
	}

	return &CheckSavedResponse{
		Body: struct {
			IsSaved bool `json:"is_saved"`
		}{
			IsSaved: isSaved,
		},
	}, nil
}
