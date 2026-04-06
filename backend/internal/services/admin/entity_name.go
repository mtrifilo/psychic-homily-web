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
	}

	return fmt.Sprintf("%s #%d", entityType, entityID)
}
