package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// profileMarkdown renders profile-section Content to sanitized HTML. The
// renderer is stateless after construction (goldmark + bluemonday), so a single
// package-level instance is shared by the package-level buildSectionResponse
// helpers — same policy used by tag/collection descriptions (PSY-747).
var profileMarkdown = utils.NewMarkdownRenderer()

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

	var user authm.User
	err := s.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		BioHTML:           renderBioHTML(user.Bio),
		AvatarURL:         user.AvatarURL,
		DisplayName:       user.DisplayName,
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

	var user authm.User
	err := s.db.First(&user, userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		BioHTML:           renderBioHTML(user.Bio),
		AvatarURL:         user.AvatarURL,
		DisplayName:       user.DisplayName,
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

	if err := s.db.Model(&authm.User{}).Where("id = ?", userID).Update("privacy_settings", &rawMsg).Error; err != nil {
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
	s.db.Model(&catalogm.Show{}).Where("submitted_by = ?", userID).Count(&stats.ShowsSubmitted)
	s.db.Model(&catalogm.Venue{}).Where("submitted_by = ?", userID).Count(&stats.VenuesSubmitted)
	s.db.Table("pending_entity_edits").
		Where("submitted_by = ? AND entity_type = ?", userID, adminm.PendingEditEntityVenue).
		Count(&stats.VenueEditsSubmitted)

	// Count actions from audit_log grouped by action.
	// PSY-618: edit_<type> rows live in entity_edit_audit_logs now and are
	// counted separately below so the contributor activity feed stops
	// dual-rendering trusted-user direct-edits and the stats counters read
	// from a single source of truth.
	type actionCount struct {
		Action string
		Count  int64
	}
	var actionCounts []actionCount
	s.db.Model(&adminm.AuditLog{}).
		Select("action, count(*) as count").
		Where("actor_id = ?", userID).
		Group("action").
		Scan(&actionCounts)

	for _, ac := range actionCounts {
		if moderationActions[ac.Action] {
			stats.ModerationActions += ac.Count
		} else {
			switch ac.Action {
			case "create_release":
				stats.ReleasesCreated += ac.Count
			case "create_label":
				stats.LabelsCreated += ac.Count
			case "create_festival",
				"add_festival_artist", "remove_festival_artist",
				"update_festival_artist",
				"add_festival_venue", "remove_festival_venue":
				stats.FestivalsCreated += ac.Count
			}
		}
	}

	// Count edit events from entity_edit_audit_logs (PSY-618). Edits used to
	// live in audit_logs as "edit_<type>" actions but were split out so
	// trusted-user direct-edits stop dual-rendering in the activity feed.
	type entityEditCount struct {
		EntityType string
		Count      int64
	}
	var entityEditCounts []entityEditCount
	s.db.Model(&adminm.EntityEditAuditLog{}).
		Select("entity_type, count(*) as count").
		Where("actor_id = ?", userID).
		Group("entity_type").
		Scan(&entityEditCounts)

	for _, ec := range entityEditCounts {
		switch ec.EntityType {
		case "artist":
			stats.ArtistsEdited += ec.Count
		case "release":
			stats.ReleasesCreated += ec.Count
		case "label":
			stats.LabelsCreated += ec.Count
		case "festival":
			stats.FestivalsCreated += ec.Count
		}
	}

	// Revisions made
	s.db.Model(&adminm.Revision{}).Where("user_id = ?", userID).Count(&stats.RevisionsMade)

	// Pending entity edits submitted
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ?", userID).Count(&stats.PendingEditsSubmitted)

	// Community participation: votes
	s.db.Model(&catalogm.TagVote{}).Where("user_id = ?", userID).Count(&stats.TagVotesCast)
	s.db.Model(&catalogm.ArtistRelationshipVote{}).Where("user_id = ?", userID).Count(&stats.RelationshipVotesCast)
	s.db.Model(&communitym.RequestVote{}).Where("user_id = ?", userID).Count(&stats.RequestVotesCast)

	// Community participation: collections
	s.db.Model(&communitym.CollectionItem{}).Where("added_by_user_id = ?", userID).Count(&stats.CollectionItemsAdded)
	s.db.Model(&communitym.CollectionSubscriber{}).Where("user_id = ?", userID).Count(&stats.CollectionSubscriptions)

	// Reports filed (entity_reports + show_reports + artist_reports)
	var entityReportsFiled, showReportsFiled, artistReportsFiled int64
	s.db.Model(&communitym.EntityReport{}).Where("reported_by = ?", userID).Count(&entityReportsFiled)
	s.db.Model(&communitym.ShowReport{}).Where("reported_by = ?", userID).Count(&showReportsFiled)
	s.db.Model(&communitym.ArtistReport{}).Where("reported_by = ?", userID).Count(&artistReportsFiled)
	stats.ReportsFiled = entityReportsFiled + showReportsFiled + artistReportsFiled

	// Reports resolved (entity_reports reviewed by this user with resolved/dismissed status)
	var entityReportsResolved, showReportsResolved, artistReportsResolved int64
	s.db.Model(&communitym.EntityReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&entityReportsResolved)
	s.db.Model(&communitym.ShowReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&showReportsResolved)
	s.db.Model(&communitym.ArtistReport{}).Where("reviewed_by = ? AND status IN ?", userID, []string{"resolved", "dismissed"}).Count(&artistReportsResolved)
	stats.ReportsResolved = entityReportsResolved + showReportsResolved + artistReportsResolved

	// Social: followers and following via user_bookmarks with action = 'follow'
	// (PSY-1496). Followers = other users who follow this user (entity_type=user).
	// Following = catalog entities this user follows — exclude entity_type=user
	// so the count stays "entities I follow" (aligned with ProfileFollowing / Library).
	s.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND action = ? AND entity_type <> ?",
			userID, engagementm.BookmarkActionFollow, "user").
		Count(&stats.FollowingCount)
	s.db.Model(&engagementm.UserBookmark{}).
		Where("entity_type = ? AND entity_id = ? AND action = ?",
			"user", userID, engagementm.BookmarkActionFollow).
		Count(&stats.FollowersCount)

	// Approval rate from pending_entity_edits
	var approved, rejected int64
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ? AND status = ?", userID, adminm.PendingEditStatusApproved).Count(&approved)
	s.db.Model(&adminm.PendingEntityEdit{}).Where("submitted_by = ? AND status = ?", userID, adminm.PendingEditStatusRejected).Count(&rejected)
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
	// Direct entity-edit audits live in their own table post-PSY-618. These
	// are the terminal "edit applied" events (formerly `edit_<type>` rows in
	// audit_logs). We synthesise the `edit_<type>` action prefix so the
	// frontend's icon/label map — which keys on action="edit_artist" et al.
	// — keeps working without churn.
	entityEditAuditQuery := `SELECT id, 'edit_' || entity_type as action, entity_type, entity_id, metadata, created_at, 'edit_audit' as source FROM entity_edit_audit_logs WHERE actor_id = ?`

	args := []interface{}{userID, userID, userID, userID, userID}

	unionSQL := fmt.Sprintf("(%s) UNION ALL (%s) UNION ALL (%s) UNION ALL (%s) UNION ALL (%s)",
		auditQuery, showQuery, venueQuery, entityEditQuery, entityEditAuditQuery)

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

			UNION ALL

			-- PSY-618: edit_<type> rows moved out of audit_logs into
			-- entity_edit_audit_logs. Without this UNION the heatmap
			-- under-counts trusted-user direct edits post-backfill.
			SELECT DATE(created_at) AS activity_date, COUNT(*) AS cnt
			FROM entity_edit_audit_logs
			WHERE actor_id = ? AND created_at >= NOW() - INTERVAL '365 days'
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
	err := s.db.Raw(query, userID, userID, userID, userID, userID, userID).Scan(&rows).Error
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

	var sections []authm.UserProfileSection
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

	var sections []authm.UserProfileSection
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
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("database not initialized"))
	}

	if len(title) == 0 || len(title) > 255 {
		return nil, apperrors.ErrProfileSectionInvalid("title must be between 1 and 255 characters")
	}
	if len(content) > 10000 {
		return nil, apperrors.ErrProfileSectionInvalid("content must be at most 10000 characters")
	}
	if position < 0 || position >= maxProfileSections {
		return nil, apperrors.ErrProfileSectionInvalid(fmt.Sprintf("position must be between 0 and %d", maxProfileSections-1))
	}

	// Check section count
	var count int64
	s.db.Model(&authm.UserProfileSection{}).Where("user_id = ?", userID).Count(&count)
	if count >= int64(maxProfileSections) {
		return nil, apperrors.ErrProfileSectionInvalid(fmt.Sprintf("maximum %d profile sections allowed", maxProfileSections))
	}

	section := authm.UserProfileSection{
		UserID:    userID,
		Title:     title,
		Content:   content,
		Position:  position,
		IsVisible: true,
	}

	if err := s.db.Create(&section).Error; err != nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("failed to create profile section: %w", err))
	}

	return buildSectionResponse(&section), nil
}

