package admin

import (
	"fmt"

	"gorm.io/gorm"
)

// resolveEntityName looks up an entity's display name by type and ID.
// Returns the entity's name/title, or a fallback like "artist #123" if the lookup fails.
func resolveEntityName(db *gorm.DB, entityType string, entityID uint) string {
	if db == nil {
		return fmt.Sprintf("%s #%d", entityType, entityID)
	}

	switch entityType {
	case "artist":
		var result struct{ Name string }
		if err := db.Table("artists").Select("name").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Name != "" {
			return result.Name
		}
	case "venue":
		var result struct{ Name string }
		if err := db.Table("venues").Select("name").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Name != "" {
			return result.Name
		}
	case "festival":
		var result struct{ Name string }
		if err := db.Table("festivals").Select("name").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Name != "" {
			return result.Name
		}
	case "show":
		var result struct{ Title string }
		if err := db.Table("shows").Select("title").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Title != "" {
			return result.Title
		}
	case "comment":
		// For comments, return a truncated body as the "name"
		var result struct{ Body string }
		if err := db.Table("comments").Select("body").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Body != "" {
			if len(result.Body) > 60 {
				return result.Body[:60] + "..."
			}
			return result.Body
		}
	case "collection":
		var result struct{ Title string }
		if err := db.Table("collections").Select("title").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Title != "" {
			return result.Title
		}
	}

	return fmt.Sprintf("%s #%d", entityType, entityID)
}

// resolveEntitySlug returns the entity's URL slug for entity types that are
// addressed by slug in the public app (currently only collections — every
// other report-able entity type uses ID-based URLs in the admin moderation
// queue, so this returns nil for those). Returns nil when no slug exists.
//
// PSY-357: callers use this to render a slug-based link in the admin
// moderation card without exposing the slug everywhere — the response
// contract carries an optional `entity_slug` field that's omitted on every
// other type.
func resolveEntitySlug(db *gorm.DB, entityType string, entityID uint) *string {
	if db == nil {
		return nil
	}
	if entityType != "collection" {
		return nil
	}
	var result struct{ Slug string }
	if err := db.Table("collections").Select("slug").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Slug != "" {
		return &result.Slug
	}
	return nil
}
