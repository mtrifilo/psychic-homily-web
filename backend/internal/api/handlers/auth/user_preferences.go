package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// UserPreferencesHandler handles user preferences endpoints
type UserPreferencesHandler struct {
	userService contracts.UserServiceInterface
	jwtSecret   string
}

// NewUserPreferencesHandler creates a new user preferences handler
func NewUserPreferencesHandler(userService contracts.UserServiceInterface, jwtSecret string) *UserPreferencesHandler {
	return &UserPreferencesHandler{
		userService: userService,
		jwtSecret:   jwtSecret,
	}
}

// SetFavoriteCitiesRequest represents the request to update favorite cities
type SetFavoriteCitiesRequest struct {
	Body struct {
		Cities []authm.FavoriteCity `json:"cities" doc:"List of favorite cities (max 20)"`
	}
}

// SetFavoriteCitiesResponse represents the response after updating favorite cities
type SetFavoriteCitiesResponse struct {
	Body struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Cities  []authm.FavoriteCity `json:"cities"`
	}
}

// SetFavoriteCitiesHandler handles PUT /auth/preferences/favorite-cities
func (h *UserPreferencesHandler) SetFavoriteCitiesHandler(ctx context.Context, req *SetFavoriteCitiesRequest) (*SetFavoriteCitiesResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	cities := req.Body.Cities
	if cities == nil {
		cities = []authm.FavoriteCity{}
	}

	if err := h.userService.SetFavoriteCities(user.ID, cities); err != nil {
		logger.FromContext(ctx).Error("set_favorite_cities_failed",
			"error", err.Error(),
			"user_id", user.ID,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to save favorite cities: %s", err.Error()),
		)
	}

	logger.FromContext(ctx).Info("set_favorite_cities_success",
		"user_id", user.ID,
		"count", len(cities),
	)

	return &SetFavoriteCitiesResponse{
		Body: struct {
			Success bool                 `json:"success"`
			Message string               `json:"message"`
			Cities  []authm.FavoriteCity `json:"cities"`
		}{
			Success: true,
			Message: "Favorite cities updated",
			Cities:  cities,
		},
	}, nil
}

// SetShowRemindersRequest represents the request to toggle show reminders
type SetShowRemindersRequest struct {
	Body struct {
		Enabled bool `json:"enabled" doc:"Enable or disable show reminders"`
	}
}

// SetShowRemindersResponse represents the response after toggling show reminders
type SetShowRemindersResponse struct {
	Body struct {
		Success       bool `json:"success"`
		ShowReminders bool `json:"show_reminders"`
	}
}

// SetShowRemindersHandler handles PATCH /auth/preferences/show-reminders
func (h *UserPreferencesHandler) SetShowRemindersHandler(ctx context.Context, req *SetShowRemindersRequest) (*SetShowRemindersResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if err := h.userService.SetShowReminders(user.ID, req.Body.Enabled); err != nil {
		logger.FromContext(ctx).Error("set_show_reminders_failed",
			"error", err.Error(),
			"user_id", user.ID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to update show reminders: %s", err.Error()),
		)
	}

	logger.FromContext(ctx).Info("set_show_reminders_success",
		"user_id", user.ID,
		"enabled", req.Body.Enabled,
	)

	return &SetShowRemindersResponse{
		Body: struct {
			Success       bool `json:"success"`
			ShowReminders bool `json:"show_reminders"`
		}{
			Success:       true,
			ShowReminders: req.Body.Enabled,
		},
	}, nil
}

// UnsubscribeShowRemindersRequest represents the unsubscribe request (public, no auth)
type UnsubscribeShowRemindersRequest struct {
	Body struct {
		UID uint   `json:"uid" doc:"User ID"`
		Sig string `json:"sig" doc:"HMAC signature"`
	}
}

