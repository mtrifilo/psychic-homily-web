package engagement

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// User follows (PSY-1496). Users are addressed by USERNAME, not id — the FE
// never holds a numeric user id for board-C Follow — so they get dedicated
// routes instead of joining the generic /{entity_type}/{entity_id}/follow map.
// Once the username resolves to a row id, everything below delegates to the
// same FollowService with FollowEntityUser ("user") that PSY-296 reply gating
// already uses.
//
// No public follower list ships here (PSY-1059 deferred). No activity_events /
// notify / feed work.

const userEntityType = "user"

// UserFollowHandler handles follow/unfollow for users-by-username.
type UserFollowHandler struct {
	followService contracts.FollowServiceInterface
	userService   contracts.UserServiceInterface
}

// NewUserFollowHandler creates a user follow handler.
func NewUserFollowHandler(
	followService contracts.FollowServiceInterface,
	userService contracts.UserServiceInterface,
) *UserFollowHandler {
	return &UserFollowHandler{followService: followService, userService: userService}
}

// UserFollowRequest is the request for POST /users/{username}/follow.
type UserFollowRequest struct {
	Username string `path:"username" doc:"Username of the user to follow"`
}

// UserUnfollowRequest is the request for DELETE /users/{username}/follow.
type UserUnfollowRequest struct {
	Username string `path:"username" doc:"Username of the user to unfollow"`
}

// UserFollowersRequest is the request for GET /users/{username}/followers.
type UserFollowersRequest struct {
	Username string `path:"username" doc:"Username of the user"`
}

// UserFollowersResponse mirrors the entity followers summary, keyed by username.
type UserFollowersResponse struct {
	Body struct {
		Username      string `json:"username"`
		FollowerCount int64  `json:"follower_count"`
		IsFollowing   bool   `json:"is_following"`
	}
}

// resolveFollowTarget looks up the follow target by username and applies the
// same master profile-visibility gate as public profile reads: a private
// profile (or missing user) resolves to 404 so hidden profiles are
// indistinguishable from nonexistent ones. Owners may resolve their own
// private profile (needed for GET …/followers on a private account).
func (h *UserFollowHandler) resolveFollowTarget(ctx context.Context, username string) (*authm.User, error) {
	requestID := logger.GetRequestID(ctx)

	target, err := h.userService.GetUserByUsername(username)
	if err != nil {
		logger.FromContext(ctx).Error("user_follow_lookup_failed",
			"username", username, "error", err.Error(), "request_id", requestID)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to look up user (request_id: %s)", requestID),
		)
	}
	if target == nil {
		return nil, huma.Error404NotFound("User not found")
	}

	viewer := middleware.GetUserFromContext(ctx)
	isOwner := viewer != nil && viewer.ID == target.ID
	if target.ProfileVisibility == "private" && !isOwner {
		return nil, huma.Error404NotFound("User not found")
	}
	return target, nil
}

// UserFollowHandler handles POST /users/{username}/follow.
func (h *UserFollowHandler) UserFollowHandler(ctx context.Context, req *UserFollowRequest) (*FollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	target, err := h.resolveFollowTarget(ctx, req.Username)
	if err != nil {
		return nil, err
	}

	if user.ID == target.ID {
		return nil, huma.Error422UnprocessableEntity("Cannot follow yourself")
	}

	if err := h.followService.Follow(user.ID, userEntityType, target.ID); err != nil {
		logger.FromContext(ctx).Error("user_follow_failed",
			"user_id", user.ID, "username", req.Username, "target_id", target.ID,
			"error", err.Error(), "request_id", requestID)
		if mapped := shared.MapFollowError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to follow (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("user_follow_success",
		"user_id", user.ID, "username", req.Username, "target_id", target.ID, "request_id", requestID)

	return &FollowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{Success: true, Message: "Now following user"},
	}, nil
}

// UserUnfollowHandler handles DELETE /users/{username}/follow.
func (h *UserFollowHandler) UserUnfollowHandler(ctx context.Context, req *UserUnfollowRequest) (*UnfollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	target, err := h.resolveFollowTarget(ctx, req.Username)
	if err != nil {
		return nil, err
	}

	if err := h.followService.Unfollow(user.ID, userEntityType, target.ID); err != nil {
		logger.FromContext(ctx).Error("user_unfollow_failed",
			"user_id", user.ID, "username", req.Username, "target_id", target.ID,
			"error", err.Error(), "request_id", requestID)
		if mapped := shared.MapFollowError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to unfollow (request_id: %s)", requestID),
		)
	}

	return &UnfollowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{Success: true, Message: "Unfollowed"},
	}, nil
}

// UserFollowersHandler handles GET /users/{username}/followers (optional auth).
// Returns follower_count + is_following; no public follower list (PSY-1059).
func (h *UserFollowHandler) UserFollowersHandler(ctx context.Context, req *UserFollowersRequest) (*UserFollowersResponse, error) {
	requestID := logger.GetRequestID(ctx)

	target, err := h.resolveFollowTarget(ctx, req.Username)
	if err != nil {
		return nil, err
	}

	resp := &UserFollowersResponse{}
	resp.Body.Username = req.Username

	count, err := h.followService.GetFollowerCount(userEntityType, target.ID)
	if err != nil {
		logger.FromContext(ctx).Error("user_followers_count_failed",
			"username", req.Username, "target_id", target.ID,
			"error", err.Error(), "request_id", requestID)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get follower count (request_id: %s)", requestID),
		)
	}
	resp.Body.FollowerCount = count

	if viewer := middleware.GetUserFromContext(ctx); viewer != nil {
		following, err := h.followService.IsFollowing(viewer.ID, userEntityType, target.ID)
		if err == nil {
			resp.Body.IsFollowing = following
		}
	}

	return resp, nil
}
