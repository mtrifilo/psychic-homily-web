package catalog

import (
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

// The table alone is not enough of a fence: both the table AND the column are
// interpolated, so a listed table paired with a supplied column name would still
// assemble whatever statement the caller asked for.
func TestApplyRefDeleteDispositionRejectsAWrongIDColumn(t *testing.T) {
	if _, err := applyRefDeleteDisposition(
		unusableTx(), "user_bookmarks", "id", entityTypeArtist, 1); err == nil {
		t.Error("user_bookmarks keys on entity_id; sweeping it by id must be refused")
	}
	if _, err := applyRefDeleteDisposition(
		unusableTx(), "requests", "entity_id", entityTypeArtist, 1); err == nil {
		t.Error("requests keys on requested_entity_id; the inventory's column is the only legal one")
	}
	if _, err := applyRefDeleteDisposition(
		unusableTx(), "user_bookmarks", "entity_id = 0 OR TRUE --", entityTypeArtist, 1); err == nil {
		t.Error("a column name that is really a predicate must be refused, not interpolated")
	}
}

// And the fence must not be so tight that the real call sites cannot pass it.
// Checked against the derived map rather than through the helper, because an
// accepted pair goes on to execute and a zero *gorm.DB panics there.
func TestEveryInventoriedTableHasAnIDColumn(t *testing.T) {
	columns := entityRefIDColumns()
	for table := range entityRefTables() {
		if columns[table] == "" {
			t.Errorf("%s has no entity id column in the derived map, so every sweep of it "+
				"would be refused", table)
		}
	}
}

// A table whose recorded disposition is keepRefRowsAsTombstone must run NO
// statement at all. Checked through unusableTx rather than by inspecting the
// map, because "the decision is recorded" and "the decision is honoured" are
// different claims and only the second one protects contributor history.
func TestKeptTablesIssueNoStatement(t *testing.T) {
	columns := entityRefIDColumns()
	for table, disposition := range entityRefDeleteDispositions {
		if disposition != keepRefRowsAsTombstone {
			continue
		}
		n, err := applyRefDeleteDisposition(unusableTx(), table, columns[table], entityTypeArtist, 1)
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

// clearRefEntityColumn is only legal where NULL is a value the column already
// carries and the readers already understand. Nulling a NOT NULL column would
// abort every delete; nulling a nullable column whose NULL drives a work queue
// is worse, because it succeeds — see entity_requests' entry, where NULL would
// resurrect a fulfilled request into the admin rescue queue.
//
// So the two entries are pinned by name. Adding a third is meant to be a
// deliberate act that fails here first.
func TestOnlyTheFulfillmentPointerIsCleared(t *testing.T) {
	cleared := map[string]bool{}
	for table, disposition := range entityRefDeleteDispositions {
		if disposition == clearRefEntityColumn {
			cleared[table] = true
		}
	}

	if !cleared["requests"] {
		t.Error("requests.requested_entity_id is a fulfillment pointer whose NULL means " +
			"'unfulfilled'; clearing it is the whole reason this disposition exists")
	}
	if cleared["entity_requests"] {
		t.Error("entity_requests.created_entity_id must NOT be cleared: " +
			"(approved AND created_entity_id IS NULL) is the admin rescue queue's predicate, " +
			"so nulling it manufactures a work item and invites a duplicate entity")
	}
	if len(cleared) != 1 {
		t.Errorf("cleared = %v, want exactly [requests]. A nullable column is not on its own "+
			"a reason to clear one: check what reads its NULL first.", cleared)
	}
}

// The alert-row sweep covers the two shapes the inventory cannot see, and no
// others. Both maps are hand-maintained, and the failure mode of dropping an
// entry is silent — an inbox row pointing at a 404 raises nothing.
func TestAlertSweepCoversBothInvisibleShapes(t *testing.T) {
	if len(alertTypesKeyedOnEntity[entityTypeShow]) == 0 {
		t.Error("an artist show alert's entity_id is a SHOW id, so deleting a show must sweep it; " +
			"the inventory's entity_type='show' DELETE cannot see it")
	}
	if len(alertTypesKeyedOnEntity[entityTypeVenue]) == 0 {
		t.Error("a venue show alert's entity_id is a VENUE id, so deleting a venue must sweep it")
	}
	if len(alertTypesKeyedOnSubject[entityTypeArtist]) == 0 {
		t.Error("an artist show alert's subject_entity_id is an ARTIST id, so deleting an artist " +
			"must sweep it; the column is not entity_id and the inventory has no concept of it")
	}
	// Venue show alerts carry no subject: the followed entity and the subject are
	// the same venue, so it lives in entity_id and subject_entity_id stays NULL.
	// Listing one would delete rows on a column that is never set for them.
	if len(alertTypesKeyedOnSubject[entityTypeVenue]) != 0 {
		t.Error("venue show alerts have a NULL subject_entity_id by design; sweeping on it is " +
			"either a no-op or, if the design changes, a delete nobody reasoned about")
	}
}
