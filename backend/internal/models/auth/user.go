package auth

import (
	"encoding/json"
	"time"
)

// FavoriteCity represents a city+state pair for user's favorite cities
type FavoriteCity struct {
	City  string `json:"city"`
	State string `json:"state"`
}

// ChartDefaults is the saved /charts landing preference (PSY-1423).
// Window must be month|quarter|all_time. Scene is a metro CBSA code, or nil/empty
// for "all scenes".
type ChartDefaults struct {
	Window string  `json:"window"`
	Scene  *string `json:"scene"`
}

// Nav-mode preference values (PSY-1115): the global navigation chrome a user
// prefers. NavModeTop is the default top-bar nav; NavModeSide is the left
// sidebar nav. Kept in sync with the users.nav_mode CHECK constraint and the
// frontend's nav-mode cookie/encoding.
const (
	NavModeTop  = "top"
	NavModeSide = "side"
)

// User represents a user account
type User struct {
	ID                  uint             `json:"id" gorm:"primaryKey"`
	Email               *string          `json:"email" gorm:"uniqueIndex"`
	Username            *string          `json:"username" gorm:"uniqueIndex"`
	PasswordHash        *string          `json:"-" gorm:"column:password_hash"`           // Hidden from JSON
	DisplayName         *string          `json:"display_name" gorm:"column:display_name"` // Preferred over first/last in attribution (PSY-1063)
	FirstName           *string          `json:"first_name" gorm:"column:first_name"`
	LastName            *string          `json:"last_name" gorm:"column:last_name"`
	AvatarURL           *string          `json:"avatar_url" gorm:"column:avatar_url"`
	Location            *string          `json:"location" gorm:"column:location"` // Free-text "City, state" (PSY-1416); not in attribution chain
	Bio                 *string          `json:"bio"`
	ProfileVisibility   string           `json:"profile_visibility" gorm:"column:profile_visibility;not null;default:'public'"`
	PrivacySettings     *json.RawMessage `json:"privacy_settings" gorm:"column:privacy_settings;type:jsonb;not null;default:'{\"contributions\":\"visible\",\"saved_shows\":\"hidden\",\"following\":\"visible\",\"collections\":\"visible\",\"last_active\":\"visible\",\"profile_sections\":\"visible\"}'"`
	NavMode             string           `json:"nav_mode" gorm:"column:nav_mode;not null;default:'top'"` // Global nav chrome preference: 'top' | 'side' (PSY-1115)
	UserTier            string           `json:"user_tier" gorm:"column:user_tier;not null;default:'new_user'"`
	IsActive            bool             `json:"is_active" gorm:"default:true"`
	IsAdmin             bool             `json:"is_admin" gorm:"default:false"`
	EmailVerified       bool             `json:"email_verified" gorm:"default:false"`
	TermsAcceptedAt     *time.Time       `json:"-" gorm:"column:terms_accepted_at"` // Legal acceptance evidence
	TermsVersion        *string          `json:"-" gorm:"column:terms_version"`
	PrivacyVersion      *string          `json:"-" gorm:"column:privacy_version"`
	AgeConfirmedAt      *time.Time       `json:"-" gorm:"column:age_confirmed_at"` // Age-confirmation evidence (mirrors TermsAcceptedAt)
	MinAgeAttested      *int             `json:"-" gorm:"column:min_age_attested"` // Minimum age the user attested to at signup (e.g. 16)
	FailedLoginAttempts int              `json:"-" gorm:"default:0"`
	LockedUntil         *time.Time       `json:"-" gorm:"column:locked_until"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
	DeletedAt           *time.Time       `json:"deleted_at,omitempty" gorm:"column:deleted_at"`
	DeletionReason      *string          `json:"-" gorm:"column:deletion_reason"` // Hidden from JSON

	// Relationships
	OAuthAccounts      []OAuthAccount       `json:"oauth_accounts,omitempty" gorm:"foreignKey:UserID"`
	Preferences        *UserPreferences     `json:"preferences,omitempty" gorm:"foreignKey:UserID"`
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
	ID                uint             `json:"id" gorm:"primaryKey"`
	UserID            uint             `json:"user_id" gorm:"uniqueIndex;not null"`
	NotificationEmail bool             `json:"notification_email" gorm:"default:true"`
	NotificationPush  bool             `json:"notification_push" gorm:"default:false"`
	Theme             string           `json:"theme" gorm:"default:light"`
	Timezone          string           `json:"timezone" gorm:"default:UTC"`
	Language          string           `json:"language" gorm:"default:en"`
	ShowReminders     bool             `json:"show_reminders" gorm:"default:false"`
	FavoriteCities    *json.RawMessage `json:"favorite_cities" gorm:"type:jsonb;default:'[]'"`
	// PSY-1423: saved /charts window + scene. NULL = no saved defaults.
	ChartDefaults *json.RawMessage `json:"chart_defaults" gorm:"type:jsonb"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`

	// Relationships
	User User `json:"-" gorm:"foreignKey:UserID"`

	// PSY-296: Default reply permission applied to new top-level comments.
	// Valid values: 'anyone' (default), 'followers', 'author_only'.
	DefaultReplyPermission string `json:"default_reply_permission" gorm:"column:default_reply_permission;not null;default:'anyone'"`

	// PSY-289: comment subscription + mention notification preferences.
	NotifyOnCommentSubscription bool `json:"notify_on_comment_subscription" gorm:"column:notify_on_comment_subscription;not null;default:true"`
	NotifyOnMention             bool `json:"notify_on_mention" gorm:"column:notify_on_mention;not null;default:true"`

	// PSY-350: collection digest email preference. Toggles whether the user
	// receives a once-per-week batched email summarizing items added to any
	// of their subscribed collections in the last 7 days.
	//
	// Defaults to FALSE (opt-IN) at the column level — divergent from the
	// PSY-289 opt-OUT defaults. See migration
	// 20260428003421_collection_digest_columns.up.sql for rationale.
	NotifyOnCollectionDigest bool `json:"notify_on_collection_digest" gorm:"column:notify_on_collection_digest;not null;default:false"`

	// PSY-1342: weekly scene digest email preference — a once-per-week batched
	// email of this-week shows + new bands for every scene the user follows.
	// Opt-IN (default FALSE) for the same bulk-sender reason as the collection
	// digest: following a scene implicitly subscribes to a recurring email, so
	// the column defaults OFF and users opt in via the settings toggle.
	NotifyOnSceneDigest bool `json:"notify_on_scene_digest" gorm:"column:notify_on_scene_digest;not null;default:false"`

	// Per-category opt-out for tier-change and edit-review emails. Each is a
	// single email per discrete action, so both default TRUE (opt-OUT) like
	// the comment/mention flags. Flipped off by the one-click unsubscribe link
	// in those emails and gated on by the senders' callers.
	NotifyOnTierNotifications bool `json:"notify_on_tier_notifications" gorm:"column:notify_on_tier_notifications;not null;default:true"`
	NotifyOnEditNotifications bool `json:"notify_on_edit_notifications" gorm:"column:notify_on_edit_notifications;not null;default:true"`
}

// TableName specifies the table name for UserPreferences
func (UserPreferences) TableName() string {
	return "user_preferences"
}
