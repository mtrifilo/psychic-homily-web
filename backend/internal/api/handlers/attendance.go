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

// AttendanceHandler handles show attendance (going/interested) HTTP requests.
type AttendanceHandler struct {
	attendanceService services.AttendanceServiceInterface
}

// NewAttendanceHandler creates a new attendance handler.
func NewAttendanceHandler(attendanceService services.AttendanceServiceInterface) *AttendanceHandler {
	return &AttendanceHandler{attendanceService: attendanceService}
}

// ──────────────────────────────────────────────
// Request / Response types
// ──────────────────────────────────────────────

// SetAttendanceRequest is the request for POST /shows/{show_id}/attendance
type SetAttendanceRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
	Body   struct {
		Status string `json:"status" doc:"Attendance status: going, interested, or empty string to clear"`
	}
}

// SetAttendanceResponse is the response for POST /shows/{show_id}/attendance
type SetAttendanceResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// RemoveAttendanceRequest is the request for DELETE /shows/{show_id}/attendance
type RemoveAttendanceRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// RemoveAttendanceResponse is the response for DELETE /shows/{show_id}/attendance
type RemoveAttendanceResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// GetAttendanceRequest is the request for GET /shows/{show_id}/attendance
type GetAttendanceRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// GetAttendanceResponse is the response for GET /shows/{show_id}/attendance
type GetAttendanceResponse struct {
	Body struct {
		ShowID          uint   `json:"show_id"`
		GoingCount      int    `json:"going_count"`
		InterestedCount int    `json:"interested_count"`
		UserStatus      string `json:"user_status,omitempty"`
	}
}

// BatchAttendanceRequest is the request for POST /shows/attendance/batch
type BatchAttendanceRequest struct {
	Body struct {
		ShowIDs []int `json:"show_ids" validate:"required,max=100" doc:"List of show IDs (max 100)"`
	}
}

// BatchAttendanceEntry represents a single show's attendance data in a batch response
type BatchAttendanceEntry struct {
	GoingCount      int    `json:"going_count"`
	InterestedCount int    `json:"interested_count"`
	UserStatus      string `json:"user_status,omitempty"`
}

// BatchAttendanceResponse is the response for POST /shows/attendance/batch
type BatchAttendanceResponse struct {
	Body struct {
		Attendance map[string]*BatchAttendanceEntry `json:"attendance"`
	}
}