// UpdateSection updates a profile section owned by the user.
func (s *ContributorProfileService) UpdateSection(userID uint, sectionID uint, updates map[string]interface{}) (*contracts.ProfileSectionResponse, error) {
	if s.db == nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("database not initialized"))
	}

	var section authm.UserProfileSection
	err := s.db.Where("id = ? AND user_id = ?", sectionID, userID).First(&section).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrProfileSectionNotFound()
		}
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("failed to find profile section: %w", err))
	}

	// Validate updates
	if title, ok := updates["title"]; ok {
		t := title.(string)
		if len(t) == 0 || len(t) > 255 {
			return nil, apperrors.ErrProfileSectionInvalid("title must be between 1 and 255 characters")
		}
	}
	if content, ok := updates["content"]; ok {
		c := content.(string)
		if len(c) > 10000 {
			return nil, apperrors.ErrProfileSectionInvalid("content must be at most 10000 characters")
		}
	}
	if position, ok := updates["position"]; ok {
		p := position.(int)
		if p < 0 || p >= maxProfileSections {
			return nil, apperrors.ErrProfileSectionInvalid(fmt.Sprintf("position must be between 0 and %d", maxProfileSections-1))
		}
	}

	if err := s.db.Model(&section).Updates(updates).Error; err != nil {
		return nil, apperrors.ErrProfileInternal(fmt.Errorf("failed to update profile section: %w", err))
	}

	// Reload after update
	s.db.First(&section, section.ID)

	return buildSectionResponse(&section), nil
}

