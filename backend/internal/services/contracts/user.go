package contracts

import "time"

// ──────────────────────────────────────────────
// User types
// ──────────────────────────────────────────────

// AdminUserFilters contains filter criteria for listing users
type AdminUserFilters struct {
	Search string // ILIKE match on email or username
}

// UserSubmissionStats contains show submission counts by status
type UserSubmissionStats struct {
	Approved int64 `json:"approved"`
	Pending  int64 `json:"pending"`
	Rejected int64 `json:"rejected"`
	Total    int64 `json:"total"`
}

// AdminUserResponse is the response type for the admin user list
type AdminUserResponse struct {
	ID              uint                `json:"id"`
	Email           *string             `json:"email"`
	Username        *string             `json:"username"`
	FirstName       *string             `json:"first_name"`
	LastName        *string             `json:"last_name"`
	AvatarURL       *string             `json:"avatar_url"`
	IsActive        bool                `json:"is_active"`
	IsAdmin         bool                `json:"is_admin"`
	EmailVerified   bool                `json:"email_verified"`
	AuthMethods     []string            `json:"auth_methods"`
	SubmissionStats UserSubmissionStats `json:"submission_stats"`
	CreatedAt       time.Time           `json:"created_at"`
	DeletedAt       *time.Time          `json:"deleted_at,omitempty"`
}

// DeletionSummary contains counts of data that will be affected by account deletion
type DeletionSummary struct {
	ShowsCount      int64 `json:"shows_count"`
	SavedShowsCount int64 `json:"saved_shows_count"`
	PasskeysCount   int64 `json:"passkeys_count"`
}

// UserDataExport represents all user data in a portable format (GDPR compliance)
type UserDataExport struct {
	ExportedAt     time.Time              `json:"exported_at"`
	ExportVersion  string                 `json:"export_version"`
	Profile        UserProfileExport      `json:"profile"`
	Preferences    *UserPreferencesExport `json:"preferences,omitempty"`
	OAuthAccounts  []OAuthAccountExport   `json:"oauth_accounts,omitempty"`
	Passkeys       []PasskeyExport        `json:"passkeys,omitempty"`
	SavedShows     []SavedShowExport      `json:"saved_shows,omitempty"`
	SubmittedShows []SubmittedShowExport  `json:"submitted_shows,omitempty"`
}

// UserProfileExport contains user profile data for export
type UserProfileExport struct {
	ID            uint      `json:"id"`
	Email         *string   `json:"email"`
	Username      *string   `json:"username,omitempty"`
	FirstName     *string   `json:"first_name,omitempty"`
	LastName      *string   `json:"last_name,omitempty"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
	Bio           *string   `json:"bio,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"account_created_at"`
	UpdatedAt     time.Time `json:"last_updated_at"`
}

// UserPreferencesExport contains user preferences for export
type UserPreferencesExport struct {
	NotificationEmail bool   `json:"notification_email"`
	NotificationPush  bool   `json:"notification_push"`
	Theme             string `json:"theme"`
	Timezone          string `json:"timezone"`
	Language          string `json:"language"`
}

// OAuthAccountExport contains OAuth account data for export (no tokens)
type OAuthAccountExport struct {
	Provider      string    `json:"provider"`
	ProviderEmail *string   `json:"provider_email,omitempty"`
	ProviderName  *string   `json:"provider_name,omitempty"`
	LinkedAt      time.Time `json:"linked_at"`
}