// GetMyShowsRequest is the request for GET /attendance/my-shows
type GetMyShowsRequest struct {
	Status string `query:"status" default:"all" doc:"Filter: going, interested, or all"`
	Limit  int    `query:"limit" default:"20" doc:"Number of shows per page"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetMyShowsResponse is the response for GET /attendance/my-shows
type GetMyShowsResponse struct {
	Body struct {
		Shows  []*services.AttendingShowResponse `json:"shows"`
		Total  int64                             `json:"total"`
		Limit  int                               `json:"limit"`
		Offset int                               `json:"offset"`
	}
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────

// SetAttendanceHandler handles POST /shows/{show_id}/attendance
func (h *AttendanceHandler) SetAttendanceHandler(ctx context.Context, req *SetAttendanceRequest) (*SetAttendanceResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	status := req.Body.Status
	if status != "going" && status != "interested" && status != "" {
		return nil, huma.Error400BadRequest("Status must be 'going', 'interested', or empty string")
	}

	logger.FromContext(ctx).Debug("set_attendance_attempt",
		"user_id", user.ID,
		"show_id", showID,
		"status", status,
	)

	if err := h.attendanceService.SetAttendance(user.ID, uint(showID), status); err != nil {
		logger.FromContext(ctx).Error("set_attendance_failed",
			"user_id", user.ID,
			"show_id", showID,
			"status", status,
			"error", err.Error(),
			"request_id", requestID,
		)
		if err.Error() == "show not found" {
			return nil, huma.Error404NotFound("Show not found")
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to set attendance (request_id: %s)", requestID),
		)
	}

	msg := "Attendance cleared"
	if status != "" {
		msg = fmt.Sprintf("Marked as %s", status)
	}

	logger.FromContext(ctx).Info("set_attendance_success",
		"user_id", user.ID,
		"show_id", showID,
		"status", status,
		"request_id", requestID,
	)

	return &SetAttendanceResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: msg,
		},
	}, nil
}

// RemoveAttendanceHandler handles DELETE /shows/{show_id}/attendance
func (h *AttendanceHandler) RemoveAttendanceHandler(ctx context.Context, req *RemoveAttendanceRequest) (*RemoveAttendanceResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	logger.FromContext(ctx).Debug("remove_attendance_attempt",
		"user_id", user.ID,
		"show_id", showID,
	)

	if err := h.attendanceService.RemoveAttendance(user.ID, uint(showID)); err != nil {
		logger.FromContext(ctx).Error("remove_attendance_failed",
			"user_id", user.ID,
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to remove attendance (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("remove_attendance_success",
		"user_id", user.ID,
		"show_id", showID,
		"request_id", requestID,
	)

	return &RemoveAttendanceResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Attendance removed",
		},
	}, nil
}

// GetAttendanceHandler handles GET /shows/{show_id}/attendance
// Uses optional auth: if authenticated, includes user's status.
func (h *AttendanceHandler) GetAttendanceHandler(ctx context.Context, req *GetAttendanceRequest) (*GetAttendanceResponse, error) {
	requestID := logger.GetRequestID(ctx)

	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	counts, err := h.attendanceService.GetAttendanceCounts(uint(showID))
	if err != nil {
		logger.FromContext(ctx).Error("get_attendance_counts_failed",
			"show_id", showID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get attendance counts (request_id: %s)", requestID),
		)
	}

	resp := &GetAttendanceResponse{}
	resp.Body.ShowID = uint(showID)
	resp.Body.GoingCount = counts.GoingCount
	resp.Body.InterestedCount = counts.InterestedCount

	// If user is authenticated, include their status
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		userStatus, err := h.attendanceService.GetUserAttendance(user.ID, uint(showID))
		if err != nil {
			logger.FromContext(ctx).Warn("get_user_attendance_failed",
				"user_id", user.ID,
				"show_id", showID,
				"error", err.Error(),
			)
			// Non-fatal — still return counts
		} else {
			resp.Body.UserStatus = userStatus
		}
	}

	return resp, nil
}

// BatchAttendanceHandler handles POST /shows/attendance/batch
// Uses optional auth: if authenticated, includes user's status for each show.
func (h *AttendanceHandler) BatchAttendanceHandler(ctx context.Context, req *BatchAttendanceRequest) (*BatchAttendanceResponse, error) {
	requestID := logger.GetRequestID(ctx)

	if len(req.Body.ShowIDs) == 0 {
		resp := &BatchAttendanceResponse{}
		resp.Body.Attendance = make(map[string]*BatchAttendanceEntry)
		return resp, nil
	}

	if len(req.Body.ShowIDs) > 100 {
		return nil, huma.Error400BadRequest("Maximum 100 show IDs allowed")
	}

	// Convert to []uint and validate
	showIDs := make([]uint, len(req.Body.ShowIDs))
	for i, id := range req.Body.ShowIDs {
		if id <= 0 {
			return nil, huma.Error400BadRequest("Invalid show ID")
		}
		showIDs[i] = uint(id)
	}

	// Get counts
	countsMap, err := h.attendanceService.GetBatchAttendanceCounts(showIDs)
	if err != nil {
		logger.FromContext(ctx).Error("batch_attendance_counts_failed",
			"count", len(showIDs),
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get attendance counts (request_id: %s)", requestID),
		)
	}

	// Build response
	attendance := make(map[string]*BatchAttendanceEntry, len(showIDs))
	for id, counts := range countsMap {
		attendance[strconv.FormatUint(uint64(id), 10)] = &BatchAttendanceEntry{
			GoingCount:      counts.GoingCount,
			InterestedCount: counts.InterestedCount,
		}
	}

	// If user is authenticated, include their statuses
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		userStatuses, err := h.attendanceService.GetBatchUserAttendance(user.ID, showIDs)
		if err != nil {
			logger.FromContext(ctx).Warn("batch_user_attendance_failed",
				"user_id", user.ID,
				"count", len(showIDs),
				"error", err.Error(),
			)
			// Non-fatal — still return counts
		} else {
			for showID, status := range userStatuses {
				key := strconv.FormatUint(uint64(showID), 10)
				if entry, ok := attendance[key]; ok {
					entry.UserStatus = status
				}
			}
		}
	}

	resp := &BatchAttendanceResponse{}
	resp.Body.Attendance = attendance
	return resp, nil
}

// GetMyShowsHandler handles GET /attendance/my-shows
func (h *AttendanceHandler) GetMyShowsHandler(ctx context.Context, req *GetMyShowsRequest) (*GetMyShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Validate status filter
	status := req.Status
	if status != "going" && status != "interested" && status != "all" && status != "" {
		return nil, huma.Error400BadRequest("Status must be 'going', 'interested', or 'all'")
	}
	if status == "" {
		status = "all"
	}

	// Clamp pagination
	limit := req.Limit
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	logger.FromContext(ctx).Debug("get_my_shows_attempt",
		"user_id", user.ID,
		"status", status,
		"limit", limit,
		"offset", offset,
	)

	shows, total, err := h.attendanceService.GetUserAttendingShows(user.ID, status, limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("get_my_shows_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get attending shows (request_id: %s)", requestID),
		)
	}

	return &GetMyShowsResponse{
		Body: struct {
			Shows  []*services.AttendingShowResponse `json:"shows"`
			Total  int64                             `json:"total"`
			Limit  int                               `json:"limit"`
			Offset int                               `json:"offset"`
		}{
			Shows:  shows,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}, nil
}
