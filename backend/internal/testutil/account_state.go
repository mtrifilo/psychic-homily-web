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

// SoftDeleteAccount gives the account userID the state soft deletion leaves:
// is_active false and deleted_at set, the pair users_live_account_check
// requires. It does not revoke the account's sessions, as
// UserService.SoftDeleteAccount does, so a test of what sessions survive a
// closure closes the account through the service.
func SoftDeleteAccount(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	setAccountState(t, db, userID, map[string]any{"is_active": false, "deleted_at": time.Now()})
}

// ReopenAccount makes the account userID live again from either closed state.
func ReopenAccount(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	setAccountState(t, db, userID, map[string]any{"is_active": true, "deleted_at": nil})
}

// SetProfileVisibility sets the profile_visibility of the account userID
// ("public" or "private").
func SetProfileVisibility(t *testing.T, db *gorm.DB, userID uint, visibility string) {
	t.Helper()
	setAccountState(t, db, userID, map[string]any{"profile_visibility": visibility})
}

func setAccountState(t *testing.T, db *gorm.DB, userID uint, updates map[string]any) {
	t.Helper()
	if err := db.Table("users").Where("id = ?", userID).Updates(updates).Error; err != nil {
		t.Fatalf("set account state for user %d: %v", userID, err)
	}
}
