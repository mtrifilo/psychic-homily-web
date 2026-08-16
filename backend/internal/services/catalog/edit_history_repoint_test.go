package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These cover the guards, which are the whole point of the helper: every one of
// them is a merge that would otherwise have written something wrong and
// committed it.
//
// No database. Each case must be rejected BEFORE any statement runs, and
// unusableTx() (a zero-value *gorm.DB, shared with revision_repoint_test.go)
// makes that self-enforcing: a guard that stopped rejecting would reach Exec
// and panic, failing the test rather than quietly passing it.

func TestRepointEditHistory_RejectsUndecidedProvenance(t *testing.T) {
	_, _, err := repointEditHistory(
		unusableTx(), pendingEditsHistory, mergeEntityShow, 1, 2, editHistoryUndecided)
	if err == nil {
		t.Fatal("the zero provenance must be rejected — it is 'nobody decided', not 'no stamp'")
	}
}

func TestRepointEditHistory_RejectsUnknownProvenance(t *testing.T) {
	_, _, err := repointEditHistory(
		unusableTx(), pendingEditsHistory, mergeEntityShow, 1, 2, editHistoryProvenance(99))
	if err == nil {
		t.Fatal("a provenance outside the declared set must be rejected")
	}
}

func TestRepointEditHistory_RejectsUnknownEntityType(t *testing.T) {
	_, _, err := repointEditHistory(
		unusableTx(), pendingEditsHistory, mergeEntityType("venues"), 1, 2, editHistoryCarriesNoRedaction)
	if err == nil {
		t.Fatal("a mistyped entity type must be rejected — it would match no rows and look like success")
	}
}

func TestRepointEditHistory_RejectsUnknownTable(t *testing.T) {
	_, _, err := repointEditHistory(
		unusableTx(), editHistoryTable{}, mergeEntityVenue, 1, 2, editHistoryCarriesNoRedaction)
	if err == nil {
		t.Fatal("an empty table descriptor must be rejected rather than interpolated into SQL")
	}
}

// A self-merge is a harmless no-op for the move and destructive for the dedupe:
// the EXISTS correlation would match every row against itself and delete the
// surviving entity's entire edit history.
func TestRepointEditHistory_RejectsSelfMerge(t *testing.T) {
	_, _, err := repointEditHistory(
		unusableTx(), pendingEditsHistory, mergeEntityVenue, 7, 7, editHistoryCarriesNoRedaction)
	if err == nil {
		t.Fatal("re-pointing an entity onto itself must be rejected")
	}
}

func TestRepointEditHistory_RejectsZeroIDs(t *testing.T) {
	if _, _, err := repointEditHistory(
		unusableTx(), pendingEditsHistory, mergeEntityVenue, 0, 2, editHistoryCarriesNoRedaction); err == nil {
		t.Fatal("a zero canonical id must be rejected")
	}
	if _, _, err := repointEditHistory(
		unusableTx(), pendingEditsHistory, mergeEntityVenue, 1, 0, editHistoryCarriesNoRedaction); err == nil {
		t.Fatal("a zero merge-from id must be rejected")
	}
}

// The dedupe policy belongs to the table, not the call site. This pins which
// table carries which, because getting it backwards is silent both ways: a
// missing dedupe aborts a merge on a constraint violation, and a spurious one
// deletes an append-only audit row.
func TestEditHistoryTablesCarryTheirOwnDedupePolicy(t *testing.T) {
	if got := pendingEditsHistory.dedupeOn; len(got) != 1 || got[0] != "submitted_by" {
		t.Errorf("pending_entity_edits must dedupe on submitted_by (idx_pending_entity_edits_unique), got %v", got)
	}
	if got := entityEditAuditHistory.dedupeOn; len(got) != 0 {
		t.Errorf("entity_edit_audit_logs is append-only and has no unique key on its entity reference, got %v", got)
	}
}

// A required parameter only binds an author who reaches for the helper. This is
// what binds the one who does not: a merge that re-points edit history with its
// own UPDATE never has to answer the provenance question, and the leak class
// this file exists to prevent comes back on a path nobody reviewed for it.
//
// The same idiom as TestNoRevisionRepointOutsideTheHelper, and deliberately as
// broad: it flags ANY raw update against either table, not only one that names
// entity_id, because the point is to make a bypass impossible to write by
// accident rather than to police a particular spelling.
//
// Two shapes are checked, because the raw UPDATE both merges used is not the
// only way to write one: a GORM chain setting entity_id would compile, bypass
// the helper, and read nothing like SQL. The second check is scoped to files
// that also name one of the tables or its model, so re-pointing some other
// polymorphic table through the builder does not trip it.
func TestNoEditHistoryRepointOutsideTheHelper(t *testing.T) {
	root := backendRoot(t)
	rawUpdate := regexp.MustCompile(`(?is)\bupdate\s+(?:pending_entity_edits|entity_edit_audit_logs)\b`)
	builderSetsEntityID := regexp.MustCompile(`Updates?\(\s*"entity_id"`)
	namesEditHistory := regexp.MustCompile(
		`adminm?\.PendingEntityEdit\{|adminm?\.EntityEditAuditLog\{|Table\("pending_entity_edits"\)|Table\("entity_edit_audit_logs"\)`)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// Third-party code is not ours to route through the helper, and
			// `go mod vendor` would otherwise drop it into the scan.
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Tests seed and inspect rows directly; the rule is about production
		// merge paths.
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "edit_history_repoint.go" {
			return nil
		}

		source, readErr := os.ReadFile(path) // #nosec G304 -- path comes from walking the module's own tree
		if readErr != nil {
			return readErr
		}
		if rawUpdate.Match(source) || (builderSetsEntityID.Match(source) && namesEditHistory.Match(source)) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s writes a raw update against pending_entity_edits or entity_edit_audit_logs. "+
				"Re-pointing edit history must go through catalog.repointEditHistory, which requires a "+
				"provenance decision — without it a merge silently republishes history that was being "+
				"withheld, and skips the unique index that aborts the transaction. If this write is not "+
				"a re-point, narrow this guard rather than deleting it.", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to scan backend sources: %v", err)
	}
}
