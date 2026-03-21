package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
)

// requireAdmin verifies the request is from an admin user.
// Returns the user on success, or a 403 Forbidden error.
func requireAdmin(ctx context.Context) (*models.User, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil || !user.IsAdmin {
		logger.FromContext(ctx).Warn("admin_access_denied",
			"user_id", getUserID(user),
			"request_id", logger.GetRequestID(ctx),
		)
		return nil, huma.Error403Forbidden("Admin access required")
	}
	return user, nil
}

// getUserID safely gets user ID or returns 0 if user is nil
func getUserID(user *models.User) uint {
	if user == nil {
		return 0
	}
	return user.ID
}

// ptrString converts a string to *string.
func ptrString(s string) *string {
	return &s
}

// parseDate parses a date string in YYYY-MM-DD format
func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}
