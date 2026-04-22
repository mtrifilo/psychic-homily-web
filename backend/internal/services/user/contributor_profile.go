package user

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ContributorProfileService handles contributor profile and contribution history operations.
type ContributorProfileService struct {
	db *gorm.DB
}

// NewContributorProfileService creates a new contributor profile service.
func NewContributorProfileService(database *gorm.DB) *ContributorProfileService {
	if database == nil {
		database = db.GetDB()
	}
	return &ContributorProfileService{
		db: database,
	}
}

// binaryOnlyFields are privacy fields that only support visible/hidden (not count_only).
var binaryOnlyFields = map[string]bool{
	"last_active":      true,
	"profile_sections": true,
}

// ValidatePrivacySettings checks that all fields have valid values.
func ValidatePrivacySettings(ps contracts.PrivacySettings) error {
	fields := map[string]contracts.PrivacyLevel{
		"contributions":    ps.Contributions,
		"saved_shows":      ps.SavedShows,
		"attendance":       ps.Attendance,
		"following":        ps.Following,
		"collections":      ps.Collections,
		"last_active":      ps.LastActive,
		"profile_sections": ps.ProfileSections,
	}
	for name, level := range fields {
		if level != contracts.PrivacyVisible && level != contracts.PrivacyCountOnly && level != contracts.PrivacyHidden {
			return fmt.Errorf("invalid privacy level %q for field %q", level, name)
		}
		if binaryOnlyFields[name] && level == contracts.PrivacyCountOnly {
			return fmt.Errorf("field %q only supports 'visible' or 'hidden'", name)
		}
	}
	return nil
}

// parsePrivacySettings extracts contracts.PrivacySettings from a user's JSONB column.
func parsePrivacySettings(raw *json.RawMessage) contracts.PrivacySettings {
	if raw == nil {
		return contracts.DefaultPrivacySettings()
	}
	var ps contracts.PrivacySettings
	if err := json.Unmarshal(*raw, &ps); err != nil {
		return contracts.DefaultPrivacySettings()
	}
	return ps
}


// contributionRow is a raw scan target for the UNION query.
type contributionRow struct {
	ID         uint
	Action     string
	EntityType string
	EntityID   uint
	Metadata   *json.RawMessage
	CreatedAt  time.Time
	Source     string
}

// moderationActions are audit_log actions that count as moderation, not content creation.
var moderationActions = map[string]bool{
	"approve_show":          true,
	"reject_show":           true,
	"verify_venue":          true,
	"approve_venue_edit":    true,
	"reject_venue_edit":     true,
	"dismiss_report":        true,
	"resolve_report":        true,
	"dismiss_artist_report": true,
	"resolve_artist_report": true,
}

// =============================================================================
// Profile Endpoints
// =============================================================================

// GetPublicProfile returns a user's public profile with privacy-gated fields.
func (s *ContributorProfileService) GetPublicProfile(username string, viewerID *uint) (*contracts.PublicProfileResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User
	err := s.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	isOwner := viewerID != nil && *viewerID == user.ID

	// Private profile check: only the owner can view
	if user.ProfileVisibility == "private" && !isOwner {
		return nil, nil
	}

	privacy := parsePrivacySettings(user.PrivacySettings)

	username_str := ""
	if user.Username != nil {
		username_str = *user.Username
	}

	resp := &contracts.PublicProfileResponse{
		Username:          username_str,
		Bio:               user.Bio,
		AvatarURL:         user.AvatarURL,
		FirstName:         user.FirstName,
		ProfileVisibility: user.ProfileVisibility,
		UserTier:          user.UserTier,
		JoinedAt:          user.CreatedAt,
	}

	// Owner always sees everything + privacy settings for editing
	if isOwner {
		resp.PrivacySettings = &privacy
		resp.LastActive = &user.UpdatedAt

		stats, err := s.GetContributionStats(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get contribution stats: %w", err)
		}
		resp.Stats = stats

		sections, err := s.GetOwnSections(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get profile sections: %w", err)
		}
		resp.Sections = sections

		return resp, nil
	}

	// Non-owner: apply privacy gating
	switch privacy.Contributions {
	case contracts.PrivacyVisible:
		stats, err := s.GetContributionStats(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get contribution stats: %w", err)
		}
		resp.Stats = stats
	case contracts.PrivacyCountOnly:
		stats, err := s.GetContributionStats(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get contribution stats: %w", err)
		}
		resp.StatsCount = &stats.TotalContributions
	}
	// contracts.PrivacyHidden: Stats and StatsCount both remain nil

	if privacy.LastActive == contracts.PrivacyVisible {
		resp.LastActive = &user.UpdatedAt
	}

	if privacy.ProfileSections == contracts.PrivacyVisible {
		sections, err := s.GetUserSections(user.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get profile sections: %w", err)
		}
		resp.Sections = sections
	}

	return resp, nil
}

