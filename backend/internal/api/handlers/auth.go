package handlers

import (
	"context"
	"errors"
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
	authService       *services.AuthService
	jwtService        *services.JWTService
	userService       *services.UserService
	emailService      *services.EmailService
	discordService    *services.DiscordService
	passwordValidator *services.PasswordValidator
	config            *config.Config
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(
	authService *services.AuthService,
	jwtService *services.JWTService,
	userService *services.UserService,
	emailService *services.EmailService,
	discordService *services.DiscordService,
	passwordValidator *services.PasswordValidator,
	cfg *config.Config,
) *AuthHandler {
	return &AuthHandler{
		authService:       authService,
		jwtService:        jwtService,
		userService:       userService,
		emailService:      emailService,
		discordService:    discordService,
		passwordValidator: passwordValidator,
		config:            cfg,
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
		Token     string       `json:"token,omitempty" example:"eyJhbGciOiJIUzI1NiIs..." doc:"JWT token for non-cookie clients (e.g. mobile apps)"`
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
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) {
			switch authErr.Code {
			case autherrors.CodeAccountLocked:
				logger.AuthWarn(ctx, "login_account_locked",
					"email_hash", logger.HashEmail(input.Body.Email),
					"minutes_remaining", authErr.Minutes,
				)
				resp.Body.Success = false
				resp.Body.Message = authErr.UserMessage()
				resp.Body.ErrorCode = autherrors.CodeAccountLocked
				return resp, nil
			case autherrors.CodeInvalidCredentials:
				logger.AuthWarn(ctx, "login_failed",
					"email_hash", logger.HashEmail(input.Body.Email),
					"error", err.Error(),
				)
				resp.Body.Success = false
				resp.Body.Message = authErr.UserMessage()
				resp.Body.ErrorCode = autherrors.ToExternalCode(autherrors.CodeInvalidCredentials)
				return resp, nil
			}
		}

		// Generic fallback for unexpected errors
		logger.AuthWarn(ctx, "login_failed",
			"email_hash", logger.HashEmail(input.Body.Email),
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid email or password"
		resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
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
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "login_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Login successful"
	resp.Body.Token = token
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

	resp.SetCookie = h.config.Session.ClearAuthCookie()

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
		Email     string  `json:"email" example:"test@example.com" doc:"User email" validate:"required,email"`
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
		Token     string       `json:"token,omitempty" example:"eyJhbGciOiJIUzI1NiIs..." doc:"JWT token for non-cookie clients (e.g. mobile apps)"`
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

	// Validate email and password presence
	if input.Body.Email == "" || input.Body.Password == "" {
		authErr := autherrors.ErrValidationFailed("Email and password are required")
		logger.AuthWarn(ctx, "register_validation_failed",
			"error", authErr.Message,
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.Message
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Validate password using the password validator (min 12, max 128, breach check, common password check)
	if h.passwordValidator != nil {
		validationResult, err := h.passwordValidator.ValidatePassword(input.Body.Password)
		if err != nil {
			logger.AuthWarn(ctx, "password_validation_error",
				"error", err.Error(),
			)
			// Continue with registration even if validation service fails
		} else if !validationResult.Valid {
			// Return the first error message
			errorMessage := "Password does not meet security requirements"
			if len(validationResult.Errors) > 0 {
				errorMessage = validationResult.Errors[0]
			}
			authErr := autherrors.ErrValidationFailed(errorMessage)
			logger.AuthWarn(ctx, "register_validation_failed",
				"error", authErr.Message,
				"all_errors", strings.Join(validationResult.Errors, "; "),
			)
			resp.Body.Success = false
			resp.Body.Message = authErr.Message
			resp.Body.ErrorCode = autherrors.CodeValidationFailed
			return resp, nil
		}
	}

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
		errorCode := autherrors.CodeUnknown
		message := "Failed to create user"

		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) && authErr.Code == autherrors.CodeUserExists {
			errorCode = autherrors.CodeUserExists
			message = authErr.UserMessage()
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
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "register_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	// Send Discord notification for new user signup
	h.discordService.NotifyNewUser(user)

	resp.Body.Success = true
	resp.Body.Token = token
	resp.Body.User = user
	resp.Body.Message = "Registration successful and you are now logged in"
	return resp, nil
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

// SendMagicLinkRequest represents a magic link request
type SendMagicLinkRequest struct {
	Body struct {
		Email string `json:"email" example:"user@example.com" doc:"Email address to send magic link to"`
	}
}

// SendMagicLinkResponse represents the response for sending a magic link
type SendMagicLinkResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"Success status"`
		Message   string `json:"message" example:"Magic link sent" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"EMAIL_NOT_VERIFIED" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// SendMagicLinkHandler sends a magic link email for passwordless login
func (h *AuthHandler) SendMagicLinkHandler(ctx context.Context, input *SendMagicLinkRequest) (*SendMagicLinkResponse, error) {
	resp := &SendMagicLinkResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "magic_link_request",
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	// Validate email presence
	if input.Body.Email == "" {
		resp.Body.Success = false
		resp.Body.Message = "Email is required"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Check if email service is configured
	if !h.emailService.IsConfigured() {
		logger.AuthError(ctx, "magic_link_email_not_configured", nil)
		resp.Body.Success = false
		resp.Body.Message = "Email service is not configured"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Find user by email
	user, err := h.userService.GetUserByEmail(input.Body.Email)
	if err != nil {
		logger.AuthError(ctx, "magic_link_user_lookup_failed", err,
			"email_hash", logger.HashEmail(input.Body.Email),
		)
		// Don't reveal if user exists - return success message anyway
		resp.Body.Success = true
		resp.Body.Message = "If an account exists with this email, a magic link has been sent."
		return resp, nil
	}

	if user == nil {
		// User doesn't exist - return success to avoid email enumeration
		logger.AuthDebug(ctx, "magic_link_user_not_found",
			"email_hash", logger.HashEmail(input.Body.Email),
		)
		resp.Body.Success = true
		resp.Body.Message = "If an account exists with this email, a magic link has been sent."
		return resp, nil
	}

	// Check if email is verified - magic links only work for verified emails
	if !user.EmailVerified {
		logger.AuthDebug(ctx, "magic_link_email_not_verified",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Please verify your email address first. Check your inbox for a verification email, or sign in with your password to request a new one."
		resp.Body.ErrorCode = "EMAIL_NOT_VERIFIED"
		return resp, nil
	}

	// Generate magic link token
	token, err := h.jwtService.CreateMagicLinkToken(user.ID, *user.Email)
	if err != nil {
		logger.AuthError(ctx, "magic_link_token_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate magic link"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Send magic link email
	if err := h.emailService.SendMagicLinkEmail(*user.Email, token); err != nil {
		logger.AuthError(ctx, "magic_link_email_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to send magic link email"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthInfo(ctx, "magic_link_sent",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Magic link sent! Check your email to sign in."
	return resp, nil
}

// VerifyMagicLinkRequest represents the request to verify a magic link
type VerifyMagicLinkRequest struct {
	Body struct {
		Token string `json:"token" doc:"Magic link token from email"`
	}
}

// VerifyMagicLinkResponse represents the response after verifying a magic link
type VerifyMagicLinkResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool         `json:"success" example:"true" doc:"Success status"`
		Message   string       `json:"message" example:"Login successful" doc:"Response message"`
		Token     string       `json:"token,omitempty" example:"eyJhbGciOiJIUzI1NiIs..." doc:"JWT token for non-cookie clients"`
		User      *models.User `json:"user,omitempty" doc:"User information"`
		ErrorCode string       `json:"error_code,omitempty" example:"INVALID_TOKEN" doc:"Error code for programmatic handling"`
		RequestID string       `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// VerifyMagicLinkHandler verifies a magic link token and logs the user in
func (h *AuthHandler) VerifyMagicLinkHandler(ctx context.Context, input *VerifyMagicLinkRequest) (*VerifyMagicLinkResponse, error) {
	resp := &VerifyMagicLinkResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Validate token presence
	if input.Body.Token == "" {
		logger.AuthWarn(ctx, "magic_link_no_token")
		resp.Body.Success = false
		resp.Body.Message = "Magic link token is required"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Validate the magic link token
	claims, err := h.jwtService.ValidateMagicLinkToken(input.Body.Token)
	if err != nil {
		logger.AuthWarn(ctx, "magic_link_invalid_token",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired magic link. Please request a new one."
		resp.Body.ErrorCode = "INVALID_TOKEN"
		return resp, nil
	}

	// Get the user
	user, err := h.userService.GetUserByID(claims.UserID)
	if err != nil {
		logger.AuthError(ctx, "magic_link_user_not_found", err,
			"user_id", claims.UserID,
		)
		resp.Body.Success = false
		resp.Body.Message = "User not found"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Verify the email still matches (in case user changed email)
	if user.Email == nil || *user.Email != claims.Email {
		logger.AuthWarn(ctx, "magic_link_email_mismatch",
			"user_id", user.ID,
			"token_email_hash", logger.HashEmail(claims.Email),
		)
		resp.Body.Success = false
		resp.Body.Message = "This magic link is no longer valid"
		resp.Body.ErrorCode = "INVALID_TOKEN"
		return resp, nil
	}

	// Check user is still active
	if !user.IsActive {
		logger.AuthWarn(ctx, "magic_link_user_inactive",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account is not active"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Generate JWT token for session
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		logger.AuthError(ctx, "magic_link_session_token_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to create session"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Set HTTP-only cookie
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "magic_link_login_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(claims.Email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Login successful"
	resp.Body.Token = token
	resp.Body.User = user
	return resp, nil
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	Body struct {
		CurrentPassword string `json:"current_password" doc:"Current password"`
		NewPassword     string `json:"new_password" doc:"New password"`
	}
}

// ChangePasswordResponse represents a password change response
type ChangePasswordResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"Success status"`
		Message   string `json:"message" example:"Password changed successfully" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"INVALID_CREDENTIALS" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// ChangePasswordHandler handles password change requests for authenticated users
func (h *AuthHandler) ChangePasswordHandler(ctx context.Context, input *ChangePasswordRequest) (*ChangePasswordResponse, error) {
	resp := &ChangePasswordResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user from context
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "change_password_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "change_password_attempt",
		"user_id", contextUser.ID,
	)

	// Validate input
	if input.Body.CurrentPassword == "" || input.Body.NewPassword == "" {
		authErr := autherrors.ErrValidationFailed("Current password and new password are required")
		logger.AuthWarn(ctx, "change_password_validation_failed",
			"user_id", contextUser.ID,
			"error", authErr.Message,
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.Message
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Check if new password is different from current
	if input.Body.CurrentPassword == input.Body.NewPassword {
		logger.AuthWarn(ctx, "change_password_same_password",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "New password must be different from current password"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Validate new password using the password validator
	if h.passwordValidator != nil {
		validationResult, err := h.passwordValidator.ValidatePassword(input.Body.NewPassword)
		if err != nil {
			logger.AuthWarn(ctx, "change_password_validation_error",
				"user_id", contextUser.ID,
				"error", err.Error(),
			)
			// Continue even if validation service fails
		} else if !validationResult.Valid {
			errorMessage := "Password does not meet security requirements"
			if len(validationResult.Errors) > 0 {
				errorMessage = validationResult.Errors[0]
			}
			authErr := autherrors.ErrValidationFailed(errorMessage)
			logger.AuthWarn(ctx, "change_password_weak_password",
				"user_id", contextUser.ID,
				"error", authErr.Message,
				"all_errors", strings.Join(validationResult.Errors, "; "),
			)
			resp.Body.Success = false
			resp.Body.Message = authErr.Message
			resp.Body.ErrorCode = autherrors.CodeValidationFailed
			return resp, nil
		}
	}

	// Update password
	if err := h.userService.UpdatePassword(contextUser.ID, input.Body.CurrentPassword, input.Body.NewPassword); err != nil {
		errorMessage := "Failed to change password"
		errorCode := autherrors.CodeUnknown

		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) {
			switch authErr.Code {
			case autherrors.CodeInvalidCredentials:
				errorMessage = "Current password is incorrect"
				errorCode = autherrors.CodeInvalidCredentials
			case autherrors.CodeNoPasswordSet:
				errorMessage = authErr.UserMessage()
				errorCode = autherrors.CodeNoPasswordSet
			}
		}

		logger.AuthWarn(ctx, "change_password_failed",
			"user_id", contextUser.ID,
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = errorMessage
		resp.Body.ErrorCode = errorCode
		return resp, nil
	}

	logger.AuthInfo(ctx, "change_password_success",
		"user_id", contextUser.ID,
	)

	resp.Body.Success = true
	resp.Body.Message = "Password changed successfully"
	return resp, nil
}

// DeletionSummaryResponse represents the response for account deletion summary
type DeletionSummaryResponse struct {
	Body struct {
		Success         bool   `json:"success" example:"true" doc:"Success status"`
		Message         string `json:"message" example:"Deletion summary retrieved" doc:"Response message"`
		ShowsCount      int64  `json:"shows_count" example:"5" doc:"Number of shows submitted by user"`
		SavedShowsCount int64  `json:"saved_shows_count" example:"12" doc:"Number of saved shows"`
		PasskeysCount   int64  `json:"passkeys_count" example:"2" doc:"Number of registered passkeys"`
		HasPassword     bool   `json:"has_password" example:"true" doc:"Whether user has a password set"`
		ErrorCode       string `json:"error_code,omitempty" example:"UNAUTHORIZED" doc:"Error code for programmatic handling"`
		RequestID       string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// GetDeletionSummaryHandler returns a summary of data that will be affected by account deletion
func (h *AuthHandler) GetDeletionSummaryHandler(ctx context.Context, input *struct{}) (*DeletionSummaryResponse, error) {
	resp := &DeletionSummaryResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user from context
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "deletion_summary_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "deletion_summary_request",
		"user_id", contextUser.ID,
	)

	// Get deletion summary from user service
	summary, err := h.userService.GetDeletionSummary(contextUser.ID)
	if err != nil {
		logger.AuthError(ctx, "deletion_summary_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to retrieve deletion summary"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.Message = "Deletion summary retrieved"
	resp.Body.ShowsCount = summary.ShowsCount
	resp.Body.SavedShowsCount = summary.SavedShowsCount
	resp.Body.PasskeysCount = summary.PasskeysCount
	resp.Body.HasPassword = contextUser.PasswordHash != nil

	logger.AuthDebug(ctx, "deletion_summary_success",
		"user_id", contextUser.ID,
		"shows_count", summary.ShowsCount,
		"saved_shows_count", summary.SavedShowsCount,
		"passkeys_count", summary.PasskeysCount,
	)

	return resp, nil
}

// DeleteAccountRequest represents a delete account request
type DeleteAccountRequest struct {
	Body struct {
		Password string  `json:"password" doc:"Current password for re-authentication"`
		Reason   *string `json:"reason,omitempty" doc:"Optional reason for leaving"`
	}
}

// DeleteAccountResponse represents a delete account response
type DeleteAccountResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie (cleared)"`
	Body      struct {
		Success         bool   `json:"success" example:"true" doc:"Success status"`
		Message         string `json:"message" example:"Account scheduled for deletion" doc:"Response message"`
		DeletionDate    string `json:"deletion_date,omitempty" example:"2024-02-15T00:00:00Z" doc:"Date when account will be permanently deleted"`
		GracePeriodDays int    `json:"grace_period_days,omitempty" example:"30" doc:"Number of days before permanent deletion"`
		ErrorCode       string `json:"error_code,omitempty" example:"INVALID_CREDENTIALS" doc:"Error code for programmatic handling"`
		RequestID       string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// DeleteAccountHandler handles account deletion requests
func (h *AuthHandler) DeleteAccountHandler(ctx context.Context, input *DeleteAccountRequest) (*DeleteAccountResponse, error) {
	resp := &DeleteAccountResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user from context
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "delete_account_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "delete_account_attempt",
		"user_id", contextUser.ID,
	)

	// Check if user has a password set
	if contextUser.PasswordHash == nil {
		// OAuth-only user - would need email confirmation flow
		// For now, return an error indicating they need to use email confirmation
		logger.AuthWarn(ctx, "delete_account_no_password",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "OAuth users must confirm deletion via email. Please use the email confirmation option."
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Validate password is provided
	if input.Body.Password == "" {
		authErr := autherrors.ErrValidationFailed("Password is required")
		logger.AuthWarn(ctx, "delete_account_no_password_provided",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.Message
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Verify password
	if err := h.userService.VerifyPassword(*contextUser.PasswordHash, input.Body.Password); err != nil {
		logger.AuthWarn(ctx, "delete_account_invalid_password",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Password is incorrect"
		resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
		return resp, nil
	}

	// Perform soft delete
	if err := h.userService.SoftDeleteAccount(contextUser.ID, input.Body.Reason); err != nil {
		logger.AuthError(ctx, "delete_account_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to delete account"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Clear auth cookie to log out the user
	resp.SetCookie = h.config.Session.ClearAuthCookie()

	// Calculate deletion date (30 days from now)
	gracePeriodDays := 30
	deletionDate := time.Now().AddDate(0, 0, gracePeriodDays)

	logger.AuthInfo(ctx, "delete_account_success",
		"user_id", contextUser.ID,
		"deletion_date", deletionDate.Format(time.RFC3339),
	)

	resp.Body.Success = true
	resp.Body.Message = "Your account has been scheduled for deletion. You have 30 days to recover your account by contacting support."
	resp.Body.DeletionDate = deletionDate.Format(time.RFC3339)
	resp.Body.GracePeriodDays = gracePeriodDays

	return resp, nil
}

// ExportDataResponse represents the response for data export
// Note: The Body field contains the raw JSON export data
type ExportDataResponse struct {
	ContentType        string `header:"Content-Type" doc:"Content type of the response"`
	ContentDisposition string `header:"Content-Disposition" doc:"Content disposition header for file download"`
	Body               []byte
}

// RecoverAccountRequest represents a password-based account recovery request
type RecoverAccountRequest struct {
	Body struct {
		Email    string `json:"email" doc:"Email address of the account to recover"`
		Password string `json:"password" doc:"Password for re-authentication"`
	}
}

// RecoverAccountResponse represents the response for account recovery
type RecoverAccountResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool         `json:"success" example:"true" doc:"Success status"`
		Message   string       `json:"message" example:"Account recovered successfully" doc:"Response message"`
		User      *models.User `json:"user,omitempty" doc:"User information"`
		ErrorCode string       `json:"error_code,omitempty" example:"ACCOUNT_NOT_RECOVERABLE" doc:"Error code for programmatic handling"`
		RequestID string       `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// RequestAccountRecoveryRequest represents a request for magic link recovery
type RequestAccountRecoveryRequest struct {
	Body struct {
		Email string `json:"email" doc:"Email address of the account to recover"`
	}
}

// RequestAccountRecoveryResponse represents the response for requesting recovery
type RequestAccountRecoveryResponse struct {
	Body struct {
		Success      bool   `json:"success" example:"true" doc:"Success status"`
		Message      string `json:"message" example:"Recovery email sent" doc:"Response message"`
		HasPassword  bool   `json:"has_password,omitempty" doc:"Whether the account has a password set"`
		DaysRemaining int   `json:"days_remaining,omitempty" doc:"Days remaining before permanent deletion"`
		ErrorCode    string `json:"error_code,omitempty" example:"ACCOUNT_NOT_FOUND" doc:"Error code for programmatic handling"`
		RequestID    string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// ConfirmAccountRecoveryRequest represents a magic link recovery confirmation
type ConfirmAccountRecoveryRequest struct {
	Body struct {
		Token string `json:"token" doc:"Recovery token from email"`
	}
}

// ConfirmAccountRecoveryResponse represents the response for confirming recovery
type ConfirmAccountRecoveryResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool         `json:"success" example:"true" doc:"Success status"`
		Message   string       `json:"message" example:"Account recovered successfully" doc:"Response message"`
		User      *models.User `json:"user,omitempty" doc:"User information"`
		ErrorCode string       `json:"error_code,omitempty" example:"INVALID_TOKEN" doc:"Error code for programmatic handling"`
		RequestID string       `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// ExportDataHandler handles GDPR data export requests (Right to Portability)
func (h *AuthHandler) ExportDataHandler(ctx context.Context, input *struct{}) (*ExportDataResponse, error) {
	resp := &ExportDataResponse{}

	// Get authenticated user from context
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "export_data_no_user")
		// Return an error JSON instead of the file
		resp.ContentType = "application/json"
		resp.Body = []byte(`{"success":false,"error":"unauthorized","message":"User not found in context"}`)
		return resp, nil
	}

	logger.AuthDebug(ctx, "export_data_request",
		"user_id", contextUser.ID,
	)

	// Export user data as JSON
	exportData, err := h.userService.ExportUserDataJSON(contextUser.ID)
	if err != nil {
		logger.AuthError(ctx, "export_data_failed", err,
			"user_id", contextUser.ID,
		)
		resp.ContentType = "application/json"
		resp.Body = []byte(`{"success":false,"error":"export_failed","message":"Failed to export user data"}`)
		return resp, nil
	}

	logger.AuthInfo(ctx, "export_data_success",
		"user_id", contextUser.ID,
		"export_size_bytes", len(exportData),
	)

	// Set headers for file download
	resp.ContentType = "application/json"
	resp.ContentDisposition = "attachment; filename=\"psychic-homily-data-export.json\""
	resp.Body = exportData

	return resp, nil
}

// RecoverAccountHandler handles password-based account recovery
// This is for users who have a password set and remember it
func (h *AuthHandler) RecoverAccountHandler(ctx context.Context, input *RecoverAccountRequest) (*RecoverAccountResponse, error) {
	resp := &RecoverAccountResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "recover_account_attempt",
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	// Validate input
	if input.Body.Email == "" || input.Body.Password == "" {
		authErr := autherrors.ErrValidationFailed("Email and password are required")
		logger.AuthWarn(ctx, "recover_account_validation_failed",
			"error", authErr.Message,
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.Message
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Get user by email including soft-deleted accounts
	user, err := h.userService.GetUserByEmailIncludingDeleted(input.Body.Email)
	if err != nil {
		logger.AuthError(ctx, "recover_account_lookup_failed", err,
			"email_hash", logger.HashEmail(input.Body.Email),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credentials"
		resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
		return resp, nil
	}

	if user == nil {
		// Constant-time response to prevent email enumeration
		logger.AuthDebug(ctx, "recover_account_user_not_found",
			"email_hash", logger.HashEmail(input.Body.Email),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credentials"
		resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
		return resp, nil
	}

	// Check if account is actually deleted and recoverable
	if user.IsActive {
		logger.AuthDebug(ctx, "recover_account_already_active",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account is already active. Please log in normally."
		resp.Body.ErrorCode = "ACCOUNT_ACTIVE"
		return resp, nil
	}

	if !h.userService.IsAccountRecoverable(user) {
		logger.AuthWarn(ctx, "recover_account_expired",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account can no longer be recovered. The 30-day recovery period has expired."
		resp.Body.ErrorCode = "ACCOUNT_NOT_RECOVERABLE"
		return resp, nil
	}

	// Verify password
	if user.PasswordHash == nil {
		logger.AuthDebug(ctx, "recover_account_no_password",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account does not have a password. Please use the email recovery option."
		resp.Body.ErrorCode = "NO_PASSWORD"
		return resp, nil
	}

	if err := h.userService.VerifyPassword(*user.PasswordHash, input.Body.Password); err != nil {
		logger.AuthWarn(ctx, "recover_account_invalid_password",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credentials"
		resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
		return resp, nil
	}

	// Restore the account
	if err := h.userService.RestoreAccount(user.ID); err != nil {
		logger.AuthError(ctx, "recover_account_restore_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to restore account"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Fetch the restored user
	restoredUser, err := h.userService.GetUserByID(user.ID)
	if err != nil {
		logger.AuthError(ctx, "recover_account_fetch_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Account restored but failed to fetch user data"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Generate JWT token and log user in
	token, err := h.jwtService.CreateToken(restoredUser)
	if err != nil {
		logger.AuthError(ctx, "recover_account_token_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Account restored but failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "recover_account_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Account recovered successfully. Welcome back!"
	resp.Body.User = restoredUser

	return resp, nil
}

// RequestAccountRecoveryHandler handles requests to send a recovery email
// This is for OAuth-only users or users who forgot their password
func (h *AuthHandler) RequestAccountRecoveryHandler(ctx context.Context, input *RequestAccountRecoveryRequest) (*RequestAccountRecoveryResponse, error) {
	resp := &RequestAccountRecoveryResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "request_recovery_attempt",
		"email_hash", logger.HashEmail(input.Body.Email),
	)

	// Validate input
	if input.Body.Email == "" {
		authErr := autherrors.ErrValidationFailed("Email is required")
		logger.AuthWarn(ctx, "request_recovery_validation_failed",
			"error", authErr.Message,
		)
		resp.Body.Success = false
		resp.Body.Message = authErr.Message
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Get user by email including soft-deleted accounts
	user, err := h.userService.GetUserByEmailIncludingDeleted(input.Body.Email)
	if err != nil {
		logger.AuthError(ctx, "request_recovery_lookup_failed", err,
			"email_hash", logger.HashEmail(input.Body.Email),
		)
		// Don't reveal if lookup failed - return generic message
		resp.Body.Success = true
		resp.Body.Message = "If an account exists with this email and is eligible for recovery, a recovery email has been sent."
		return resp, nil
	}

	if user == nil {
		// Don't reveal if user doesn't exist
		logger.AuthDebug(ctx, "request_recovery_user_not_found",
			"email_hash", logger.HashEmail(input.Body.Email),
		)
		resp.Body.Success = true
		resp.Body.Message = "If an account exists with this email and is eligible for recovery, a recovery email has been sent."
		return resp, nil
	}

	// Check if account is deleted
	if user.IsActive {
		logger.AuthDebug(ctx, "request_recovery_account_active",
			"user_id", user.ID,
		)
		// Inform user that account is active
		resp.Body.Success = false
		resp.Body.Message = "This account is active. Please log in normally."
		resp.Body.ErrorCode = "ACCOUNT_ACTIVE"
		resp.Body.HasPassword = user.PasswordHash != nil
		return resp, nil
	}

	// Check if account is recoverable
	if !h.userService.IsAccountRecoverable(user) {
		logger.AuthWarn(ctx, "request_recovery_expired",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account can no longer be recovered. The 30-day recovery period has expired."
		resp.Body.ErrorCode = "ACCOUNT_NOT_RECOVERABLE"
		return resp, nil
	}

	// Check if email service is configured
	if !h.emailService.IsConfigured() {
		logger.AuthError(ctx, "request_recovery_email_not_configured", nil,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Email service is not configured"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Calculate days remaining
	daysRemaining := h.userService.GetDaysUntilPermanentDeletion(user)

	// Generate recovery token
	token, err := h.jwtService.CreateAccountRecoveryToken(user.ID, *user.Email)
	if err != nil {
		logger.AuthError(ctx, "request_recovery_token_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate recovery token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Send recovery email
	if err := h.emailService.SendAccountRecoveryEmail(*user.Email, token, daysRemaining); err != nil {
		logger.AuthError(ctx, "request_recovery_email_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to send recovery email"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthInfo(ctx, "request_recovery_email_sent",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(input.Body.Email),
		"days_remaining", daysRemaining,
	)

	resp.Body.Success = true
	resp.Body.Message = "Recovery email sent. Please check your inbox."
	resp.Body.HasPassword = user.PasswordHash != nil
	resp.Body.DaysRemaining = daysRemaining

	return resp, nil
}

// ConfirmAccountRecoveryHandler handles magic link account recovery confirmation
func (h *AuthHandler) ConfirmAccountRecoveryHandler(ctx context.Context, input *ConfirmAccountRecoveryRequest) (*ConfirmAccountRecoveryResponse, error) {
	resp := &ConfirmAccountRecoveryResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Validate token presence
	if input.Body.Token == "" {
		logger.AuthWarn(ctx, "confirm_recovery_no_token")
		resp.Body.Success = false
		resp.Body.Message = "Recovery token is required"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Validate the recovery token
	claims, err := h.jwtService.ValidateAccountRecoveryToken(input.Body.Token)
	if err != nil {
		logger.AuthWarn(ctx, "confirm_recovery_invalid_token",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired recovery token. Please request a new one."
		resp.Body.ErrorCode = "INVALID_TOKEN"
		return resp, nil
	}

	// Get the user (including deleted)
	user, err := h.userService.GetUserByEmailIncludingDeleted(claims.Email)
	if err != nil || user == nil {
		logger.AuthError(ctx, "confirm_recovery_user_not_found", err,
			"user_id", claims.UserID,
		)
		resp.Body.Success = false
		resp.Body.Message = "User not found"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Verify user ID matches
	if user.ID != claims.UserID {
		logger.AuthWarn(ctx, "confirm_recovery_user_mismatch",
			"token_user_id", claims.UserID,
			"actual_user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid recovery token"
		resp.Body.ErrorCode = "INVALID_TOKEN"
		return resp, nil
	}

	// Check if account is still recoverable
	if user.IsActive {
		logger.AuthDebug(ctx, "confirm_recovery_already_active",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account is already active"
		resp.Body.ErrorCode = "ACCOUNT_ACTIVE"
		return resp, nil
	}

	if !h.userService.IsAccountRecoverable(user) {
		logger.AuthWarn(ctx, "confirm_recovery_expired",
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "This account can no longer be recovered. The 30-day recovery period has expired."
		resp.Body.ErrorCode = "ACCOUNT_NOT_RECOVERABLE"
		return resp, nil
	}

	// Restore the account
	if err := h.userService.RestoreAccount(user.ID); err != nil {
		logger.AuthError(ctx, "confirm_recovery_restore_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to restore account"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Fetch the restored user
	restoredUser, err := h.userService.GetUserByID(user.ID)
	if err != nil {
		logger.AuthError(ctx, "confirm_recovery_fetch_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Account restored but failed to fetch user data"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Generate JWT token and log user in
	token, err := h.jwtService.CreateToken(restoredUser)
	if err != nil {
		logger.AuthError(ctx, "confirm_recovery_token_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Account restored but failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "confirm_recovery_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(claims.Email),
	)

	resp.Body.Success = true
	resp.Body.Message = "Account recovered successfully. Welcome back!"
	resp.Body.User = restoredUser

	return resp, nil
}

// GenerateCLITokenResponse represents the response for CLI token generation
type GenerateCLITokenResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"Success status"`
		Token     string `json:"token,omitempty" doc:"CLI authentication token"`
		ExpiresIn int    `json:"expires_in,omitempty" example:"86400" doc:"Token expiry in seconds"`
		Message   string `json:"message" example:"CLI token generated" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"UNAUTHORIZED" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" doc:"Request ID for debugging"`
	}
}

// GenerateCLITokenHandler generates a token for CLI authentication
// This allows users to copy a token from the web UI and paste it into the CLI
func (h *AuthHandler) GenerateCLITokenHandler(ctx context.Context, input *struct{}) (*GenerateCLITokenResponse, error) {
	resp := &GenerateCLITokenResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user from context
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "generate_cli_token_no_user")
		resp.Body.Success = false
		resp.Body.Message = "User not found in context"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Only allow admins to generate CLI tokens
	if !contextUser.IsAdmin {
		logger.AuthWarn(ctx, "generate_cli_token_not_admin",
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "CLI tokens are only available for admin users"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "generate_cli_token_attempt",
		"user_id", contextUser.ID,
	)

	// Generate a fresh JWT token for CLI use (24 hour expiry)
	token, err := h.jwtService.CreateToken(contextUser)
	if err != nil {
		logger.AuthError(ctx, "generate_cli_token_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate CLI token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	logger.AuthInfo(ctx, "generate_cli_token_success",
		"user_id", contextUser.ID,
	)

	resp.Body.Success = true
	resp.Body.Token = token
	resp.Body.ExpiresIn = 86400 // 24 hours
	resp.Body.Message = "CLI token generated successfully. This token expires in 24 hours."

	return resp, nil
}
