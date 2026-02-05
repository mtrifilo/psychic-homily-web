package models

import (
	"time"
)

// APIToken represents a long-lived API token for authentication
// Used by the local scraper app and other admin tools
type APIToken struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	UserID      uint       `json:"user_id" gorm:"not null"`
	TokenHash   string     `json:"-" gorm:"column:token_hash;uniqueIndex;not null"` // Never expose hash
	Description *string    `json:"description" gorm:"column:description"`
	Scope       string     `json:"scope" gorm:"default:admin"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at" gorm:"column:last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" gorm:"column:revoked_at"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for APIToken
func (APIToken) TableName() string {
	return "api_tokens"
}

// IsValid checks if the token is valid (not expired and not revoked)
func (t *APIToken) IsValid() bool {
	if t.RevokedAt != nil {
		return false
	}
	return time.Now().Before(t.ExpiresAt)
}

// IsExpired checks if the token has expired
func (t *APIToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsRevoked checks if the token has been revoked
func (t *APIToken) IsRevoked() bool {
	return t.RevokedAt != nil
}
