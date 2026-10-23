package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"

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

// passkeyRefusal returns err's AuthError when its code is one of codes, the
// refusals the calling step answers with their own code and message. Any
// other error, an AuthError with another code included, is a failure the
// caller answers with a 5xx, so a code a step does not expect is never shown
// to the client as a refusal.
func passkeyRefusal(err error, codes ...string) (*autherrors.AuthError, bool) {
	var refusal *autherrors.AuthError
	if !errors.As(err, &refusal) {
		return nil, false
	}
	for _, code := range codes {
		if refusal.Code == code {
			return refusal, true
		}
	}
	return nil, false
}

// passkeyResponse is a passkey endpoint's response. setRefusal writes a
// refused step into its body: success false, with the code and message given.
type passkeyResponse interface {
	setRefusal(code, message string)
}

// refusePasskey answers a refused step with resp carrying code and message,
// which the API layer writes as a 200. A failure is never answered this way:
// it returns autherrors.ErrServiceUnavailable, which the API layer writes as
// a 503.
func refusePasskey[R passkeyResponse](resp R, code, message string) (R, error) {
	resp.setRefusal(code, message)
	return resp, nil
}

func (r *BeginRegisterResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *FinishRegisterResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *BeginLoginResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *FinishLoginResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *ListCredentialsResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *DeleteCredentialResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *BeginSignupResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

func (r *FinishSignupResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

// refuseWith answers a refused step with refusal's code and user message.
func refuseWith[R passkeyResponse](resp R, refusal *autherrors.AuthError) (R, error) {
	return refusePasskey(resp, refusal.Code, refusal.UserMessage())
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
		return refusePasskey(resp, autherrors.CodeUnauthorized, "Authentication required")
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
		return refusePasskey(resp, autherrors.CodeValidationFailed, errMsg)
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
		return nil, autherrors.ErrServiceUnavailable("passkey_register_begin", err)
	}

	// Store challenge
	challengeID, err := h.webauthnService.StoreChallenge(contextUser.ID, session, contracts.WebAuthnRegistrationOperation)
	if err != nil {
		logger.AuthError(ctx, "passkey_challenge_store_failed", err,
			"user_id", contextUser.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_register_challenge_store", err)
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
		return refusePasskey(resp, autherrors.CodeUnauthorized, "Authentication required")
	}

	logger.AuthDebug(ctx, "passkey_register_finish",
		"user_id", contextUser.ID,
	)

	session, err := h.webauthnService.ConsumeRegistrationChallenge(input.Body.ChallengeID, contextUser.ID)
	if err != nil {
		if refusal, ok := passkeyRefusal(err, autherrors.CodeUnauthorized, autherrors.CodeValidationFailed); ok {
			logger.AuthWarn(ctx, "passkey_challenge_refused",
				"user_id", contextUser.ID,
				"error_code", refusal.Code,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_challenge_consume_failed", err,
			"user_id", contextUser.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_register_challenge", err)
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBytes(input.Body.Response)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"user_id", contextUser.ID,
			"error", err.Error(),
		)
		return refuseWith(resp, autherrors.ErrInvalidCredentialResponse(err))
	}

	credential, err := h.webauthnService.FinishRegistration(contextUser, session, parsedResponse, passkeyName(input.Body.DisplayName))
	if err != nil {
		if refusal, ok := passkeyRefusal(err, autherrors.CodeValidationFailed, autherrors.CodeAccountInactive); ok {
			logger.AuthWarn(ctx, "passkey_register_refused",
				"user_id", contextUser.ID,
				"error_code", refusal.Code,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_register_finish_failed", err,
			"user_id", contextUser.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_register_finish", err)
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

// BeginLoginRequest represents the request to begin a discoverable passkey
// login. Its body declares no fields, so the schema refuses an email, like any
// field it does not declare, with a 422.
type BeginLoginRequest struct {
	Body struct{}
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

// BeginLoginHandler starts a discoverable (usernameless) passkey login: the
// account is known only once login/finish validates the assertion. A failure
// at any step answers 503 with no challenge issued.
func (h *PasskeyHandler) BeginLoginHandler(ctx context.Context, input *BeginLoginRequest) (*BeginLoginResponse, error) {
	resp := &BeginLoginResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	options, session, err := h.webauthnService.BeginDiscoverableLogin()
	if err != nil {
		logger.AuthError(ctx, "passkey_discoverable_login_begin_failed", err)
		return nil, autherrors.ErrServiceUnavailable("passkey_login_begin", err)
	}

	// The challenge names no account; FinishDiscoverableLogin finds the
	// account from the credential the assertion carries.
	challengeID, err := h.webauthnService.StoreChallenge(0, session, "authentication")
	if err != nil {
		logger.AuthError(ctx, "passkey_challenge_store_failed", err)
		return nil, autherrors.ErrServiceUnavailable("passkey_login_challenge_store", err)
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

	session, err := h.webauthnService.ConsumeChallenge(input.Body.ChallengeID, "authentication")
	if err != nil {
		if refusal, ok := passkeyRefusal(err, autherrors.CodeValidationFailed); ok {
			logger.AuthWarn(ctx, "passkey_challenge_refused",
				"error_code", refusal.Code,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_challenge_consume_failed", err)
		return nil, autherrors.ErrServiceUnavailable("passkey_login_challenge", err)
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBytes(input.Body.Response)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"error", err.Error(),
		)
		return refuseWith(resp, autherrors.ErrInvalidCredentialResponse(err))
	}

	// The service decides the sign-in, including whether the account exists
	// and may sign in, and returns the account it decided on.
	user, _, err := h.webauthnService.FinishDiscoverableLogin(session, parsedResponse)
	if err != nil {
		if refusal, ok := passkeyRefusal(err, autherrors.CodeInvalidCredentials, autherrors.CodeAccountInactive, autherrors.CodeAccountRecoverable); ok {
			logger.AuthWarn(ctx, "passkey_login_refused",
				"error_code", refusal.Code,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_login_finish_failed", err)
		return nil, autherrors.ErrServiceUnavailable("passkey_login_finish", err)
	}

	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		logger.AuthError(ctx, "passkey_token_generation_failed", err,
			"user_id", user.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_login_token", err)
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
		return refusePasskey(resp, autherrors.CodeUnauthorized, "Authentication required")
	}

	credentials, err := h.webauthnService.GetUserCredentials(contextUser.ID)
	if err != nil {
		logger.AuthError(ctx, "passkey_list_failed", err,
			"user_id", contextUser.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_list_credentials", err)
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
		return refusePasskey(resp, autherrors.CodeUnauthorized, "Authentication required")
	}

	err := h.webauthnService.DeleteCredential(contextUser.ID, input.CredentialID)
	if err != nil {
		if refusal, ok := passkeyRefusal(err, autherrors.CodeValidationFailed); ok {
			logger.AuthWarn(ctx, "passkey_delete_refused",
				"user_id", contextUser.ID,
				"credential_id", input.CredentialID,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_delete_failed", err,
			"user_id", contextUser.ID,
			"credential_id", input.CredentialID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_delete_credential", err)
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
		TermsVersion   string `json:"terms_version,omitempty" doc:"Accepted Terms of Service version: the current one, or the previous one while a grace window is open (both published at GET /legal/versions); anything else is refused with TERMS_ACCEPTANCE_REQUIRED"`
		PrivacyVersion string `json:"privacy_version,omitempty" doc:"Accepted Privacy Policy version: the current one, or the previous one while a grace window is open (both published at GET /legal/versions); anything else is refused with TERMS_ACCEPTANCE_REQUIRED"`
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
		return refusePasskey(resp, autherrors.CodeValidationFailed, "Email is required")
	}
	// FinishSignupHandler writes the users row from the address this handler
	// stores in the challenge, never from its own request body, so the guard
	// belongs here.
	if errMsg, ok := validateEmailAddress(email); !ok {
		return refusePasskey(resp, autherrors.CodeValidationFailed, errMsg)
	}
	if !input.Body.TermsAccepted {
		return refusePasskey(resp, autherrors.CodeValidationFailed, "You must accept the Terms of Service and Privacy Policy")
	}
	if refusal := refuseStaleConsentVersions(input.Body.TermsVersion, input.Body.PrivacyVersion); refusal != nil {
		logger.AuthWarn(ctx, "passkey_signup_begin_consent_refused",
			"email_hash", logger.HashEmail(email),
			"error_code", refusal.Code,
		)
		return refuseWith(resp, refusal)
	}
	if errCode, errMsg, ok := validateSignupAgeConfirmation(input.Body.AgeConfirmed, input.Body.MinAgeAttested); !ok {
		return refusePasskey(resp, errCode, errMsg)
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
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_begin_email_check", err)
	}

	if existingUser != nil {
		return refusePasskey(resp, autherrors.CodeUserExists, "An account with this email already exists")
	}

	options, session, err := h.webauthnService.BeginRegistrationForEmail(email)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_begin_failed", err,
			"email_hash", logger.HashEmail(email),
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_begin", err)
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
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_challenge_store", err)
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

// FinishSignupHandler completes the passkey signup process. A refusal answers
// 200 with its code; a failure at any step answers 503 with no session.
func (h *PasskeyHandler) FinishSignupHandler(ctx context.Context, input *FinishSignupRequest) (*FinishSignupResponse, error) {
	resp := &FinishSignupResponse{}
	requestID := logger.GetRequestID(ctx)
	resp.Body.RequestID = requestID

	logger.AuthDebug(ctx, "passkey_signup_finish")

	session, signup, err := h.webauthnService.ConsumeSignupChallenge(input.Body.ChallengeID, "signup")
	if err != nil {
		if refusal, ok := passkeyRefusal(err, autherrors.CodeValidationFailed); ok {
			logger.AuthWarn(ctx, "passkey_challenge_refused",
				"error_code", refusal.Code,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_challenge_consume_failed", err)
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_challenge", err)
	}
	email := signup.Email
	// The challenge's consent passed these checks at begin. A challenge that
	// carries none, or carries versions that stopped being accepted since,
	// fails them here.
	if refusal := refuseStaleConsentVersions(signup.Acceptance.TermsVersion, signup.Acceptance.PrivacyVersion); refusal != nil {
		logger.AuthWarn(ctx, "passkey_signup_finish_consent_refused",
			"email_hash", logger.HashEmail(email),
			"error_code", refusal.Code,
		)
		return refuseWith(resp, refusal)
	}
	if errCode, errMsg, ok := validateSignupAgeConfirmation(!signup.Acceptance.AgeConfirmedAt.IsZero(), signup.Acceptance.MinAgeAttested); !ok {
		logger.AuthWarn(ctx, "passkey_signup_finish_consent_refused",
			"email_hash", logger.HashEmail(email),
			"error_code", errCode,
		)
		return refusePasskey(resp, errCode, errMsg)
	}

	// Double-check email isn't already taken (race condition protection)
	existingUser, err := h.userService.GetUserByEmail(email)
	if err != nil {
		logger.AuthError(ctx, "passkey_signup_check_failed", err,
			"email_hash", logger.HashEmail(email),
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_email_check", err)
	}
	if existingUser != nil {
		return refusePasskey(resp, autherrors.CodeUserExists, "An account with this email already exists")
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBytes(input.Body.Response)
	if err != nil {
		logger.AuthWarn(ctx, "passkey_response_parse_failed",
			"email_hash", logger.HashEmail(email),
			"error", err.Error(),
		)
		return refuseWith(resp, autherrors.ErrInvalidCredentialResponse(err))
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
		if refusal, ok := passkeyRefusal(err, autherrors.CodeUserExists, autherrors.CodeValidationFailed); ok {
			logger.AuthWarn(ctx, "passkey_signup_finish_refused",
				"email_hash", logger.HashEmail(email),
				"error_code", refusal.Code,
				"error", err.Error(),
			)
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "passkey_signup_finish_failed", err,
			"email_hash", logger.HashEmail(email),
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_finish", err)
	}

	token, err := h.jwtService.CreateToken(user)
	if err != nil {
		logger.AuthError(ctx, "passkey_token_generation_failed", err,
			"user_id", user.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("passkey_signup_token", err)
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
