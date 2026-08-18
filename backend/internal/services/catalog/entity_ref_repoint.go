package catalog

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// This file holds the mechanism every entity merge uses to move the polymorphic
// (entity_type, entity_id) rows off the losing entity, so that the venue merge
// and the artist merge share one implementation instead of two that drift.
//
// The tables it walks carry NO foreign key to the entity, so a row left behind
// does not fail loudly — it silently points at an id that no longer exists.
// Which tables exist is per-merge data (venueEntityRefs, artistEntityRefs); how
// a row moves is not, and lives here.

// entityRef describes one polymorphic (entity_type, entity_id) reference table.
type entityRef struct {
	// table is the SQL table name. Interpolated into SQL, so it MUST stay a
	// hardcoded literal in the per-merge inventories — never caller input.
	table string
	// idCol holds the entity id. Almost always "entity_id"; `requests` and
	// `entity_requests` name theirs differently.
	idCol string
	// dedupe is true when a unique index could reject the re-point, so the
	// losing rows that would collide are deleted first. False means the table
	// is append-only / has no relevant unique key and can take a bare UPDATE.
	dedupe bool
	// key lists the OTHER columns in that unique index besides entity_type and
	// idCol. Empty with dedupe=true means the index is on (entity_type, idCol)
	// alone.
	key []string
}

// repointEntityRefs walks refs, dropping rows that would collide on a unique key
// before re-pointing the rest at the canonical entity.
//
// Must run inside the merge's transaction, which is where the rest of the
// merge's atomicity comes from.
//
// Returns moved-row counts keyed by table rather than one total, because callers
// report different slices of it: the venue merge sums them into a single
// summary, the artist merge surfaces two tables individually in its API
// response. A total would force the second caller to re-query what the UPDATE
// already returned.
//
// A self-merge is rejected: the dedupe's EXISTS correlation would match every
// row against itself and delete the surviving entity's rows before the no-op
// move ran.
func repointEntityRefs(
	tx *gorm.DB,
	refs []entityRef,
	entity mergeEntityType,
	canonicalID, mergeFromID uint,
) (map[string]int64, error) {
	if !entity.valid() {
		return nil, fmt.Errorf("repoint entity refs: unknown entity type %q", string(entity))
	}
	if canonicalID == 0 || mergeFromID == 0 {
		return nil, fmt.Errorf("repoint entity refs: canonical and merge-from ids are required")
	}
	if canonicalID == mergeFromID {
		return nil, fmt.Errorf(
			"repoint entity refs: cannot re-point %s %d onto itself", entity, canonicalID)
	}

	moved := make(map[string]int64, len(refs))
	for _, ref := range refs {
		if ref.table == "" || ref.idCol == "" {
			return nil, fmt.Errorf("repoint entity refs: table and id column are required")
		}

		if ref.dedupe {
			joinPred := ""
			for _, col := range ref.key {
				joinPred += fmt.Sprintf(" AND w.%s = l.%s", col, col)
			}
			// #nosec G201 -- table/column names come from the hardcoded per-merge
			// inventories, never from caller input; the ids and the entity type
			// are bound parameters.
			del := fmt.Sprintf(`
				DELETE FROM %[1]s l
				WHERE l.entity_type = ?
				  AND l.%[2]s = ?
				  AND EXISTS (
				        SELECT 1 FROM %[1]s w
				        WHERE w.entity_type = ?
				          AND w.%[2]s = ?%[3]s
				      )
			`, ref.table, ref.idCol, joinPred)
			if err := tx.Exec(del, string(entity), mergeFromID, string(entity), canonicalID).Error; err != nil {
				return nil, fmt.Errorf("failed to drop conflicting %s rows: %w", ref.table, err)
			}
		}

		// #nosec G201 -- see above.
		upd := fmt.Sprintf(
			"UPDATE %[1]s SET %[2]s = ? WHERE entity_type = ? AND %[2]s = ?",
			ref.table, ref.idCol,
		)
		r := tx.Exec(upd, canonicalID, string(entity), mergeFromID)
		if r.Error != nil {
			return nil, fmt.Errorf("failed to move %s rows: %w", ref.table, r.Error)
		}
		moved[ref.table] = r.RowsAffected
	}
	return moved, nil
}

// entityRefTableSet flattens an inventory plus the tables a merge handles
// through a dedicated step into the lookup the schema-coverage guards use.
func entityRefTableSet(refs []entityRef, repointedElsewhere []string) map[string]bool {
	out := make(map[string]bool, len(refs)+len(repointedElsewhere))
	for _, ref := range refs {
		out[strings.ToLower(ref.table)] = true
	}
	for _, table := range repointedElsewhere {
		out[strings.ToLower(table)] = true
	}
	return out
}
