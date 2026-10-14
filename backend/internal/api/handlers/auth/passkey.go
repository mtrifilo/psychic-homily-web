package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// PasskeyHandler handles passkey/WebAuthn requests
type PasskeyHandler struct {
	webauthnService contracts.WebAuthnServiceInterface
	jwtService      contracts.JWTServiceInterface
	userService     contracts.UserServiceInterface
	// emailService exists solely to send the verification email at passkey
	// signup. Passkey-first accounts are created with email_verified=false
	// (webauthn.go FinishSignupRegistrationWithLegal), exactly like password
	// registration, so they need the same link in their inbox.
	emailService contracts.EmailServiceInterface
	config       *config.Config
}

// NewPasskeyHandler creates a new passkey handler
func NewPasskeyHandler(webauthnService contracts.WebAuthnServiceInterface, jwtService contracts.JWTServiceInterface, userService contracts.UserServiceInterface, emailService contracts.EmailServiceInterface, cfg *config.Config) *PasskeyHandler {
	return &PasskeyHandler{
		webauthnService: webauthnService,
		jwtService:      jwtService,
		userService:     userService,
		emailService:    emailService,
		config:          cfg,
	}
}

// defaultPasskeyName is stored for a passkey registered without a usable name.
const defaultPasskeyName = "My Passkey"

// passkeyName is the name a finish handler stores. Finish runs after the
// authenticator has written the credential, so it never refuses on the name:
// an empty value, or one validateProfileName refuses, stores the default.
func passkeyName(raw string) string {
	name, _, ok := validateProfileName("Passkey name", raw)
	if !ok || name == "" {
		return defaultPasskeyName
	}
	return name
}

// --- Registration Endpoints ---

// BeginRegisterRequest represents the request to begin passkey registration
type BeginRegisterRequest struct {
	Body struct {
		DisplayName string `json:"display_name,omitempty" example:"My MacBook" doc:"Name for this passkey; refused when longer than 100 characters or containing control characters"`
	}
}

