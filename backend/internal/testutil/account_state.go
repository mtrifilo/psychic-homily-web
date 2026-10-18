package testutil

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

// DeactivateAccount closes the account userID the way deactivation does:
// is_active false, deleted_at untouched.
func DeactivateAccount(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	setAccountState(t, db, userID, map[string]any{"is_active": false})
}

// SoftDeleteAccount closes the account userID the way soft deletion does:
// is_active false and deleted_at set, the pair users_live_account_check
// requires.
func SoftDeleteAccount(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	setAccountState(t, db, userID, map[string]any{"is_active": false, "deleted_at": time.Now()})
}

// ReopenAccount makes the account userID live again from either closed state.
func ReopenAccount(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	setAccountState(t, db, userID, map[string]any{"is_active": true, "deleted_at": nil})
}

func setAccountState(t *testing.T, db *gorm.DB, userID uint, updates map[string]any) {
	t.Helper()
	if err := db.Table("users").Where("id = ?", userID).Updates(updates).Error; err != nil {
		t.Fatalf("set account state for user %d: %v", userID, err)
	}
}
