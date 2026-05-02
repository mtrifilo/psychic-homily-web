package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// LeaderboardHandler handles public leaderboard endpoints.
type LeaderboardHandler struct {
	leaderboardService contracts.LeaderboardServiceInterface
}

// NewLeaderboardHandler creates a new leaderboard handler.
func NewLeaderboardHandler(
	leaderboardService contracts.LeaderboardServiceInterface,
) *LeaderboardHandler {
	return &LeaderboardHandler{
		leaderboardService: leaderboardService,
	}
}

// --- GetLeaderboard ---

// GetLeaderboardRequest is the Huma request for GET /community/leaderboard
type GetLeaderboardRequest struct {
	Dimension string `query:"dimension" required:"false" doc:"Leaderboard dimension: overall, shows, venues, tags, edits, requests (default: overall)"`
	Period    string `query:"period" required:"false" doc:"Time period: all_time, month, week (default: all_time)"`
	Limit     int    `query:"limit" required:"false" doc:"Number of results (default 25, max 100)"`
}

// LeaderboardEntryResponse is a single entry in the leaderboard response.
type LeaderboardEntryResponse struct {
	Rank      int     `json:"rank"`
	UserID    uint    `json:"user_id"`
	Username  string  `json:"username"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	UserTier  string  `json:"user_tier"`
	Count     int64   `json:"count"`
}

// GetLeaderboardResponse is the Huma response for GET /community/leaderboard
type GetLeaderboardResponse struct {
	Body struct {
		Entries   []LeaderboardEntryResponse `json:"entries"`
		Dimension string                     `json:"dimension"`
		Period    string                     `json:"period"`
		UserRank  *int                       `json:"user_rank,omitempty"`
	}
}

// GetLeaderboardHandler handles GET /community/leaderboard
func (h *LeaderboardHandler) GetLeaderboardHandler(ctx context.Context, req *GetLeaderboardRequest) (*GetLeaderboardResponse, error) {
	dimension := req.Dimension
	if dimension == "" {
		dimension = "overall"
	}

	period := req.Period
	if period == "" {
		period = "all_time"
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}

	entries, err := h.leaderboardService.GetLeaderboard(dimension, period, limit)
	if err != nil {
		if err.Error() == "invalid dimension: "+dimension {
			return nil, huma.Error400BadRequest("Invalid dimension: " + dimension)
		}
		if err.Error() == "invalid period: "+period {
			return nil, huma.Error400BadRequest("Invalid period: " + period)
		}
		logger.FromContext(ctx).Error("leaderboard_failed", "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to get leaderboard")
	}

	resp := &GetLeaderboardResponse{}
	resp.Body.Dimension = dimension
	resp.Body.Period = period
	resp.Body.Entries = make([]LeaderboardEntryResponse, len(entries))
	for i, e := range entries {
		resp.Body.Entries[i] = LeaderboardEntryResponse{
			Rank:      e.Rank,
			UserID:    e.UserID,
			Username:  e.Username,
			AvatarURL: e.AvatarURL,
			UserTier:  e.UserTier,
			Count:     e.Count,
		}
	}

	// If the user is authenticated, compute their rank
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		rank, err := h.leaderboardService.GetUserRank(user.ID, dimension, period)
		if err != nil {
			// Non-fatal — log and continue without user rank
			logger.FromContext(ctx).Warn("leaderboard_user_rank_failed",
				"user_id", user.ID,
				"error", err.Error(),
			)
		} else {
			resp.Body.UserRank = rank
		}
	}

	return resp, nil
}
