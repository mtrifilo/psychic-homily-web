package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The delete sweep's guards, with no database.
//
// Same idiom and same reason as entity_ref_repoint_test.go: every case here must
// be rejected BEFORE any statement runs, and unusableTx() (a zero-value
// *gorm.DB) makes that self-enforcing — a guard that stopped rejecting would
// reach Exec and panic, failing the test rather than quietly passing it.

// The completeness guard, and the one that makes the disposition list
// load-bearing rather than documentary.
//
// The failure this protects against is silent in the worst way: a migration adds
// an entity_type table, the author dutifully adds it to polymorphicEntityRefs
// (which the merge coverage guards demand), and every delete path then either
// strands its rows or destroys them depending on nothing but which branch of a
// switch happened to be first.
func TestEveryInventoriedRefHasADeleteDisposition(t *testing.T) {
	for table := range entityRefTables() {
		d, ok := entityRefDeleteDispositions[table]
		if !ok {
			t.Errorf("%s is in the entity-ref inventory but has no delete disposition. "+
				"A delete cannot re-point it, so someone has to say whether its rows go with "+
				"the entity, stay as a tombstone, or keep the row and lose the pointer.", table)
			continue
		}
		if d == deleteDispositionUndecided {
			t.Errorf("%s is recorded as %s, which is the zero value and not a decision", table, d)
		}
	}
}

// And the reverse: a disposition for a table the inventory does not carry is a
// decision nothing executes. It reads as coverage in review and is not.
func TestNoDeleteDispositionForATableOutsideTheInventory(t *testing.T) {
	inventory := entityRefTables()
	for table := range entityRefDeleteDispositions {
		if !inventory[table] {
			t.Errorf("entityRefDeleteDispositions answers for %s, which is in neither "+
				"polymorphicEntityRefs nor refsRepointedElsewhere. Nothing walks it, so the "+
				"decision is never carried out.", table)
		}
	}
}

// The decision register: every table's disposition pinned by name, with the
// reason it was given.
//
// This is deliberately a second, independent statement of what
// entityRefDeleteDispositions already says, and it earns the duplication.
// Everything else in this file and in artist_delete_refs_test.go reads the map
// to decide what to assert, so the map is the SPEC — which means a one-line flip
// of a table from keep to drop changes the behaviour AND the expectation
// together, and every test stays green while an artist delete starts destroying
// contributor history. (That gap was found by deliberately flipping audit_logs
// and watching the seeding sweep pass.)
//
// Same shape as showFKColumns' ElementsMatch guard: a hand-kept list that has to
// be edited on purpose, in a place where the reason is written down next to it.
func TestRecordedDeleteDispositionsAreTheOnesReviewed(t *testing.T) {
	// Read on an axis the deleted entity does not gate — by actor on the
	// contributor profile and leaderboard, by submitter in the trusted-tier
	// promotion and demotion counts, by time in the global admin trail. Dropping
	// any of these silently rewrites somebody's record of their own work, on an
	// endpoint any authenticated user can call.
	for _, table := range []string{
		"audit_logs",
		"comments",
		"entity_reports",
		"revisions",
		"pending_entity_edits",
		"entity_edit_audit_logs",
		"entity_requests",
		"requests",
	} {
		if got := entityRefDeleteDispositions[table]; got != keepRefRowsAsTombstone {
			t.Errorf("%s is recorded as %s, but its rows are read on an axis the deleted entity "+
				"does not gate. Changing this destroys contributor or moderation history; if that "+
				"is really intended, change it here too and say why.", table, got)
		}
	}

	// Pointers, per-user cursors and work-queue entries. Kept, each of these
	// inflates a counter somebody reads or addresses a reader at an entity that
	// is not there.
	for _, table := range []string{
		"collection_items",
		"comment_last_read",
		"comment_subscriptions",
		"entity_tags",
		"tag_votes",
		"user_bookmarks",
		"image_enrich_queue",
		"source_configs",
		"notification_log",
	} {
		if got := entityRefDeleteDispositions[table]; got != dropRefRows {
			t.Errorf("%s is recorded as %s, but its rows only mean anything while the entity "+
				"exists. Keeping them strands a dangling reference, which is the bug PSY-1868 "+
				"closed.", table, got)
		}
	}

	// The register has to stay exhaustive, or a table could be quietly moved out
	// of both lists above and lose its second opinion.
	if len(entityRefDeleteDispositions) != 17 {
		t.Errorf("the inventory holds %d tables but this register was written for 17. "+
			"Add the new table to whichever list above matches the reason for its disposition.",
			len(entityRefDeleteDispositions))
	}
}

