package shared

import (
	"fmt"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// ClearCalendarSyncTx resets userID's calendar sync choice to "not decided"
// (user_preferences.calendar_sync NULL) through db, a transaction when the
// clear is one step of several.
//
// It lives here, not in services/user, because the calendar service clears
// the choice in the transaction that deletes the feed token, and services/user
// imports services/engagement, so engagement cannot import user.
//
// A user with no preferences row is already undecided, so zero rows affected
// is success and no row is created.
func ClearCalendarSyncTx(db *gorm.DB, userID uint) error {
	err := db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("calendar_sync", gorm.Expr("NULL")).Error
	if err != nil {
		return fmt.Errorf("failed to clear calendar sync choice: %w", err)
	}
	return nil
}
