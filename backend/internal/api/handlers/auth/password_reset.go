package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// The emailed password reset routes. A request sends the link; the check and
// the completion take its token.
const (
	PasswordResetRequestRoute  = "/auth/password-reset/request"
	PasswordResetSendOwnRoute  = "/auth/password-reset/send-own"
	PasswordResetCheckRoute    = "/auth/password-reset/check"
	PasswordResetCompleteRoute = "/auth/password-reset/complete"
)

// The reset operations' names in the logs and 5xx errors.
const (
	passwordResetCheckOperation    = "password_reset_check"
	passwordResetCompleteOperation = "password_reset_complete"
)

// passwordResetLinkInvalidCode is the refusal for a reset link a new link
// would fix: malformed, expired, already used, or minted before the account's
// password or address changed. It is the code the magic-link and recovery
// pages read as an expired link.
const passwordResetLinkInvalidCode = "INVALID_TOKEN"

// passwordResetLinkInvalidMessage goes with passwordResetLinkInvalidCode.
const passwordResetLinkInvalidMessage = "This password reset link no longer works. Request a new one."

// passwordResetSentMessage answers every accepted request, whether or not an
// account uses the address.
const passwordResetSentMessage = "If an account uses this email, a password reset link has been sent."

// RequestPasswordResetRequest is POST /auth/password-reset/request.
type RequestPasswordResetRequest struct {
	Body struct {
		Email string `json:"email" example:"user@example.com" doc:"Address to send the password reset link to"`
	}
}

// RequestPasswordResetResponse answers a reset request. Every request with a
// well-formed address is a 202 with the same body, whether or not an account
// uses the address and whatever state that account is in.
type RequestPasswordResetResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"Always true: the response never says whether an account uses the address"`
		Message   string `json:"message" example:"If an account uses this email, a password reset link has been sent." doc:"Response message"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// RequestPasswordResetHandler queues the emailed password reset link for the
// account using the address, when one does and it may sign in.
//
// Enumeration-safe: every path after the address check answers the same 202
// body in about the same time (the email is queued, not sent). A malformed
// address is refused with a 400 and an unconfigured email provider with a
// 503; neither depends on any account.
func (h *AuthHandler) RequestPasswordResetHandler(ctx context.Context, input *RequestPasswordResetRequest) (*RequestPasswordResetResponse, error) {
	email := input.Body.Email
	if email == "" {
		return nil, autherrors.ErrValidationFailed("Email is required")
	}
	if message, ok := validateEmailAddress(email); !ok {
		return nil, autherrors.ErrValidationFailed(message)
	}
	if !h.emailService.IsConfigured() {
		logger.AuthError(ctx, "password_reset_request_email_not_configured", nil)
		return nil, autherrors.ErrServiceUnavailable("password_reset_request", errors.New("email service not configured"))
	}

	resp := &RequestPasswordResetResponse{}
	resp.Body.Success = true
	resp.Body.Message = passwordResetSentMessage
	resp.Body.RequestID = logger.GetRequestID(ctx)

	user, err := h.userService.GetUserByEmail(email)
	if err != nil {
		logger.AuthError(ctx, "password_reset_request_lookup_failed", err, "email_hash", logger.HashEmail(email))
		return resp, nil
	}
	if user == nil {
		logger.AuthDebug(ctx, "password_reset_request_user_not_found", "email_hash", logger.HashEmail(email))
		return resp, nil
	}
	// The outbox checks again when it sends; this keeps a closed account from
	// queueing a row at all.
	if !user.CanSignIn() {
		logger.AuthInfo(ctx, "password_reset_request_account_closed", "user_id", user.ID)
		return resp, nil
	}
	if err := h.emailOutbox.Enqueue(ctx, authm.AuthEmailKindPasswordReset, user.ID); err != nil {
		logger.AuthError(ctx, "password_reset_request_enqueue_failed", err, "user_id", user.ID)
		return resp, nil
	}
	logger.AuthInfo(ctx, "password_reset_request_queued", "user_id", user.ID, "email_hash", logger.HashEmail(email))
	return resp, nil
}

