package models

import "time"

// UserFavoriteVenue represents the junction table for users' favorite venues
type UserFavoriteVenue struct {
	UserID      uint      `gorm:"primaryKey;column:user_id"`
	VenueID     uint      `gorm:"primaryKey;column:venue_id"`
	FavoritedAt time.Time `gorm:"not null;column:favorited_at"`
}

// TableName specifies the table name for UserFavoriteVenue
func (UserFavoriteVenue) TableName() string {
	return "user_favorite_venues"
}
