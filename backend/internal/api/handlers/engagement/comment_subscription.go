package engagement

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// CommentSubscriptionHandler handles comment subscription API requests.
type CommentSubscriptionHandler struct {
	subscriptionService contracts.CommentSubscriptionServiceInterface
	auditLogService     contracts.AuditLogServiceInterface
}

// NewCommentSubscriptionHandler creates a new CommentSubscriptionHandler.
func NewCommentSubscriptionHandler(
	subscriptionService contracts.CommentSubscriptionServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
) *CommentSubscriptionHandler {
	return &CommentSubscriptionHandler{
		subscriptionService: subscriptionService,
		auditLogService:     auditLogService,
	}
}

// ============================================================================
// Subscribe (protected)
// ============================================================================

// SubscribeRequest represents the request for subscribing to an entity's comments.
type SubscribeRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)" example:"show"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
}

// SubscribeResponse represents the response after subscribing.
type SubscribeResponse struct {
	Body struct {
		Success bool `json:"success" doc:"Whether the subscription was created"`
	}
}

// SubscribeHandler handles POST /entities/{entity_type}/{entity_id}/subscribe
func (h *CommentSubscriptionHandler) SubscribeHandler(ctx context.Context, req *SubscribeRequest) (*SubscribeResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	err = h.subscriptionService.Subscribe(user.ID, req.EntityType, uint(entityID))
	if err != nil {
		if strings.Contains(err.Error(), "unsupported entity type") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to subscribe (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "subscribe_comments", req.EntityType, uint(entityID), nil)
		}()
	}

	resp := &SubscribeResponse{}
	resp.Body.Success = true
	return resp, nil
}

// ============================================================================
// Unsubscribe (protected)
// ============================================================================

// UnsubscribeRequest represents the request for unsubscribing from an entity's comments.
type UnsubscribeRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)" example:"show"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
}

// UnsubscribeHandler handles DELETE /entities/{entity_type}/{entity_id}/subscribe
func (h *CommentSubscriptionHandler) UnsubscribeHandler(ctx context.Context, req *UnsubscribeRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	err = h.subscriptionService.Unsubscribe(user.ID, req.EntityType, uint(entityID))
	if err != nil {
		if strings.Contains(err.Error(), "unsupported entity type") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to unsubscribe (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "unsubscribe_comments", req.EntityType, uint(entityID), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Subscription Status (protected)
// ============================================================================

// SubscriptionStatusRequest represents the request for checking subscription status.
type SubscriptionStatusRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)" example:"show"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
}

// SubscriptionStatusResponse represents the subscription status and unread count.
type SubscriptionStatusResponse struct {
	Body contracts.SubscriptionStatusResponse
}

// SubscriptionStatusHandler handles GET /entities/{entity_type}/{entity_id}/subscribe/status
func (h *CommentSubscriptionHandler) SubscriptionStatusHandler(ctx context.Context, req *SubscriptionStatusRequest) (*SubscriptionStatusResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	subscribed, err := h.subscriptionService.IsSubscribed(user.ID, req.EntityType, uint(entityID))
	if err != nil {
		if strings.Contains(err.Error(), "unsupported entity type") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("Failed to check subscription status")
	}

	var unreadCount int
	if subscribed {
		unreadCount, _ = h.subscriptionService.GetUnreadCount(user.ID, req.EntityType, uint(entityID))
	}

	resp := &SubscriptionStatusResponse{}
	resp.Body.Subscribed = subscribed
	resp.Body.UnreadCount = unreadCount
	return resp, nil
}

// ============================================================================
// Mark Read (protected)
// ============================================================================

// MarkReadRequest represents the request for marking comments as read.
type MarkReadRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, venue, show, release, label, festival, collection)" example:"show"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
}

// MarkReadResponse represents the response after marking comments as read.
type MarkReadResponse struct {
	Body struct {
		Success bool `json:"success" doc:"Whether the mark-read operation succeeded"`
	}
}

// MarkReadHandler handles POST /entities/{entity_type}/{entity_id}/mark-read
func (h *CommentSubscriptionHandler) MarkReadHandler(ctx context.Context, req *MarkReadRequest) (*MarkReadResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	err = h.subscriptionService.MarkRead(user.ID, req.EntityType, uint(entityID))
	if err != nil {
		if strings.Contains(err.Error(), "unsupported entity type") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to mark as read (request_id: %s)", requestID),
		)
	}

	resp := &MarkReadResponse{}
	resp.Body.Success = true
	return resp, nil
}
