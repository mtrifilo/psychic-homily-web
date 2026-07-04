package engagement

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// Scene follows (PSY-1339). Scenes are addressed by SLUG, not id — the
// registry row materializes lazily on first follow — so scenes get dedicated
// routes instead of joining validFollowEntityTypes: the generic
// /{entity_type}/{entity_id}/follow shape would collide with
// /scenes/{slug}/follow (chi resolves the static "scenes" segment first) and
// the FE never holds a scene id anyway. Once the slug resolves to a row id,
// everything below delegates to the same FollowService as every other entity.

// SceneFollowHandler handles follow/unfollow for scenes-by-slug.
type SceneFollowHandler struct {
	followService contracts.FollowServiceInterface
	scenes        contracts.SceneServiceInterface
}

// NewSceneFollowHandler creates a scene follow handler.
func NewSceneFollowHandler(
	followService contracts.FollowServiceInterface,
	scenes contracts.SceneServiceInterface,
) *SceneFollowHandler {
	return &SceneFollowHandler{followService: followService, scenes: scenes}
}

const sceneEntityType = "scene"

// SceneFollowRequest is the request for POST /scenes/{slug}/follow. The body
// is optional; NotifyMode configures the new-show notification mode
// (PSY-1341) — re-POSTing with a different mode updates it (follow is
// idempotent).
type SceneFollowRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)"`
	// POINTER body: huma marks a non-pointer Body as a REQUIRED request body,
	// which would 400 the existing body-less FollowButton POST (review-caught;
	// handler tests bypass huma so no test sees it).
	Body *SceneFollowBody `required:"false"`
}

// SceneFollowBody is the optional POST /scenes/{slug}/follow body (PSY-1341).
type SceneFollowBody struct {
	NotifyMode string `json:"notify_mode,omitempty" enum:"all,followed_bands_only" doc:"New-show notification mode (default all)"`
}

// SceneUnfollowRequest is the request for DELETE /scenes/{slug}/follow.
type SceneUnfollowRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)"`
}

// SceneFollowersRequest is the request for GET /scenes/{slug}/followers.
type SceneFollowersRequest struct {
	Slug string `path:"slug" doc:"Scene slug (e.g. phoenix-az)"`
}

// SceneFollowersResponse mirrors GetFollowersResponse, keyed by slug.
// NotifyMode is the requesting user's mode when following (PSY-1341).
type SceneFollowersResponse struct {
	Body struct {
		Slug          string `json:"slug"`
		FollowerCount int64  `json:"follower_count"`
		IsFollowing   bool   `json:"is_following"`
		NotifyMode    string `json:"notify_mode,omitempty"`
	}
}

// mapSceneSlugError converts ParseSceneSlug's SceneError (not-found → 404)
// via the shared mapper; anything else surfaces as a 500 with the request id.
func mapSceneSlugError(err error, requestID string) error {
	if mapped := shared.MapSceneError(err); mapped != nil {
		return mapped
	}
	return huma.Error500InternalServerError(
		fmt.Sprintf("Failed to resolve scene (request_id: %s)", requestID),
	)
}

