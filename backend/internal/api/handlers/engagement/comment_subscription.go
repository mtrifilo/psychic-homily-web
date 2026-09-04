package engagement

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// CommentSubscriptionHandler handles comment subscription API requests.
type CommentSubscriptionHandler struct {
	subscriptionService contracts.CommentSubscriptionServiceInterface
	auditLogService     contracts.AuditLogServiceInterface
	// showVisibility gates the SUBSCRIBE, STATUS and MARK-READ routes on the rule
	// the named entity's own detail route enforces. Which entity types have such
	// a rule is services/shared/entity_visibility.go's registry, and a type with
	// no entry there is refused. Not every route on this handler:
	//   - UnsubscribeHandler is deliberately ungated; its own doc says why.
	//   - ListSubscriptionsHandler is gated in the SERVICE instead, by the viewer
	//     it hands ListWatching, because the watching list is addressed by the
	//     caller rather than by a show id.
	// Required, but note what a nil checker actually does: it answers false for
	// every show, so subscribe and mark-read 404 while STATUS returns a bare
	// {subscribed:false, unread_count:0} with a 200. That last one is a
	// fail-QUIET — the toggle would read "not subscribed" site-wide with no log
	// line — so a construction bug here is not self-announcing on every route.
	// See shared.EntitySubResourceVisible, which is what the call sites use.
	showVisibility contracts.ShowVisibilityInterface
}

// NewCommentSubscriptionHandler creates a new CommentSubscriptionHandler.
func NewCommentSubscriptionHandler(
	subscriptionService contracts.CommentSubscriptionServiceInterface,
	auditLogService contracts.AuditLogServiceInterface,
	showVisibility contracts.ShowVisibilityInterface,
) *CommentSubscriptionHandler {
	return &CommentSubscriptionHandler{
		subscriptionService: subscriptionService,
		auditLogService:     auditLogService,
		showVisibility:      showVisibility,
	}
}

// refuseAsMissingEntity is the answer every gated route on this handler gives:
// the service's own entity-not-found error, so a gated entity and an id nobody
// has ever used produce one response.
//
// The gate answers false for a MISSING show or collection as well as for a
// gated one, which is what collapses the two cases into a single answer instead
// of leaving the pair as a two-valued oracle over a dense id space.
func refuseAsMissingEntity(entityType string, entityID uint) error {
	if mapped := shared.MapCommentError(
		apperrors.ErrCommentEntityNotFound(entityType, entityID),
	); mapped != nil {
		return mapped
	}
	return huma.Error404NotFound("Entity not found")
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

	// A subscription is a standing request for an entity's activity, so it is
	// governed by whether the caller may see that entity at all. Ungated, one
	// POST turns a guessed id into a monitored feed: the watching list publishes
	// the entity's title, slug and URL, and every new comment mails the caller an
	// excerpt.
	if !shared.EntitySubResourceVisible(h.showVisibility, req.EntityType, uint(entityID), middleware.GetShowViewerFromContext(ctx)) {
		return nil, refuseAsMissingEntity(req.EntityType, uint(entityID))
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
		servicesshared.SubmitAuditWrite("audit_log", func() {
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
//
// DELIBERATELY UNGATED, and it is the only route on this handler that is. It
// deletes the caller's own row and answers the same whether or not one was
// there: no body, no rows-affected count, so there is nothing for a gate to
// withhold and no oracle for one to close. Gating it would add a failure mode
// and remove the last direct path to a row the watching list already hides.
// Same reasoning as GetUserCollectionsContainingEntity.
//
// It does NOT rest on "otherwise the mail keeps coming": the fan-out gate stops
// that. The row simply persists, invisible, and this is what removes it.
func (h *CommentSubscriptionHandler) UnsubscribeHandler(ctx context.Context, req *UnsubscribeRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	removed, err := h.subscriptionService.Unsubscribe(user.ID, req.EntityType, uint(entityID))
	if err != nil {
		if mapped := shared.MapCommentError(err); mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to unsubscribe (request_id: %s)", requestID),
		)
	}

	// NOTHING HAPPENED, NOTHING IS RECORDED. This route is ungated and accepts
	// any (entity_type, entity_id) pair the caller names, so writing an audit row
	// per call lets a caller mint rows against ids they have no relationship
	// with. The response is identical either way, so the count never reaches the
	// caller.
	if removed == 0 {
		return nil, nil
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.SubmitAuditWrite("audit_log", func() {
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

	// NOT SUBSCRIBED, not a refusal. This route reports a live unread count,
	// which is a running signal of activity on the entity, and an id nobody has
	// ever used already answers `{subscribed:false, unread_count:0}`. So a gated
	// entity answers that too: a 404 here would say the id is real, and the true
	// answer would say how busy an entity the caller cannot see is.
	if !shared.EntitySubResourceVisible(h.showVisibility, req.EntityType, uint(entityID), middleware.GetShowViewerFromContext(ctx)) {
		return &SubscriptionStatusResponse{}, nil
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
//
// Self-scoped: the viewer always comes from the authenticated context, and it
// carries both the identity the rows are owned by and the tier they are read at
// (PSY-1983). The subscribe gate stops new subscriptions to a show the caller
// cannot see; this is what keeps a show that is taken private AFTER the
// subscription from publishing itself through a row that was legitimate when it
// was made.
func (h *CommentSubscriptionHandler) ListSubscriptionsHandler(ctx context.Context, req *ListCommentSubscriptionsRequest) (*ListCommentSubscriptionsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	items, total, err := h.subscriptionService.ListWatching(middleware.GetShowViewerFromContext(ctx), req.Limit, req.Offset)
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

	// The read gate's twin. MarkRead reads the entity's newest comment id and
	// stores it, so leaving it open lets a caller who is refused the listing and
	// the status still move a pointer over a gated entity's discussion and, once
	// the entity is published again, read off how far it had advanced.
	if !shared.EntitySubResourceVisible(h.showVisibility, req.EntityType, uint(entityID), middleware.GetShowViewerFromContext(ctx)) {
		return nil, refuseAsMissingEntity(req.EntityType, uint(entityID))
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