// GetOwnProfile returns the authenticated user's own profile, bypassing visibility checks.
func (s *ContributorProfileService) GetOwnProfile(userID uint) (*contracts.PublicProfileResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User
	err := s.db.First(&user, userID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	stats, err := s.GetContributionStats(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get contribution stats: %w", err)
	}

	sections, err := s.GetOwnSections(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile sections: %w", err)
	}

	privacy := parsePrivacySettings(user.PrivacySettings)

	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	return &contracts.PublicProfileResponse{
		Username:          username,
		Bio:               user.Bio,
		AvatarURL:         user.AvatarURL,
		FirstName:         user.FirstName,
		ProfileVisibility: user.ProfileVisibility,
		UserTier:          user.UserTier,
		PrivacySettings:   &privacy,
		JoinedAt:          user.CreatedAt,
		LastActive:        &user.UpdatedAt,
		Stats:             stats,
		Sections:          sections,
	}, nil
}

// =============================================================================
// Privacy Settings
// =============================================================================

// UpdatePrivacySettings validates and persists new privacy settings for a user.
func (s *ContributorProfileService) UpdatePrivacySettings(userID uint, settings contracts.PrivacySettings) (*contracts.PrivacySettings, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if err := ValidatePrivacySettings(settings); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal privacy settings: %w", err)
	}
	rawMsg := json.RawMessage(raw)

	if err := s.db.Model(&models.User{}).Where("id = ?", userID).Update("privacy_settings", &rawMsg).Error; err != nil {
		return nil, fmt.Errorf("failed to update privacy settings: %w", err)
	}

	return &settings, nil
}

// =============================================================================
// Contribution Stats
// =============================================================================

