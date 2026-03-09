package models

import "time"

// UserProfileSection represents a custom content section on a user's profile page.
type UserProfileSection struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"column:user_id;not null"`
	Title     string    `json:"title" gorm:"column:title;not null"`
	Content   string    `json:"content" gorm:"column:content;not null;default:''"`
	Position  int       `json:"position" gorm:"column:position;not null"`
	IsVisible bool      `json:"is_visible" gorm:"column:is_visible;not null;default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for UserProfileSection.
func (UserProfileSection) TableName() string {
	return "user_profile_sections"
}
