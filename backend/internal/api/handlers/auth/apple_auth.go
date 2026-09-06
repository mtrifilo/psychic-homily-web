package auth

import (
	"context"
	"net/http"
	"time"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// AppleAuthHandler handles Sign in with Apple authentication
type AppleAuthHandler struct {
	appleAuthService contracts.AppleAuthServiceInterface
	discordService   contracts.DiscordServiceInterface
	config           *config.Config
}

// NewAppleAuthHandler creates a new Apple auth handler
func NewAppleAuthHandler(appleAuthService contracts.AppleAuthServiceInterface, discordService contracts.DiscordServiceInterface, cfg *config.Config) *AppleAuthHandler {
	return &AppleAuthHandler{
		appleAuthService: appleAuthService,
		discordService:   discordService,
		config:           cfg,
	}
}

// AppleCallbackRequest represents the Sign in with Apple callback request
type AppleCallbackRequest struct {
	Body struct {
		IdentityToken string  `json:"identity_token" doc:"Apple identity token (JWT)"`
		FirstName     *string `json:"first_name,omitempty" doc:"User's first name (only provided on first sign-in)"`
		LastName      *string `json:"last_name,omitempty" doc:"User's last name (only provided on first sign-in)"`
	}
}

// AppleCallbackResponse represents the Sign in with Apple callback response
type AppleCallbackResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool        `json:"success" example:"true" doc:"Success status"`
		Message   string      `json:"message" example:"Login successful" doc:"Response message"`
		Token     string      `json:"token,omitempty" doc:"JWT token for non-cookie clients"`
		User      *authm.User `json:"user,omitempty" doc:"User information"`
		ErrorCode string      `json:"error_code,omitempty" doc:"Error code for programmatic handling"`
		RequestID string      `json:"request_id,omitempty" doc:"Request ID for debugging"`
	}
}

// AppleCallbackHandler handles POST /auth/apple/callback
func (h *AppleAuthHandler) AppleCallbackHandler(ctx context.Context, input *AppleCallbackRequest) (*AppleCallbackResponse, error) {
	resp := &AppleCallbackResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "apple_auth_attempt")

	// Validate identity token presence
	if input.Body.IdentityToken == "" {
		logger.AuthWarn(ctx, "apple_auth_missing_token")
		resp.Body.Success = false
		resp.Body.Message = "Identity token is required"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Names are request-body fields, not identity-token claims: the token
	// authenticates the Apple account but says nothing about the names, so they
	// are caller-controlled and take the same guard as the other request-body
	// handlers that write these columns (RegisterHandler, UpdateProfileHandler).
	//
	// A refused name is DROPPED, not turned into an error, which is where this
	// diverges from those two. Apple sends a name only on the first authorization,
	// so clients cache it and resend it on later callbacks, and
	// FindOrCreateAppleUser ignores both names entirely once the Apple account is
	// known. Refusing would therefore fail an authentication over a field the
	// request was going to discard. Dropping keeps the column clean, which is the
	// property the guard is for, without making a cosmetic field able to block
	// sign-in. firstName/lastName already default to "" when Apple omits them, so
	// a dropped name is a shape the create path handles.
	firstName := ""
	lastName := ""
	if input.Body.FirstName != nil {
		if name, errMsg, ok := validateProfileName("First name", *input.Body.FirstName); ok {
			firstName = name
		} else {
			logger.AuthWarn(ctx, "apple_auth_name_dropped", "field", "first_name", "error", errMsg)
		}
	}
	if input.Body.LastName != nil {
		if name, errMsg, ok := validateProfileName("Last name", *input.Body.LastName); ok {
			lastName = name
		} else {
			logger.AuthWarn(ctx, "apple_auth_name_dropped", "field", "last_name", "error", errMsg)
		}
	}

	// Validate the Apple identity token
	claims, err := h.appleAuthService.ValidateIdentityToken(input.Body.IdentityToken)
	if err != nil {
		logger.AuthWarn(ctx, "apple_auth_token_invalid",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid Apple identity token"
		resp.Body.ErrorCode = autherrors.CodeTokenInvalid
		return resp, nil
	}

	logger.AuthDebug(ctx, "apple_auth_token_validated",
		"apple_sub", claims.Subject,
		"has_email", claims.Email != "",
	)

	// Find or create user
	user, err := h.appleAuthService.FindOrCreateAppleUser(claims, firstName, lastName)
	if err != nil {
		logger.AuthError(ctx, "apple_auth_user_create_failed", err)
		resp.Body.Success = false
		resp.Body.Message = "Failed to process Apple sign-in"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Generate JWT
	token, err := h.appleAuthService.GenerateToken(user)
	if err != nil {
		logger.AuthError(ctx, "apple_auth_token_generation_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Set cookie (for web clients)
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "apple_auth_success",
		"user_id", user.ID,
	)

	// Notify Discord for new users (check if user was just created by checking created_at)
	if time.Since(user.CreatedAt) < 10*time.Second {
		h.discordService.NotifyNewUser(user)
	}

	resp.Body.Success = true
	resp.Body.Message = "Login successful"
	resp.Body.Token = token
	resp.Body.User = user

	return resp, nil
}
