package auth

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AdvancementHandler serves the self-scoped tier-advancement progress endpoint.
type AdvancementHandler struct {
	autoPromotionService contracts.AutoPromotionServiceInterface
}

// NewAdvancementHandler creates an AdvancementHandler.
func NewAdvancementHandler(
	autoPromotionService contracts.AutoPromotionServiceInterface,
) *AdvancementHandler {
	return &AdvancementHandler{autoPromotionService: autoPromotionService}
}

// GetAdvancementResponse is the HTTP response for GET /auth/profile/advancement.
type GetAdvancementResponse struct {
	Body *contracts.AdvancementProgress
}

// GetAdvancementHandler handles GET /auth/profile/advancement.
// Auth required; always evaluates the caller (no user_id param — no admin leak).
func (h *AdvancementHandler) GetAdvancementHandler(ctx context.Context, _ *struct{}) (*GetAdvancementResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	progress, err := h.autoPromotionService.GetAdvancementProgress(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_advancement_progress_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		if mapped := shared.MapAutoPromotionError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get advancement progress (request_id: %s)", requestID),
		)
	}

	return &GetAdvancementResponse{Body: progress}, nil
}
