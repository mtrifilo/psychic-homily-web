package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/shared"
)

// SignOutEverywhereElseRoute is the Settings control that signs out every
// other session the account holds and keeps the caller's.
const SignOutEverywhereElseRoute = "/auth/sessions/sign-out-everywhere-else"

// signOutEverywhereElseOperation names the operation in the logs.
const signOutEverywhereElseOperation = "sign_out_everywhere_else"

// SignOutEverywhereElseResponse answers POST
// /auth/sessions/sign-out-everywhere-else. Success is a 200 carrying the
// caller's renewed session in Set-Cookie. A session revoked, or an account
// closed, since validation is a 401 TOKEN_INVALID in the JWT middleware's
// envelope that clears the cookie; any other failure is a 5xx, and in both
// cases no session was revoked by this request.
type SignOutEverywhereElseResponse struct {
	SetCookie http.Cookie `header:"Set-Cookie" doc:"Authentication cookie carrying the caller's renewed session"`
	Body      struct {
		Success   bool   `json:"success" example:"true" doc:"Every other session is signed out and this one is renewed"`
		RequestID string `json:"request_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000" doc:"Request ID for debugging"`
	}
}

// SignOutEverywhereElseHandler signs out every session the account holds
// except the caller's, which it renews in the same step keeping its
// authentication time. It removes access rather than adding a way in, so it
// is not gated on a recent authentication (middleware.RequireRecentSessionAuth
// covers adding only).
func (h *AuthHandler) SignOutEverywhereElseHandler(ctx context.Context, _ *struct{}) (*SignOutEverywhereElseResponse, error) {
	contextUser := middleware.GetUserFromContext(ctx)
	if contextUser == nil {
		logger.AuthWarn(ctx, "sign_out_everywhere_else_no_user")
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	token, err := h.authService.SignOutOtherSessions(contextUser, middleware.GetSessionAuthTimeFromContext(ctx))
	if err != nil {
		if shared.IsSessionRefusal(err) {
			return nil, middleware.RefuseSession(ctx, signOutEverywhereElseOperation, err, h.config.Session)
		}
		logger.AuthError(ctx, "sign_out_everywhere_else_failed", err, "user_id", contextUser.ID)
		return nil, autherrors.ErrServiceUnavailable(signOutEverywhereElseOperation, err)
	}

	// 24 hours, as ChangePasswordHandler's cookie and for the reason stated
	// there.
	resp := &SignOutEverywhereElseResponse{}
	resp.SetCookie = h.config.Session.NewAuthCookie(token, 24*time.Hour)
	resp.Body.Success = true
	resp.Body.RequestID = logger.GetRequestID(ctx)
	logger.AuthInfo(ctx, "sign_out_everywhere_else_success", "user_id", contextUser.ID)
	return resp, nil
}