// GetContributionStats computes aggregate contribution counts for a user.
func (s *ContributorProfileService) GetContributionStats(userID uint) (*contracts.ContributionStats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	stats := &contracts.ContributionStats{}

	// Count submissions from entity tables
	s.db.Model(&models.Show{}).Where("submitted_by = ?", userID).Count(&stats.ShowsSubmitted)
	s.db.Model(&models.Venue{}).Where("submitted_by = ?", userID).Count(&stats.VenuesSubmitted)
	s.db.Table("pending_entity_edits").
		Where("submitted_by = ? AND entity_type = ?", userID, models.PendingEditEntityVenue).
		Count(&stats.VenueEditsSubmitted)

	// Count actions from audit_log grouped by action
	type actionCount struct {
		Action string
		Count  int64
	}
	var actionCounts []actionCount
	s.db.Model(&models.AuditLog{}).
		Select("action, count(*) as count").
		Where("actor_id = ?", userID).
		Group("action").
		Scan(&actionCounts)

	for _, ac := range actionCounts {
		if moderationActions[ac.Action] {
			stats.ModerationActions += ac.Count
		} else {
			switch {
			case ac.Action == "create_release" || ac.Action == "edit_release":
				stats.ReleasesCreated += ac.Count
			case ac.Action == "create_label" || ac.Action == "edit_label":
				stats.LabelsCreated += ac.Count
			case ac.Action == "create_festival" || ac.Action == "edit_festival" ||
				ac.Action == "add_festival_artist" || ac.Action == "remove_festival_artist" ||
				ac.Action == "update_festival_artist" || ac.Action == "add_festival_venue" ||
				ac.Action == "remove_festival_venue":
				stats.FestivalsCreated += ac.Count
			case ac.Action == "edit_artist":
				stats.ArtistsEdited += ac.Count
			}
		}
	}

	// Revisions made
	s.db.Model(&models.Revision{}).Where("user_id = ?", userID).Count(&stats.RevisionsMade)

	// Pending entity edits submitted
	s.db.Model(&models.PendingEntityEdit{}).Where("submitted_by = ?", userID).Count(&stats.PendingEditsSubmitted)

	// Community participation: votes
	s.db.Model(&models.TagVote{}).Where("user_id = ?", userID).Count(&stats.TagVotesCast)
	s.db.Model(&models.ArtistRelationshipVote{}).Where("user_id = ?", userID).Count(&stats.RelationshipVotesCast)
	s.db.Model(&models.RequestVote{}).Where("user_id = ?", userID).Count(&stats.RequestVotesCast)

	// Community participation: collections
	s.db.Model(&models.CollectionItem{}).Where("added_by_user_id = ?", userID).Count(&stats.CollectionItemsAdded)
	s.db.Model(&models.CollectionSubscriber{}).Where("user_id = ?", userID).Count(&stats.CollectionSubscriptions)

	// Shows attended (user_bookmarks with action = 'going')
	s.db.Model(&models.UserBookmark{}).Where("user_id = ? AND action = ?", userID, models.BookmarkActionGoing).Count(&stats.ShowsAttended)

	// Reports filed (entity_reports + show_reports + artist_reports)
	var entityReportsFiled, showReportsFiled, artistReportsFiled int64
	s.db.Model(&models.EntityReport{}).Where("reported_by = ?", userID).Count(&entityReportsFiled)
	s.db.Model(&models.ShowReport{}).Where("reported_by = ?", userID).Count(&showReportsFiled)
	s.db.Model(&models.ArtistReport{}).Where("reported_by = ?", userID).Count(&artistReportsFiled)
	stats.ReportsFiled = entityReportsFiled + showReportsFiled + artistReportsFiled

	// Reports resolved (entity_reports reviewed by this user with resolved/dismissed status)
	var entityReportsResolved, showReportsResolved, artistReportsResolved int64
	s.db.Model(&models.EntityReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&entityReportsResolved)
	s.db.Model(&models.ShowReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&showReportsResolved)
	s.db.Model(&models.ArtistReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&artistReportsResolved)
	stats.ReportsResolved = entityReportsResolved + showReportsResolved + artistReportsResolved

	// Social: followers and following via user_bookmarks with action = 'follow'
	// Followers = other users who follow entities that *are* this user (not applicable with current schema)
	// Following = entities this user follows
	s.db.Model(&models.UserBookmark{}).Where("user_id = ? AND action = ?", userID, models.BookmarkActionFollow).Count(&stats.FollowingCount)
	// FollowersCount is not directly queryable in the current schema (bookmarks are entity-based, not user-to-user)
	// Leave at 0 until a user-to-user follow system exists

	// Approval rate from pending_entity_edits
	var approved, rejected int64
	s.db.Model(&models.PendingEntityEdit{}).Where("submitted_by = ? AND status = ?", userID, models.PendingEditStatusApproved).Count(&approved)
	s.db.Model(&models.PendingEntityEdit{}).Where("submitted_by = ? AND status = ?", userID, models.PendingEditStatusRejected).Count(&rejected)
	if total := approved + rejected; total > 0 {
		rate := float64(approved) / float64(total)
		stats.ApprovalRate = &rate
	}

	// Total contributions: content creation + moderation + community participation
	stats.TotalContributions = stats.ShowsSubmitted + stats.VenuesSubmitted +
		stats.VenueEditsSubmitted + stats.ReleasesCreated + stats.LabelsCreated +
		stats.FestivalsCreated + stats.ArtistsEdited + stats.ModerationActions +
		stats.RevisionsMade + stats.PendingEditsSubmitted +
		stats.TagVotesCast + stats.RelationshipVotesCast + stats.RequestVotesCast +
		stats.CollectionItemsAdded + stats.ReportsFiled + stats.ReportsResolved

	return stats, nil
}

