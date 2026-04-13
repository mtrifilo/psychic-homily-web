package admin

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminStatsService handles admin dashboard statistics
type AdminStatsService struct {
	db *gorm.DB
}

// NewAdminStatsService creates a new admin stats service
func NewAdminStatsService(database *gorm.DB) *AdminStatsService {
	if database == nil {
		database = db.GetDB()
	}
	return &AdminStatsService{
		db: database,
	}
}

// GetDashboardStats returns all dashboard statistics
func (s *AdminStatsService) GetDashboardStats() (*contracts.AdminDashboardStats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	stats := &contracts.AdminDashboardStats{}
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	// Action items
	if err := s.db.Model(&models.Show{}).Where("status = ?", models.ShowStatusPending).Count(&stats.PendingShows).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.PendingVenueEdit{}).Where("status = ?", "pending").Count(&stats.PendingVenueEdits).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.ShowReport{}).Where("status = ?", models.ShowReportStatusPending).Count(&stats.PendingReports).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.ArtistReport{}).Where("status = ?", models.ShowReportStatusPending).Count(&stats.PendingArtistReports).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.Venue{}).Where("verified = ?", false).Count(&stats.UnverifiedVenues).Error; err != nil {
		return nil, err
	}

	// Content totals
	if err := s.db.Model(&models.Show{}).Where("status = ?", models.ShowStatusApproved).Count(&stats.TotalShows).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.Venue{}).Where("verified = ?", true).Count(&stats.TotalVenues).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.Artist{}).Count(&stats.TotalArtists).Error; err != nil {
		return nil, err
	}

	// Users
	if err := s.db.Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}

	// Recent activity
	if err := s.db.Model(&models.Show{}).Where("created_at > ?", sevenDaysAgo).Count(&stats.ShowsSubmittedLast7Days).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&models.User{}).Where("created_at > ?", sevenDaysAgo).Count(&stats.UsersRegisteredLast7Days).Error; err != nil {
		return nil, err
	}

	// Period-over-period trends (current 7 days vs previous 7 days)
	fourteenDaysAgo := time.Now().AddDate(0, 0, -14)

	var showsCurrent, showsPrevious int64
	if err := s.db.Model(&models.Show{}).Where("status = ? AND created_at > ?", models.ShowStatusApproved, sevenDaysAgo).Count(&showsCurrent).Error; err != nil {
		// Log but don't fail — trends are non-critical
		showsCurrent = 0
	}
	if err := s.db.Model(&models.Show{}).Where("status = ? AND created_at > ? AND created_at <= ?", models.ShowStatusApproved, fourteenDaysAgo, sevenDaysAgo).Count(&showsPrevious).Error; err != nil {
		showsPrevious = 0
	}
	stats.TotalShowsTrend = showsCurrent - showsPrevious

	var venuesCurrent, venuesPrevious int64
	if err := s.db.Model(&models.Venue{}).Where("verified = ? AND created_at > ?", true, sevenDaysAgo).Count(&venuesCurrent).Error; err != nil {
		venuesCurrent = 0
	}
	if err := s.db.Model(&models.Venue{}).Where("verified = ? AND created_at > ? AND created_at <= ?", true, fourteenDaysAgo, sevenDaysAgo).Count(&venuesPrevious).Error; err != nil {
		venuesPrevious = 0
	}
	stats.TotalVenuesTrend = venuesCurrent - venuesPrevious

	var artistsCurrent, artistsPrevious int64
	if err := s.db.Model(&models.Artist{}).Where("created_at > ?", sevenDaysAgo).Count(&artistsCurrent).Error; err != nil {
		artistsCurrent = 0
	}
	if err := s.db.Model(&models.Artist{}).Where("created_at > ? AND created_at <= ?", fourteenDaysAgo, sevenDaysAgo).Count(&artistsPrevious).Error; err != nil {
		artistsPrevious = 0
	}
	stats.TotalArtistsTrend = artistsCurrent - artistsPrevious

	var usersCurrent, usersPrevious int64
	if err := s.db.Model(&models.User{}).Where("created_at > ?", sevenDaysAgo).Count(&usersCurrent).Error; err != nil {
		usersCurrent = 0
	}
	if err := s.db.Model(&models.User{}).Where("created_at > ? AND created_at <= ?", fourteenDaysAgo, sevenDaysAgo).Count(&usersPrevious).Error; err != nil {
		usersPrevious = 0
	}
	stats.TotalUsersTrend = usersCurrent - usersPrevious

	return stats, nil
}

