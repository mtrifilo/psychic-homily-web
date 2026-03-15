package contracts

import "time"

// ──────────────────────────────────────────────
// Saved Show types
// ──────────────────────────────────────────────

// SavedShowResponse represents a saved show with metadata
type SavedShowResponse struct {
	ShowResponse
	SavedAt time.Time `json:"saved_at"`
}

// ──────────────────────────────────────────────
// Favorite Venue types
// ──────────────────────────────────────────────

// FavoriteVenueResponse represents a favorite venue with metadata
type FavoriteVenueResponse struct {
	ID                uint      `json:"id"`
	Slug              string    `json:"slug"`
	Name              string    `json:"name"`
	Address           *string   `json:"address"`
	City              string    `json:"city"`
	State             string    `json:"state"`
	Verified          bool      `json:"verified"`
	FavoritedAt       time.Time `json:"favorited_at"`
	UpcomingShowCount int       `json:"upcoming_show_count"`
}

// FavoriteVenueShowResponse represents a show from a favorite venue
type FavoriteVenueShowResponse struct {
	ID             uint             `json:"id"`
	Slug           string           `json:"slug"`
	Title          string           `json:"title"`
	EventDate      time.Time        `json:"event_date"`
	City           *string          `json:"city"`
	State          *string          `json:"state"`
	Price          *float64         `json:"price"`
	AgeRequirement *string          `json:"age_requirement"`
	VenueID        uint             `json:"venue_id"`
	VenueName      string           `json:"venue_name"`
	VenueSlug      string           `json:"venue_slug"`
	Artists        []ArtistResponse `json:"artists"`
}

// ──────────────────────────────────────────────
// Show Report types
// ──────────────────────────────────────────────

// ShowReportResponse represents a show report response with show info
type ShowReportResponse struct {
	ID         uint      `json:"id"`
	ShowID     uint      `json:"show_id"`
	ReportType string    `json:"report_type"`
	Details    *string   `json:"details"`
	Status     string    `json:"status"`
	AdminNotes *string   `json:"admin_notes,omitempty"`
	ReviewedBy *uint     `json:"reviewed_by,omitempty"`
	ReviewedAt *string   `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Show info (for admin view)
	Show *ShowReportShowInfo `json:"show,omitempty"`
}

// ShowReportShowInfo contains show information for report responses
type ShowReportShowInfo struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	EventDate time.Time `json:"event_date"`
	City      *string   `json:"city"`
	State     *string   `json:"state"`
}

// ──────────────────────────────────────────────
// Artist Report types
// ──────────────────────────────────────────────

// ArtistReportResponse represents an artist report response with artist info
type ArtistReportResponse struct {
	ID         uint      `json:"id"`
	ArtistID   uint      `json:"artist_id"`
	ReportType string    `json:"report_type"`
	Details    *string   `json:"details"`
	Status     string    `json:"status"`
	AdminNotes *string   `json:"admin_notes,omitempty"`
	ReviewedBy *uint     `json:"reviewed_by,omitempty"`
	ReviewedAt *string   `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Artist info (for admin view)
	Artist *ArtistReportArtistInfo `json:"artist,omitempty"`
}

// ArtistReportArtistInfo contains artist information for report responses
type ArtistReportArtistInfo struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ──────────────────────────────────────────────
// Calendar types
// ──────────────────────────────────────────────

// CalendarTokenCreateResponse is returned when a token is created
type CalendarTokenCreateResponse struct {
	Token     string    `json:"token"`
	FeedURL   string    `json:"feed_url"`
	CreatedAt time.Time `json:"created_at"`
}

// CalendarTokenStatusResponse is returned for token status checks
type CalendarTokenStatusResponse struct {
	HasToken  bool       `json:"has_token"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// ──────────────────────────────────────────────
// Attendance types (going/interested)
// ──────────────────────────────────────────────

// AttendanceCountsResponse contains going and interested counts for a show
type AttendanceCountsResponse struct {
	ShowID          uint `json:"show_id"`
	GoingCount      int  `json:"going_count"`
	InterestedCount int  `json:"interested_count"`
}

// AttendingShowResponse represents a show the user is attending or interested in
type AttendingShowResponse struct {
	ShowID    uint      `json:"show_id"`
	Title     string    `json:"title"`
	Slug      string    `json:"slug"`
	EventDate time.Time `json:"event_date"`
	Status    string    `json:"status"` // "going" or "interested"
	VenueName *string   `json:"venue_name"`
	VenueSlug *string   `json:"venue_slug"`
	City      *string   `json:"city"`
	State     *string   `json:"state"`
}

// ──────────────────────────────────────────────
// Follow types
// ──────────────────────────────────────────────

// FollowingEntityResponse represents an entity a user is following.
type FollowingEntityResponse struct {
	EntityType string    `json:"entity_type"`
	EntityID   uint      `json:"entity_id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	FollowedAt time.Time `json:"followed_at"`
}

// FollowerResponse represents a follower of an entity.
type FollowerResponse struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
}

// FollowStatusResponse contains follow status for a single entity.
type FollowStatusResponse struct {
	EntityType    string `json:"entity_type"`
	EntityID      uint   `json:"entity_id"`
	FollowerCount int64  `json:"follower_count"`
	IsFollowing   bool   `json:"is_following"`
}
