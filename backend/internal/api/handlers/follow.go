package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// FollowHandler handles follow/unfollow HTTP requests.
type FollowHandler struct {
	followService contracts.FollowServiceInterface
}

// NewFollowHandler creates a new follow handler.
func NewFollowHandler(followService contracts.FollowServiceInterface) *FollowHandler {
	return &FollowHandler{followService: followService}
}

// validFollowEntityTypes maps plural URL path segments to singular bookmark entity types.
var validFollowEntityTypes = map[string]string{
	"artists":   "artist",
	"venues":    "venue",
	"labels":    "label",
	"festivals": "festival",
}

// parseEntityType converts a plural URL entity type to singular, returning an error if invalid.
func parseEntityType(entityType string) (string, error) {
	singular, ok := validFollowEntityTypes[entityType]
	if !ok {
		return "", fmt.Errorf("invalid entity type: %s", entityType)
	}
	return singular, nil
}

// ──────────────────────────────────────────────
// Request / Response types
// ──────────────────────────────────────────────

// FollowRequest is the request for POST /{entity_type}/{entity_id}/follow
type FollowRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artists, venues, labels, festivals)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
}

// FollowResponse is the response for POST /{entity_type}/{entity_id}/follow
type FollowResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnfollowRequest is the request for DELETE /{entity_type}/{entity_id}/follow
type UnfollowRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artists, venues, labels, festivals)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
}

// UnfollowResponse is the response for DELETE /{entity_type}/{entity_id}/follow
type UnfollowResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// GetFollowersRequest is the request for GET /{entity_type}/{entity_id}/followers
type GetFollowersRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artists, venues, labels, festivals)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
}

// GetFollowersResponse is the response for GET /{entity_type}/{entity_id}/followers
type GetFollowersResponse struct {
	Body struct {
		EntityType    string `json:"entity_type"`
		EntityID      uint   `json:"entity_id"`
		FollowerCount int64  `json:"follower_count"`
		IsFollowing   bool   `json:"is_following"`
	}
}

// BatchFollowRequest is the request for POST /follows/batch
type BatchFollowRequest struct {
	Body struct {
		EntityType string `json:"entity_type" doc:"Entity type (artist, venue, label, festival)"`
		EntityIDs  []int  `json:"entity_ids" validate:"required,max=100" doc:"List of entity IDs (max 100)"`
	}
}

// BatchFollowEntry represents a single entity's follow data in a batch response
type BatchFollowEntry struct {
	FollowerCount int64 `json:"follower_count"`
	IsFollowing   bool  `json:"is_following"`
}

// BatchFollowResponse is the response for POST /follows/batch
type BatchFollowResponse struct {
	Body struct {
		Follows map[string]*BatchFollowEntry `json:"follows"`
	}
}

