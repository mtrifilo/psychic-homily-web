package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
)

// AdminErrorResponse represents the error response for admin authorization failures.
// Mirrors JWTErrorResponse so frontend code paths handling 401/403 are uniform.
type AdminErrorResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode string `json:"error_code"`
	RequestID string `json:"request_id,omitempty"`
}

// HumaAdminMiddleware enforces that the authenticated user has IsAdmin=true.
// MUST be installed AFTER HumaJWTMiddleware (which populates the user in context);
// otherwise no user will be present and every request will be rejected.
//
// Returns 403 Forbidden if the user is missing or not an admin. The response
// shape matches JWT error responses so frontend handling is uniform.
//
// Pair with a Huma group whose middleware chain is JWT then Admin:
//
//	adminGroup := huma.NewGroup(api, "")
//	adminGroup.UseMiddleware(middleware.HumaJWTMiddleware(...))
//	adminGroup.UseMiddleware(middleware.HumaAdminMiddleware())
//
// Endpoints registered on adminGroup are gated at the route level — handlers
// no longer need to call shared.RequireAdmin(ctx) (PSY-423).
func HumaAdminMiddleware() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		var requestID string
		if id, ok := ctx.Context().Value(logger.RequestIDContextKey).(string); ok {
			requestID = id
		}

		user := GetUserFromContext(ctx.Context())
		if user == nil {
			// JWT middleware should have rejected the request already; a nil
			// user here means the admin group was wired without JWT upstream.
			logger.FromContext(ctx.Context()).Warn("admin_middleware_no_user",
				"path", ctx.URL().Path,
				"request_id", requestID,
			)
			writeHumaAdminError(ctx, requestID, autherrors.CodeUnauthorized, "Authentication required")
			return
		}

		if !user.IsAdmin {
			logger.FromContext(ctx.Context()).Warn("admin_access_denied",
				"user_id", user.ID,
				"path", ctx.URL().Path,
				"request_id", requestID,
			)
			writeHumaAdminError(ctx, requestID, autherrors.CodeUnauthorized, "Admin access required")
			return
		}

		next(ctx)
	}
}

// writeHumaAdminError writes a JSON 403 response for admin authorization failures.
func writeHumaAdminError(ctx huma.Context, requestID, errorCode, message string) {
	ctx.SetStatus(http.StatusForbidden)
	ctx.SetHeader("Content-Type", "application/json")

	resp := AdminErrorResponse{
		Success:   false,
		Message:   message,
		ErrorCode: errorCode,
		RequestID: requestID,
	}
	data, _ := json.Marshal(resp)
	ctx.BodyWriter().Write(data)
}
