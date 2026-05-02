package admin

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminStatsHandler handles admin dashboard stats and activity feed
type AdminStatsHandler struct {
	adminStatsService contracts.AdminStatsServiceInterface
}

// NewAdminStatsHandler creates a new admin stats handler
func NewAdminStatsHandler(
	adminStatsService contracts.AdminStatsServiceInterface,
) *AdminStatsHandler {
	return &AdminStatsHandler{
		adminStatsService: adminStatsService,
	}
}

// GetAdminStatsRequest represents the HTTP request for getting admin dashboard stats
type GetAdminStatsRequest struct{}

// GetAdminStatsResponse represents the HTTP response for admin dashboard stats
type GetAdminStatsResponse struct {
	Body contracts.AdminDashboardStats
}

// GetAdminStatsHandler handles GET /admin/stats
func (h *AdminStatsHandler) GetAdminStatsHandler(ctx context.Context, req *GetAdminStatsRequest) (*GetAdminStatsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Debug("admin_stats_attempt",
		"admin_id", user.ID,
	)

	stats, err := h.adminStatsService.GetDashboardStats()
	if err != nil {
		logger.FromContext(ctx).Error("admin_stats_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get dashboard stats (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_stats_success",
		"admin_id", user.ID,
	)

	return &GetAdminStatsResponse{Body: *stats}, nil
}

// GetActivityFeedRequest represents the HTTP request for getting admin activity feed
type GetActivityFeedRequest struct{}

// GetActivityFeedResponse represents the HTTP response for admin activity feed
type GetActivityFeedResponse struct {
	Body contracts.ActivityFeedResponse
}

// GetActivityFeedHandler handles GET /admin/activity
func (h *AdminStatsHandler) GetActivityFeedHandler(ctx context.Context, req *GetActivityFeedRequest) (*GetActivityFeedResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Debug("admin_activity_feed_attempt",
		"admin_id", user.ID,
	)

	feed, err := h.adminStatsService.GetRecentActivity()
	if err != nil {
		logger.FromContext(ctx).Error("admin_activity_feed_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get activity feed (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_activity_feed_success",
		"admin_id", user.ID,
		"event_count", len(feed.Events),
	)

	return &GetActivityFeedResponse{Body: *feed}, nil
}
