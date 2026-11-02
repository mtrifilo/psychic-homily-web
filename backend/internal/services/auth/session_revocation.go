package auth

import (
	"time"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
)

// RevokeAllSessions is shared.RevokeAllSessions: it refuses, from now on,
// every session user holds, writing users.sessions_valid_after through db and
// setting the same value on user.
func RevokeAllSessions(db *gorm.DB, user *authm.User, now time.Time) error {
	return shared.RevokeAllSessions(db, user, now)
}