// SendOwnPasswordResetResponse answers POST /auth/password-reset/send-own:
// a 202 once the link to the signed-in account's own address is queued.
type SendOwnPasswordResetResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"The reset link is queued for the account's own address"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// SendOwnPasswordResetHandler queues the emailed password reset link for the
// signed-in account, to its own address. Sending changes nothing about the
// account, so it asks for no recent authentication. The caller is the
// account, so a failure to queue is reported (503) rather than hidden.
func (h *AuthHandler) SendOwnPasswordResetHandler(ctx context.Context, _ *struct{}) (*SendOwnPasswordResetResponse, error) {
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "password_reset_send_own_no_user")
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if contextUser.Email == nil || *contextUser.Email == "" {
		logger.AuthWarn(ctx, "password_reset_send_own_no_email", "user_id", contextUser.ID)
		return nil, autherrors.ErrValidationFailed("User does not have an email address")
	}
	if !h.emailService.IsConfigured() {
		logger.AuthError(ctx, "password_reset_send_own_email_not_configured", nil, "user_id", contextUser.ID)
		return nil, autherrors.ErrServiceUnavailable("password_reset_send_own", errors.New("email service not configured"))
	}
	if err := h.emailOutbox.Enqueue(ctx, authm.AuthEmailKindPasswordReset, contextUser.ID); err != nil {
		logger.AuthError(ctx, "password_reset_send_own_enqueue_failed", err, "user_id", contextUser.ID)
		return nil, autherrors.ErrServiceUnavailable("password_reset_send_own", err)
	}

	logger.AuthInfo(ctx, "password_reset_send_own_queued", "user_id", contextUser.ID)
	resp := &SendOwnPasswordResetResponse{}
	resp.Body.Success = true
	resp.Body.RequestID = logger.GetRequestID(ctx)
	return resp, nil
}

// CheckPasswordResetRequest is POST /auth/password-reset/check.
type CheckPasswordResetRequest struct {
	Body struct {
		Token string `json:"token" doc:"Password reset token from the emailed link"`
	}
}

