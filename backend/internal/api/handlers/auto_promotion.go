package handlers

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// AutoPromotionHandler handles admin auto-promotion endpoints.
type AutoPromotionHandler struct {
	autoPromotionService contracts.AutoPromotionServiceInterface
}

// NewAutoPromotionHandler creates a new auto-promotion handler.
func NewAutoPromotionHandler(
	autoPromotionService contracts.AutoPromotionServiceInterface,
) *AutoPromotionHandler {
	return &AutoPromotionHandler{
		autoPromotionService: autoPromotionService,
	}
}

// EvaluateAllUsersRequest represents the HTTP request for triggering auto-promotion evaluation.
type EvaluateAllUsersRequest struct{}

// EvaluateAllUsersResponse represents the HTTP response for auto-promotion evaluation.
type EvaluateAllUsersResponse struct {
	Body contracts.AutoPromotionResult
}

// EvaluateAllUsersHandler handles POST /admin/auto-promotion/evaluate.
func (h *AutoPromotionHandler) EvaluateAllUsersHandler(ctx context.Context, req *EvaluateAllUsersRequest) (*EvaluateAllUsersResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info("auto_promotion_evaluate_attempt",
		"admin_id", user.ID,
	)

	result, err := h.autoPromotionService.EvaluateAllUsers()
	if err != nil {
		logger.FromContext(ctx).Error("auto_promotion_evaluate_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to evaluate users for auto-promotion (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("auto_promotion_evaluate_success",
		"admin_id", user.ID,
		"promoted", len(result.Promoted),
		"demoted", len(result.Demoted),
		"unchanged", result.Unchanged,
		"errors", result.Errors,
	)

	return &EvaluateAllUsersResponse{Body: *result}, nil
}

// EvaluateUserRequest represents the HTTP request for evaluating a single user.
type EvaluateUserRequest struct {
	UserID uint `path:"user_id" doc:"The user ID to evaluate"`
}

// EvaluateUserResponse represents the HTTP response for single user evaluation.
type EvaluateUserResponse struct {
	Body contracts.UserEvaluationResult
}

// EvaluateUserHandler handles GET /admin/auto-promotion/evaluate/{user_id}.
func (h *AutoPromotionHandler) EvaluateUserHandler(ctx context.Context, req *EvaluateUserRequest) (*EvaluateUserResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info("auto_promotion_evaluate_user_attempt",
		"admin_id", user.ID,
		"target_user_id", req.UserID,
	)

	result, err := h.autoPromotionService.EvaluateUser(req.UserID)
	if err != nil {
		logger.FromContext(ctx).Error("auto_promotion_evaluate_user_failed",
			"error", err.Error(),
			"request_id", requestID,
			"target_user_id", req.UserID,
		)
		if err.Error() == "user not found" {
			return nil, huma.Error404NotFound("User not found")
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to evaluate user (request_id: %s)", requestID),
		)
	}

	return &EvaluateUserResponse{Body: *result}, nil
}