// GetMyFollowingRequest is the request for GET /me/following
type GetMyFollowingRequest struct {
	Type   string `query:"type" default:"all" doc:"Entity type filter: artist, venue, label, festival, or all"`
	Limit  int    `query:"limit" default:"20" doc:"Number of items per page"`
	Offset int    `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetMyFollowingResponse is the response for GET /me/following
type GetMyFollowingResponse struct {
	Body struct {
		Following []*contracts.FollowingEntityResponse `json:"following"`
		Total     int64                               `json:"total"`
		Limit     int                                 `json:"limit"`
		Offset    int                                 `json:"offset"`
	}
}

// GetFollowersListRequest is the request for GET /{entity_type}/{entity_id}/followers/list
type GetFollowersListRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artists, venues, labels, festivals)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
	Limit      int    `query:"limit" default:"20" doc:"Number of followers per page"`
	Offset     int    `query:"offset" default:"0" doc:"Offset for pagination"`
}

// GetFollowersListResponse is the response for GET /{entity_type}/{entity_id}/followers/list
type GetFollowersListResponse struct {
	Body struct {
		Followers []*contracts.FollowerResponse `json:"followers"`
		Total     int64                        `json:"total"`
		Limit     int                          `json:"limit"`
		Offset    int                          `json:"offset"`
	}
}

// ──────────────────────────────────────────────
// Handlers
// ──────────────────────────────────────────────

// FollowHandler handles POST /{entity_type}/{entity_id}/follow
func (h *FollowHandler) FollowEntityHandler(ctx context.Context, req *FollowRequest) (*FollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	singular, err := parseEntityType(req.EntityType)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity type. Must be: artists, venues, labels, or festivals")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	logger.FromContext(ctx).Debug("follow_attempt",
		"user_id", user.ID,
		"entity_type", singular,
		"entity_id", entityID,
	)

	if err := h.followService.Follow(user.ID, singular, uint(entityID)); err != nil {
		logger.FromContext(ctx).Error("follow_failed",
			"user_id", user.ID,
			"entity_type", singular,
			"entity_id", entityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to follow (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("follow_success",
		"user_id", user.ID,
		"entity_type", singular,
		"entity_id", entityID,
		"request_id", requestID,
	)

	return &FollowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: fmt.Sprintf("Now following %s", singular),
		},
	}, nil
}

// UnfollowEntityHandler handles DELETE /{entity_type}/{entity_id}/follow
func (h *FollowHandler) UnfollowEntityHandler(ctx context.Context, req *UnfollowRequest) (*UnfollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	singular, err := parseEntityType(req.EntityType)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity type. Must be: artists, venues, labels, or festivals")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	logger.FromContext(ctx).Debug("unfollow_attempt",
		"user_id", user.ID,
		"entity_type", singular,
		"entity_id", entityID,
	)

	if err := h.followService.Unfollow(user.ID, singular, uint(entityID)); err != nil {
		logger.FromContext(ctx).Error("unfollow_failed",
			"user_id", user.ID,
			"entity_type", singular,
			"entity_id", entityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to unfollow (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("unfollow_success",
		"user_id", user.ID,
		"entity_type", singular,
		"entity_id", entityID,
		"request_id", requestID,
	)

	return &UnfollowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Unfollowed",
		},
	}, nil
}

// GetFollowersHandler handles GET /{entity_type}/{entity_id}/followers
// Uses optional auth: if authenticated, includes whether the user is following.
func (h *FollowHandler) GetFollowersHandler(ctx context.Context, req *GetFollowersRequest) (*GetFollowersResponse, error) {
	requestID := logger.GetRequestID(ctx)

	singular, err := parseEntityType(req.EntityType)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity type. Must be: artists, venues, labels, or festivals")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	count, err := h.followService.GetFollowerCount(singular, uint(entityID))
	if err != nil {
		logger.FromContext(ctx).Error("get_follower_count_failed",
			"entity_type", singular,
			"entity_id", entityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get follower count (request_id: %s)", requestID),
		)
	}

	resp := &GetFollowersResponse{}
	resp.Body.EntityType = singular
	resp.Body.EntityID = uint(entityID)
	resp.Body.FollowerCount = count

	// If user is authenticated, check if they follow this entity
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		isFollowing, err := h.followService.IsFollowing(user.ID, singular, uint(entityID))
		if err != nil {
			logger.FromContext(ctx).Warn("get_is_following_failed",
				"user_id", user.ID,
				"entity_type", singular,
				"entity_id", entityID,
				"error", err.Error(),
			)
			// Non-fatal — still return count
		} else {
			resp.Body.IsFollowing = isFollowing
		}
	}

	return resp, nil
}

// BatchFollowHandler handles POST /follows/batch
// Uses optional auth: if authenticated, includes whether the user follows each entity.
func (h *FollowHandler) BatchFollowHandler(ctx context.Context, req *BatchFollowRequest) (*BatchFollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	if len(req.Body.EntityIDs) == 0 {
		resp := &BatchFollowResponse{}
		resp.Body.Follows = make(map[string]*BatchFollowEntry)
		return resp, nil
	}

	if len(req.Body.EntityIDs) > 100 {
		return nil, huma.Error400BadRequest("Maximum 100 entity IDs allowed")
	}

	// Validate entity type — accept both "artist" and "artists" forms
	singular := req.Body.EntityType
	// If plural form was sent, look it up in the mapping
	if mapped, ok := validFollowEntityTypes[singular]; ok {
		singular = mapped
	} else {
		// Verify the singular form is valid by checking values
		valid := false
		for _, v := range validFollowEntityTypes {
			if v == singular {
				valid = true
				break
			}
		}
		if !valid {
			return nil, huma.Error400BadRequest("Invalid entity type. Must be: artist, venue, label, or festival")
		}
	}

	// Convert to []uint and validate
	entityIDs := make([]uint, len(req.Body.EntityIDs))
	for i, id := range req.Body.EntityIDs {
		if id <= 0 {
			return nil, huma.Error400BadRequest("Invalid entity ID")
		}
		entityIDs[i] = uint(id)
	}

	// Get counts
	countsMap, err := h.followService.GetBatchFollowerCounts(singular, entityIDs)
	if err != nil {
		logger.FromContext(ctx).Error("batch_follow_counts_failed",
			"entity_type", singular,
			"count", len(entityIDs),
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get follow counts (request_id: %s)", requestID),
		)
	}

	// Build response
	follows := make(map[string]*BatchFollowEntry, len(entityIDs))
	for id, count := range countsMap {
		follows[strconv.FormatUint(uint64(id), 10)] = &BatchFollowEntry{
			FollowerCount: count,
		}
	}

	// If user is authenticated, include their follow statuses
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		userFollowing, err := h.followService.GetBatchUserFollowing(user.ID, singular, entityIDs)
		if err != nil {
			logger.FromContext(ctx).Warn("batch_user_following_failed",
				"user_id", user.ID,
				"entity_type", singular,
				"count", len(entityIDs),
				"error", err.Error(),
			)
			// Non-fatal — still return counts
		} else {
			for entityID, isFollowing := range userFollowing {
				key := strconv.FormatUint(uint64(entityID), 10)
				if entry, ok := follows[key]; ok {
					entry.IsFollowing = isFollowing
				}
			}
		}
	}

	resp := &BatchFollowResponse{}
	resp.Body.Follows = follows
	return resp, nil
}

// GetMyFollowingHandler handles GET /me/following
func (h *FollowHandler) GetMyFollowingHandler(ctx context.Context, req *GetMyFollowingRequest) (*GetMyFollowingResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Validate type filter
	entityTypeFilter := req.Type
	if entityTypeFilter != "artist" && entityTypeFilter != "venue" && entityTypeFilter != "label" && entityTypeFilter != "festival" && entityTypeFilter != "all" && entityTypeFilter != "" {
		return nil, huma.Error400BadRequest("Type must be 'artist', 'venue', 'label', 'festival', or 'all'")
	}
	if entityTypeFilter == "all" || entityTypeFilter == "" {
		entityTypeFilter = ""
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

	logger.FromContext(ctx).Debug("get_my_following_attempt",
		"user_id", user.ID,
		"type", entityTypeFilter,
		"limit", limit,
		"offset", offset,
	)

	following, total, err := h.followService.GetUserFollowing(user.ID, entityTypeFilter, limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("get_my_following_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get following list (request_id: %s)", requestID),
		)
	}

	return &GetMyFollowingResponse{
		Body: struct {
			Following []*contracts.FollowingEntityResponse `json:"following"`
			Total     int64                               `json:"total"`
			Limit     int                                 `json:"limit"`
			Offset    int                                 `json:"offset"`
		}{
			Following: following,
			Total:     total,
			Limit:     limit,
			Offset:    offset,
		},
	}, nil
}

// GetFollowersListHandler handles GET /{entity_type}/{entity_id}/followers/list
func (h *FollowHandler) GetFollowersListHandler(ctx context.Context, req *GetFollowersListRequest) (*GetFollowersListResponse, error) {
	requestID := logger.GetRequestID(ctx)

	singular, err := parseEntityType(req.EntityType)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity type. Must be: artists, venues, labels, or festivals")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
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

	followers, total, err := h.followService.GetFollowers(singular, uint(entityID), limit, offset)
	if err != nil {
		logger.FromContext(ctx).Error("get_followers_list_failed",
			"entity_type", singular,
			"entity_id", entityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get followers list (request_id: %s)", requestID),
		)
	}

	return &GetFollowersListResponse{
		Body: struct {
			Followers []*contracts.FollowerResponse `json:"followers"`
			Total     int64                        `json:"total"`
			Limit     int                          `json:"limit"`
			Offset    int                          `json:"offset"`
		}{
			Followers: followers,
			Total:     total,
			Limit:     limit,
			Offset:    offset,
		},
	}, nil
}
