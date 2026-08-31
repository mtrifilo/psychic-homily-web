package shared

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
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
// SHOW-typed ids are FENCED HERE, against viewer, as well as by the row gates
// the callers apply upstream (PSY-1983). The two are not redundant and are not
// alternatives:
//
//   - The callers' row gates are what remove the SIGNAL. Suppressing the entry
//     AND its count is the only thing that does; a de-identified row is still a
//     row, and its position in a list is the disclosure restated. Nothing here
//     can do that, because by the time a name is being resolved the row already
//     exists.
//   - This fence is what stops the NEXT caller. A digest, an export or an admin
//     view that assembles rows differently would otherwise resolve a private
//     show's title and slug with nothing in the path to stop it, and the failure
//     would be silent.
//
// So: every enrichment pass that resolves a show's identity fences itself, and
// every listing that renders one drops the row first. A new caller inherits the
// first for free and still owes the second.
//
// COLLECTION-typed rows are NOT fenced, by either mechanism, and that is a KNOWN
// OPEN LEAK rather than a claim of safety: collections have a read-time rule of
// their own (is_public OR the owner, services/community/collection.go) which
// neither this function nor its callers consult, so a subscription to a guessed
// private-collection id renders its name, slug and comment activity. Pre-existing,
// out of PSY-1983's scope, which was shows. Disclosed rather than inherited in
// silence.
func LoadCommentEntityNames(db *gorm.DB, idsByType map[string][]uint, viewer contracts.ShowViewer) map[string]map[uint]EntityNameRow {
	out := make(map[string]map[uint]EntityNameRow, len(idsByType))
	for entityType, ids := range idsByType {
		_, table, nameCol, ok := engagementm.CommentEntityPathAndTable(entityType)
		if !ok || len(ids) == 0 {
			continue
		}
		var rows []EntityNameRow
		// Aliased SELECT so shows (column "title") and the rest (column
		// "name") scan into the same struct field.
		q := db.Table(table).
			Select(fmt.Sprintf("id, %s AS name, slug", nameCol)).
			Where("id IN ?", ids)
		if entityType == CommentEntityTypeShow {
			// The table name is the alias here: this query has no AS clause.
			cond, args := VisibleShowPredicateSQL(table, viewer)
			q = q.Where(cond, args...)
		}
		err := q.Scan(&rows).Error
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