// The disposition lookup is not only a completeness check — it is the fence that
// pins the interpolated table and column names to the hardcoded inventory. Same
// role, and the same reason, as moveShowFKRows' showFKColumns check.
func TestApplyRefDeleteDispositionRejectsAnUndispositionedTable(t *testing.T) {
	for _, table := range []string{
		"artists_backup",
		"show_setlists",
		"user_bookmarks; DROP TABLE artists --",
	} {
		if _, err := applyRefDeleteDisposition(
			unusableTx(), table, "entity_id", entityTypeArtist, 1); err == nil {
			t.Errorf("%s has no recorded disposition and must be refused rather than "+
				"interpolated into a DELETE", table)
		}
	}
}

// A table whose recorded disposition is keepRefRowsAsTombstone must run NO
// statement at all. Checked through unusableTx rather than by inspecting the
// map, because "the decision is recorded" and "the decision is honoured" are
// different claims and only the second one protects contributor history.
func TestKeptTablesIssueNoStatement(t *testing.T) {
	for table, disposition := range entityRefDeleteDispositions {
		if disposition != keepRefRowsAsTombstone {
			continue
		}
		n, err := applyRefDeleteDisposition(unusableTx(), table, "entity_id", entityTypeArtist, 1)
		if err != nil {
			t.Errorf("%s is kept, so it must be a no-op rather than an error: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s is kept but reported %d rows affected", table, n)
		}
	}
}

func TestDeleteEntityRefsRejectsUnknownEntityType(t *testing.T) {
	if _, err := deleteEntityRefs(unusableTx(), polymorphicEntityType("artists"), 1); err == nil {
		t.Fatal("a mistyped entity type must be rejected — it would match no rows and look like success")
	}
}

// A zero id is the shape a caller reaches by passing an unloaded struct's ID
// field. Left unchecked it is not a no-op: `entity_id = 0` matches nothing today,
// but the sweep would report success for an entity it never swept.
func TestDeleteEntityRefsRejectsZeroID(t *testing.T) {
	if _, err := deleteEntityRefs(unusableTx(), entityTypeArtist, 0); err == nil {
		t.Fatal("a zero entity id must be rejected")
	}
	if _, err := deleteAlertRowsNamingEntity(unusableTx(), entityTypeArtist, 0); err == nil {
		t.Fatal("a zero entity id must be rejected by the alert sweep too")
	}
	if err := releaseTagUsageCounts(unusableTx(), entityTypeArtist, 0); err == nil {
		t.Fatal("a zero entity id must be rejected by the tag-count release too")
	}
}

// The delete sweep accepts all six catalog entity types; the merge helpers must
// still accept only the three that have a merge.
//
// This is the pair of assertions that keeps widening the vocabulary for PSY-1868
// from having quietly widened the merges' own fence, where an unsupported entity
// type would make every re-point match zero rows and report success.
func TestMergeableIsNarrowerThanValid(t *testing.T) {
	for _, entity := range allPolymorphicEntityTypes {
		if !entity.valid() {
			t.Errorf("%s is in allPolymorphicEntityTypes but valid() rejects it", entity)
		}
	}

	merges := map[polymorphicEntityType]bool{
		entityTypeVenue: true, entityTypeArtist: true, entityTypeShow: true,
	}
	for _, entity := range allPolymorphicEntityTypes {
		if got := entity.mergeable(); got != merges[entity] {
			t.Errorf("%s.mergeable() = %v, want %v", entity, got, merges[entity])
		}
	}

	// And the merge helpers must actually gate on it. Reached through the helpers
	// rather than the predicate, because a helper that switched to valid() would
	// leave the predicate above perfectly correct and still accept the call.
	for _, entity := range []polymorphicEntityType{
		entityTypeRelease, entityTypeLabel, entityTypeFestival,
	} {
		if _, _, err := repointEntityRefs(unusableTx(), oneRef(), entity, 1, 2); err == nil {
			t.Errorf("repointEntityRefs accepted %s, which has no merge", entity)
		}
		if _, err := repointRevisions(unusableTx(), entity, 1, 2, noRedactionCarryover); err == nil {
			t.Errorf("repointRevisions accepted %s, which has no merge", entity)
		}
		if _, _, err := repointEditHistory(
			unusableTx(), pendingEditsHistory, entity, 1, 2, editHistoryCarriesNoRedaction,
		); err == nil {
			t.Errorf("repointEditHistory accepted %s, which has no merge", entity)
		}
	}
}

