package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// SavedShowHandler handles saved show HTTP requests
type SavedShowHandler struct {
	savedShowService *services.SavedShowService
}

// NewSavedShowHandler creates a new saved show handler
func NewSavedShowHandler(savedShowService *services.SavedShowService) *SavedShowHandler {
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
	Limit  int `query:"limit" default:"50" doc:"Number of shows per page"`
	Offset int `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetSavedShowsResponse represents the HTTP response for listing saved shows
type GetSavedShowsResponse struct {
	Body struct {
		Shows  []*services.SavedShowResponse `json:"shows"`
		Total  int64                         `json:"total"`
		Limit  int                           `json:"limit"`
		Offset int                           `json:"offset"`
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
	)

	// Get saved shows
	shows, total, err := h.savedShowService.GetUserSavedShows(user.ID, limit, offset)
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
			Shows  []*services.SavedShowResponse `json:"shows"`
			Total  int64                         `json:"total"`
			Limit  int                           `json:"limit"`
			Offset int                           `json:"offset"`
		}{
			Shows:  shows,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}, nil
}

// CheckBatchSavedRequest represents the HTTP request for batch checking saved shows
type CheckBatchSavedRequest struct {
	Body struct {
		ShowIDs []int `json:"show_ids" validate:"required,max=200" doc:"List of show IDs to check (max 200)"`
	}
}

// CheckBatchSavedResponse represents the HTTP response for batch checking saved shows
type CheckBatchSavedResponse struct {
	Body struct {
		SavedShowIDs []int `json:"saved_show_ids"`
	}
}

// CheckBatchSavedHandler handles POST /saved-shows/check-batch
func (h *SavedShowHandler) CheckBatchSavedHandler(ctx context.Context, req *CheckBatchSavedRequest) (*CheckBatchSavedResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if len(req.Body.ShowIDs) == 0 {
		return &CheckBatchSavedResponse{
			Body: struct {
				SavedShowIDs []int `json:"saved_show_ids"`
			}{
				SavedShowIDs: []int{},
			},
		}, nil
	}

	if len(req.Body.ShowIDs) > 200 {
		return nil, huma.Error400BadRequest("Maximum 200 show IDs allowed")
	}

	// Convert to []uint
	showIDs := make([]uint, len(req.Body.ShowIDs))
	for i, id := range req.Body.ShowIDs {
		if id <= 0 {
			return nil, huma.Error400BadRequest("Invalid show ID")
		}
		showIDs[i] = uint(id)
	}

	// Batch check
	savedMap, err := h.savedShowService.GetSavedShowIDs(user.ID, showIDs)
	if err != nil {
		logger.FromContext(ctx).Error("check_batch_saved_failed",
			"user_id", user.ID,
			"count", len(showIDs),
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to check saved shows (request_id: %s)", requestID),
		)
	}

	// Convert map to list of saved IDs
	savedIDs := make([]int, 0, len(savedMap))
	for id, saved := range savedMap {
		if saved {
			savedIDs = append(savedIDs, int(id))
		}
	}

	return &CheckBatchSavedResponse{
		Body: struct {
			SavedShowIDs []int `json:"saved_show_ids"`
		}{
			SavedShowIDs: savedIDs,
		},
	}, nil
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