// UnsubscribeShowRemindersResponse represents the unsubscribe response
type UnsubscribeShowRemindersResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnsubscribeShowRemindersHandler handles POST /auth/unsubscribe/show-reminders (public, no auth)
func (h *UserPreferencesHandler) UnsubscribeShowRemindersHandler(ctx context.Context, req *UnsubscribeShowRemindersRequest) (*UnsubscribeShowRemindersResponse, error) {
	if !engagement.VerifyUnsubscribeSignature(req.Body.UID, req.Body.Sig, h.jwtSecret) {
		return nil, huma.Error403Forbidden("Invalid unsubscribe link")
	}

	if err := h.userService.SetShowReminders(req.Body.UID, false); err != nil {
		logger.FromContext(ctx).Error("unsubscribe_show_reminders_failed",
			"error", err.Error(),
			"user_id", req.Body.UID,
		)
		return nil, huma.Error500InternalServerError("Failed to unsubscribe")
	}

	logger.FromContext(ctx).Info("unsubscribe_show_reminders_success",
		"user_id", req.Body.UID,
	)

	return &UnsubscribeShowRemindersResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Show reminders disabled",
		},
	}, nil
}

// ===========================================================================
// PSY-296: default reply permission
// ===========================================================================

// SetDefaultReplyPermissionRequest updates the user's default reply permission.
type SetDefaultReplyPermissionRequest struct {
	Body struct {
		Permission string `json:"permission" doc:"Default reply permission: anyone, followers, or author_only" example:"anyone"`
	}
}

// SetDefaultReplyPermissionResponse returns the new default value.
type SetDefaultReplyPermissionResponse struct {
	Body struct {
		Success                bool   `json:"success"`
		DefaultReplyPermission string `json:"default_reply_permission"`
	}
}

// SetDefaultReplyPermissionHandler handles PATCH /auth/preferences/default-reply-permission.
func (h *UserPreferencesHandler) SetDefaultReplyPermissionHandler(ctx context.Context, req *SetDefaultReplyPermissionRequest) (*SetDefaultReplyPermissionResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	perm := req.Body.Permission
	if !engagementm.IsValidReplyPermission(perm) {
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("invalid reply_permission: %q (want anyone, followers, or author_only)", perm),
		)
	}

	if err := h.userService.SetDefaultReplyPermission(user.ID, perm); err != nil {
		logger.FromContext(ctx).Error("set_default_reply_permission_failed",
			"error", err.Error(),
			"user_id", user.ID,
		)
		if strings.Contains(err.Error(), "invalid reply_permission") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to update default reply permission: %s", err.Error()),
		)
	}

	logger.FromContext(ctx).Info("set_default_reply_permission_success",
		"user_id", user.ID,
		"permission", perm,
	)

	return &SetDefaultReplyPermissionResponse{
		Body: struct {
			Success                bool   `json:"success"`
			DefaultReplyPermission string `json:"default_reply_permission"`
		}{
			Success:                true,
			DefaultReplyPermission: perm,
		},
	}, nil
}

// ──────────────────────────────────────────────
// PSY-289: comment + mention notification preferences
// ──────────────────────────────────────────────

// SetCommentNotificationsRequest toggles the two PSY-289 email preferences.
// Both fields are pointers so a caller can update one without touching the
// other; Huma's default-required rule applies to non-nil fields only when
// explicitly marked optional — these use `required:"false"` to opt out.
type SetCommentNotificationsRequest struct {
	Body struct {
		NotifyOnCommentSubscription *bool `json:"notify_on_comment_subscription,omitempty" required:"false" doc:"Receive emails when new comments appear on entities you're subscribed to"`
		NotifyOnMention             *bool `json:"notify_on_mention,omitempty" required:"false" doc:"Receive emails when someone @mentions you in a comment"`
	}
}

// SetCommentNotificationsResponse reports the resulting preference state.
type SetCommentNotificationsResponse struct {
	Body struct {
		Success                     bool `json:"success"`
		NotifyOnCommentSubscription bool `json:"notify_on_comment_subscription"`
		NotifyOnMention             bool `json:"notify_on_mention"`
	}
}

