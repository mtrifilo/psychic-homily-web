// Package shared holds cross-cutting helpers used by every handler
// sub-package (catalog, engagement, admin, auth, community, notification,
// pipeline, system). It deliberately depends on a narrow set of internal
// packages (middleware, logger, models) so it can be imported anywhere
// without creating cycles.
package shared

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
)

// RequireAdmin verifies the request is from an admin user.
// Returns the user on success, or a 403 Forbidden error.
func RequireAdmin(ctx context.Context) (*models.User, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", GetUserID(user),
			"request_id", logger.GetRequestID(ctx),
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}
	return user, nil
}

// GetUserID safely gets user ID or returns 0 if user is nil.
func GetUserID(user *models.User) uint {
	if user == nil {
		return 0
	}
	return user.ID
}

// PtrString converts a string to *string.
func PtrString(s string) *string {
	return &s
}

// ParseDate parses a date string in YYYY-MM-DD format.
func ParseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}