// =============================================================================
// Contribution History
// =============================================================================

// GetContributionHistory returns a paginated, unified contribution timeline for a user.
func (s *ContributorProfileService) GetContributionHistory(userID uint, limit, offset int, entityType string) ([]*contracts.ContributionEntry, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	auditQuery := `SELECT id, action, entity_type, entity_id, metadata, created_at, 'audit_log' as source FROM audit_logs WHERE actor_id = ?`
	showQuery := `SELECT id, 'submit_show' as action, 'show' as entity_type, id as entity_id, NULL as metadata, created_at, 'submission' as source FROM shows WHERE submitted_by = ?`
	venueQuery := `SELECT id, 'submit_venue' as action, 'venue' as entity_type, id as entity_id, NULL as metadata, created_at, 'submission' as source FROM venues WHERE submitted_by = ?`
	// Suggested edits pulled from the unified pending_entity_edits table (PSY-503
	// retired the legacy pending_venue_edits queue). The source entity_type is
	// normalized back to "{type}_edit" so the activity UI keeps a distinct
	// event icon for edits vs. submissions.
	entityEditQuery := `SELECT id, 'submit_' || entity_type || '_edit' as action, entity_type || '_edit' as entity_type, entity_id as entity_id, NULL as metadata, created_at, 'submission' as source FROM pending_entity_edits WHERE submitted_by = ?`

	args := []interface{}{userID, userID, userID, userID}

	unionSQL := fmt.Sprintf("(%s) UNION ALL (%s) UNION ALL (%s) UNION ALL (%s)",
		auditQuery, showQuery, venueQuery, entityEditQuery)

	entityFilter := ""
	if entityType != "" {
		entityFilter = " WHERE entity_type = ?"
		args = append(args, entityType)
	}

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS unified%s", unionSQL, entityFilter)
	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	if err := s.db.Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count contributions: %w", err)
	}

	dataSQL := fmt.Sprintf("SELECT * FROM (%s) AS unified%s ORDER BY created_at DESC LIMIT ? OFFSET ?",
		unionSQL, entityFilter)
	args = append(args, limit, offset)

	var rows []contributionRow
	if err := s.db.Raw(dataSQL, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get contributions: %w", err)
	}

	entries := make([]*contracts.ContributionEntry, len(rows))
	for i, row := range rows {
		entry := &contracts.ContributionEntry{
			ID:         row.ID,
			Action:     row.Action,
			EntityType: row.EntityType,
			EntityID:   row.EntityID,
			CreatedAt:  row.CreatedAt,
			Source:     row.Source,
		}
		if row.Metadata != nil {
			var metadata map[string]interface{}
			if err := json.Unmarshal(*row.Metadata, &metadata); err == nil {
				entry.Metadata = metadata
			}
		}
		entries[i] = entry
	}

	s.enrichEntityNames(entries)

	return entries, total, nil
}

// =============================================================================
// Activity Heatmap
// =============================================================================

