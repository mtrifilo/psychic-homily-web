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
// GATED ids are FENCED HERE, against viewer, as well as by the row gates the
// callers of THIS function apply upstream. Which types those are is
// EntityIdentityFenceSQL's decision, not a list repeated here, so a type that
// gains a rule gains this fence in the same edit. The claim is about this batch
// loader and the two passes that call it; an enrichment pass that resolves a
// name without coming through here carries whatever fence its own code splices
// in. The two mechanisms are not redundant and are not alternatives:
//
//   - The callers' row gates are what remove the SIGNAL. Suppressing the entry
//     AND its count is the only thing that does; a de-identified row is still a
//     row, and its position in a list is the disclosure restated. Nothing here
//     can do that, because by the time a name is being resolved the row already
//     exists.
//   - This fence is what stops the NEXT caller. A digest, an export or an admin
//     view that assembles rows differently would otherwise resolve a private
//     entity's title and slug with nothing in the path to stop it, and the
//     failure would be silent.
//
// So: this loader fences every batch it resolves, and every listing that renders
// one drops the row first. A caller that comes through here inherits the first
// for free and still owes the second.
//
// THE FENCE IS WHAT MAKES A RESOLVED TITLE SAFE TO RENDER. The SELECT below asks
// each parent table for its display column, so a private collection's title is
// in reach of this query and is withheld only because the fence is spliced in
// unconditionally. Deleting it does not break a test on the happy path; it
// publishes the titles of the collections the callers' row gates then drop.
func LoadCommentEntityNames(db *gorm.DB, idsByType map[string][]uint, viewer contracts.ShowViewer) map[string]map[uint]EntityNameRow {
	out := make(map[string]map[uint]EntityNameRow, len(idsByType))
	for entityType, ids := range idsByType {
		_, table, nameCol, ok := engagementm.CommentEntityPathAndTable(entityType)
		if !ok || len(ids) == 0 {
			continue
		}
		var rows []EntityNameRow
		// Aliased SELECT so the tables spelling the display column "title"
		// and the ones spelling it "name" scan into the same struct field.
		q := db.Table(table).
			Select(fmt.Sprintf("id, %s AS name, slug", nameCol)).
			Where("id IN ?", ids)
		// The table name is the alias here: this query has no AS clause. Spliced
		// unconditionally — the fence answers TRUE where no rule applies, so
		// there is no branch here to forget.
		fence, fenceArgs := EntityIdentityFenceSQL(entityType, table, viewer)
		q = q.Where(fence, fenceArgs...)
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
