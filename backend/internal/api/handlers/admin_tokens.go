package handlers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// AdminTokenHandler handles admin API token management
type AdminTokenHandler struct {
	apiTokenService services.APITokenServiceInterface
}

// NewAdminTokenHandler creates a new admin token handler
func NewAdminTokenHandler(
	apiTokenService services.APITokenServiceInterface,
) *AdminTokenHandler {
	return &AdminTokenHandler{
		apiTokenService: apiTokenService,
	}
}

// CreateAPITokenRequest represents the HTTP request for creating an API token
type CreateAPITokenRequest struct {
	Body struct {
		Description    string `json:"description" doc:"Optional description for the token (e.g., 'Mike laptop discovery')"`
		ExpirationDays int    `json:"expiration_days" doc:"Token expiration in days (default: 90, max: 365)"`
	}
}

// CreateAPITokenResponse represents the HTTP response for creating an API token
type CreateAPITokenResponse struct {
	Body services.APITokenCreateResponse `json:"body"`
}

// CreateAPITokenHandler handles POST /admin/tokens
func (h *AdminTokenHandler) CreateAPITokenHandler(ctx context.Context, req *CreateAPITokenRequest) (*CreateAPITokenResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Validate expiration days
	expirationDays := req.Body.ExpirationDays
	if expirationDays <= 0 {
		expirationDays = 90 // Default
	}
	if expirationDays > 365 {
		return nil, huma.Error400BadRequest("Token expiration cannot exceed 365 days")
	}

	logger.FromContext(ctx).Debug("admin_create_token_attempt",
		"admin_id", user.ID,
		"expiration_days", expirationDays,
	)

	// Create description pointer
	var description *string
	if req.Body.Description != "" {
		description = &req.Body.Description
	}

	// Create the token
	tokenResponse, err := h.apiTokenService.CreateToken(user.ID, description, expirationDays)
	if err != nil {
		logger.FromContext(ctx).Error("admin_create_token_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create token (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("admin_create_token_success",
		"token_id", tokenResponse.ID,
		"admin_id", user.ID,
		"expires_at", tokenResponse.ExpiresAt,
		"request_id", requestID,
	)

	return &CreateAPITokenResponse{Body: *tokenResponse}, nil
}

// ListAPITokensRequest represents the HTTP request for listing API tokens
type ListAPITokensRequest struct{}

// ListAPITokensResponse represents the HTTP response for listing API tokens
type ListAPITokensResponse struct {
	Body struct {
		Tokens []services.APITokenResponse `json:"tokens"`
	}
}

// ListAPITokensHandler handles GET /admin/tokens
func (h *AdminTokenHandler) ListAPITokensHandler(ctx context.Context, req *ListAPITokensRequest) (*ListAPITokensResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Debug("admin_list_tokens_attempt",
		"admin_id", user.ID,
	)

	tokens, err := h.apiTokenService.ListTokens(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("admin_list_tokens_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to list tokens (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_list_tokens_success",
		"count", len(tokens),
		"admin_id", user.ID,
	)

	return &ListAPITokensResponse{
		Body: struct {
			Tokens []services.APITokenResponse `json:"tokens"`
		}{
			Tokens: tokens,
		},
	}, nil
}

// RevokeAPITokenRequest represents the HTTP request for revoking an API token
type RevokeAPITokenRequest struct {
	TokenID string `path:"token_id" validate:"required" doc:"Token ID to revoke"`
}

// RevokeAPITokenResponse represents the HTTP response for revoking an API token
type RevokeAPITokenResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

// RevokeAPITokenHandler handles DELETE /admin/tokens/{token_id}
func (h *AdminTokenHandler) RevokeAPITokenHandler(ctx context.Context, req *RevokeAPITokenRequest) (*RevokeAPITokenResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Parse token ID
	tokenID, err := strconv.ParseUint(req.TokenID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid token ID")
	}

	logger.FromContext(ctx).Debug("admin_revoke_token_attempt",
		"token_id", tokenID,
		"admin_id", user.ID,
	)

	err = h.apiTokenService.RevokeToken(user.ID, uint(tokenID))
	if err != nil {
		logger.FromContext(ctx).Error("admin_revoke_token_failed",
			"token_id", tokenID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error404NotFound("Token not found or already revoked")
	}

	logger.FromContext(ctx).Info("admin_revoke_token_success",
		"token_id", tokenID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &RevokeAPITokenResponse{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: "Token revoked successfully",
		},
	}, nil
}