// BeginRegisterResponse represents the response with WebAuthn registration options
type BeginRegisterResponse struct {
	Body struct {
		Success     bool                         `json:"success" doc:"Success status"`
		Message     string                       `json:"message" doc:"Response message"`
		Options     *protocol.CredentialCreation `json:"options,omitempty" doc:"WebAuthn registration options"`
		ChallengeID string                       `json:"challenge_id,omitempty" doc:"Challenge ID for finishing registration"`
		ErrorCode   string                       `json:"error_code,omitempty" doc:"Error code"`
		RequestID   string                       `json:"request_id,omitempty" doc:"Request ID"`
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

	// A passkey attached here is a permanent, independent way to sign in as
	// this account, which is the same change the OAuth link path is gated for
	// and a larger one than any token mint: an attacker who attaches one owns
	// the account, and signing in with it stamps the freshness every other gate
	// asks for.
	//
	// The refusal belongs at BEGIN, not at finish. Asking again at finish would
	// put it after the user completed the biometric or security-key ceremony,
	// by which point their authenticator has already written the credential:
	// they would be left holding a passkey the server never recorded.
	//
	// What that leaves, stated exactly: no challenge is issued without a recent
	// factor, a challenge lives five minutes (webauthn.go StoreChallenge), and
	// only the user it was issued to can spend it, which the first finish that
	// user presents does (ConsumeRegistrationChallenge). It is NOT
	// bound to the session that passed this gate, so within those five
	// minutes any live session of that account can complete a registration
	// with a challenge some other session obtained. Reaching that needs the
	// challenge id, which is returned only in the response to the request that
	// passed here. Binding the challenge to the authentication time that
	// obtained it would close it and needs a column to store one.
	//
	// Composed, the terms add up: RecentSessionWindow's ten minutes, plus the
	// two the claim reader believes of a future stamp (maxAuthTimeSkew), plus
	// these five. A passkey can land about seventeen minutes after the factor
	// that allowed it.
	if err := middleware.RequireRecentSessionAuth(ctx, "passkey_register"); err != nil {
		return nil, err
	}

	// The name is refused here, before any challenge exists, because finish
	// runs after the authenticator has written the credential.
	if _, errMsg, ok := validateProfileName("Passkey name", input.Body.DisplayName); !ok {
		resp.Body.Success = false
		resp.Body.Message = errMsg
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
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

// FinishRegisterRequest represents the request to complete passkey registration
type FinishRegisterRequest struct {
	Body struct {
		ChallengeID string `json:"challenge_id" doc:"Challenge ID from begin registration"`
		DisplayName string `json:"display_name,omitempty" example:"My MacBook" doc:"Name for this passkey; My Passkey is stored when it is empty or invalid"`
		// Response is the RegistrationResponseJSON the browser client sends
		// (@simplewebauthn/browser startRegistration). It is kept as raw JSON
		// and handed to the WebAuthn library unmodified, so every field the
		// client sends reaches the parser and none is rejected here.
		Response json.RawMessage `json:"response" doc:"The PublicKeyCredential from navigator.credentials.create(), serialized as RegistrationResponseJSON"`
	}
}

// FinishRegisterResponse represents the response after completing registration
type FinishRegisterResponse struct {
	Body struct {
		Success    bool                      `json:"success" doc:"Success status"`
		Message    string                    `json:"message" doc:"Response message"`
		Credential *authm.WebAuthnCredential `json:"credential,omitempty" doc:"Registered credential"`
		ErrorCode  string                    `json:"error_code,omitempty" doc:"Error code"`
		RequestID  string                    `json:"request_id,omitempty" doc:"Request ID"`
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

	session, err := h.webauthnService.ConsumeRegistrationChallenge(input.Body.ChallengeID, contextUser.ID)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_challenge_invalid",
			"user_id", contextUser.ID,
			"error", err.Error(),
		)
		resp.Body.Success = false
		var refusal *autherrors.AuthError
		if errors.As(err, &refusal) && refusal.Code == autherrors.CodeUnauthorized {
			resp.Body.Message = refusal.UserMessage()
			resp.Body.ErrorCode = refusal.Code
			return resp, nil
		}
		resp.Body.Message = "Invalid or expired challenge"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBytes(input.Body.Response)
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

	credential, err := h.webauthnService.FinishRegistration(contextUser, session, parsedResponse, passkeyName(input.Body.DisplayName))
	if err != nil {
		logger.AuthError(ctx, "passkey_register_finish_failed", err,
			"user_id", contextUser.ID,
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to register passkey"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

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
		Success     bool                          `json:"success" doc:"Success status"`
		Message     string                        `json:"message" doc:"Response message"`
		Options     *protocol.CredentialAssertion `json:"options,omitempty" doc:"WebAuthn login options"`
		ChallengeID string                        `json:"challenge_id,omitempty" doc:"Challenge ID for finishing login"`
		ErrorCode   string                        `json:"error_code,omitempty" doc:"Error code"`
		RequestID   string                        `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// refuseEmailPasskeyLogin is the one answer an email-specific login begin
// gives for an address that cannot start a passkey sign-in, whether or not an
// account holds it.
func refuseEmailPasskeyLogin(resp *BeginLoginResponse) {
	resp.Body.Success = false
	resp.Body.Message = "No passkeys registered for this account"
	resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
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
		// User-specific login. An address with no account and an account with
		// no passkey get the same refusal. An account with a passkey gets
		// options naming its credentials.
		user, err := h.userService.GetUserByEmail(input.Body.Email)
		if err != nil {
			resp.Body.Success = false
			resp.Body.Message = "Invalid credentials"
			resp.Body.ErrorCode = autherrors.CodeInvalidCredentials
			return resp, nil
		}
		if user == nil {
			logger.AuthDebug(ctx, "passkey_login_begin_failed",
				"error", "no account for this address",
			)
			refuseEmailPasskeyLogin(resp)
			return resp, nil
		}

		opts, sess, err := h.webauthnService.BeginLogin(user)
		if err != nil {
			logger.AuthDebug(ctx, "passkey_login_begin_failed",
				"error", err.Error(),
			)
			refuseEmailPasskeyLogin(resp)
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

// FinishLoginRequest represents the request to complete passkey login
type FinishLoginRequest struct {
	Body struct {
		ChallengeID string `json:"challenge_id" doc:"Challenge ID from begin login"`
		// Response is the AuthenticationResponseJSON the browser client sends
		// (@simplewebauthn/browser startAuthentication), kept raw and handed to
		// the WebAuthn library unmodified.
		Response json.RawMessage `json:"response" doc:"The PublicKeyCredential from navigator.credentials.get(), serialized as AuthenticationResponseJSON"`
	}
}

// FinishLoginResponse represents the response after completing login
type FinishLoginResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool        `json:"success" doc:"Success status"`
		Message   string      `json:"message" doc:"Response message"`
		User      *authm.User `json:"user,omitempty" doc:"Authenticated user"`
		ErrorCode string      `json:"error_code,omitempty" doc:"Error code"`
		RequestID string      `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// FinishLoginHandler completes the passkey login process
func (h *PasskeyHandler) FinishLoginHandler(ctx context.Context, input *FinishLoginRequest) (*FinishLoginResponse, error) {
	resp := &FinishLoginResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "passkey_login_finish")

	session, userID, err := h.webauthnService.ConsumeChallenge(input.Body.ChallengeID, "authentication")
	if err != nil {
		logger.AuthWarn(ctx, "passkey_challenge_invalid",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired challenge"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBytes(input.Body.Response)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credential response"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	var user *authm.User

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
		// The service answers a refusal as an AuthError carrying its code and
		// message, and anything else means it reached no decision. A code with
		// no case here is treated as the latter.
		var refusal *autherrors.AuthError
		if errors.As(err, &refusal) {
			switch refusal.Code {
			case autherrors.CodeInvalidCredentials, autherrors.CodeAccountInactive:
				logger.AuthWarn(ctx, "passkey_login_refused",
					"error_code", refusal.Code,
					"error", err.Error(),
				)
				resp.Body.Success = false
				resp.Body.Message = refusal.UserMessage()
				resp.Body.ErrorCode = refusal.Code
				return resp, nil
			}
		}
		logger.AuthError(ctx, "passkey_login_finish_failed", err)
		return nil, autherrors.ErrServiceUnavailable("passkey_login_finish", err)
	}

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
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

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
		Credentials []authm.WebAuthnCredential `json:"credentials" doc:"User's passkey credentials"`
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

// --- Passkey Signup Endpoints (passkey-first registration) ---

// BeginSignupRequest represents the request to begin passkey signup
type BeginSignupRequest struct {
	Body struct {
		Email          string `json:"email" example:"user@example.com" doc:"Email for the new account"`
		TermsAccepted  bool   `json:"terms_accepted" doc:"Whether user accepted Terms of Service"`
		TermsVersion   string `json:"terms_version,omitempty" doc:"Accepted Terms of Service version; anything other than the current version is refused with TERMS_ACCEPTANCE_REQUIRED"`
		PrivacyVersion string `json:"privacy_version,omitempty" doc:"Accepted Privacy Policy version; anything other than the current version is refused with TERMS_ACCEPTANCE_REQUIRED"`
		AgeConfirmed   bool   `json:"age_confirmed" doc:"Whether user confirmed they meet the minimum age"`
		MinAgeAttested int    `json:"min_age_attested,omitempty" doc:"Minimum age the user attested to"`
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
	// FinishSignupHandler writes the users row from the address this handler
	// stores in the challenge, never from its own request body, so the guard
	// belongs here.
	if errMsg, ok := validateEmailAddress(email); !ok {
		resp.Body.Success = false
		resp.Body.Message = errMsg
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}
	if !input.Body.TermsAccepted {
		resp.Body.Success = false
		resp.Body.Message = "You must accept the Terms of Service and Privacy Policy"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}
	if refusal := refuseStaleConsentVersions(input.Body.TermsVersion, input.Body.PrivacyVersion); refusal != nil {
		logger.AuthWarn(ctx, "passkey_signup_begin_consent_refused",
			"email_hash", logger.HashEmail(email),
			"error_code", refusal.Code,
		)
		resp.Body.Success = false
		resp.Body.Message = refusal.UserMessage()
		resp.Body.ErrorCode = refusal.Code
		return resp, nil
	}
	if errCode, errMsg, ok := validateSignupAgeConfirmation(input.Body.AgeConfirmed, input.Body.MinAgeAttested); !ok {
		resp.Body.Success = false
		resp.Body.Message = errMsg
		resp.Body.ErrorCode = errCode
		return resp, nil
	}
	acceptedAt := time.Now().UTC()
	acceptance := contracts.LegalAcceptance{
		TermsAcceptedAt: acceptedAt,
		TermsVersion:    input.Body.TermsVersion,
		PrivacyVersion:  input.Body.PrivacyVersion,
		AgeConfirmedAt:  acceptedAt,
		MinAgeAttested:  input.Body.MinAgeAttested,
	}

	logger.AuthDebug(ctx, "passkey_signup_begin",
		"email_hash", logger.HashEmail(email),
	)

	// Check if user already exists
	existingUser, err := h.userService.GetUserByEmail(email)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_check_failed", err,
			"email_hash", logger.HashEmail(email),
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

	options, session, err := h.webauthnService.BeginRegistrationForEmail(email)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_begin_failed", err,
			"email_hash", logger.HashEmail(email),
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to start passkey registration"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

	// FinishSignupHandler creates the account from this address and records
	// this consent, both read back from the challenge.
	challengeID, err := h.webauthnService.StoreSignupChallenge(
		contracts.SignupChallenge{Email: email, Acceptance: acceptance}, session, "signup",
	)
	if err != nil {
		logger.AuthError(ctx, "passkey_challenge_store_failed", err,
			"email_hash", logger.HashEmail(email),
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
		ChallengeID string `json:"challenge_id" doc:"Challenge ID from begin signup"`
		DisplayName string `json:"display_name,omitempty" example:"My MacBook" doc:"Name for this passkey; My Passkey is stored when it is empty or invalid"`
		// Response is the RegistrationResponseJSON the browser client sends
		// (@simplewebauthn/browser startRegistration), kept raw and handed to
		// the WebAuthn library unmodified.
		Response json.RawMessage `json:"response" doc:"The PublicKeyCredential from navigator.credentials.create(), serialized as RegistrationResponseJSON"`
		// The consent fields are accepted and ignored: the account records the
		// consent signup/begin validated, read back from the challenge.
		TermsAccepted  bool   `json:"terms_accepted" doc:"Ignored; the consent recorded is the one signup/begin accepted"`
		TermsVersion   string `json:"terms_version,omitempty" doc:"Ignored; the consent recorded is the one signup/begin accepted"`
		PrivacyVersion string `json:"privacy_version,omitempty" doc:"Ignored; the consent recorded is the one signup/begin accepted"`
		AgeConfirmed   bool   `json:"age_confirmed" doc:"Ignored; the consent recorded is the one signup/begin accepted"`
		MinAgeAttested int    `json:"min_age_attested,omitempty" doc:"Ignored; the consent recorded is the one signup/begin accepted"`
	}
}

// FinishSignupResponse represents the response after completing signup
type FinishSignupResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool        `json:"success" doc:"Success status"`
		Message   string      `json:"message" doc:"Response message"`
		User      *authm.User `json:"user,omitempty" doc:"Created user"`
		ErrorCode string      `json:"error_code,omitempty" doc:"Error code"`
		RequestID string      `json:"request_id,omitempty" doc:"Request ID"`
	}
}

// FinishSignupHandler completes the passkey signup process
func (h *PasskeyHandler) FinishSignupHandler(ctx context.Context, input *FinishSignupRequest) (*FinishSignupResponse, error) {
	resp := &FinishSignupResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "passkey_signup_finish")

	session, signup, err := h.webauthnService.ConsumeSignupChallenge(input.Body.ChallengeID, "signup")
	if err != nil {
		logger.AuthWarn(ctx, "passkey_challenge_invalid",
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid or expired challenge"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}
	email := signup.Email
	// The challenge's consent passed these checks at begin. A challenge that
	// carries none, or carries versions that stopped being current since,
	// fails them here.
	if refusal := refuseStaleConsentVersions(signup.Acceptance.TermsVersion, signup.Acceptance.PrivacyVersion); refusal != nil {
		logger.AuthWarn(ctx, "passkey_signup_finish_consent_refused",
			"email_hash", logger.HashEmail(email),
			"error_code", refusal.Code,
		)
		resp.Body.Success = false
		resp.Body.Message = refusal.UserMessage()
		resp.Body.ErrorCode = refusal.Code
		return resp, nil
	}
	if errCode, errMsg, ok := validateSignupAgeConfirmation(!signup.Acceptance.AgeConfirmedAt.IsZero(), signup.Acceptance.MinAgeAttested); !ok {
		logger.AuthWarn(ctx, "passkey_signup_finish_consent_refused",
			"email_hash", logger.HashEmail(email),
			"error_code", errCode,
		)
		resp.Body.Success = false
		resp.Body.Message = errMsg
		resp.Body.ErrorCode = errCode
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

	parsedResponse, err := protocol.ParseCredentialCreationResponseBytes(input.Body.Response)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"email_hash", logger.HashEmail(email),
			"error", err.Error(),
		)
		resp.Body.Success = false
		resp.Body.Message = "Invalid credential response"
		resp.Body.ErrorCode = autherrors.CodeValidationFailed
		return resp, nil
	}

	// Complete registration and create user
	user, err := h.webauthnService.FinishSignupRegistrationWithLegal(
		email,
		session,
		parsedResponse,
		passkeyName(input.Body.DisplayName),
		signup.Acceptance,
	)
	if err != nil {
		// A duplicate address here is the same condition the pre-check above
		// reports as USER_EXISTS, reached by losing a race instead of by
		// failing the check. It gets the same answer, so the response does not
		// depend on which of the two saw it.
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) && authErr.Code == autherrors.CodeUserExists {
			logger.AuthWarn(ctx, "passkey_signup_finish_user_exists",
				"email_hash", logger.HashEmail(email),
				"error_code", autherrors.CodeUserExists,
			)
			resp.Body.Success = false
			resp.Body.Message = authErr.UserMessage()
			resp.Body.ErrorCode = autherrors.CodeUserExists
			return resp, nil
		}
		logger.AuthError(ctx, "passkey_signup_finish_failed", err,
			"email_hash", logger.HashEmail(email),
		)
		resp.Body.Success = false
		resp.Body.Message = "Failed to complete signup"
		resp.Body.ErrorCode = autherrors.CodeServiceUnavailable
		return resp, nil
	}

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
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)

	logger.AuthInfo(ctx, "passkey_signup_success",
		"user_id", user.ID,
		"email_hash", logger.HashEmail(email),
	)

	// Same rationale as password registration: the account is created
	// unverified and show submission is gated on email_verified, so the
	// verification link has to reach the inbox. Best-effort — the account and
	// session already exist by this point.
	sendVerificationEmailBestEffort(ctx, h.jwtService, h.emailService, user, verificationTriggerPasskeySignup)

	resp.Body.Success = true
	resp.Body.Message = "Account created successfully"
	resp.Body.User = user
	return resp, nil
}