// SceneFollowHandler handles POST /scenes/{slug}/follow.
func (h *SceneFollowHandler) SceneFollowHandler(ctx context.Context, req *SceneFollowRequest) (*FollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	sceneID, err := h.scenes.GetOrCreateSceneID(req.Slug)
	if err != nil {
		logger.FromContext(ctx).Error("scene_follow_resolve_failed",
			"user_id", user.ID, "slug", req.Slug, "error", err.Error(), "request_id", requestID)
		return nil, mapSceneSlugError(err, requestID)
	}

	if err := h.followService.Follow(user.ID, sceneEntityType, sceneID); err != nil {
		logger.FromContext(ctx).Error("scene_follow_failed",
			"user_id", user.ID, "slug", req.Slug, "scene_id", sceneID,
			"error", err.Error(), "request_id", requestID)
		if mapped := shared.MapFollowError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to follow (request_id: %s)", requestID),
		)
	}

	if req.Body != nil && req.Body.NotifyMode != "" {
		if err := h.followService.SetSceneNotifyMode(user.ID, sceneID, req.Body.NotifyMode); err != nil {
			logger.FromContext(ctx).Error("scene_follow_mode_failed",
				"user_id", user.ID, "slug", req.Slug, "scene_id", sceneID,
				"mode", req.Body.NotifyMode, "error", err.Error(), "request_id", requestID)
			if mapped := shared.MapFollowError(err); mapped != nil {
				return nil, mapped
			}
			return nil, huma.Error500InternalServerError(
				fmt.Sprintf("Failed to set notify mode (request_id: %s)", requestID),
			)
		}
	}

	logger.FromContext(ctx).Info("scene_follow_success",
		"user_id", user.ID, "slug", req.Slug, "scene_id", sceneID, "request_id", requestID)

	return &FollowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{Success: true, Message: "Now following scene"},
	}, nil
}

// SceneUnfollowHandler handles DELETE /scenes/{slug}/follow.
func (h *SceneFollowHandler) SceneUnfollowHandler(ctx context.Context, req *SceneUnfollowRequest) (*UnfollowResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Lookup, not get-or-create: unfollowing a scene nobody follows must not
	// materialize a row. Absent row → nothing to unfollow → idempotent success
	// (Unfollow is already idempotent for missing bookmarks).
	sceneID, ok, err := h.scenes.LookupSceneID(req.Slug)
	if err != nil {
		logger.FromContext(ctx).Error("scene_unfollow_resolve_failed",
			"user_id", user.ID, "slug", req.Slug, "error", err.Error(), "request_id", requestID)
		return nil, mapSceneSlugError(err, requestID)
	}
	if ok {
		if err := h.followService.Unfollow(user.ID, sceneEntityType, sceneID); err != nil {
			logger.FromContext(ctx).Error("scene_unfollow_failed",
				"user_id", user.ID, "slug", req.Slug, "scene_id", sceneID,
				"error", err.Error(), "request_id", requestID)
			if mapped := shared.MapFollowError(err); mapped != nil {
				return nil, mapped
			}
			return nil, huma.Error500InternalServerError(
				fmt.Sprintf("Failed to unfollow (request_id: %s)", requestID),
			)
		}
	}

	return &UnfollowResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{Success: true, Message: "Unfollowed"},
	}, nil
}

// SceneFollowersHandler handles GET /scenes/{slug}/followers (optional auth).
func (h *SceneFollowHandler) SceneFollowersHandler(ctx context.Context, req *SceneFollowersRequest) (*SceneFollowersResponse, error) {
	requestID := logger.GetRequestID(ctx)

	resp := &SceneFollowersResponse{}
	resp.Body.Slug = req.Slug

	sceneID, ok, err := h.scenes.LookupSceneID(req.Slug)
	if err != nil {
		logger.FromContext(ctx).Error("scene_followers_resolve_failed",
			"slug", req.Slug, "error", err.Error(), "request_id", requestID)
		return nil, mapSceneSlugError(err, requestID)
	}
	if !ok {
		// No registry row yet — a real scene with zero follows.
		return resp, nil
	}

	count, err := h.followService.GetFollowerCount(sceneEntityType, sceneID)
	if err != nil {
		logger.FromContext(ctx).Error("scene_followers_count_failed",
			"slug", req.Slug, "scene_id", sceneID, "error", err.Error(), "request_id", requestID)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get follower count (request_id: %s)", requestID),
		)
	}
	resp.Body.FollowerCount = count

	if user := middleware.GetUserFromContext(ctx); user != nil {
		following, err := h.followService.IsFollowing(user.ID, sceneEntityType, sceneID)
		if err == nil {
			resp.Body.IsFollowing = following
		}
		if following {
			if mode, err := h.followService.SceneNotifyMode(user.ID, sceneID); err == nil {
				resp.Body.NotifyMode = mode
			}
		}
	}

	return resp, nil
}
