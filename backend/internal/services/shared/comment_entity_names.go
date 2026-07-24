package shared

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	engagementm "psychic-homily-backend/internal/models/engagement"
)

// EntityNameRow projects the (id, name|title, slug) tuple from a comment
// parent-entity table. Slug scans to "" for entities whose slug column is
// NULL.
type EntityNameRow struct {
	ID   uint
	Name string
	Slug string
}

// LoadCommentEntityNames batch-loads (id, name, slug) for comment parent
// entities grouped by entity type — one SELECT per distinct entity table,
// not per row. Unknown entity types are skipped; a failed per-table query
// is logged and skipped so callers degrade to their fallback rendering.
// Returns nested map[entityType]map[entityID]EntityNameRow.
func LoadCommentEntityNames(db *gorm.DB, idsByType map[string][]uint) map[string]map[uint]EntityNameRow {
	out := make(map[string]map[uint]EntityNameRow, len(idsByType))
	for entityType, ids := range idsByType {
		_, table, nameCol, ok := engagementm.CommentEntityPathAndTable(entityType)
		if !ok || len(ids) == 0 {
			continue
		}
		var rows []EntityNameRow
		// Aliased SELECT so shows (column "title") and the rest (column
		// "name") scan into the same struct field.
		err := db.Table(table).
			Select(fmt.Sprintf("id, %s AS name, slug", nameCol)).
			Where("id IN ?", ids).
			Scan(&rows).Error
		if err != nil {
			log.Printf("warning: failed to load parent entities for table %s: %v", table, err)
			continue
		}
		byID := make(map[uint]EntityNameRow, len(rows))
		for _, r := range rows {
			byID[r.ID] = r
		}
		out[entityType] = byID
	}
	return out
}