// DeleteSection deletes a profile section owned by the user.
func (s *ContributorProfileService) DeleteSection(userID uint, sectionID uint) error {
	if s.db == nil {
		return apperrors.ErrProfileInternal(fmt.Errorf("database not initialized"))
	}

	result := s.db.Where("id = ? AND user_id = ?", sectionID, userID).Delete(&authm.UserProfileSection{})
	if result.Error != nil {
		return apperrors.ErrProfileInternal(fmt.Errorf("failed to delete profile section: %w", result.Error))
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrProfileSectionNotFound()
	}

	return nil
}

// renderBioHTML renders a user's raw bio markdown to sanitized HTML using the
// shared profileMarkdown renderer (goldmark + bluemonday), mirroring profile
// sections. Returns "" when the bio is nil or empty so the response omits
// bio_html.
func renderBioHTML(bio *string) string {
	if bio == nil {
		return ""
	}
	return profileMarkdown.Render(*bio)
}

func buildSectionResponses(sections []authm.UserProfileSection) []*contracts.ProfileSectionResponse {
	responses := make([]*contracts.ProfileSectionResponse, len(sections))
	for i := range sections {
		responses[i] = buildSectionResponse(&sections[i])
	}
	return responses
}

func buildSectionResponse(section *authm.UserProfileSection) *contracts.ProfileSectionResponse {
	return &contracts.ProfileSectionResponse{
		ID:          section.ID,
		Title:       section.Title,
		Content:     section.Content,
		ContentHTML: profileMarkdown.Render(section.Content),
		Position:    section.Position,
		IsVisible:   section.IsVisible,
		CreatedAt:   section.CreatedAt,
		UpdatedAt:   section.UpdatedAt,
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
	if err := s.db.Model(&authm.User{}).Where("is_active = ?", true).Count(&totalUsers).Error; err != nil {
		return nil, fmt.Errorf("failed to count active users: %w", err)
	}
	if totalUsers < 10 {
		return nil, nil
	}

	// Get user's counts for each dimension
	userCounts := make(map[string]int64)

	// shows_submitted
	var showCount int64
	s.db.Model(&catalogm.Show{}).Where("submitted_by = ?", userID).Count(&showCount)
	userCounts["shows_submitted"] = showCount

	// venues_submitted
	var venueCount int64
	s.db.Model(&catalogm.Venue{}).Where("submitted_by = ?", userID).Count(&venueCount)
	userCounts["venues_submitted"] = venueCount

	// tags_applied
	var tagCount int64
	s.db.Model(&catalogm.EntityTag{}).Where("added_by_user_id = ?", userID).Count(&tagCount)
	userCounts["tags_applied"] = tagCount

	// edits_approved: pending_entity_edits approved + revisions
	var pendingEditsApproved int64
	s.db.Model(&adminm.PendingEntityEdit{}).
		Where("submitted_by = ? AND status = ?", userID, adminm.PendingEditStatusApproved).
		Count(&pendingEditsApproved)
	var revisionCount int64
	s.db.Model(&adminm.Revision{}).Where("user_id = ?", userID).Count(&revisionCount)
	userCounts["edits_approved"] = pendingEditsApproved + revisionCount

	// requests_fulfilled
	var requestsFulfilledCount int64
	s.db.Model(&communitym.Request{}).Where("fulfiller_id = ?", userID).Count(&requestsFulfilledCount)
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
		// "<type>_edit" cases handle the synthetic discriminator emitted by
		// the pending_entity_edits UNION in GetContributionHistory; they
		// resolve from the same underlying table as their base type.
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
		case "artist", "artist_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("artists").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "release", "release_edit":
			var results []struct {
				ID    uint
				Title string
			}
			s.db.Table("releases").Select("id, title").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Title
			}
		case "label", "label_edit":
			var results []struct {
				ID   uint
				Name string
			}
			s.db.Table("labels").Select("id, name").Where("id IN ?", ids).Scan(&results)
			for _, r := range results {
				names[r.ID] = r.Name
			}
		case "festival", "festival_edit":
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
