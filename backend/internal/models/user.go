package models

import (
	"time"
)

// User represents a user account
type User struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	Email         *string   `json:"email" gorm:"uniqueIndex"`
	Username      *string   `json:"username" gorm:"uniqueIndex"`
	PasswordHash  *string   `json:"-" gorm:"column:password_hash"` // Hidden from JSON
	FirstName     *string   `json:"first_name" gorm:"column:first_name"`
	LastName      *string   `json:"last_name" gorm:"column:last_name"`
	AvatarURL     *string   `json:"avatar_url" gorm:"column:avatar_url"`
	Bio           *string   `json:"bio"`
	IsActive      bool      `json:"is_active" gorm:"default:true"`
	IsAdmin       bool      `json:"is_admin" gorm:"default:false"`
	EmailVerified bool      `json:"email_verified" gorm:"default:false"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Relationships
	OAuthAccounts  []OAuthAccount       `json:"oauth_accounts,omitempty" gorm:"foreignKey:UserID"`
	Preferences    *UserPreferences     `json:"preferences,omitempty" gorm:"foreignKey:UserID"`
	PasskeyCredentials []WebAuthnCredential `json:"passkey_credentials,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for User
func (User) TableName() string {
	return "users"
}

// OAuthAccount represents an OAuth provider connection
type OAuthAccount struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	UserID            uint       `json:"user_id" gorm:"not null"`
	Provider          string     `json:"provider" gorm:"not null"`
	ProviderUserID    string     `json:"provider_user_id" gorm:"not null"`
	ProviderEmail     *string    `json:"provider_email"`
	ProviderName      *string    `json:"provider_name"`
	ProviderAvatarURL *string    `json:"provider_avatar_url"`
	AccessToken       *string    `json:"-" gorm:"column:access_token"`  // Hidden from JSON
	RefreshToken      *string    `json:"-" gorm:"column:refresh_token"` // Hidden from JSON
	ExpiresAt         *time.Time `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for OAuthAccount
func (OAuthAccount) TableName() string {
	return "oauth_accounts"
}

// UserPreferences represents user preferences
type UserPreferences struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	UserID            uint      `json:"user_id" gorm:"uniqueIndex;not null"`
	NotificationEmail bool      `json:"notification_email" gorm:"default:true"`
	NotificationPush  bool      `json:"notification_push" gorm:"default:false"`
	Theme             string    `json:"theme" gorm:"default:light"`
	Timezone          string    `json:"timezone" gorm:"default:UTC"`
	Language          string    `json:"language" gorm:"default:en"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Relationships
	User User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for UserPreferences
func (UserPreferences) TableName() string {
	return "user_preferences"
}