// Every entity type in the vocabulary must actually be swept by some delete
// path, checked by SCANNING THE SOURCE rather than by comparing two lists.
//
// The first version of this guard did the obvious thing: build a literal slice
// of the six "wired" types and assert it matches allPolymorphicEntityTypes. That
// is vacuous, and an adversarial reviewer caught it. The failure message told the
// next author to add their new type "here" as well as to the vocabulary, and
// doing exactly that turns the test green with no delete path written. It
// compared two hardcoded lists to each other and asserted nothing about any
// delete.
//
// So it now looks for a real `sweepEntityRefsForDelete(tx, entityTypeX,` call in
// this package's non-test sources, which is the same source-scanning idiom
// TestNoRevisionRepointOutsideTheHelper already uses next door. A seventh entity
// type fails here until its Delete* method genuinely calls the sweep.
func TestEveryEntityTypeHasADeletePath(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("failed to read package dir: %v", err)
	}

	var sources []byte
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, readErr := os.ReadFile(filepath.Join(pkgDir, name)) // #nosec G304 -- package's own sources
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", name, readErr)
		}
		sources = append(sources, b...)
	}
	if len(sources) == 0 {
		t.Fatal("scanned no sources, so this guard cannot fail")
	}

	for _, entity := range allPolymorphicEntityTypes {
		call := fmt.Sprintf("sweepEntityRefsForDelete(tx, entityType%s%s",
			strings.ToUpper(string(entity)[:1]), string(entity)[1:])
		if !bytes.Contains(sources, []byte(call)) {
			t.Errorf("%s is in the entity-type vocabulary but no delete path in this package "+
				"calls %s...). Either wire sweepEntityRefsForDelete into its Delete* method, or "+
				"remove it from allPolymorphicEntityTypes.", entity, call)
		}
		if !entity.valid() {
			t.Errorf("%s must be accepted by valid(), or its sweep is refused at runtime", entity)
		}
	}
}

// The alert-row sweep covers the discriminator-keyed shape the inventory cannot
// see, and ONLY on the entity axis.
//
// The subject axis is deliberately absent and this test pins that, because
// sweeping it is the intuitive thing to do and it is wrong: an artist_show_alert
// row is keyed on the SHOW (uq_notification_log_artist_show_alert is
// (user_id, entity_id, channel)) and is the exactly-once guard claimArtistAlertRow
// relies on, while subject_entity_id is only the row's label and can stand for a
// user's follow of several artists on the same bill. Deleting on that axis
// re-arms a duplicate email and removes a bell entry for a show that still
// exists. An earlier revision of this change did exactly that.
func TestAlertSweepCoversTheEntityAxisOnly(t *testing.T) {
	if len(alertTypesKeyedOnEntity[entityTypeShow]) == 0 {
		t.Error("an artist show alert's entity_id is a SHOW id, so deleting a show must sweep it; " +
			"the inventory's entity_type='show' DELETE cannot see it")
	}
	if len(alertTypesKeyedOnEntity[entityTypeVenue]) == 0 {
		t.Error("a venue show alert's entity_id is a VENUE id, so deleting a venue must sweep it")
	}
	// Artist is the type whose alerts name it only as a SUBJECT, so it must have
	// NO entry here: an artist delete must not touch notification_log through this
	// helper at all.
	if len(alertTypesKeyedOnEntity[entityTypeArtist]) != 0 {
		t.Error("no notification_log discriminator holds an ARTIST id in entity_id; " +
			"sweeping one on an artist delete would delete another entity's inbox rows")
	}
}
