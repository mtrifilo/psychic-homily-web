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
)

// Alert subscription carried by a follow (PSY-1893). Following an artist or a
// venue subscribes the user to that entity's alerts; these endpoints read and
// adjust that subscription. There is no separate subscribe call: the follow is
// the subscription, so both endpoints 404 when the user does not follow the
// entity.

// FollowAlertsRequest is the request for GET /{entity_type}/{entity_id}/follow/alerts.
type FollowAlertsRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artists, venues)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
}

// FollowAlertsResponse carries a follow's resolved alert subscription.
// Uncached: it is per-user state that a control flips optimistically.
type FollowAlertsResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         contracts.FollowAlertSettings
}

// FollowAlertPreferenceBody is a partial update to one alert type. Every field
// is an optional pointer: omitting a field leaves that axis untouched, and an
// axis the user never set keeps inheriting the account default.
type FollowAlertPreferenceBody struct {
	Enabled *bool   `json:"enabled,omitempty" required:"false" doc:"Whether this alert type is on for this follow"`
	InApp   *bool   `json:"in_app,omitempty" required:"false" doc:"Deliver to the in-app feed"`
	Email   *bool   `json:"email,omitempty" required:"false" doc:"Deliver by email (opt-in)"`
	Scope   *string `json:"scope,omitempty" required:"false" enum:"near_me,everywhere" doc:"Artist show alerts only: near me (home area) or everywhere"`
}

// UpdateFollowAlertsRequest is the request for PATCH /{entity_type}/{entity_id}/follow/alerts.
type UpdateFollowAlertsRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artists, venues)"`
	EntityID   string `path:"entity_id" doc:"Entity ID"`
	Body       struct {
		Shows    *FollowAlertPreferenceBody `json:"shows,omitempty" required:"false" doc:"New-show alerts for this follow"`
		Releases *FollowAlertPreferenceBody `json:"releases,omitempty" required:"false" doc:"New-release alerts (artist follows only)"`
	}
}

// toPreferenceUpdate converts an optional request body preference to the
// service-layer partial update.
func (b *FollowAlertPreferenceBody) toPreferenceUpdate() *contracts.FollowAlertPreferenceUpdate {
	if b == nil {
		return nil
	}
	return &contracts.FollowAlertPreferenceUpdate{
		Enabled: b.Enabled,
		InApp:   b.InApp,
		Email:   b.Email,
		Scope:   b.Scope,
	}
}

// parseFollowAlertTarget resolves the shared path params of both endpoints.
func parseFollowAlertTarget(entityType, entityID string) (string, uint, error) {
	singular, err := parseEntityType(entityType)
	if err != nil {
		return "", 0, huma.Error400BadRequest("Invalid entity type. Must be: artists or venues")
	}
	id, err := strconv.ParseUint(entityID, 10, 32)
	if err != nil {
		return "", 0, huma.Error400BadRequest("Invalid entity ID")
	}
	return singular, uint(id), nil
}

// GetFollowAlertsHandler handles GET /{entity_type}/{entity_id}/follow/alerts.
func (h *FollowHandler) GetFollowAlertsHandler(ctx context.Context, req *FollowAlertsRequest) (*FollowAlertsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	singular, entityID, err := parseFollowAlertTarget(req.EntityType, req.EntityID)
	if err != nil {
		return nil, err
	}

	settings, err := h.followService.GetFollowAlertSettings(user.ID, singular, entityID)
	if err != nil {
		if mapped := shared.MapFollowError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("get_follow_alerts_failed",
			"user_id", user.ID,
			"entity_type", singular,
			"entity_id", entityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get follow alerts (request_id: %s)", requestID),
		)
	}

	if settings == nil {
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to resolve follow alerts (request_id: %s)", requestID),
		)
	}

	return &FollowAlertsResponse{CacheControl: "no-store", Body: *settings}, nil
}

// UpdateFollowAlertsHandler handles PATCH /{entity_type}/{entity_id}/follow/alerts.
func (h *FollowHandler) UpdateFollowAlertsHandler(ctx context.Context, req *UpdateFollowAlertsRequest) (*FollowAlertsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	singular, entityID, err := parseFollowAlertTarget(req.EntityType, req.EntityID)
	if err != nil {
		return nil, err
	}

	update := contracts.FollowAlertUpdate{
		Shows:    req.Body.Shows.toPreferenceUpdate(),
		Releases: req.Body.Releases.toPreferenceUpdate(),
	}

	settings, err := h.followService.SetFollowAlertSettings(user.ID, singular, entityID, update)
	if err != nil {
		if mapped := shared.MapFollowError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("update_follow_alerts_failed",
			"user_id", user.ID,
			"entity_type", singular,
			"entity_id", entityID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update follow alerts (request_id: %s)", requestID),
		)
	}

	if settings == nil {
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to resolve follow alerts (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("update_follow_alerts_success",
		"user_id", user.ID,
		"entity_type", singular,
		"entity_id", entityID,
		"request_id", requestID,
	)

	return &FollowAlertsResponse{CacheControl: "no-store", Body: *settings}, nil
}
