package community

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// CollectionLikeHandler handles collection like/unlike API requests. PSY-352.
//
// The toggle is split across POST and DELETE so each verb has a single
// idempotent meaning: POST = like (create or no-op if already liked),
// DELETE = unlike (delete or no-op if not liked). Both endpoints return
// the post-mutation aggregate so the client doesn't need a follow-up read.
type CollectionLikeHandler struct {
	collectionService contracts.CollectionServiceInterface
}

// NewCollectionLikeHandler creates a new handler.
func NewCollectionLikeHandler(collectionService contracts.CollectionServiceInterface) *CollectionLikeHandler {
	return &CollectionLikeHandler{collectionService: collectionService}
}

// LikeCollectionRequest is the request shape for POST /collections/{slug}/like.
type LikeCollectionRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// LikeCollectionResponse contains the post-mutation aggregate and caller state.
type LikeCollectionResponse struct {
	Body contracts.CollectionLikeResponse
}

// LikeCollectionHandler handles POST /collections/{slug}/like.
func (h *CollectionLikeHandler) LikeCollectionHandler(ctx context.Context, req *LikeCollectionRequest) (*LikeCollectionResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	resp, err := h.collectionService.Like(req.Slug, user.ID)
	if err != nil {
		if mapped := mapCollectionError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("like_collection_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to like collection (request_id: %s)", requestID),
		)
	}

	return &LikeCollectionResponse{Body: *resp}, nil
}

// UnlikeCollectionRequest is the request shape for DELETE /collections/{slug}/like.
type UnlikeCollectionRequest struct {
	Slug string `path:"slug" doc:"Collection slug" example:"my-favorite-artists"`
}

// UnlikeCollectionResponse contains the post-mutation aggregate and caller state.
type UnlikeCollectionResponse struct {
	Body contracts.CollectionLikeResponse
}

// UnlikeCollectionHandler handles DELETE /collections/{slug}/like.
func (h *CollectionLikeHandler) UnlikeCollectionHandler(ctx context.Context, req *UnlikeCollectionRequest) (*UnlikeCollectionResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	resp, err := h.collectionService.Unlike(req.Slug, user.ID)
	if err != nil {
		if mapped := mapCollectionError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("unlike_collection_failed",
			"slug", req.Slug,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to unlike collection (request_id: %s)", requestID),
		)
	}

	return &UnlikeCollectionResponse{Body: *resp}, nil
}
