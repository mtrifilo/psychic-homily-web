package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/shared"
)

// setPasswordOperation names the set-password write in the re-authentication
// and session-refusal logs.
const setPasswordOperation = "set_password"

// setPasswordOriginRefusal is the 403 detail for a request that does not come
// from the frontend.
const setPasswordOriginRefusal = "Set a password from Settings"

// SetPasswordRequest is POST /auth/password. There is no current password to
// confirm: the route serves only accounts that have none. Origin and Referer
// are the browser's account of where the request came from; the handler
// refuses one that does not name the frontend (namesFrontend).
type SetPasswordRequest struct {
	Origin  string `header:"Origin"`
	Referer string `header:"Referer"`
	Body    struct {
		NewPassword string `json:"new_password" doc:"The account's first password"`
	}
}

// SetPasswordResponse answers POST /auth/password. A refusal the person can act
// on (a weak or breached password, a password already set, a closed account)
// is a 200 with success false and its code, as change-password answers; a
// stale session is a 403 REAUTH_REQUIRED and a revoked one a 401
// TOKEN_INVALID, both in the JWT middleware's envelope; a request that does
// not come from the frontend is a 403 huma error.
type SetPasswordResponse struct {
	// Carries the renewed session on success only. Every other path leaves it
	// zero, which huma writes as an empty Set-Cookie header that clears
	// nothing (see ChangePasswordResponse).
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie"`
	Body      struct {
		Success   bool   `json:"success" example:"true" doc:"Success status"`
		Message   string `json:"message" example:"Password set" doc:"Response message"`
		ErrorCode string `json:"error_code,omitempty" example:"PASSWORD_ALREADY_SET" doc:"Error code for programmatic handling"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

func (r *SetPasswordResponse) setRefusal(code, message string) {
	r.Body.Success, r.Body.Message, r.Body.ErrorCode = false, message, code
}

// SetPasswordHandler gives a signed-in account that has no password its first
// one. Adding a way to sign in is a credential mint, so the session must have
// authenticated recently (middleware.RequireRecentSessionAuth). It sends no
// email and signs no other session out.
func (h *AuthHandler) SetPasswordHandler(ctx context.Context, input *SetPasswordRequest) (*SetPasswordResponse, error) {
	resp := &SetPasswordResponse{}
	resp.Body.RequestID = logger.GetRequestID(ctx)

	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "set_password_no_user")
		return refusePasskey(resp, autherrors.CodeUnauthorized, "User not found in context")
	}

	// The cookie rides on any request from a same-site origin, and a forged
	// POST needs no response to set a password the forger knows, so the
	// write answers only a request from the frontend. One that names
	// neither Origin nor Referer is refused: nothing then says where it came
	// from.
	if !namesFrontend(input.Origin, input.Referer, h.config.Email.FrontendURL) {
		logger.AuthWarn(ctx, "set_password_refused_origin")
		return nil, huma.Error403Forbidden(setPasswordOriginRefusal)
	}

	if err := middleware.RequireRecentSessionAuth(ctx, setPasswordOperation); err != nil {
		return nil, err
	}

	// The principal was loaded when the session was validated, so this answers
	// the common case before the validator's outbound breach check and the
	// hash. SetPassword decides it again under the row lock.
	if contextUser.HasPassword() {
		logger.AuthWarn(ctx, "set_password_already_set", "user_id", contextUser.ID)
		return refuseWith(resp, autherrors.ErrPasswordAlreadySet())
	}

	// A password signs in with the account's email address, so an account
	// with none would hold a password nothing can use. The code and message
	// are the verification resend's for the same state.
	if contextUser.Email == nil || *contextUser.Email == "" {
		logger.AuthWarn(ctx, "set_password_no_email", "user_id", contextUser.ID)
		return refusePasskey(resp, "NO_EMAIL", "User does not have an email address")
	}

	if input.Body.NewPassword == "" {
		return refusePasskey(resp, autherrors.CodeValidationFailed, "New password is required")
	}

	// This route has no other length or strength check, so a missing validator
	// refuses rather than letting any string through.
	if h.passwordValidator == nil {
		logger.AuthError(ctx, "set_password_no_validator", errors.New("password validator not configured"),
			"user_id", contextUser.ID,
		)
		return nil, autherrors.ErrServiceUnavailable("set_password_validator", errors.New("not configured"))
	}
	validation, err := h.passwordValidator.ValidatePassword(input.Body.NewPassword)
	if err != nil {
		logger.AuthError(ctx, "set_password_validation_error", err, "user_id", contextUser.ID)
		return nil, autherrors.ErrServiceUnavailable("set_password_validator", err)
	}
	if !validation.Valid {
		message := "Password does not meet security requirements"
		if len(validation.Errors) > 0 {
			message = validation.Errors[0]
		}
		logger.AuthWarn(ctx, "set_password_weak_password",
			"user_id", contextUser.ID,
			"all_errors", strings.Join(validation.Errors, "; "),
		)
		return refusePasskey(resp, autherrors.CodeValidationFailed, message)
	}

	if err := h.userService.SetPassword(contextUser.ID, input.Body.NewPassword, contextUser.SessionsValidAfter); err != nil {
		if errors.Is(err, shared.ErrSessionRevoked) {
			return nil, middleware.RefuseSession(ctx, setPasswordOperation, err, h.config.Session)
		}
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) {
			switch authErr.Code {
			case autherrors.CodePasswordAlreadySet, autherrors.CodeAccountInactive, autherrors.CodeValidationFailed:
				logger.AuthWarn(ctx, "set_password_refused",
					"user_id", contextUser.ID,
					"error_code", authErr.Code,
					"error", err.Error(),
				)
				return refuseWith(resp, authErr)
			}
		}
		logger.AuthError(ctx, "set_password_failed", err, "user_id", contextUser.ID)
		return nil, autherrors.ErrServiceUnavailable("set_password", err)
	}

	// A renewal, not a re-stamp: setting a password proves no factor, so the
	// session keeps the authentication time it arrived with. A mint failure is
	// logged and not surfaced, because the password is already set.
	//
	// The cookie lasts 24 hours, as ChangePasswordHandler's does and for the
	// reason stated there: a session that arrived on the seven-day cookie the
	// OAuth callback sets comes away with the shorter one.
	if token, err := h.jwtService.RenewSessionToken(contextUser, middleware.GetSessionAuthTimeFromContext(ctx)); err != nil {
		logger.AuthError(ctx, "set_password_renew_failed", err, "user_id", contextUser.ID)
	} else {
		resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)
	}

	logger.AuthInfo(ctx, "set_password_success", "user_id", contextUser.ID)
	resp.Body.Success = true
	resp.Body.Message = "Password set"
	return resp, nil
}