// SetCommentNotificationsHandler handles PATCH /auth/preferences/comment-notifications.
// Sends one update per non-nil field, then reloads the current state.
func (h *UserPreferencesHandler) SetCommentNotificationsHandler(ctx context.Context, req *SetCommentNotificationsRequest) (*SetCommentNotificationsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if req.Body.NotifyOnCommentSubscription == nil && req.Body.NotifyOnMention == nil {
		return nil, huma.Error400BadRequest("No preferences provided")
	}

	if req.Body.NotifyOnCommentSubscription != nil {
		if err := h.userService.SetNotifyOnCommentSubscription(user.ID, *req.Body.NotifyOnCommentSubscription); err != nil {
			logger.FromContext(ctx).Error("set_notify_on_comment_subscription_failed",
				"error", err.Error(),
				"user_id", user.ID,
			)
			return nil, huma.Error500InternalServerError(
				fmt.Sprintf("Failed to update preference: %s", err.Error()),
			)
		}
	}
	if req.Body.NotifyOnMention != nil {
		if err := h.userService.SetNotifyOnMention(user.ID, *req.Body.NotifyOnMention); err != nil {
			logger.FromContext(ctx).Error("set_notify_on_mention_failed",
				"error", err.Error(),
				"user_id", user.ID,
			)
			return nil, huma.Error500InternalServerError(
				fmt.Sprintf("Failed to update preference: %s", err.Error()),
			)
		}
	}

	// Reload current state from DB (authoritative).
	refreshed, err := h.userService.GetUserByID(user.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to reload user")
	}
	resp := &SetCommentNotificationsResponse{}
	resp.Body.Success = true
	if refreshed.Preferences != nil {
		resp.Body.NotifyOnCommentSubscription = refreshed.Preferences.NotifyOnCommentSubscription
		resp.Body.NotifyOnMention = refreshed.Preferences.NotifyOnMention
	} else {
		// No prefs row — return the defaults we requested.
		resp.Body.NotifyOnCommentSubscription = shared.DerefOr(req.Body.NotifyOnCommentSubscription, true)
		resp.Body.NotifyOnMention = shared.DerefOr(req.Body.NotifyOnMention, true)
	}
	return resp, nil
}

// UnsubscribeCommentSubscriptionRequest is the public one-click unsubscribe
// payload sent from the email "Unsubscribe" link. All fields required.
type UnsubscribeCommentSubscriptionRequest struct {
	Body struct {
		UID        uint   `json:"uid" doc:"User ID"`
		EntityType string `json:"entity_type" doc:"Entity type the user was subscribed to"`
		EntityID   uint   `json:"entity_id" doc:"Entity ID the user was subscribed to"`
		Sig        string `json:"sig" doc:"HMAC signature"`
	}
}

// UnsubscribeCommentSubscriptionResponse is the confirmation shape.
type UnsubscribeCommentSubscriptionResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnsubscribeCommentSubscriptionHandler handles POST /unsubscribe/comment-subscription.
// Verifies HMAC then flips the user's notify_on_comment_subscription preference
// to false. Public endpoint — no auth required.
//
// Note: the unsubscribe is account-wide (flips the preference flag), not
// per-entity. That matches the ticket ("flip the relevant preference off")
// and What.cd behavior. We still bind the signature to (user, entity_type,
// entity_id) so a link leaked from one email can't be mutated to target
// other users or resources.
func (h *UserPreferencesHandler) UnsubscribeCommentSubscriptionHandler(ctx context.Context, req *UnsubscribeCommentSubscriptionRequest) (*UnsubscribeCommentSubscriptionResponse, error) {
	if !engagement.VerifyCommentSubscriptionUnsubscribeSignature(
		req.Body.UID, req.Body.EntityType, req.Body.EntityID, req.Body.Sig, h.jwtSecret,
	) {
		return nil, huma.Error403Forbidden("Invalid unsubscribe link")
	}

	if err := h.userService.SetNotifyOnCommentSubscription(req.Body.UID, false); err != nil {
		logger.FromContext(ctx).Error("unsubscribe_comment_subscription_failed",
			"error", err.Error(),
			"user_id", req.Body.UID,
		)
		return nil, huma.Error500InternalServerError("Failed to unsubscribe")
	}

	logger.FromContext(ctx).Info("unsubscribe_comment_subscription_success",
		"user_id", req.Body.UID,
	)

	return &UnsubscribeCommentSubscriptionResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Comment notifications disabled",
		},
	}, nil
}

