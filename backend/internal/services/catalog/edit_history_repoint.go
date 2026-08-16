package catalog

import (
	"fmt"

	"gorm.io/gorm"
)

// This file is the edit-history counterpart to revision_repoint.go: it exists
// so that "a merge re-points a contributor's edit history" cannot be written
// without answering "and what happens to the redaction those rows were
// carrying".
//
// revisions is not the only history table a merge moves. pending_entity_edits
// is the one the venue READ side actually treats as history — venueEditCounts
// builds the venue provenance stamp from it, not from entity_edit_audit_logs —
// and two merge paths re-point it: MergeVenues, and MergeDuplicateShow, which
// runs from cmd/dedup-shows with no admin in the loop. entity_edit_audit_logs
// is re-pointed by MergeVenues.
//
// Both moved through UPDATEs that answered no provenance question at all. That
// is the same setup PSY-1716 walked into on revisions: the moment pending-edit
// history gains a read-time gate, every merge that already ran has published
// rows the gate would have withheld, and nothing in the code forces the next
// merge author to notice. The forcing function lands first, so the gate arrives
// to call sites that are already asking the question.
//
// What keeps a merge from bypassing it altogether is
// TestNoEditHistoryRepointOutsideTheHelper, which fails on a raw update against
// either table anywhere else in the backend.

// editHistoryTable describes one edit-history table a merge re-points.
//
// dedupeOn is a property of the TABLE's unique index, never of the call site:
// letting a caller choose whether to dodge the index is exactly the footgun
// that left MergeDuplicateShow able to violate it.
type editHistoryTable struct {
	// name is the SQL table name. Interpolated into SQL, so the only legal
	// values are the package-level literals below — never caller input.
	name string
	// dedupeOn lists the columns, besides entity_type and entity_id, that make
	// a row unique. Non-empty means losing rows that would collide with a
	// winning row are deleted before the re-point. Empty means the table has no
	// unique key on its entity reference and can take a bare UPDATE.
	dedupeOn []string
}

var (
	// pendingEditsHistory carries idx_pending_entity_edits_unique, which is
	// UNIQUE (entity_type, entity_id, submitted_by) WHERE status = 'pending'.
	//
	// The dedupe deletes a losing row whenever the winner has ANY row from the
	// same contributor, which is stricter than the partial index needs. That is
	// the behavior MergeVenues has always had, and it is the safe direction:
	// the alternative failure is a constraint violation that aborts the whole
	// merge transaction.
	pendingEditsHistory = editHistoryTable{
		name:     "pending_entity_edits",
		dedupeOn: []string{"submitted_by"},
	}

	// auditedEditsHistory has no unique key on its entity reference — it is an
	// append-only trail, so every row moves.
	auditedEditsHistory = editHistoryTable{name: "entity_edit_audit_logs"}
)

// editHistoryProvenance is the decision a merge makes about the redaction
// posture of the edit history it carries off the losing entity.
//
// There is no default. The zero value is rejected at runtime, so a caller that
// passes an uninitialized variable fails loudly on the first merge rather than
// silently publishing history that was being withheld.
type editHistoryProvenance int

const (
	// editHistoryUndecided is the zero value. It is never a legal argument —
	// its only job is to make "I forgot to decide" a rejected call rather than
	// an accidental no-stamp.
	editHistoryUndecided editHistoryProvenance = iota

	// editHistoryCarriesNoRedaction states that the losing entity's edit
	// history carries no read-time redaction that the re-point would strip, so
	// the rows move as they are.
	//
	// True today for BOTH tables and every entity type, and the reason is the
	// read surface rather than the merge: pending-edit and audit-log CONTENT
	// (field_changes, metadata) is served only to the submitter
	// (GET /my/pending-edits, scoped by submitted_by) and to admins
	// (/admin/pending-edits/*). Everything anonymous reads off these tables is
	// an aggregate — venueEditCounts, the contributor stats counters, the
	// leaderboard — and a count carries no address.
	//
	// That is what makes it different from revisions, where /revisions/venue/{id}
	// publishes the stored diff to anonymous callers and PSY-1700 therefore had
	// to mask an unverified venue's address.
	//
	// It is NOT true by default for anything that gains a gate later. If one of
	// these tables starts serving withheld content to anonymous readers, the
	// merge has to preserve the withholding across the re-point, and there is no
	// marker column here to preserve it with: add one in the shape of
	// revisions.from_unverified_venue (PSY-1716), add the constant that stamps
	// it beside this one, and teach repointEditHistory to write it. The call
	// sites will not compile past the new decision without an author choosing.
	editHistoryCarriesNoRedaction
)

