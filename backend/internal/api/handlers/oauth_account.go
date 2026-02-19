package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// OAuthAccountHandler handles OAuth account management HTTP requests
type OAuthAccountHandler struct {
	userService services.UserServiceInterface
}

// NewOAuthAccountHandler creates a new OAuth account handler
func NewOAuthAccountHandler(userService services.UserServiceInterface) *OAuthAccountHandler {
	return &OAuthAccountHandler{
		userService: userService,
	}
}

// OAuthAccountResponse represents a connected OAuth account
type OAuthAccountResponse struct {
	Provider   string  `json:"provider" example:"google" doc:"OAuth provider name"`
	Email      *string `json:"email,omitempty" example:"user@gmail.com" doc:"Email from OAuth provider"`
	Name       *string `json:"name,omitempty" example:"John Doe" doc:"Name from OAuth provider"`
	AvatarURL  *string `json:"avatar_url,omitempty" example:"https://..." doc:"Avatar URL from OAuth provider"`
	ConnectedAt string `json:"connected_at" example:"2024-01-15T10:30:00Z" doc:"When the account was connected"`
}

// GetOAuthAccountsRequest represents the request for listing OAuth accounts
type GetOAuthAccountsRequest struct{}

// GetOAuthAccountsResponse represents the response for listing OAuth accounts
type GetOAuthAccountsResponse struct {
	Body struct {
		Success  bool                   `json:"success"`
		Accounts []OAuthAccountResponse `json:"accounts"`
	}
}

// GetOAuthAccountsHandler handles GET /auth/oauth/accounts
func (h *OAuthAccountHandler) GetOAuthAccountsHandler(ctx context.Context, req *GetOAuthAccountsRequest) (*GetOAuthAccountsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	logger.FromContext(ctx).Debug("get_oauth_accounts_attempt",
		"user_id", user.ID,
		"request_id", requestID,
	)

	// Get OAuth accounts for user
	accounts, err := h.userService.GetOAuthAccounts(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("get_oauth_accounts_failed",
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError("Failed to get OAuth accounts")
	}

	// Convert to response format
	responseAccounts := make([]OAuthAccountResponse, len(accounts))
	for i, acc := range accounts {
		responseAccounts[i] = OAuthAccountResponse{
			Provider:    acc.Provider,
			Email:       acc.ProviderEmail,
			Name:        acc.ProviderName,
			AvatarURL:   acc.ProviderAvatarURL,
			ConnectedAt: acc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	logger.FromContext(ctx).Debug("get_oauth_accounts_success",
		"user_id", user.ID,
		"count", len(accounts),
		"request_id", requestID,
	)

	return &GetOAuthAccountsResponse{
		Body: struct {
			Success  bool                   `json:"success"`
			Accounts []OAuthAccountResponse `json:"accounts"`
		}{
			Success:  true,
			Accounts: responseAccounts,
		},
	}, nil
}

// UnlinkOAuthAccountRequest represents the request for unlinking an OAuth account
type UnlinkOAuthAccountRequest struct {
	Provider string `path:"provider" validate:"required" doc:"OAuth provider to unlink (e.g., google)"`
}

// UnlinkOAuthAccountResponse represents the response for unlinking an OAuth account
type UnlinkOAuthAccountResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// UnlinkOAuthAccountHandler handles DELETE /auth/oauth/accounts/{provider}
func (h *OAuthAccountHandler) UnlinkOAuthAccountHandler(ctx context.Context, req *UnlinkOAuthAccountRequest) (*UnlinkOAuthAccountResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	logger.FromContext(ctx).Debug("unlink_oauth_account_attempt",
		"user_id", user.ID,
		"provider", req.Provider,
		"request_id", requestID,
	)

	// Validate provider
	if req.Provider != "google" && req.Provider != "github" {
		return nil, huma.Error400BadRequest("Invalid provider")
	}

	// Check if user can safely unlink (has other auth methods)
	canUnlink, reason, err := h.userService.CanUnlinkOAuthAccount(user.ID, req.Provider)
	if err != nil {
		logger.FromContext(ctx).Error("unlink_oauth_check_failed",
			"user_id", user.ID,
			"provider", req.Provider,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError("Failed to check unlink eligibility")
	}

	if !canUnlink {
		logger.FromContext(ctx).Warn("unlink_oauth_blocked",
			"user_id", user.ID,
			"provider", req.Provider,
			"reason", reason,
			"request_id", requestID,
		)
		return nil, huma.Error400BadRequest(reason)
	}

	// Unlink the OAuth account
	err = h.userService.UnlinkOAuthAccount(user.ID, req.Provider)
	if err != nil {
		logger.FromContext(ctx).Error("unlink_oauth_failed",
			"user_id", user.ID,
			"provider", req.Provider,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError("Failed to unlink OAuth account")
	}

	logger.FromContext(ctx).Info("unlink_oauth_success",
		"user_id", user.ID,
		"provider", req.Provider,
		"request_id", requestID,
	)

	return &UnlinkOAuthAccountResponse{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "OAuth account unlinked successfully",
		},
	}, nil
}