// UnsubscribeMentionRequest is the public one-click unsubscribe payload for
// mention emails.
type UnsubscribeMentionRequest struct {
	Body struct {
		UID uint   `json:"uid" doc:"User ID"`
		Sig string `json:"sig" doc:"HMAC signature"`
	}
}

// UnsubscribeMentionResponse is the confirmation shape.
type UnsubscribeMentionResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnsubscribeMentionHandler handles POST /unsubscribe/mention. Flips the
// user's notify_on_mention preference to false. Public — HMAC-signed.
func (h *UserPreferencesHandler) UnsubscribeMentionHandler(ctx context.Context, req *UnsubscribeMentionRequest) (*UnsubscribeMentionResponse, error) {
	if !engagement.VerifyMentionUnsubscribeSignature(req.Body.UID, req.Body.Sig, h.jwtSecret) {
		return nil, huma.Error403Forbidden("Invalid unsubscribe link")
	}

	if err := h.userService.SetNotifyOnMention(req.Body.UID, false); err != nil {
		logger.FromContext(ctx).Error("unsubscribe_mention_failed",
			"error", err.Error(),
			"user_id", req.Body.UID,
		)
		return nil, huma.Error500InternalServerError("Failed to unsubscribe")
	}

	logger.FromContext(ctx).Info("unsubscribe_mention_success",
		"user_id", req.Body.UID,
	)

	return &UnsubscribeMentionResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Mention notifications disabled",
		},
	}, nil
}

// ──────────────────────────────────────────────
// PSY-350: collection digest preference (weekly cadence; opt-IN by default)
// ──────────────────────────────────────────────

// SetCollectionDigestRequest toggles the collection digest preference.
type SetCollectionDigestRequest struct {
	Body struct {
		Enabled bool `json:"enabled" doc:"Enable or disable the weekly collection-subscription digest email"`
	}
}

// SetCollectionDigestResponse reports the resulting preference state.
type SetCollectionDigestResponse struct {
	Body struct {
		Success                  bool `json:"success"`
		NotifyOnCollectionDigest bool `json:"notify_on_collection_digest"`
	}
}

// SetCollectionDigestHandler handles PATCH /auth/preferences/collection-digest.
func (h *UserPreferencesHandler) SetCollectionDigestHandler(ctx context.Context, req *SetCollectionDigestRequest) (*SetCollectionDigestResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if err := h.userService.SetNotifyOnCollectionDigest(user.ID, req.Body.Enabled); err != nil {
		logger.FromContext(ctx).Error("set_notify_on_collection_digest_failed",
			"error", err.Error(),
			"user_id", user.ID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update preference: %s", err.Error()),
		)
	}

	logger.FromContext(ctx).Info("set_notify_on_collection_digest_success",
		"user_id", user.ID,
		"enabled", req.Body.Enabled,
	)

	resp := &SetCollectionDigestResponse{}
	resp.Body.Success = true
	resp.Body.NotifyOnCollectionDigest = req.Body.Enabled
	return resp, nil
}

// PSY-350: collection digest unsubscribe handler is registered as a chi
// route (not Huma) so the same path can serve:
//   - GET (manual link click from the email body): renders an HTML
//     confirmation page so the user has visible feedback that the
//     unsubscribe succeeded (and a path back to settings to re-enable).
//   - POST (RFC 8058 / RFC 2369 one-click from Gmail/Yahoo's bulk-sender
//     unsubscribe button): returns 200 with a small JSON body.
// See UnsubscribeCollectionDigestPageHandler in this package and the
// route registration in routes.go.
