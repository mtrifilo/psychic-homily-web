package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// PasskeyHandler handles passkey/WebAuthn requests
type PasskeyHandler struct {
	webauthnService *services.WebAuthnService
	jwtService      *services.JWTService
	userService     *services.UserService
	config          *config.Config
}

// NewPasskeyHandler creates a new passkey handler
func NewPasskeyHandler(webauthnService *services.WebAuthnService, jwtService *services.JWTService, userService *services.UserService, cfg *config.Config) *PasskeyHandler {
	return &PasskeyHandler{
		webauthnService: webauthnService,
		jwtService:      jwtService,
		userService:     userService,
		config:          cfg,
	}
}

// --- Registration Endpoints ---

// BeginRegisterRequest represents the request to begin passkey registration
type BeginRegisterRequest struct {
	Body struct {
		DisplayName string `json:"display_name" example:"My MacBook" doc:"Name for this passkey"`
	}
}

// BeginRegisterResponse represents the response with WebAuthn registration options
type BeginRegisterResponse struct {
	Body struct {
		Success     bool                          `json:"success" doc:"Success status"`
		Message     string                        `json:"message" doc:"Response message"`
		Options     *protocol.CredentialCreation  `json:"options,omitempty" doc:"WebAuthn registration options"`
		ChallengeID string                        `json:"challenge_id,omitempty" doc:"Challenge ID for finishing registration"`
		ErrorCode   string                        `json:"error_code,omitempty" doc:"Error code"`
		RequestID   string                        `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// BeginRegisterHandler starts the passkey registration process
func (h *PasskeyHandler) BeginRegisterHandler(ctx context.Context, input *BeginRegisterRequest) (*BeginRegisterResponse, error) {
	resp := &BeginRegisterResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		resp.Body.Success = false
		resp.Body.Message = "Authentication required"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "passkey_register_begin",
		"user_id", contextUser.ID,
	)

	// Begin registration
	options, session, err := h.webauthnService.BeginRegistration(contextUser)
	if err != nil {
		logger.AuthError(ctx, "passkey_register_begin_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to start passkey registration"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Store challenge
	challengeID, err := h.webauthnService.StoreChallenge(contextUser.ID, session, "registration")
	if err != nil {
		logger.AuthError(ctx, "passkey_challenge_store_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to store challenge"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.Message = "Registration options created"
	resp.Body.Options = options
	resp.Body.ChallengeID = challengeID
	return resp, nil
}

// CredentialCreationAttestationResponse contains the attestation data from the authenticator
type CredentialCreationAttestationResponse struct {
	AttestationObject string   `json:"attestationObject"`
	ClientDataJSON    string   `json:"clientDataJSON"`
	Transports        []string `json:"transports,omitempty"`
}

// CredentialCreationResponse represents the WebAuthn credential creation response from the browser
type CredentialCreationResponse struct {
	ID                      string                                `json:"id"`
	RawID                   string                                `json:"rawId"`
	Type                    string                                `json:"type"`
	AuthenticatorAttachment string                                `json:"authenticatorAttachment,omitempty"`
	Response                CredentialCreationAttestationResponse `json:"response"`
}

// FinishRegisterRequest represents the request to complete passkey registration
type FinishRegisterRequest struct {
	Body struct {
		ChallengeID string                     `json:"challenge_id" doc:"Challenge ID from begin registration"`
		DisplayName string                     `json:"display_name" example:"My MacBook" doc:"Name for this passkey"`
		Response    CredentialCreationResponse `json:"response" doc:"The credential response from the browser"`
	}
}

// FinishRegisterResponse represents the response after completing registration
type FinishRegisterResponse struct {
	Body struct {
		Success    bool                       `json:"success" doc:"Success status"`
		Message    string                     `json:"message" doc:"Response message"`
		Credential *models.WebAuthnCredential `json:"credential,omitempty" doc:"Registered credential"`
		ErrorCode  string                     `json:"error_code,omitempty" doc:"Error code"`
		RequestID  string                     `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// FinishRegisterHandler completes the passkey registration process
func (h *PasskeyHandler) FinishRegisterHandler(ctx context.Context, input *FinishRegisterRequest) (*FinishRegisterResponse, error) {
	resp := &FinishRegisterResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		resp.Body.Success = false
		resp.Body.Message = "Authentication required"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	logger.AuthDebug(ctx, "passkey_register_finish",
		"user_id", contextUser.ID,
	)

	// Get challenge
	session, userID, err := h.webauthnService.GetChallenge(input.Body.ChallengeID, "registration")
	if err != nil {
		logger.AuthWarn(ctx, "passkey_challenge_invalid",
			"user_id", contextUser.ID,
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired challenge"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Verify user ID matches
	if userID != contextUser.ID {
		resp.Body.Success = false
		resp.Body.Message = "Challenge belongs to different user"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	// Parse the credential response
	// Note: In production, you'd use protocol.ParseCredentialCreationResponseBody
	// For simplicity, we'll construct the parsed response manually
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(
		createCredentialCreationReader(input),
	)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"user_id", contextUser.ID,
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credential response"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Get display name
	displayName := input.Body.DisplayName
	if displayName == "" {
		displayName = "My Passkey"
	}

	// Complete registration
	credential, err := h.webauthnService.FinishRegistration(contextUser, session, parsedResponse, displayName)
	if err != nil {
		logger.AuthError(ctx, "passkey_register_finish_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to register passkey"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Delete used challenge
	_ = h.webauthnService.DeleteChallenge(input.Body.ChallengeID)

	logger.AuthInfo(ctx, "passkey_registered",
		"user_id", contextUser.ID,
		"credential_id", credential.ID,
	)

	resp.Body.Success = true
	resp.Body.Message = "Passkey registered successfully"
	resp.Body.Credential = credential
	return resp, nil
}

// --- Login Endpoints ---

// BeginLoginRequest represents the request to begin passkey login
type BeginLoginRequest struct {
	Body struct {
		Email string `json:"email,omitempty" example:"user@example.com" doc:"Email for user-specific login (optional)"`
	}
}

// BeginLoginResponse represents the response with WebAuthn login options
type BeginLoginResponse struct {
	Body struct {
		Success     bool                         `json:"success" doc:"Success status"`
		Message     string                       `json:"message" doc:"Response message"`
		Options     *protocol.CredentialAssertion `json:"options,omitempty" doc:"WebAuthn login options"`
		ChallengeID string                       `json:"challenge_id,omitempty" doc:"Challenge ID for finishing login"`
		ErrorCode   string                       `json:"error_code,omitempty" doc:"Error code"`
		RequestID   string                       `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// BeginLoginHandler starts the passkey login process
func (h *PasskeyHandler) BeginLoginHandler(ctx context.Context, input *BeginLoginRequest) (*BeginLoginResponse, error) {
	resp := &BeginLoginResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	var options *protocol.CredentialAssertion
	var session *webauthn.SessionData
	var userID uint

	if input.Body.Email != "" {
		// User-specific login
		user, err := h.userService.GetUserByEmail(input.Body.Email)
		if err != nil {
			// Don't reveal if user exists
			resp.Body.Success = false
			resp.Body.Message = "Invalid credentials"
			resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
			return resp, nil
		}

		opts, sess, err := h.webauthnService.BeginLogin(user)
		if err != nil {
			logger.AuthDebug(ctx, "passkey_login_begin_failed",
				"error", err.Error(),
			)
			resp.Body.Success = false
			resp.Body.Message = "No passkeys registered for this account"
			resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
			return resp, nil
		}

		options = opts
		session = sess
		userID = user.ID
	} else {
		// Discoverable login (usernameless)
		opts, sess, err := h.webauthnService.BeginDiscoverableLogin()
		if err != nil {
			logger.AuthError(ctx, "passkey_discoverable_login_begin_failed", err)
			resp.Body.Success = false
			resp.Body.Message = "Failed to start passkey login"
			resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
			return resp, nil
		}

		options = opts
		session = sess
		userID = 0 // Will be determined during finish
	}

	// Store challenge
	challengeID, err := h.webauthnService.StoreChallenge(userID, session, "authentication")
	if err != nil {
		logger.AuthError(ctx, "passkey_challenge_store_failed", err)
		resp.Body.Success = false
		resp.Body.Message = "Failed to store challenge"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.Message = "Login options created"
	resp.Body.Options = options
	resp.Body.ChallengeID = challengeID
	return resp, nil
}

// CredentialAssertionAuthenticatorResponse contains the authenticator assertion data
type CredentialAssertionAuthenticatorResponse struct {
	AuthenticatorData string `json:"authenticatorData"`
	ClientDataJSON    string `json:"clientDataJSON"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

// CredentialAssertionResponse represents the WebAuthn credential assertion response from the browser
type CredentialAssertionResponse struct {
	ID       string                                   `json:"id"`
	RawID    string                                   `json:"rawId"`
	Type     string                                   `json:"type"`
	Response CredentialAssertionAuthenticatorResponse `json:"response"`
}

// FinishLoginRequest represents the request to complete passkey login
type FinishLoginRequest struct {
	Body struct {
		ChallengeID string                      `json:"challenge_id" doc:"Challenge ID from begin login"`
		Response    CredentialAssertionResponse `json:"response" doc:"The assertion response from the browser"`
	}
}

// FinishLoginResponse represents the response after completing login
type FinishLoginResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool         `json:"success" doc:"Success status"`
		Message   string       `json:"message" doc:"Response message"`
		User      *models.User `json:"user,omitempty" doc:"Authenticated user"`
		ErrorCode string       `json:"error_code,omitempty" doc:"Error code"`
		RequestID string       `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// FinishLoginHandler completes the passkey login process
func (h *PasskeyHandler) FinishLoginHandler(ctx context.Context, input *FinishLoginRequest) (*FinishLoginResponse, error) {
	resp := &FinishLoginResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "passkey_login_finish")

	// Get challenge
	session, userID, err := h.webauthnService.GetChallenge(input.Body.ChallengeID, "authentication")
	if err != nil {
		logger.AuthWarn(ctx, "passkey_challenge_invalid",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired challenge"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Parse the assertion response
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(
		createCredentialRequestReader(input),
	)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credential response"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	var user *models.User

	if userID != 0 {
		// User-specific login
		user, err = h.userService.GetUserByID(userID)
		if err != nil {
			resp.Body.Success = false
			resp.Body.Message = "User not found"
			resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
			return resp, nil
		}

		_, err = h.webauthnService.FinishLogin(user, session, parsedResponse)
	} else {
		// Discoverable login
		user, _, err = h.webauthnService.FinishDiscoverableLogin(session, parsedResponse)
	}

	if err != nil {
		logger.AuthWarn(ctx, "passkey_login_finish_failed",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid passkey"
		resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
		return resp, nil
	}

	// Delete used challenge
	_ = h.webauthnService.DeleteChallenge(input.Body.ChallengeID)

	// Generate JWT token
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		logger.AuthError(ctx, "passkey_token_generation_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Set cookie
	resp.SetCookie = http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.Session.Secure,
		SameSite: h.config.Session.GetSameSite(),
		Expires:  time.Now().Add(24 * time.Hour),
	}

	logger.AuthInfo(ctx, "passkey_login_success",
		"user_id", user.ID,
	)

	resp.Body.Success = true
	resp.Body.Message = "Login successful"
	resp.Body.User = user
	return resp, nil
}

// --- Credential Management Endpoints ---

// ListCredentialsResponse represents the response with user's passkeys
type ListCredentialsResponse struct {
	Body struct {
		Success     bool                       `json:"success" doc:"Success status"`
		Message     string                     `json:"message" doc:"Response message"`
		Credentials []models.WebAuthnCredential `json:"credentials" doc:"User's passkey credentials"`
		ErrorCode   string                     `json:"error_code,omitempty" doc:"Error code"`
		RequestID   string                     `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// ListCredentialsHandler returns all passkeys for the authenticated user
func (h *PasskeyHandler) ListCredentialsHandler(ctx context.Context, input *struct{}) (*ListCredentialsResponse, error) {
	resp := &ListCredentialsResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		resp.Body.Success = false
		resp.Body.Message = "Authentication required"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	credentials, err := h.webauthnService.GetUserCredentials(contextUser.ID)
	if err != nil {
		logger.AuthError(ctx, "passkey_list_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to get credentials"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.Message = "Credentials retrieved"
	resp.Body.Credentials = credentials
	return resp, nil
}

// DeleteCredentialRequest represents the request to delete a passkey
type DeleteCredentialRequest struct {
	CredentialID uint `path:"credential_id" doc:"Credential ID to delete"`
}

// DeleteCredentialResponse represents the response after deleting a passkey
type DeleteCredentialResponse struct {
	Body struct {
		Success   bool   `json:"success" doc:"Success status"`
		Message   string `json:"message" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" doc:"Error code"`
		RequestID string `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// DeleteCredentialHandler removes a passkey credential
func (h *PasskeyHandler) DeleteCredentialHandler(ctx context.Context, input *DeleteCredentialRequest) (*DeleteCredentialResponse, error) {
	resp := &DeleteCredentialResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	// Get authenticated user
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		resp.Body.Success = false
		resp.Body.Message = "Authentication required"
		resp.Body.ErrorCode = autherrors.CodeUnauthorized
		return resp, nil
	}

	err := h.webauthnService.DeleteCredential(contextUser.ID, input.CredentialID)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_delete_failed",
			"user_id", contextUser.ID,
			"credential_id", input.CredentialID,
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to delete credential"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	logger.AuthInfo(ctx, "passkey_deleted",
		"user_id", contextUser.ID,
		"credential_id", input.CredentialID,
	)

	resp.Body.Success = true
	resp.Body.Message = "Passkey deleted successfully"
	return resp, nil
}

// Helper functions for parsing WebAuthn responses

func createCredentialCreationReader(input *FinishRegisterRequest) io.Reader {
	// Create a proper JSON structure for the WebAuthn response
	response := map[string]interface{}{
		"id":    input.Body.Response.ID,
		"rawId": input.Body.Response.RawID,
		"type":  input.Body.Response.Type,
		"response": map[string]interface{}{
			"attestationObject": input.Body.Response.Response.AttestationObject,
			"clientDataJSON":    input.Body.Response.Response.ClientDataJSON,
			"transports":        input.Body.Response.Response.Transports,
		},
	}
	if input.Body.Response.AuthenticatorAttachment != "" {
		response["authenticatorAttachment"] = input.Body.Response.AuthenticatorAttachment
	}
	data, _ := json.Marshal(response)
	return bytes.NewReader(data)
}

func createCredentialRequestReader(input *FinishLoginRequest) io.Reader {
	response := map[string]interface{}{
		"id":    input.Body.Response.ID,
		"rawId": input.Body.Response.RawID,
		"type":  input.Body.Response.Type,
		"response": map[string]interface{}{
			"authenticatorData": input.Body.Response.Response.AuthenticatorData,
			"clientDataJSON":    input.Body.Response.Response.ClientDataJSON,
			"signature":         input.Body.Response.Response.Signature,
			"userHandle":        input.Body.Response.Response.UserHandle,
		},
	}
	data, _ := json.Marshal(response)
	return bytes.NewReader(data)
}

// --- Passkey Signup Endpoints (passkey-first registration) ---

// BeginSignupRequest represents the request to begin passkey signup
type BeginSignupRequest struct {
	Body struct {
		Email string `json:"email" example:"user@example.com" doc:"Email for the new account"`
	}
}

// BeginSignupResponse represents the response with WebAuthn registration options
type BeginSignupResponse struct {
	Body struct {
		Success     bool                         `json:"success" doc:"Success status"`
		Message     string                       `json:"message" doc:"Response message"`
		Options     *protocol.CredentialCreation `json:"options,omitempty" doc:"WebAuthn registration options"`
		ChallengeID string                       `json:"challenge_id,omitempty" doc:"Challenge ID for finishing registration"`
		ErrorCode   string                       `json:"error_code,omitempty" doc:"Error code"`
		RequestID   string                       `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// BeginSignupHandler starts the passkey signup process (creates new account with passkey)
func (h *PasskeyHandler) BeginSignupHandler(ctx context.Context, input *BeginSignupRequest) (*BeginSignupResponse, error) {
	resp := &BeginSignupResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	email := input.Body.Email
	if email == "" {
		resp.Body.Success = false
		resp.Body.Message = "Email is required"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	logger.AuthDebug(ctx, "passkey_signup_begin",
		"email", email,
	)

	// Check if user already exists
	existingUser, err := h.userService.GetUserByEmail(email)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_check_failed", err,
			"email", email,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to check email"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	if existingUser != nil {
		resp.Body.Success = false
		resp.Body.Message = "An account with this email already exists"
		resp.Body.ErrorCode = autherrors.CodeUserExists
		return resp, nil
	}

	// Create a temporary user object for WebAuthn registration
	// We use a placeholder ID (0) since the user doesn't exist yet
	tempUser := &models.User{
		Email: &email,
	}
	// Use a temporary ID based on email hash for WebAuthn
	tempUser.ID = 0

	// Begin registration
	options, session, err := h.webauthnService.BeginRegistrationForEmail(email)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_begin_failed", err,
			"email", email,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to start passkey registration"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Store challenge with email (userID = 0 indicates signup flow)
	challengeID, err := h.webauthnService.StoreChallengeWithEmail(email, session, "signup")
	if err != nil {
		logger.AuthError(ctx, "passkey_challenge_store_failed", err,
			"email", email,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to store challenge"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	resp.Body.Success = true
	resp.Body.Message = "Registration options created"
	resp.Body.Options = options
	resp.Body.ChallengeID = challengeID
	return resp, nil
}

// FinishSignupRequest represents the request to complete passkey signup
type FinishSignupRequest struct {
	Body struct {
		ChallengeID string                     `json:"challenge_id" doc:"Challenge ID from begin signup"`
		DisplayName string                     `json:"display_name" example:"My MacBook" doc:"Name for this passkey"`
		Response    CredentialCreationResponse `json:"response" doc:"The credential response from the browser"`
	}
}

// FinishSignupResponse represents the response after completing signup
type FinishSignupResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool         `json:"success" doc:"Success status"`
		Message   string       `json:"message" doc:"Response message"`
		User      *models.User `json:"user,omitempty" doc:"Created user"`
		ErrorCode string       `json:"error_code,omitempty" doc:"Error code"`
		RequestID string       `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// FinishSignupHandler completes the passkey signup process
func (h *PasskeyHandler) FinishSignupHandler(ctx context.Context, input *FinishSignupRequest) (*FinishSignupResponse, error) {
	resp := &FinishSignupResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "passkey_signup_finish")

	// Get challenge with email
	session, email, err := h.webauthnService.GetChallengeWithEmail(input.Body.ChallengeID, "signup")
	if err != nil {
		logger.AuthWarn(ctx, "passkey_challenge_invalid",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired challenge"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Double-check email isn't already taken (race condition protection)
	existingUser, err := h.userService.GetUserByEmail(email)
	if err != nil {
		resp.Body.Success = false
		resp.Body.Message = "Failed to verify email"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}
	if existingUser != nil {
		resp.Body.Success = false
		resp.Body.Message = "An account with this email already exists"
		resp.Body.ErrorCode = autherrors.CodeUserExists
		return resp, nil
	}

	// Parse the credential response
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(
		createCredentialCreationReaderFromSignup(input),
	)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"email", email,
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credential response"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Get display name
	displayName := input.Body.DisplayName
	if displayName == "" {
		displayName = "My Passkey"
	}

	// Complete registration and create user
	user, err := h.webauthnService.FinishSignupRegistration(email, session, parsedResponse, displayName)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_finish_failed", err,
			"email", email,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to complete signup"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Delete used challenge
	_ = h.webauthnService.DeleteChallenge(input.Body.ChallengeID)

	// Generate JWT token
	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		logger.AuthError(ctx, "passkey_token_generation_failed", err,
			"user_id", user.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to generate authentication token"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// Set cookie
	resp.SetCookie = http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.Session.Secure,
		SameSite: h.config.Session.GetSameSite(),
		Expires:  time.Now().Add(24 * time.Hour),
	}

	logger.AuthInfo(ctx, "passkey_signup_success",
		"user_id", user.ID,
		"email", email,
	)

	resp.Body.Success = true
	resp.Body.Message = "Account created successfully"
	resp.Body.User = user
	return resp, nil
}

func createCredentialCreationReaderFromSignup(input *FinishSignupRequest) io.Reader {
	response := map[string]interface{}{
		"id":    input.Body.Response.ID,
		"rawId": input.Body.Response.RawID,
		"type":  input.Body.Response.Type,
		"response": map[string]interface{}{
			"attestationObject": input.Body.Response.Response.AttestationObject,
			"clientDataJSON":    input.Body.Response.Response.ClientDataJSON,
			"transports":        input.Body.Response.Response.Transports,
		},
	}
	if input.Body.Response.AuthenticatorAttachment != "" {
		response["authenticatorAttachment"] = input.Body.Response.AuthenticatorAttachment
	}
	data, _ := json.Marshal(response)
	return bytes.NewReader(data)
}