// CheckPasswordResetResponse says whether a reset link may still set a
// password, and for which address. A refusal is a 200 with success false and
// its code: INVALID_TOKEN for a link a new one would fix, and the closed-account
// codes (ACCOUNT_INACTIVE, ACCOUNT_RECOVERABLE) otherwise.
type CheckPasswordResetResponse struct {
	Body struct {
		Success   bool   `json:"success" example:"true" doc:"The link may still set a new password"`
		Message   string `json:"message,omitempty" doc:"Refusal message"`
		Email     string `json:"email,omitempty" example:"user@example.com" doc:"The address of the account the link resets"`
		ErrorCode string `json:"error_code,omitempty" example:"INVALID_TOKEN" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

func (r *CheckPasswordResetResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

// CheckPasswordResetHandler answers whether a reset link may still set a
// password. It writes nothing, so opening the link, or a mail scanner
// fetching it, spends nothing.
func (h *AuthHandler) CheckPasswordResetHandler(ctx context.Context, input *CheckPasswordResetRequest) (*CheckPasswordResetResponse, error) {
	resp := &CheckPasswordResetResponse{}
	resp.Body.RequestID = logger.GetRequestID(ctx)

	claims, refused := h.passwordResetClaims(ctx, input.Body.Token)
	if refused != nil {
		return refuseWith(resp, refused)
	}
	account, err := h.authService.PasswordResetAccount(claims)
	if err != nil {
		if refusal, ok := passwordResetRefusal(ctx, passwordResetCheckOperation, claims.UserID, err); ok {
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "password_reset_check_failed", err, "user_id", claims.UserID)
		return nil, autherrors.ErrServiceUnavailable(passwordResetCheckOperation, err)
	}

	resp.Body.Success = true
	resp.Body.Email = *account.Email
	return resp, nil
}

// CompletePasswordResetRequest is POST /auth/password-reset/complete.
type CompletePasswordResetRequest struct {
	Body struct {
		Token       string `json:"token" doc:"Password reset token from the emailed link"`
		NewPassword string `json:"new_password" doc:"The account's new password"`
	}
}

// CompletePasswordResetResponse answers a reset. Success carries this
// browser's new session in Set-Cookie. A refusal is a 200 with success false
// and its code: INVALID_TOKEN for a link a new one would fix,
// VALIDATION_FAILED for a password the validator refuses, and the
// closed-account codes. A failure that is not the caller's is a 5xx, and the
// link is still unspent.
type CompletePasswordResetResponse struct {
	// Zero on every path but success, which huma writes as an empty
	// Set-Cookie header that clears nothing (see ChangePasswordResponse).
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool        `json:"success" example:"true" doc:"Success status"`
		Message   string      `json:"message" example:"Password reset" doc:"Response message"`
		User      *authm.User `json:"user,omitempty" doc:"The account, signed in on this browser"`
		ErrorCode string      `json:"error_code,omitempty" example:"INVALID_TOKEN" doc:"Error code for programmatic handling"`
		RequestID string      `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

func (r *CompletePasswordResetResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

// CompletePasswordResetHandler sets a new password with an emailed reset
// link, replacing whatever password the account holds, signs every other
// session out and signs this browser in. Passkeys stay.
func (h *AuthHandler) CompletePasswordResetHandler(ctx context.Context, input *CompletePasswordResetRequest) (*CompletePasswordResetResponse, error) {
	resp := &CompletePasswordResetResponse{}
	resp.Body.RequestID = logger.GetRequestID(ctx)

	claims, refused := h.passwordResetClaims(ctx, input.Body.Token)
	if refused != nil {
		return refuseWith(resp, refused)
	}
	if input.Body.NewPassword == "" {
		return refusePasskey(resp, autherrors.CodeValidationFailed, "New password is required")
	}

	// A link that can no longer set the password is refused before the
	// validator's outbound breach check and the hash. CompletePasswordReset
	// decides it again under the row lock.
	if _, err := h.authService.PasswordResetAccount(claims); err != nil {
		if refusal, ok := passwordResetRefusal(ctx, passwordResetCompleteOperation, claims.UserID, err); ok {
			return refuseWith(resp, refusal)
		}
		logger.AuthError(ctx, "password_reset_complete_failed", err, "user_id", claims.UserID)
		return nil, autherrors.ErrServiceUnavailable(passwordResetCompleteOperation, err)
	}

	if refusal, err := h.newPasswordRefusal(ctx, passwordResetCompleteOperation, claims.UserID, input.Body.NewPassword); err != nil {
		return nil, err
	} else if refusal != "" {
		return refusePasskey(resp, autherrors.CodeValidationFailed, refusal)
	}

	completion, err := h.authService.CompletePasswordReset(claims, input.Body.NewPassword)
	if err != nil {
		if refusal, ok := passwordResetRefusal(ctx, passwordResetCompleteOperation, claims.UserID, err); ok {
			return refuseWith(resp, refusal)
		}
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) && authErr.Code == autherrors.CodeValidationFailed {
			logger.AuthWarn(ctx, "password_reset_complete_refused", "user_id", claims.UserID, "error_code", authErr.Code)
			return refuseWith(resp, authErr)
		}
		logger.AuthError(ctx, "password_reset_complete_failed", err, "user_id", claims.UserID)
		return nil, autherrors.ErrServiceUnavailable(passwordResetCompleteOperation, err)
	}

	// 24 hours, as the magic link's and the recovery link's sessions.
	resp.SetCookie = h.config.Session.NewAuthCookie(completion.Token, 24*time.Hour)
	logger.AuthInfo(ctx, "password_reset_complete_success", "user_id", completion.User.ID)
	resp.Body.Success = true
	resp.Body.Message = "Password reset"
	resp.Body.User = completion.User
	return resp, nil
}

// passwordResetClaims reads a reset link's token, or the refusal to answer it
// with: VALIDATION_FAILED for no token, INVALID_TOKEN for one that is not a
// live reset link.
func (h *AuthHandler) passwordResetClaims(ctx context.Context, token string) (*contracts.PasswordResetTokenClaims, *autherrors.AuthError) {
	if token == "" {
		return nil, autherrors.ErrValidationFailed("Password reset token is required")
	}
	claims, err := h.jwtService.ValidatePasswordResetToken(token)
	if err != nil {
		logger.AuthWarn(ctx, "password_reset_invalid_token", "error", err.Error())
		return nil, autherrors.NewAuthError(passwordResetLinkInvalidCode, passwordResetLinkInvalidMessage, err)
	}
	return claims, nil
}

// passwordResetRefusal maps a refusal from PasswordResetAccount or
// CompletePasswordReset that the reader can act on to the AuthError the
// response carries: INVALID_TOKEN for a spent link, and the closed-account
// refusal as the service gave it.
func passwordResetRefusal(ctx context.Context, event string, userID uint, err error) (*autherrors.AuthError, bool) {
	if errors.Is(err, contracts.ErrPasswordResetLinkSpent) {
		logger.AuthWarn(ctx, event+"_link_spent", "user_id", userID, "error", err.Error())
		return autherrors.NewAuthError(passwordResetLinkInvalidCode, passwordResetLinkInvalidMessage, err), true
	}
	var authErr *autherrors.AuthError
	if errors.As(err, &authErr) && (authErr.Code == autherrors.CodeAccountInactive || authErr.Code == autherrors.CodeAccountRecoverable) {
		logger.AuthWarn(ctx, event+"_account_closed", "user_id", userID, "error_code", authErr.Code)
		return authErr, true
	}
	return nil, false
}
