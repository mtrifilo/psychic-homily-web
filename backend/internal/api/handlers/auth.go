package handlers

import (
	"context"
	"fmt"
	"net/http"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
	"time"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	authService *services.AuthService
	jwtService  *services.JWTService
	userService *services.UserService
	config      *config.Config
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *services.AuthService, jwtService *services.JWTService,
	userService *services.UserService, config *config.Config) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		jwtService:  jwtService,
		userService: userService,
		config:      config,
	}
}

// LoginRequest represents login request
type LoginRequest struct {
	Body struct {
		Email    string `json:"email" example:"test@example.com" doc:"User email"`
		Password string `json:"password" example:"password" doc:"User password"`
	}
}

// LoginResponse represents login response
type LoginResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success bool         `json:"success" example:"true" doc:"Success status"`
		Message string       `json:"message" example:"Login successful" doc:"Response message"`
		User    *models.User `json:"user,omitempty" doc:"User information"`
	}
}

// LoginHandler handles login request with HTTP-only cookies
func (h *AuthHandler) LoginHandler(ctx context.Context, input *LoginRequest) (*LoginResponse, error) {
	resp := &LoginResponse{}

	fmt.Printf("LoginHandler called with input: %+v\n", input)
	fmt.Printf("Input type: %T\n", input)
	fmt.Printf("Email: '%s', Password: '%s'\n", input.Body.Email, input.Body.Password)

	// Validate email and password
	if input.Body.Email == "" || input.Body.Password == "" {
		resp.Body.Success = false
		resp.Body.Message = "Email and password are required"
		return resp, nil
	}

	user, err := h.userService.AuthenticateUserWithPassword(input.Body.Email, input.Body.Password)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Invalid credentials"
		return resp, nil
	}

	// Generate JWT token (integrate with your existing JWT service)
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate token"
		return resp, nil
	}

	// Set HTTP-only cookie using Huma's built-in support
	resp.SetCookie = *setCookie(token, h.config)

	resp.Body.Success = true
	resp.Body.Message = "Login successful"
	resp.Body.User = user

	return resp, nil
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
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success bool   `json:"success" example:"true" doc:"Success status"`
		Message string `json:"message" example:"Logout successful" doc:"Response message"`
	}
}

// LogoutHandler handles user logout
func (h *AuthHandler) LogoutHandler(ctx context.Context, input *struct{}) (*LogoutResponse, error) {
	resp := &LogoutResponse{}

	resp.SetCookie = http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.Session.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0), // Expire immediately
		MaxAge:   -1,
	}

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

type RegisterRequest struct {
	Body struct {
		Email     string  `json:"email" example:"test@example.com" doc:"User email" validate:"required"`
		Password  string  `json:"password" example:"password" doc:"User password" validate:"required"`
		FirstName *string `json:"first_name,omitempty" example:"John" doc:"User first name (optional)"`
		LastName  *string `json:"last_name,omitempty" example:"Doe" doc:"User last name (optional)"`
	}
}

type RegisterResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success bool         `json:"success" example:"true" doc:"Success status"`
		Message string       `json:"message" example:"Registration successful" doc:"Response message"`
		User    *models.User `json:"user,omitempty" doc:"User information"`
	}
}

// RegisterHandler handles user registration
func (h *AuthHandler) RegisterHandler(ctx context.Context, input *RegisterRequest) (*RegisterResponse, error) {
	resp := &RegisterResponse{}

	if h.userService == nil {
		resp.Body.Success = false
		resp.Body.Message = "User service not available"
		return resp, nil
	}

	// Handle optional first and last names
	var firstName, lastName string
	if input.Body.FirstName != nil {
		firstName = *input.Body.FirstName
	}
	if input.Body.LastName != nil {
		lastName = *input.Body.LastName
	}

	user, err := h.userService.CreateUserWithPassword(input.Body.Email, input.Body.Password, firstName, lastName)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Failed to create user"
		return resp, nil
	}

	// Generate JWT token for immediate authentication
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Registration successful but failed to generate authentication token"
		return resp, nil
	}

	// Set HTTP-only cookie for immediate authentication
	resp.SetCookie = *setCookie(token, h.config)

	resp.Body.Success = true
	resp.Body.User = user
	resp.Body.Message = "Registration successful and you are now logged in"
	return resp, nil
}

func setCookie(token string, config *config.Config) *http.Cookie {
	// Set HTTP-only cookie for immediate authentication
	return &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   config.Session.Secure, // Set to true in production (HTTPS only)
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	}
}