// PasskeyExport contains passkey metadata for export (no keys)
type PasskeyExport struct {
	DisplayName    string     `json:"display_name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	BackupEligible bool       `json:"backup_eligible"`
	BackupState    bool       `json:"backup_state"`
}

// SavedShowExport contains saved show data for export
type SavedShowExport struct {
	ShowID    uint      `json:"show_id"`
	Title     string    `json:"title"`
	EventDate time.Time `json:"event_date"`
	Venue     *string   `json:"venue,omitempty"`
	City      *string   `json:"city,omitempty"`
	SavedAt   time.Time `json:"saved_at"`
}

// SubmittedShowExport contains submitted show data for export
type SubmittedShowExport struct {
	ShowID      uint      `json:"show_id"`
	Title       string    `json:"title"`
	EventDate   time.Time `json:"event_date"`
	Status      string    `json:"status"`
	SubmittedAt time.Time `json:"submitted_at"`
	Venue       *string   `json:"venue,omitempty"`
	City        *string   `json:"city,omitempty"`
	Artists     []string  `json:"artists,omitempty"`
}

// ──────────────────────────────────────────────
// Contributor Profile types
// ──────────────────────────────────────────────

// PrivacyLevel represents the visibility level for a profile field.
type PrivacyLevel string

const (
	PrivacyVisible   PrivacyLevel = "visible"
	PrivacyCountOnly PrivacyLevel = "count_only"
	PrivacyHidden    PrivacyLevel = "hidden"
)

// PrivacySettings represents the granular privacy configuration for a user profile.
type PrivacySettings struct {
	Contributions   PrivacyLevel `json:"contributions"`
	SavedShows      PrivacyLevel `json:"saved_shows"`
	Attendance      PrivacyLevel `json:"attendance"`
	Following       PrivacyLevel `json:"following"`
	Collections     PrivacyLevel `json:"collections"`
	LastActive      PrivacyLevel `json:"last_active"`
	ProfileSections PrivacyLevel `json:"profile_sections"`
}

// DefaultPrivacySettings returns the default privacy configuration.
func DefaultPrivacySettings() PrivacySettings {
	return PrivacySettings{
		Contributions:   PrivacyVisible,
		SavedShows:      PrivacyHidden,
		Attendance:      PrivacyHidden,
		Following:       PrivacyCountOnly,
		Collections:     PrivacyVisible,
		LastActive:      PrivacyVisible,
		ProfileSections: PrivacyVisible,
	}
}

// ContributionStats represents aggregated contribution counts.
type ContributionStats struct {
	// Content creation
	ShowsSubmitted      int64 `json:"shows_submitted"`
	VenuesSubmitted     int64 `json:"venues_submitted"`
	VenueEditsSubmitted int64 `json:"venue_edits_submitted"`
	ReleasesCreated     int64 `json:"releases_created"`
	LabelsCreated       int64 `json:"labels_created"`
	FestivalsCreated    int64 `json:"festivals_created"`
	ArtistsEdited       int64 `json:"artists_edited"`
	RevisionsMade       int64 `json:"revisions_made"`
	PendingEditsSubmitted int64 `json:"pending_edits_submitted"`

	// Community participation
	TagVotesCast              int64 `json:"tag_votes_cast"`
	RelationshipVotesCast     int64 `json:"relationship_votes_cast"`
	RequestVotesCast          int64 `json:"request_votes_cast"`
	CollectionItemsAdded      int64 `json:"collection_items_added"`
	CollectionSubscriptions   int64 `json:"collection_subscriptions"`
	ShowsAttended             int64 `json:"shows_attended"`

	// Reports
	ReportsFiled    int64 `json:"reports_filed"`
	ReportsResolved int64 `json:"reports_resolved"`

	// Social
	FollowersCount int64 `json:"followers_count"`
	FollowingCount int64 `json:"following_count"`

	// Moderation
	ModerationActions int64 `json:"moderation_actions"`

	// Computed
	ApprovalRate       *float64 `json:"approval_rate,omitempty"`
	TotalContributions int64    `json:"total_contributions"`
}

// PublicProfileResponse is the response for the public profile endpoint.
type PublicProfileResponse struct {
	Username          string                   `json:"username"`
	Bio               *string                  `json:"bio,omitempty"`
	AvatarURL         *string                  `json:"avatar_url,omitempty"`
	FirstName         *string                  `json:"first_name,omitempty"`
	ProfileVisibility string                   `json:"profile_visibility"`
	UserTier          string                   `json:"user_tier"`
	PrivacySettings   *PrivacySettings         `json:"privacy_settings,omitempty"`
	JoinedAt          time.Time                `json:"joined_at"`
	LastActive        *time.Time               `json:"last_active,omitempty"`
	Stats             *ContributionStats       `json:"stats,omitempty"`
	StatsCount        *int64                   `json:"stats_count,omitempty"`
	Sections          []*ProfileSectionResponse `json:"sections,omitempty"`
}

// ProfileSectionResponse represents a profile section in API responses.
type ProfileSectionResponse struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Position  int       `json:"position"`
	IsVisible bool      `json:"is_visible"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContributionEntry represents a single contribution in the history.
type ContributionEntry struct {
	ID         uint                   `json:"id"`
	Action     string                 `json:"action"`
	EntityType string                 `json:"entity_type"`
	EntityID   uint                   `json:"entity_id"`
	EntityName string                 `json:"entity_name,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	Source     string                 `json:"source"`
}
