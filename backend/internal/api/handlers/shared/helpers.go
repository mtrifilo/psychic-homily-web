// Package shared holds cross-cutting helpers used by every handler
// sub-package (catalog, engagement, admin, auth, community, notification,
// pipeline, system). It deliberately depends on a narrow set of internal
// packages so it can be imported anywhere without creating cycles.
package shared

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
)

// GetUserID safely gets user ID or returns 0 if user is nil.
func GetUserID(user *authm.User) uint {
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
