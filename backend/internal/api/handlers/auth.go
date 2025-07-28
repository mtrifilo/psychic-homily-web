package handlers

import (
	"context"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/api/middleware"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	authService *services.AuthService
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// OAuthLoginRequest represents OAuth login request
type OAuthLoginRequest struct {
	Provider string `json:"provider" example:"google" doc:"OAuth provider (google, github, instagram)"`
}

// OAuthLoginResponse represents OAuth login response
type OAuthLoginResponse struct {
	Body struct {
		Success bool   `json:"success" example:"true" doc:"Success status"`
		Message string `json:"message" example:"Redirecting to OAuth provider" doc:"Response message"`
	}
}

// OAuthLoginHandler handles OAuth login initiation
func (h *AuthHandler) OAuthLoginHandler(ctx context.Context, input *OAuthLoginRequest) (*OAuthLoginResponse, error) {
	resp := &OAuthLoginResponse{}

	// Validate provider
	if input.Provider != "google" && input.Provider != "github" {
		resp.Body.Success = false
		resp.Body.Message = "Invalid provider. Supported providers: google, github"
		return resp, nil
	}

	// Check if provider is configured
	switch input.Provider {
	case "google":
		if h.authService == nil {
			resp.Body.Success = false
			resp.Body.Message = "Google OAuth not configured"
			return resp, nil
		}
	case "github":
		if h.authService == nil {
			resp.Body.Success = false
			resp.Body.Message = "GitHub OAuth not configured"
			return resp, nil
		}
	}

	resp.Body.Success = true
	resp.Body.Message = "OAuth login initiated for provider: " + input.Provider
	return resp, nil
}

// OAuthCallbackResponse represents OAuth callback response
type OAuthCallbackResponse struct {
	Body struct {
		Success bool         `json:"success" example:"true" doc:"Success status"`
		User    *models.User `json:"user,omitempty" doc:"User information"`
		Message string       `json:"message" example:"Login successful" doc:"Response message"`
	}
}

// LogoutResponse represents logout response
type LogoutResponse struct {
	Body struct {
		Success bool   `json:"success" example:"true" doc:"Success status"`
		Message string `json:"message" example:"Logout successful" doc:"Response message"`
	}
}

// LogoutHandler handles user logout
func (h *AuthHandler) LogoutHandler(ctx context.Context, input *struct{}) (*LogoutResponse, error) {
	resp := &LogoutResponse{}

	// TODO: Implement logout logic
	resp.Body.Success = true
	resp.Body.Message = "Logout successful"
	return resp, nil
}

// UserProfileResponse represents user profile response
type UserProfileResponse struct {
	Body struct {
		Success bool         `json:"success" example:"true" doc:"Success status"`
		User    *models.User `json:"user,omitempty" doc:"User information"`
		Message string       `json:"message" example:"Profile retrieved" doc:"Response message"`
	}
}

// RefreshTokenResponse represents refresh token response
type RefreshTokenResponse struct {
	Body struct {
		Success bool   `json:"success" example:"true"`
		Token   string `json:"token" example:"new.jwt.token"`
		Message string `json:"message" example:"Token refreshed"`
	}
}

// RefreshTokenHandler handles token refresh
func (h *AuthHandler) RefreshTokenHandler(ctx context.Context, input *struct{}) (*RefreshTokenResponse, error) {
	resp := &RefreshTokenResponse{}

	if h.authService == nil {
		resp.Body.Success = false
		resp.Body.Message = "Auth service not available"
		return resp, nil
	}

	// Extract user from JWT context (set by middleware)
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		return resp, nil
	}

	// Fetch fresh user data and generate new token
	user, err := h.authService.GetUserProfile(contextUser.ID)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Failed to refresh token"
		return resp, nil
	}

	// Generate new JWT token using the JWT service
	newToken, err := h.authService.RefreshUserToken(user)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate new token"
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.Token = newToken
	resp.Body.Message = "Token refreshed"
	return resp, nil
}

// GetProfileHandler handles getting user profile
func (h *AuthHandler) GetProfileHandler(ctx context.Context, input *struct{}) (*UserProfileResponse, error) {
	resp := &UserProfileResponse{}

	if h.authService == nil {
		resp.Body.Success = false
		resp.Body.Message = "Auth service not available"
		return resp, nil
	}

	// Extract user from JWT context (set by middleware)
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		return resp, nil
	}
	// Fetch fresh user data from database with all relationships
	user, err := h.authService.GetUserProfile(contextUser.ID)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Failed to fetch user profile"
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.User = user
	resp.Body.Message = "Profile retrieved"
	return resp, nil
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
