package engagement

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
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
		// validateCommentEntityType (shared with CommentService) emits
		// the only typed error on this path; database faults fall
		// through to the generic 500.
		if mapped := shared.MapCommentError(err); mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to subscribe (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "subscribe_comments", req.EntityType, uint(entityID), nil)
		})
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
		if mapped := shared.MapCommentError(err); mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to unsubscribe (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "unsubscribe_comments", req.EntityType, uint(entityID), nil)
		})
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
		if mapped := shared.MapCommentError(err); mapped != nil {
			return nil, mapped
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
// List Subscriptions / Watching (protected, self-scoped)
// ============================================================================

// ListCommentSubscriptionsRequest is the request for the watching list.
type ListCommentSubscriptionsRequest struct {
	Limit  int `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Page size"`
	Offset int `query:"offset" default:"0" minimum:"0" doc:"Pagination offset"`
}

// ListCommentSubscriptionsResponse is the paginated watching list.
type ListCommentSubscriptionsResponse struct {
	Body contracts.WatchingListResponse
}

// ListSubscriptionsHandler handles GET /me/comment-subscriptions.
// Self-scoped: the user ID always comes from the authenticated context.
func (h *CommentSubscriptionHandler) ListSubscriptionsHandler(ctx context.Context, req *ListCommentSubscriptionsRequest) (*ListCommentSubscriptionsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	items, total, err := h.subscriptionService.ListWatching(user.ID, req.Limit, req.Offset)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list subscriptions (request_id: %s)", requestID),
		)
	}

	resp := &ListCommentSubscriptionsResponse{}
	resp.Body.Items = items
	resp.Body.Total = total
	resp.Body.Limit = req.Limit
	resp.Body.Offset = req.Offset
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
		if mapped := shared.MapCommentError(err); mapped != nil {
			return nil, mapped
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