// GetRecentActivity returns the 20 most recent admin-relevant events from the audit log.
func (s *AdminStatsService) GetRecentActivity() (*contracts.ActivityFeedResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var logs []models.AuditLog
	if err := s.db.Preload("Actor").Order("created_at DESC").Limit(20).Find(&logs).Error; err != nil {
		return nil, err
	}

	events := make([]contracts.ActivityEvent, 0, len(logs))
	for _, log := range logs {
		event := contracts.ActivityEvent{
			ID:         log.ID,
			EventType:  mapActionToEventType(log.Action),
			Description: buildDescription(log.Action, log.EntityType, log.EntityID),
			EntityType: normalizeEntityType(log.EntityType),
			Timestamp:  log.CreatedAt,
		}

		// Resolve actor name
		if log.Actor != nil {
			event.ActorName = resolveActorName(log.Actor)
		}

		// Resolve entity slug
		event.EntitySlug = s.resolveEntitySlug(log.EntityType, log.EntityID)

		events = append(events, event)
	}

	return &contracts.ActivityFeedResponse{Events: events}, nil
}

// mapActionToEventType maps an audit log action string to a human-friendly event type.
func mapActionToEventType(action string) string {
	mapping := map[string]string{
		"approve_show":               "show_approved",
		"reject_show":                "show_rejected",
		"verify_venue":               "venue_verified",
		"create_artist":              "artist_created",
		"edit_artist":                "artist_edited",
		"create_venue":               "venue_created",
		"create_show":                "show_created",
		"approve_venue_edit":         "venue_edit_approved",
		"reject_venue_edit":          "venue_edit_rejected",
		"create_label":               "label_created",
		"edit_label":                 "label_edited",
		"delete_label":               "label_deleted",
		"create_release":             "release_created",
		"edit_release":               "release_edited",
		"delete_release":             "release_deleted",
		"create_festival":            "festival_created",
		"edit_festival":              "festival_edited",
		"delete_festival":            "festival_deleted",
		"create_crate":               "crate_created",
		"update_crate":               "crate_updated",
		"delete_crate":               "crate_deleted",
		"create_collection":          "crate_created",
		"update_collection":          "crate_updated",
		"delete_collection":          "crate_deleted",
		"create_request":             "request_created",
		"fulfill_request":            "request_fulfilled",
		"close_request":              "request_closed",
		"create_tag":                 "tag_created",
		"update_tag":                 "tag_updated",
		"delete_tag":                 "tag_deleted",
		"merge_artists":              "artists_merged",
		"add_artist_alias":           "artist_alias_added",
		"dismiss_report":             "report_dismissed",
		"resolve_report":             "report_resolved",
		"dismiss_artist_report":      "artist_report_dismissed",
		"resolve_artist_report":      "artist_report_resolved",
		"revision_rollback":          "revision_rolled_back",
		"create_artist_relationship": "artist_relationship_created",
		"delete_artist_relationship": "artist_relationship_deleted",
		"set_crate_featured":         "crate_featured",
		"set_collection_featured":    "crate_featured",
	}
	if eventType, ok := mapping[action]; ok {
		return eventType
	}
	return action
}

