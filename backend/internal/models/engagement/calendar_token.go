package engagement

import (
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// CalendarToken represents a per-user token for calendar feed access
type CalendarToken struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"uniqueIndex:idx_calendar_tokens_user_id;not null"`
	TokenHash string    `json:"-" gorm:"column:token_hash;uniqueIndex;not null"`
	CreatedAt time.Time `json:"created_at"`

	// Relationships
	User auth.User `json:"-" gorm:"foreignKey:UserID"`
}

func (CalendarToken) TableName() string {
	return "calendar_tokens"
}
