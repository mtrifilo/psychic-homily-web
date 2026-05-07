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
// Slug is non-nil for entity types whose public URLs are slug-based:
// `collection`, `artist`, `venue`, `festival`, `release`, `label`. For
// types not in this list (`show`, `comment`), slug is always nil so the
// JSON response omits the field.
//
// PSY-600: extended past `collection` so the contributor-facing
// /submissions pending-edits surface can render functional links to the
// affected entity (the moderation queue still uses raw IDs and is
// unaffected by this addition).
func resolveEntityNameAndSlug(db *gorm.DB, entityType string, entityID uint) (string, *string) {
	if db == nil {
		return fmt.Sprintf("%s #%d", entityType, entityID), nil
	}

	// Per-type table + display column. Returning ("", "") from any branch
	// triggers the fallback at the bottom of the function.
	type lookup struct {
		table       string
		displayCol  string
		slugNonNull bool // some tables type slug as NOT NULL
	}
	lookups := map[string]lookup{
		"artist":     {table: "artists", displayCol: "name"},
		"venue":      {table: "venues", displayCol: "name"},
		"festival":   {table: "festivals", displayCol: "name", slugNonNull: true},
		"release":    {table: "releases", displayCol: "title"},
		"label":      {table: "labels", displayCol: "name"},
		"collection": {table: "collections", displayCol: "title", slugNonNull: true},
	}
	cfg, ok := lookups[entityType]
	if !ok {
		return resolveEntityName(db, entityType, entityID), nil
	}

	if cfg.slugNonNull {
		var result struct {
			Display string
			Slug    string
		}
		err := db.Table(cfg.table).
			Select(cfg.displayCol+" AS display, slug").
			Where("id = ?", entityID).
			Scan(&result).Error
		if err != nil || result.Display == "" {
			return fmt.Sprintf("%s #%d", entityType, entityID), nil
		}
		var slug *string
		if result.Slug != "" {
			slug = &result.Slug
		}
		return result.Display, slug
	}

	var result struct {
		Display string
		Slug    *string
	}
	err := db.Table(cfg.table).
		Select(cfg.displayCol+" AS display, slug").
		Where("id = ?", entityID).
		Scan(&result).Error
	if err != nil || result.Display == "" {
		return fmt.Sprintf("%s #%d", entityType, entityID), nil
	}
	return result.Display, result.Slug
}
