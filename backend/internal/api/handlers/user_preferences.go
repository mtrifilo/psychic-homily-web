package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
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
		Cities []models.FavoriteCity `json:"cities" doc:"List of favorite cities (max 20)"`
	}
}

// SetFavoriteCitiesResponse represents the response after updating favorite cities
type SetFavoriteCitiesResponse struct {
	Body struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Cities  []models.FavoriteCity `json:"cities"`
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
		cities = []models.FavoriteCity{}
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
			Cities  []models.FavoriteCity `json:"cities"`
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
	if !models.IsValidReplyPermission(perm) {
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
