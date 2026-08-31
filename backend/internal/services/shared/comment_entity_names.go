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
//
// NO VISIBILITY RULE IS APPLIED HERE, for any entity type. A caller passing a
// show id gets that show's title and slug whatever its status, and a caller
// passing a collection id gets a PRIVATE collection's name and slug.
//
// For SHOW-typed rows that is the caller's to decide, and both callers decide it
// the same way: they drop the ROW before they get here, rather than asking for a
// name and hiding it (PSY-1983). Suppressing the entry and suppressing its count
// is the only thing that removes the signal — a de-identified row is still a
// row, and its position in a list is the disclosure restated.
//
// For COLLECTION-typed rows NOTHING gates them, and that is a KNOWN OPEN LEAK
// rather than a claim of safety: collections have a read-time rule of their own
// (is_public OR the owner, services/community/collection.go) which neither
// caller consults, so a subscription to a guessed private-collection id renders
// its name, slug and comment activity. Pre-existing, not closed by PSY-1983,
// which scoped itself to shows. Disclosed rather than silently inherited.
//
// So a new caller must gate its ids first, and must not assume the existing
// callers gate anything but shows. shared.VisibleShowCommentEntitySQL is the
// condition both use.
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
