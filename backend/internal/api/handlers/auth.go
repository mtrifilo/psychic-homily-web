package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// AuthHandler handles authentication requests
type AuthHandler struct {
	authService    *services.AuthService
	jwtService     *services.JWTService
	userService    *services.UserService
	emailService   *services.EmailService
	discordService *services.DiscordService
	config         *config.Config
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *services.AuthService, jwtService *services.JWTService,
	userService *services.UserService, config *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		jwtService:     jwtService,
		userService:    userService,
		emailService:   services.NewEmailService(config),
		discordService: services.NewDiscordService(config),
		config:         config,
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
		Success   bool         `json:"success" example:"true" doc:"Success status"`
		Message   string       `json:"message" example:"Login successful" doc:"Response message"`
		ErrorCode string       `json:"error_code,omitempty" example:"INVALID_CREDENTIALS" doc:"Error code for programmatic handling"`
		RequestID string       `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
		User      *models.User `json:"user,omitempty" doc:"User information"`
	}
}

// LoginHandler handles login request with HTTP-only cookies
func (h *AuthHandler) LoginHandler(ctx context.Context, input *LoginRequest) (*LoginResponse, error) {
	resp := &LoginResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "login_attempt",
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	// Validate email and password
	if input.Body.Email == "" || input.Body.Password == "" {
		authErr := autherrors.ErrValidationFailed("Email and password are required")
		logger.AuthWarn(ctx, "login_validation_failed",
			"error", authErr.Message,
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.Message
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	user, err := h.userService.AuthenticateUserWithPassword(input.Body.Email, input.Body.Password)
	if err != nil {
		authErr := autherrors.ErrInvalidCredentials(err)
		logger.AuthWarn(ctx, "login_failed",
			"email_hash", logger.HashEmail(input.Body.Email),
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.UserMessage()
		resp.Body.ErrorCode = autherrors.ToExternalCode(autherrors.CodeInvalidCredentials)
		return resp, nil
	}

	// Generate JWT token
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		authErr := autherrors.ErrServiceUnavailable("jwt", err)
		logger.AuthError(ctx, "token_generation_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, authErr // Return actual error for 500 handling
	}

	// Set HTTP-only cookie using Huma's built-in support
	resp.SetCookie = *setCookie(token, h.config)

	logger.AuthInfo(ctx, "login_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
	)

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

	// Try to get user info for logging (may not exist if token already expired)
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser != nil {
		logger.AuthInfo(ctx, "logout",
			"user_id", contextUser.ID,
		)
	} else {
		logger.AuthDebug(ctx, "logout_no_user")
	}

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
		Success   bool         `json:"success" example:"true" doc:"Success status"`
		User      *models.User `json:"user,omitempty" doc:"User information"`
		Message   string       `json:"message" example:"Profile retrieved" doc:"Response message"`
		ErrorCode string       `json:"error_code,omitempty" example:"UNAUTHORIZED" doc:"Error code for programmatic handling"`
		RequestID string       `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// RefreshTokenResponse represents refresh token response
type RefreshTokenResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true"`
		Token     string `json:"token" example:"new.jwt.token"`
		Message   string `json:"message" example:"Token refreshed"`
		ErrorCode string `json:"error_code,omitempty" example:"TOKEN_EXPIRED" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// RefreshTokenHandler handles token refresh
func (h *AuthHandler) RefreshTokenHandler(ctx context.Context, input *struct{}) (*RefreshTokenResponse, error) {
	resp := &RefreshTokenResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	if h.authService == nil {
		logger.AuthError(ctx, "refresh_token_failed", autherrors.ErrServiceUnavailable("auth", nil))
		resp.Body.Success = false
		resp.Body.Message = "Auth service not available"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Extract user from JWT context (set by middleware)
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "refresh_token_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "refresh_token_attempt",
		"user_id", contextUser.ID,
	)

	// Fetch fresh user data and generate new token
	user, err := h.authService.GetUserProfile(contextUser.ID)
	if err != nil {
		logger.AuthError(ctx, "refresh_token_profile_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to refresh token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Generate new JWT token using the JWT service
	newToken, err := h.authService.RefreshUserToken(user)
	if err != nil {
		logger.AuthError(ctx, "refresh_token_generation_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate new token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthInfo(ctx, "refresh_token_success",
		"user_id", user.ID,
	)

	resp.Body.Success = true
	resp.Body.Token = newToken
	resp.Body.Message = "Token refreshed"
	return resp, nil
}

// GetProfileHandler handles getting user profile
func (h *AuthHandler) GetProfileHandler(ctx context.Context, input *struct{}) (*UserProfileResponse, error) {
	resp := &UserProfileResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	if h.authService == nil {
		logger.AuthError(ctx, "get_profile_failed", autherrors.ErrServiceUnavailable("auth", nil))
		resp.Body.Success = false
		resp.Body.Message = "Auth service not available"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Extract user from JWT context (set by middleware)
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "get_profile_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "get_profile_attempt",
		"user_id", contextUser.ID,
	)

	// Fetch fresh user data from database with all relationships
	user, err := h.authService.GetUserProfile(contextUser.ID)
	if err != nil {
		logger.AuthError(ctx, "get_profile_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to fetch user profile"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthDebug(ctx, "get_profile_success",
		"user_id", user.ID,
	)

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
		Success   bool         `json:"success" example:"true" doc:"Success status"`
		Message   string       `json:"message" example:"Registration successful" doc:"Response message"`
		ErrorCode string       `json:"error_code,omitempty" example:"USER_EXISTS" doc:"Error code for programmatic handling"`
		RequestID string       `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
		User      *models.User `json:"user,omitempty" doc:"User information"`
	}
}

