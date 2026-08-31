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
// callers apply upstream (PSY-1983, PSY-1987). Which types those are is
// EntityIdentityFenceSQL's decision, not a list repeated here, so a type that
// gains a rule gains this fence in the same edit. The two mechanisms are not
// redundant and are not alternatives:
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
// So: every enrichment pass that resolves a gated entity's identity fences
// itself, and every listing that renders one drops the row first. A new caller
// inherits the first for free and still owes the second.
//
// THE COLLECTION FENCE CANNOT FIRE TODAY, and saying so is the point of this
// paragraph. engagementm.CommentEntityPathAndTable returns "name" as the
// collection display column, and the collections table's column is `title` — so
// the SELECT below fails with an undefined-column error for every collection
// batch, is logged and skipped, and both callers fall back to rendering
// "collection #<id>". The fence is added anyway, and it is not decoration: the
// column bug is a display defect somebody will fix, and fixing it must not be
// the edit that starts publishing private collections' titles. Deliberately NOT
// fixed here, because widening a disclosure path is not something a privacy
// change should do on the way past; PSY-1987's PR body files it as a follow-up.
// The row gates upstream are what actually close the leak, and they do not
// depend on this.
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
