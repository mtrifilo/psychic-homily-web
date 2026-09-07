package auth

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// OAuthAccountHandler handles OAuth account management HTTP requests
type OAuthAccountHandler struct {
	userService contracts.UserServiceInterface
}

// NewOAuthAccountHandler creates a new OAuth account handler
func NewOAuthAccountHandler(userService contracts.UserServiceInterface) *OAuthAccountHandler {
	return &OAuthAccountHandler{
		userService: userService,
	}
}

// OAuthAccountResponse represents a connected OAuth account
type OAuthAccountResponse struct {
	Provider    string  `json:"provider" example:"google" doc:"OAuth provider name"`
	Email       *string `json:"email,omitempty" example:"user@gmail.com" doc:"Email from OAuth provider"`
	Name        *string `json:"name,omitempty" example:"John Doe" doc:"Name from OAuth provider"`
	AvatarURL   *string `json:"avatar_url,omitempty" example:"https://..." doc:"Avatar URL from OAuth provider"`
	ConnectedAt string  `json:"connected_at" example:"2024-01-15T10:30:00Z" doc:"When the account was connected"`
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
			Provider:  acc.Provider,
			Email:     acc.ProviderEmail,
			Name:      acc.ProviderName,
			AvatarURL: acc.ProviderAvatarURL,
			// PSY-616 (sibling of PSY-604): must convert to UTC before
			// formatting — the literal "Z" in the layout asserts the value
			// is UTC but Format does not convert. A local time.Time would
			// otherwise be stamped with "Z" while still carrying the local
			// clock reading, drifting any downstream timestamp render by
			// the local UTC offset (e.g. 7h on Phoenix MST).
			ConnectedAt: acc.CreatedAt.UTC().Format(time.RFC3339),
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

// StartOAuthLinkRequest represents the request for minting a link token.
type StartOAuthLinkRequest struct{}

// StartOAuthLinkResponse carries the one-time token Settings puts on the
// /auth/link/{provider} URL it navigates to.
type StartOAuthLinkResponse struct {
	Body struct {
		Success bool   `json:"success"`
		Token   string `json:"token" doc:"One-time token for the /auth/link/{provider} start URL"`
	}
}

// StartOAuthLinkHandler handles POST /auth/oauth/link-token.
//
// It exists so /auth/link/{provider} can tell a start that came from this
// application's own Settings page from one an attacker's page navigated the
// user into. That route is a cookie-authenticated GET and the auth cookie is
// SameSite=Lax, which browsers DO send on a cross-site top-level navigation,
// so the route cannot make that distinction on its own.
//
// This endpoint can only be called same-origin with credentials, and CORS
// stops another origin reading the response, so the token is something only
// our own page can hold.
func (h *OAuthAccountHandler) StartOAuthLinkHandler(ctx context.Context, req *StartOAuthLinkRequest) (*StartOAuthLinkResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	token, err := mintOAuthLinkToken(user.ID)
	if err != nil {
		logger.FromContext(ctx).Error("oauth_link_token_mint_failed",
			"user_id", user.ID,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to start account connection")
	}

	resp := &StartOAuthLinkResponse{}
	resp.Body.Success = true
	resp.Body.Token = token
	return resp, nil
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
	if !isGothOAuthProvider(req.Provider) {
		return nil, huma.Error422UnprocessableEntity("Invalid provider")
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
		return nil, huma.Error422UnprocessableEntity(reason)
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
