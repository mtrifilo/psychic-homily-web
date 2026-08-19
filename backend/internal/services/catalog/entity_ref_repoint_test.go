package catalog

import (
	"testing"
)

// These cover the guards, which are the whole point of the helper. Every one of
// them is a merge that would otherwise have written something wrong and
// committed it.
//
// No database. Each case must be rejected BEFORE any statement runs, and
// unusableTx() (a zero-value *gorm.DB, shared with revision_repoint_test.go)
// makes that self-enforcing: a guard that stopped rejecting would reach Exec and
// panic, failing the test rather than quietly passing it.
//
// The same idiom, and the same reason, as edit_history_repoint_test.go. Neither
// current caller can reach these branches — both reject a self-merge first — so
// without this file the guards would ship having never executed, and the third
// caller would be the one to find out.

func oneRef() []entityRef {
	return []entityRef{{table: "entity_tags", idCol: "entity_id"}}
}

func TestRepointEntityRefs_RejectsUnknownEntityType(t *testing.T) {
	_, _, err := repointEntityRefs(unusableTx(), oneRef(), mergeEntityType("artists"), 1, 2)
	if err == nil {
		t.Fatal("a mistyped entity type must be rejected — it would match no rows and look like success")
	}
}

func TestRepointEntityRefs_RejectsZeroIDs(t *testing.T) {
	if _, _, err := repointEntityRefs(unusableTx(), oneRef(), mergeEntityArtist, 0, 2); err == nil {
		t.Fatal("a zero canonical id must be rejected")
	}
	if _, _, err := repointEntityRefs(unusableTx(), oneRef(), mergeEntityArtist, 1, 0); err == nil {
		t.Fatal("a zero merge-from id must be rejected")
	}
}

// A self-merge is a harmless no-op for the move and destructive for the dedupe:
// the EXISTS correlation would match every row against itself and delete the
// SURVIVING entity's bookmarks, crate items and tag votes.
func TestRepointEntityRefs_RejectsSelfMerge(t *testing.T) {
	_, _, err := repointEntityRefs(unusableTx(), oneRef(), mergeEntityArtist, 7, 7)
	if err == nil {
		t.Fatal("re-pointing an entity onto itself must be rejected")
	}
}

func TestRepointEntityRefs_RejectsIncompleteRef(t *testing.T) {
	if _, _, err := repointEntityRefs(
		unusableTx(), []entityRef{{idCol: "entity_id"}}, mergeEntityArtist, 1, 2); err == nil {
		t.Fatal("a ref with no table must be rejected rather than interpolated into SQL")
	}
	if _, _, err := repointEntityRefs(
		unusableTx(), []entityRef{{table: "entity_tags"}}, mergeEntityArtist, 1, 2); err == nil {
		t.Fatal("a ref with no id column must be rejected rather than interpolated into SQL")
	}
}

// The provenance-gated tables are kept out of this loop by a runtime check, not
// only by their absence from the inventory. A list edit is exactly how they
// would get in — the coverage guard's failure message even tells the next author
// to add the new table to the inventory — and because this helper assembles its
// UPDATE with fmt.Sprintf, the source-scanning guards that force a provenance
// decision (TestNoRevisionRepointOutsideTheHelper and its edit-history twin)
// cannot see it.
func TestRepointEntityRefs_RejectsProvenanceGatedTables(t *testing.T) {
	for _, table := range []string{"revisions", "pending_entity_edits", "entity_edit_audit_logs"} {
		refs := []entityRef{{table: table, idCol: "entity_id"}}
		if _, _, err := repointEntityRefs(unusableTx(), refs, mergeEntityArtist, 1, 2); err == nil {
			t.Errorf("%s must be rejected here: it may only move through the helper that "+
				"requires a provenance decision", table)
		}
	}
}

// The inventory and the reject-list have to agree, or the check above silently
// stops covering a table someone moved between them.
func TestProvenanceGatedTablesAreNotInTheLoopInventory(t *testing.T) {
	for _, ref := range polymorphicEntityRefs {
		if provenanceGatedTable(ref.table) {
			t.Errorf("%s is in polymorphicEntityRefs and in refsRepointedElsewhere; it would be "+
				"re-pointed twice, and the second attempt is a rejected call", ref.table)
		}
	}
}

// A missing key must be an error, not a zero. The merge summary reads two tables
// out of the returned map by name, and a bare lookup would report "0 bookmarks
// moved" forever after a table rename, with every schema guard still green.
func TestMovedCount_RejectsATableTheInventoryDoesNotCarry(t *testing.T) {
	moved := map[string]int64{"user_bookmarks": 3}

	got, err := movedCount(moved, "user_bookmarks")
	if err != nil {
		t.Fatalf("movedCount on a present table: %v", err)
	}
	if got != 3 {
		t.Errorf("movedCount = %d, want 3", got)
	}

	if _, err := movedCount(moved, "collection_items"); err == nil {
		t.Fatal("a table absent from the inventory must be an error, not a silent zero")
	}
}

// dedupeWhen is what keeps a PARTIAL unique index from being over-enforced.
// Pinning which entry carries one matters because getting it wrong is silent in
// both directions: without it the dedupe deletes rows the index would have kept,
// and with a wrong predicate it fails to delete rows that do collide.
func TestOnlyThePartiallyIndexedRefCarriesADedupeScope(t *testing.T) {
	for _, ref := range polymorphicEntityRefs {
		switch ref.table {
		case "image_enrich_queue":
			if ref.dedupeWhen == "" {
				t.Errorf("%s is constrained by a status-scoped partial index and needs a "+
					"dedupeWhen, or the merge deletes queued jobs that could never collide", ref.table)
			}
		default:
			if ref.dedupeWhen != "" {
				t.Errorf("%s declares dedupeWhen %q but its unique index is not partial",
					ref.table, ref.dedupeWhen)
			}
		}
	}
}