// String makes a rejected provenance readable in the error rather than printing
// an integer the reader has to count out against this file.
func (p editHistoryProvenance) String() string {
	switch p {
	case editHistoryCarriesNoRedaction:
		return "editHistoryCarriesNoRedaction"
	case editHistoryUndecided:
		return "editHistoryUndecided"
	default:
		return fmt.Sprintf("editHistoryProvenance(%d)", int(p))
	}
}

// repointEditHistory moves the losing entity's edit-history rows onto the
// canonical entity, after the caller has stated what happens to their redaction
// posture.
//
// Must run inside the merge's transaction, which is where the rest of the
// merge's atomicity comes from.
//
// Returns the number of rows moved and the number dropped as duplicates, for
// the caller's merge summary. Counting them separately matters: a merge that
// silently deletes a contributor's edit should be able to say so.
//
// The provenance decision does not currently vary the SQL, because neither
// table has a marker column to write — see editHistoryCarriesNoRedaction. It is
// still taken BEFORE any statement runs, so the decision is a precondition of
// the move rather than a comment beside it, and so the day a marker column
// lands the set clause has one place to change.
func repointEditHistory(
	tx *gorm.DB,
	table editHistoryTable,
	entity mergeEntityType,
	canonicalID, mergeFromID uint,
	provenance editHistoryProvenance,
) (moved, dropped int64, err error) {
	if table.name == "" {
		return 0, 0, fmt.Errorf("repoint edit history: table is required")
	}
	if !entity.valid() {
		return 0, 0, fmt.Errorf("repoint edit history: unknown entity type %q", string(entity))
	}
	if canonicalID == 0 || mergeFromID == 0 {
		return 0, 0, fmt.Errorf("repoint edit history: canonical and merge-from ids are required")
	}
	// A self-merge is a no-op move but NOT a no-op dedupe: the EXISTS below
	// would match every row against itself and delete the surviving entity's
	// entire edit history.
	if canonicalID == mergeFromID {
		return 0, 0, fmt.Errorf(
			"repoint edit history: cannot re-point %s %d onto itself", entity, canonicalID)
	}

	switch provenance {
	case editHistoryCarriesNoRedaction:
		// Nothing to preserve — see the constant.
	default:
		return 0, 0, fmt.Errorf(
			"repoint edit history: %s is not a provenance decision; pass "+
				"editHistoryCarriesNoRedaction", provenance)
	}

	if len(table.dedupeOn) > 0 {
		joinPred := ""
		for _, col := range table.dedupeOn {
			joinPred += fmt.Sprintf(" AND w.%s = l.%s", col, col)
		}
		// #nosec G201 -- the table and dedupe columns come from the package-level
		// literals above, never from caller input; ids and entity type are bound.
		del := fmt.Sprintf(`
			DELETE FROM %[1]s l
			WHERE l.entity_type = ?
			  AND l.entity_id = ?
			  AND EXISTS (
			        SELECT 1 FROM %[1]s w
			        WHERE w.entity_type = ?
			          AND w.entity_id = ?%[2]s
			      )
		`, table.name, joinPred)
		r := tx.Exec(del, string(entity), mergeFromID, string(entity), canonicalID)
		if r.Error != nil {
			return 0, 0, fmt.Errorf("failed to drop conflicting %s rows: %w", table.name, r.Error)
		}
		dropped = r.RowsAffected
	}

	// #nosec G201 -- see above.
	upd := fmt.Sprintf(
		"UPDATE %s SET entity_id = ? WHERE entity_type = ? AND entity_id = ?", table.name)
	r := tx.Exec(upd, canonicalID, string(entity), mergeFromID)
	if r.Error != nil {
		return 0, 0, fmt.Errorf("failed to move %s rows: %w", table.name, r.Error)
	}
	return r.RowsAffected, dropped, nil
}
