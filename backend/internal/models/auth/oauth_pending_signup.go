package auth

import "time"

// OAuthPendingSignup is a provider identity waiting on the "Create your
// account" step: what the provider asserted, and where the reader was headed.
// It is never a user. TokenHash is the SHA-256 (hex) of the id the pending
// cookie carries, never the id itself.
type OAuthPendingSignup struct {
	TokenHash      string    `gorm:"column:token_hash;primaryKey"`
	Provider       string    `gorm:"column:provider;not null"`
	ProviderUserID string    `gorm:"column:provider_user_id;not null"`
	Email          string    `gorm:"column:email;not null"`
	EmailVerified  bool      `gorm:"column:email_verified;not null"`
	Name           string    `gorm:"column:name;not null"`
	FirstName      string    `gorm:"column:first_name;not null"`
	LastName       string    `gorm:"column:last_name;not null"`
	AvatarURL      string    `gorm:"column:avatar_url;not null"`
	ReturnTo       string    `gorm:"column:return_to;not null"`
	ExpiresAt      time.Time `gorm:"column:expires_at;not null"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

// TableName specifies the table name for OAuthPendingSignup
func (OAuthPendingSignup) TableName() string {
	return "oauth_pending_signups"
}
