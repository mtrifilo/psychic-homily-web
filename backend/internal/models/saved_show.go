package models

import "time"

// UserSavedShow represents the junction table for users' saved shows
type UserSavedShow struct {
	UserID  uint      `gorm:"primaryKey;column:user_id"`
	ShowID  uint      `gorm:"primaryKey;column:show_id"`
	SavedAt time.Time `gorm:"not null;column:saved_at"`
}

// TableName specifies the table name for UserSavedShow
func (UserSavedShow) TableName() string {
	return "user_saved_shows"
}
