package user

import (
	"fmt"
	"time"
)

// DismissFirstSaveHint stamps the one-time hint that follows the user's first
// saved show as done for their account, and returns the stored time. The
// client calls it when the hint is first shown and again when it is closed.
//
// Idempotent and first-write-wins: a repeat call leaves the stored time as it
// is and returns it, so the show-time and close-time stamps (or two devices)
// agree on one value and a retry cannot move it.
//
// One statement rather than the update-then-insert pair the other preference
// writes use, because keeping the FIRST value has to hold even when two
// stamps race to create the preferences row; ON CONFLICT resolves that race
// inside Postgres. Every column the INSERT does not name takes its DDL
// default, exactly as the GORM-built inserts elsewhere in this package do.
func (s *UserService) DismissFirstSaveHint(userID uint) (time.Time, error) {
	if s.db == nil {
		return time.Time{}, fmt.Errorf("database not initialized")
	}

	var dismissedAt time.Time
	err := s.db.Raw(`
		INSERT INTO user_preferences (user_id, first_save_hint_dismissed_at)
		VALUES (?, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			first_save_hint_dismissed_at = COALESCE(
				user_preferences.first_save_hint_dismissed_at,
				EXCLUDED.first_save_hint_dismissed_at
			),
			updated_at = CASE
				WHEN user_preferences.first_save_hint_dismissed_at IS NULL THEN NOW()
				ELSE user_preferences.updated_at
			END
		RETURNING first_save_hint_dismissed_at`, userID).Row().Scan(&dismissedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to dismiss first-save hint: %w", err)
	}
	return dismissedAt, nil
}
