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
		// resolveEntityNameAndSlug is the canonical path for collections; this
		// arm exists for callers that only need the name. It still issues a
		// single query (no extra slug fetch).
		var result struct{ Title string }
		if err := db.Table("collections").Select("title").Where("id = ?", entityID).Scan(&result).Error; err == nil && result.Title != "" {
			return result.Title
		}
	}

	return fmt.Sprintf("%s #%d", entityType, entityID)
}

// resolveEntityNameAndSlug returns the entity's display name and (if the
// type is addressed by slug in the public app) its URL slug. Used by
// callers that need both — a single combined query keeps the
// non-name-only path from doing two round-trips per row in the admin
// moderation list. PSY-357.
//
// Slug is non-nil only for entity types whose public URLs are slug-based
// (currently only `collection`). All other types return slug==nil so the
// JSON response omits the field.
func resolveEntityNameAndSlug(db *gorm.DB, entityType string, entityID uint) (string, *string) {
	if entityType != "collection" {
		return resolveEntityName(db, entityType, entityID), nil
	}
	if db == nil {
		return fmt.Sprintf("%s #%d", entityType, entityID), nil
	}
	var result struct {
		Title string
		Slug  string
	}
	err := db.Table("collections").Select("title, slug").Where("id = ?", entityID).Scan(&result).Error
	if err != nil || result.Title == "" {
		return fmt.Sprintf("%s #%d", entityType, entityID), nil
	}
	var slug *string
	if result.Slug != "" {
		slug = &result.Slug
	}
	return result.Title, slug
}