// RegisterHandler handles user registration
func (h *AuthHandler) RegisterHandler(ctx context.Context, input *RegisterRequest) (*RegisterResponse, error) {
	resp := &RegisterResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "register_attempt",
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	if h.userService == nil {
		logger.AuthError(ctx, "register_failed", autherrors.ErrServiceUnavailable("user", nil))
		resp.Body.Success = false
		resp.Body.Message = "User service not available"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
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
		// Check if it's a duplicate email error
		errorCode := autherrors.CodeUnknown
		message := "Failed to create user"
		if strings.Contains(err.Error(), "already exists") {
			errorCode = autherrors.CodeUserExists
			message = autherrors.ToExternalMessage(errorCode)
		}
		logger.AuthWarn(ctx, "register_failed",
			"email_hash", logger.HashEmail(input.Body.Email),
			"error", err.Error(),
			"error_code", errorCode,
		)
		resp.Body.Success = false
		resp.Body.Message = message
		resp.Body.ErrorCode = errorCode
		return resp, nil
	}

	// Generate JWT token for immediate authentication
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		logger.AuthError(ctx, "register_token_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Registration successful but failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Set HTTP-only cookie for immediate authentication
	resp.SetCookie = *setCookie(token, h.config)

	logger.AuthInfo(ctx, "register_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	// Send Discord notification for new user signup
	h.discordService.NotifyNewUser(user)

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
		SameSite: config.Session.GetSameSite(),
		Expires:  time.Now().Add(24 * time.Hour),
	}
}

// SendVerificationEmailResponse represents the response for sending verification email
type SendVerificationEmailResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"Success status"`
		Message   string `json:"message" example:"Verification email sent" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"EMAIL_SERVICE_UNAVAILABLE" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// SendVerificationEmailHandler sends a verification email to the authenticated user
func (h *AuthHandler) SendVerificationEmailHandler(ctx context.Context, input *struct{}) (*SendVerificationEmailResponse, error) {
	resp := &SendVerificationEmailResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user from context
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "send_verification_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Check if email is already verified
	if contextUser.EmailVerified {
		logger.AuthDebug(ctx, "send_verification_already_verified",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Email is already verified"
		resp.Body.ErrorCode = "ALREADY_VERIFIED"
		return resp, nil
	}

	// Check if email service is configured
	if !h.emailService.IsConfigured() {
		logger.AuthError(ctx, "send_verification_email_not_configured", nil,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Email service is not configured"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Get user email
	if contextUser.Email == nil || *contextUser.Email == "" {
		logger.AuthWarn(ctx, "send_verification_no_email",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "User does not have an email address"
		resp.Body.ErrorCode = "NO_EMAIL"
		return resp, nil
	}

	email := *contextUser.Email

	// Generate verification token
	token, err := h.jwtService.CreateVerificationToken(contextUser.ID, email)
	if err != nil {
		logger.AuthError(ctx, "send_verification_token_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate verification token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Send verification email
	if err := h.emailService.SendVerificationEmail(email, token); err != nil {
		logger.AuthError(ctx, "send_verification_email_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to send verification email"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthInfo(ctx, "send_verification_email_success",
		"user_id", contextUser.ID,
		"email_hash", logger.HashEmail(email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Verification email sent. Please check your inbox."
	return resp, nil
}

// ConfirmVerificationRequest represents the request to confirm email verification
type ConfirmVerificationRequest struct {
	Body struct {
		Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." doc:"Verification token from email link"`
	}
}

// ConfirmVerificationResponse represents the response for confirming email verification
type ConfirmVerificationResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"Success status"`
		Message   string `json:"message" example:"Email verified successfully" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"INVALID_TOKEN" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// ConfirmVerificationHandler confirms email verification using a token
func (h *AuthHandler) ConfirmVerificationHandler(ctx context.Context, input *ConfirmVerificationRequest) (*ConfirmVerificationResponse, error) {
	resp := &ConfirmVerificationResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Validate token presence
	if input.Body.Token == "" {
		logger.AuthWarn(ctx, "confirm_verification_no_token")
		resp.Body.Success = false
		resp.Body.Message = "Verification token is required"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Validate the verification token
	claims, err := h.jwtService.ValidateVerificationToken(input.Body.Token)
	if err != nil {
		logger.AuthWarn(ctx, "confirm_verification_invalid_token",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired verification token"
		resp.Body.ErrorCode = "INVALID_TOKEN"
		return resp, nil
	}

	// Get the user
	user, err := h.userService.GetUserByID(claims.UserID)
	if err != nil {
		logger.AuthError(ctx, "confirm_verification_user_not_found", err,
			"user_id", claims.UserID,
		)
		resp.Body.Success = false
		resp.Body.Message = "User not found"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Check if already verified
	if user.EmailVerified {
		logger.AuthDebug(ctx, "confirm_verification_already_verified",
			"user_id", user.ID,
		)
		resp.Body.Success = true
		resp.Body.Message = "Email is already verified"
		return resp, nil
	}

	// Verify the email matches
	if user.Email == nil || *user.Email != claims.Email {
		logger.AuthWarn(ctx, "confirm_verification_email_mismatch",
			"user_id", user.ID,
			"token_email_hash", logger.HashEmail(claims.Email),
		)
		resp.Body.Success = false
		resp.Body.Message = "Verification token does not match current email"
		resp.Body.ErrorCode = "EMAIL_MISMATCH"
		return resp, nil
	}

	// Update user to mark email as verified
	if err := h.userService.SetEmailVerified(user.ID, true); err != nil {
		logger.AuthError(ctx, "confirm_verification_update_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to verify email"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthInfo(ctx, "confirm_verification_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(claims.Email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Email verified successfully! You can now submit shows."
	return resp, nil
}