// GetActivityHeatmap returns daily contribution counts for the last 365 days.
// It aggregates activity across audit_logs, shows, venues, pending_entity_edits, and revisions.
// Only days with count > 0 are returned; the frontend fills in gaps.
func (s *ContributorProfileService) GetActivityHeatmap(userID uint) (*contracts.ActivityHeatmapResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT activity_date, SUM(cnt) AS total_count
		FROM (
			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM audit_logs
			WHERE actor_id = ? AND created_at >= NOW() - INTERVAL '365 days'
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM shows
			WHERE submitted_by = ? AND created_at >= NOW() - INTERVAL '365 days'
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM venues
			WHERE submitted_by = ? AND created_at >= NOW() - INTERVAL '365 days'
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM pending_entity_edits
			WHERE submitted_by = ? AND created_at >= NOW() - INTERVAL '365 days'
			GROUP BY DATE(created_at)

			UNION ALL

			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM revisions
			WHERE user_id = ? AND created_at >= NOW() - INTERVAL '365 days'
			GROUP BY DATE(created_at)
		) AS combined
		GROUP BY activity_date
		ORDER BY activity_date ASC
	`

	type dayRow struct {
		ActivityDate time.Time `gorm:"column:activity_date"`
		TotalCount   int       `gorm:"column:total_count"`
	}

	var rows []dayRow
	err := s.db.Raw(query, userID, userID, userID, userID, userID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get activity heatmap: %w", err)
	}

	days := make([]contracts.ActivityDay, len(rows))
	for i, row := range rows {
		days[i] = contracts.ActivityDay{
			Date:  row.ActivityDate.Format("2006-01-02"),
			Count: row.TotalCount,
		}
	}

	return &contracts.ActivityHeatmapResponse{Days: days}, nil
}

// =============================================================================
// Profile Sections
// =============================================================================

const maxProfileSections = 3

// GetUserSections returns visible profile sections for a public user.
func (s *ContributorProfileService) GetUserSections(userID uint) ([]*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var sections []models.UserProfileSection
	err := s.db.Where("user_id = ? AND is_visible = ?", userID, true).
		Order("position ASC").
		Find(&sections).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get profile sections: %w", err)
	}

	return buildSectionResponses(sections), nil
}

// GetOwnSections returns all profile sections for the authenticated user.
func (s *ContributorProfileService) GetOwnSections(userID uint) ([]*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var sections []models.UserProfileSection
	err := s.db.Where("user_id = ?", userID).
		Order("position ASC").
		Find(&sections).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get profile sections: %w", err)
	}

	return buildSectionResponses(sections), nil
}

// CreateSection creates a new profile section. Returns error if user already has max sections.
func (s *ContributorProfileService) CreateSection(userID uint, title string, content string, position int) (*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if len(title) == 0 || len(title) > 255 {
		return nil, fmt.Errorf("title must be between 1 and 255 characters")
	}
	if len(content) > 10000 {
		return nil, fmt.Errorf("content must be at most 10000 characters")
	}
	if position < 0 || position >= maxProfileSections {
		return nil, fmt.Errorf("position must be between 0 and %d", maxProfileSections-1)
	}

	// Check section count
	var count int64
	s.db.Model(&models.UserProfileSection{}).Where("user_id = ?", userID).Count(&count)
	if count >= int64(maxProfileSections) {
		return nil, fmt.Errorf("maximum %d profile sections allowed", maxProfileSections)
	}

	section := models.UserProfileSection{
		UserID:    userID,
		Title:     title,
		Content:   content,
		Position:  position,
		IsVisible: true,
	}

	if err := s.db.Create(&section).Error; err != nil {
		return nil, fmt.Errorf("failed to create profile section: %w", err)
	}

	return buildSectionResponse(&section), nil
}

// UpdateSection updates a profile section owned by the user.
func (s *ContributorProfileService) UpdateSection(userID uint, sectionID uint, updates map[string]interface{}) (*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var section models.UserProfileSection
	err := s.db.Where("id = ? AND user_id = ?", sectionID, userID).First(&section).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("profile section not found")
		}
		return nil, fmt.Errorf("failed to find profile section: %w", err)
	}

	// Validate updates
	if title, ok := updates["title"]; ok {
		t := title.(string)
		if len(t) == 0 || len(t) > 255 {
			return nil, fmt.Errorf("title must be between 1 and 255 characters")
		}
	}
	if content, ok := updates["content"]; ok {
		c := content.(string)
		if len(c) > 10000 {
			return nil, fmt.Errorf("content must be at most 10000 characters")
		}
	}
	if position, ok := updates["position"]; ok {
		p := position.(int)
		if p < 0 || p >= maxProfileSections {
			return nil, fmt.Errorf("position must be between 0 and %d", maxProfileSections-1)
		}
	}

	if err := s.db.Model(&section).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update profile section: %w", err)
	}

	// Reload after update
	s.db.First(&section, section.ID)

	return buildSectionResponse(&section), nil
}

// DeleteSection deletes a profile section owned by the user.
func (s *ContributorProfileService) DeleteSection(userID uint, sectionID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("id = ? AND user_id = ?", sectionID, userID).Delete(&models.UserProfileSection{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete profile section: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("profile section not found")
	}

	return nil
}

func buildSectionResponses(sections []models.UserProfileSection) []*contracts.ProfileSectionResponse {
	responses := make([]*contracts.ProfileSectionResponse, len(sections))
	for i := range sections {
		responses[i] = buildSectionResponse(&sections[i])
	}
	return responses
}

func buildSectionResponse(section *models.UserProfileSection) *contracts.ProfileSectionResponse {
	return &contracts.ProfileSectionResponse{
		ID:        section.ID,
		Title:     section.Title,
		Content:   section.Content,
		Position:  section.Position,
		IsVisible: section.IsVisible,
		CreatedAt: section.CreatedAt,
		UpdatedAt: section.UpdatedAt,
	}
}

// =============================================================================
// Percentile Rankings
// =============================================================================

// percentileDimension describes a single contribution dimension for ranking.
type percentileDimension struct {
	key    string // e.g. "shows_submitted"
	label  string // human-readable label
	weight int    // weight for overall score
}

var percentileDimensions = []percentileDimension{
	{key: "shows_submitted", label: "Shows Submitted", weight: 25},
	{key: "venues_submitted", label: "Venues Submitted", weight: 15},
	{key: "tags_applied", label: "Tags Applied", weight: 10},
	{key: "edits_approved", label: "Edits Approved", weight: 25},
	{key: "requests_fulfilled", label: "Requests Fulfilled", weight: 10},
}

// GetPercentileRankings computes percentile rankings for a user across 5 contribution dimensions.
// Returns nil if fewer than 10 active users exist (rankings not meaningful).
func (s *ContributorProfileService) GetPercentileRankings(userID uint) (*contracts.PercentileRankings, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check total active users
	var totalUsers int64
	if err := s.db.Model(&models.User{}).Where("is_active = ?", true).Count(&totalUsers).Error; err != nil {
		return nil, fmt.Errorf("failed to count active users: %w", err)
	}
	if totalUsers < 10 {
		return nil, nil
	}

	// Get user's counts for each dimension
	userCounts := make(map[string]int64)

	// shows_submitted
	var showCount int64
	s.db.Model(&models.Show{}).Where("submitted_by = ?", userID).Count(&showCount)
	userCounts["shows_submitted"] = showCount

	// venues_submitted
	var venueCount int64
	s.db.Model(&models.Venue{}).Where("submitted_by = ?", userID).Count(&venueCount)
	userCounts["venues_submitted"] = venueCount

	// tags_applied
	var tagCount int64
	s.db.Model(&models.EntityTag{}).Where("added_by_user_id = ?", userID).Count(&tagCount)
	userCounts["tags_applied"] = tagCount

	// edits_approved: pending_entity_edits approved + revisions
	var pendingEditsApproved int64
	s.db.Model(&models.PendingEntityEdit{}).
		Where("submitted_by = ? AND status = ?", userID, models.PendingEditStatusApproved).
		Count(&pendingEditsApproved)
	var revisionCount int64
	s.db.Model(&models.Revision{}).Where("user_id = ?", userID).Count(&revisionCount)
	userCounts["edits_approved"] = pendingEditsApproved + revisionCount

	// requests_fulfilled
	var requestsFulfilledCount int64
	s.db.Model(&models.Request{}).Where("fulfiller_id = ?", userID).Count(&requestsFulfilledCount)
	userCounts["requests_fulfilled"] = requestsFulfilledCount

	// For each dimension, compute the percentile.
	// Use subquery pattern: count users whose contribution count for this dimension
	// is strictly less than the target user's count. Users with 0 contributions are
	// included via LEFT JOIN + COALESCE in a wrapping subquery.
	rankings := make([]contracts.PercentileRanking, 0, len(percentileDimensions))
	weightedSum := 0
	totalWeight := 0

	for _, dim := range percentileDimensions {
		userVal := userCounts[dim.key]
		var usersWithLess int64

		switch dim.key {
		case "shows_submitted":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(s.id) AS cnt
					FROM users u
					LEFT JOIN shows s ON s.submitted_by = u.id
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "venues_submitted":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(v.id) AS cnt
					FROM users u
					LEFT JOIN venues v ON v.submitted_by = u.id
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "tags_applied":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(et.id) AS cnt
					FROM users u
					LEFT JOIN entity_tags et ON et.added_by_user_id = u.id
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "edits_approved":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id,
						COALESCE((SELECT COUNT(*) FROM pending_entity_edits pe WHERE pe.submitted_by = u.id AND pe.status = 'approved'), 0) +
						COALESCE((SELECT COUNT(*) FROM revisions r WHERE r.user_id = u.id), 0) AS cnt
					FROM users u
					WHERE u.is_active = true
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		case "requests_fulfilled":
			s.db.Raw(`
				SELECT COUNT(*) FROM (
					SELECT u.id, COUNT(req.id) AS cnt
					FROM users u
					LEFT JOIN requests req ON req.fulfiller_id = u.id
					WHERE u.is_active = true
					GROUP BY u.id
				) sub WHERE sub.cnt < ?
			`, userVal).Scan(&usersWithLess)
		}

		percentile := int(float64(usersWithLess) / float64(totalUsers) * 100)
		if percentile > 100 {
			percentile = 100
		}

		rankings = append(rankings, contracts.PercentileRanking{
			Dimension:  dim.key,
			Label:      dim.label,
			Percentile: percentile,
			Value:      userVal,
		})

		weightedSum += percentile * dim.weight
		totalWeight += dim.weight
	}

	overallScore := 0
	if totalWeight > 0 {
		overallScore = weightedSum / totalWeight
	}

	return &contracts.PercentileRankings{
		Rankings:     rankings,
		OverallScore: overallScore,
	}, nil
}

// =============================================================================
// Entity Name Enrichment
// =============================================================================

func (s *ContributorProfileService) enrichEntityNames(entries []*contracts.ContributionEntry) {
	idsByType := make(map[string][]uint)
	for _, e := range entries {
		idsByType[e.EntityType] = append(idsByType[e.EntityType], e.EntityID)
	}

	nameMap := make(map[string]map[uint]string)

	for entityType, ids := range idsByType {
		if len(ids) == 0 {
			continue
		}
		names := make(map[uint]string)
		switch entityType {
		case "show":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("shows").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "venue", "venue_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("venues").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "artist":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("artists").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "release":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("releases").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "label":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("labels").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "festival":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("festivals").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "request":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("requests").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "collection":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("collections").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		}
		nameMap[entityType] = names
	}

	for _, e := range entries {
		if names, ok := nameMap[e.EntityType]; ok {
			if name, ok := names[e.EntityID]; ok {
				e.EntityName = name
			}
		}
	}
}