// buildDescription creates a human-readable description for an audit log event.
func buildDescription(action, entityType string, entityID uint) string {
	actionDescriptions := map[string]string{
		"approve_show":               "Show #%d was approved",
		"reject_show":                "Show #%d was rejected",
		"verify_venue":               "Venue #%d was verified",
		"create_artist":              "Artist #%d was created",
		"edit_artist":                "Artist #%d was edited",
		"create_venue":               "Venue #%d was created",
		"create_show":                "Show #%d was created",
		"approve_venue_edit":         "Venue edit #%d was approved",
		"reject_venue_edit":          "Venue edit #%d was rejected",
		"create_label":               "Label #%d was created",
		"edit_label":                 "Label #%d was edited",
		"delete_label":               "Label #%d was deleted",
		"create_release":             "Release #%d was created",
		"edit_release":               "Release #%d was edited",
		"delete_release":             "Release #%d was deleted",
		"create_festival":            "Festival #%d was created",
		"edit_festival":              "Festival #%d was edited",
		"delete_festival":            "Festival #%d was deleted",
		"create_crate":               "Collection #%d was created",
		"update_crate":               "Collection #%d was updated",
		"delete_crate":               "Collection was deleted",
		"create_collection":          "Collection #%d was created",
		"update_collection":          "Collection #%d was updated",
		"delete_collection":          "Collection was deleted",
		"create_request":             "Request #%d was created",
		"fulfill_request":            "Request #%d was fulfilled",
		"close_request":              "Request #%d was closed",
		"create_tag":                 "Tag #%d was created",
		"update_tag":                 "Tag #%d was updated",
		"delete_tag":                 "Tag #%d was deleted",
		"merge_artists":              "Artist #%d was merged",
		"add_artist_alias":           "Alias added to artist #%d",
		"dismiss_report":             "Report #%d was dismissed",
		"resolve_report":             "Report #%d was resolved",
		"dismiss_artist_report":      "Artist report #%d was dismissed",
		"resolve_artist_report":      "Artist report #%d was resolved",
		"revision_rollback":          "Revision #%d was rolled back",
		"create_artist_relationship": "Artist relationship created for artist #%d",
		"delete_artist_relationship": "Artist relationship deleted for artist #%d",
		"set_crate_featured":         "Collection featured status changed",
		"set_collection_featured":    "Collection featured status changed",
	}
	if template, ok := actionDescriptions[action]; ok {
		if strings.Contains(template, "%d") {
			return fmt.Sprintf(template, entityID)
		}
		return template
	}
	// Fallback: format the action nicely
	readable := strings.ReplaceAll(action, "_", " ")
	return fmt.Sprintf("%s #%d (%s)", capitalizeFirst(readable), entityID, entityType)
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// normalizeEntityType maps audit log entity types to the standard entity types used in URLs.
func normalizeEntityType(entityType string) string {
	mapping := map[string]string{
		"show":            "show",
		"venue":           "venue",
		"artist":          "artist",
		"venue_edit":      "venue",
		"show_report":     "show",
		"artist_report":   "artist",
		"label":           "label",
		"release":         "release",
		"festival":        "festival",
		"collection":      "collection",
		"request":         "request",
		"tag":             "tag",
		"revision":        "",
	}
	if normalized, ok := mapping[entityType]; ok {
		return normalized
	}
	return entityType
}

// resolveActorName returns a display name from a User model.
func resolveActorName(user *models.User) string {
	if user.FirstName != nil && user.LastName != nil && *user.FirstName != "" {
		return *user.FirstName + " " + *user.LastName
	}
	if user.Username != nil && *user.Username != "" {
		return *user.Username
	}
	if user.Email != nil && *user.Email != "" {
		return *user.Email
	}
	return "Unknown"
}

// resolveEntitySlug looks up the slug for an entity. Returns empty string if not found.
func (s *AdminStatsService) resolveEntitySlug(entityType string, entityID uint) string {
	var slug *string

	switch entityType {
	case "show":
		var show models.Show
		if err := s.db.Select("slug").First(&show, entityID).Error; err != nil {
			return ""
		}
		slug = show.Slug
	case "venue", "venue_edit":
		var venue models.Venue
		venueID := entityID
		// For venue_edit, the entityID is the edit ID, try to resolve the venue
		if entityType == "venue_edit" {
			var edit models.PendingVenueEdit
			if err := s.db.Select("venue_id").First(&edit, entityID).Error; err != nil {
				return ""
			}
			venueID = edit.VenueID
		}
		if err := s.db.Select("slug").First(&venue, venueID).Error; err != nil {
			return ""
		}
		slug = venue.Slug
	case "artist":
		var artist models.Artist
		if err := s.db.Select("slug").First(&artist, entityID).Error; err != nil {
			return ""
		}
		slug = artist.Slug
	default:
		return ""
	}

	if slug != nil {
		return *slug
	}
	return ""
}
