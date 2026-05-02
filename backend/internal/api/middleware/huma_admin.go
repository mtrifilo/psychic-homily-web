package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
)

// HumaAdminMiddleware enforces that the authenticated user has IsAdmin=true.
//
// Must be chained AFTER HumaJWTMiddleware: it reads the user that JWT placed
// at UserContextKey. If the user is missing (auth never ran or was rejected
// upstream) or not an admin, the middleware short-circuits with a 403 and
// next() is never invoked — keeping admin handlers from running for
// non-admin callers.
//
// PSY-423: replaces the inline `shared.RequireAdmin(ctx)` boilerplate
// scattered across pure-admin endpoints. Conditional-admin endpoints
// (e.g. owner-or-admin) stay on rc.Protected with handler-side logic.
func HumaAdminMiddleware(ctx huma.Context, next func(huma.Context)) {
	user := GetUserFromContext(ctx.Context())

	var requestID string
	if id, ok := ctx.Context().Value(logger.RequestIDContextKey).(string); ok {
		requestID = id
	}

	if user == nil {
		// JWT middleware should have set this. If it didn't, that's a wiring
		// bug — but defensive: refuse rather than 500.
		logger.AuthWarn(ctx.Context(), "huma_admin_missing_user",
			"path", ctx.URL().Path,
		)
		writeHumaAdminError(ctx, requestID)
		return
	}

	if !user.IsAdmin {
		logger.AuthWarn(ctx.Context(), "huma_admin_access_denied",
			"user_id", user.ID,
			"path", ctx.URL().Path,
		)
		writeHumaAdminError(ctx, requestID)
		return
	}

	next(ctx)
}

// writeHumaAdminError writes the 403 response for non-admin requests.
// Body shape mirrors writeHumaJWTError so frontend error parsing stays
// uniform across auth-layer denials.
func writeHumaAdminError(ctx huma.Context, requestID string) {
	ctx.SetStatus(http.StatusForbidden)

	resp := JWTErrorResponse{
		Success:   false,
		Message:   "Admin access required",
		ErrorCode: autherrors.CodeUnauthorized,
		RequestID: requestID,
	}
	data, _ := json.Marshal(resp)
	_, _ = ctx.BodyWriter().Write(data)
}
