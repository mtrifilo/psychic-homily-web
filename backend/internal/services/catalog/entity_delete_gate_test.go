package catalog

import (
	"testing"

	"psychic-homily-backend/internal/services/contracts"
)

// deleteAsAdmin is the actor for every test about the SWEEP rather than the
// gate. An admin bypasses the inertness check entirely, which is what the five
// admin-only sibling delete paths already do, so it keeps those tests measuring
// what they measured before PSY-1868's access gate existed.
func deleteAsAdmin() contracts.EntityDeleteActor {
	return contracts.EntityDeleteActor{UserID: adminActorUserID, IsAdmin: true}
}

// deleteAsUser is the actor the real endpoint passes for everybody else: an
// authenticated caller with no admin flag, which is what the orphan-cleanup flow
// in the show form is.
func deleteAsUser(userID uint) contracts.EntityDeleteActor {
	return contracts.EntityDeleteActor{UserID: userID}
}

// adminActorUserID is a stand-in id, deliberately not a seeded user: the admin
// path must not read the caller's rows at all, so a delete that starts consulting
// the actor's id would fail against a user that does not exist rather than
// passing quietly.
const adminActorUserID uint = 999999

// ──────────────────────────────────────────────
// The coupling between the gate and the sweep
// ──────────────────────────────────────────────

// The gate and the sweep must walk the SAME inventory.
//
// This is the failure the whole file is arranged around: a migration adds a
// polymorphic table, someone dispositions it dropRefRows, and the sweep starts
// destroying its rows while a gate walking a private list of its own keeps
// answering "no engagement": a non-admin delete silently destroying a table
// nobody remembered to protect. Both callers take entityRefsWalkedOnDelete(), so
// the coupling is structural; this pins that the list really is the whole
// inventory rather than a subset that looks like it.
func TestTheDeleteWalkCoversTheWholeInventory(t *testing.T) {
	walked := map[string]bool{}
	for _, ref := range entityRefsWalkedOnDelete() {
		if ref.table == "" || ref.idCol == "" {
			t.Fatalf("entityRefsWalkedOnDelete yielded an incomplete entry: %+v", ref)
		}
		if walked[ref.table] {
			t.Errorf("%s appears twice in the delete walk, so its statement runs twice", ref.table)
		}
		walked[ref.table] = true
	}

	for table := range entityRefTables() {
		if !walked[table] {
			t.Errorf("%s is in the entity-ref inventory but the delete walk skips it, so the "+
				"sweep never touches it and the inertness gate never counts it", table)
		}
	}
	for table := range walked {
		if !entityRefTables()[table] {
			t.Errorf("the delete walk yields %s, which is in neither polymorphicEntityRefs nor "+
				"refsRepointedElsewhere", table)
		}
	}
}

// Every table the sweep DROPS must say whose rows those are.
//
// Without an answer the gate cannot tell a stranger's follow from the caller's
// own, and the only safe thing left is to refuse the delete, which
// otherUsersEngagement does. This guard turns that runtime refusal into a CI
// failure at the moment the table is added.
func TestEveryDroppedTableHasAnEngagementActor(t *testing.T) {
	for _, ref := range entityRefsWalkedOnDelete() {
		if entityRefDeleteDispositions[ref.table] != dropRefRows {
			continue
		}
		if _, ok := engagementActorCols[ref.table]; !ok {
			t.Errorf("the delete drops %s rows, but engagementActorCols does not say which "+
				"column attributes one to a person. A non-admin delete cannot tell whose rows "+
				"it would destroy, so it will refuse outright until this is recorded.", ref.table)
		}
	}
}

// And the reverse: an actor column for a table the sweep does not drop is a
// decision nothing consults. It reads as protection in review and is not.
func TestNoEngagementActorForATableTheSweepDoesNotDrop(t *testing.T) {
	for table := range engagementActorCols {
		switch entityRefDeleteDispositions[table] {
		case dropRefRows:
		case keepRefRowsAsTombstone:
			t.Errorf("engagementActorCols answers for %s, which is dispositioned "+
				"keepRefRowsAsTombstone. The delete destroys nothing there, so the gate never "+
				"reads this column.", table)
		default:
			t.Errorf("engagementActorCols answers for %s, which is not in the delete walk at "+
				"all", table)
		}
	}
}

// The register of who counts as engagement, pinned by name.
//
// Same reasoning as TestRecordedDeleteDispositionsAreTheOnesReviewed: every other
// assertion reads engagementActorCols to decide what to expect, so the map is the
// SPEC. Without this list, quietly re-recording user_bookmarks as
// systemWrittenRows would open the destruction path back up with every test
// still green.
func TestRecordedEngagementActorsAreTheOnesReviewed(t *testing.T) {
	reviewed := map[string]string{
		"collection_items":      "added_by_user_id",
		"comment_last_read":     "user_id",
		"comment_subscriptions": "user_id",
		"entity_tags":           "added_by_user_id",
		"notification_log":      "user_id",
		"tag_votes":             "user_id",
		"user_bookmarks":        "user_id",

		// Machine-written. image_enrich_queue is the load-bearing one: wherever the
		// enrichment sweep is enabled, the create funnel enqueues a row for every
		// artist, so counting it would refuse every freshly created orphan and break
		// the flow the non-admin path exists to serve.
		"image_enrich_queue": systemWrittenRows,
		"source_configs":     systemWrittenRows,
	}

	for table, want := range reviewed {
		got, ok := engagementActorCols[table]
		if !ok {
			t.Errorf("%s lost its engagement-actor entry", table)
			continue
		}
		if got != want {
			t.Errorf("%s's engagement actor changed from %q to %q. If that is deliberate, say "+
				"why beside the entry and update this register.", table, want, got)
		}
	}
	for table := range engagementActorCols {
		if _, ok := reviewed[table]; !ok {
			t.Errorf("%s gained an engagement-actor entry that no one recorded a review for",
				table)
		}
	}
}

// ──────────────────────────────────────────────
// The guards in front of any statement
// ──────────────────────────────────────────────

// Same idiom as the rest of this package's guard tests: unusableTx() is a
// zero-value *gorm.DB, so a check that stopped rejecting would reach Raw and
// panic rather than passing quietly.
func TestEngagementCheckRejectsAMissingCaller(t *testing.T) {
	if _, err := tablesWithOtherUsersEngagement(unusableTx(), entityTypeArtist, 7, 0); err == nil {
		t.Fatal("a zero caller id must be rejected: with no caller there is no way to tell " +
			"the caller's own rows from a stranger's, and the gate would pass on rows it " +
			"cannot attribute")
	}
}

func TestEngagementCheckRejectsAnUnknownEntityType(t *testing.T) {
	if _, err := tablesWithOtherUsersEngagement(
		unusableTx(), polymorphicEntityType("scene"), 7, 3); err == nil {
		t.Fatal("an unknown entity type must be rejected, not queried")
	}
}

func TestEngagementCheckRejectsZeroID(t *testing.T) {
	if _, err := tablesWithOtherUsersEngagement(unusableTx(), entityTypeArtist, 0, 3); err == nil {
		t.Fatal("a zero entity id must be rejected: it matches nothing, so the gate would " +
			"report an engaged artist as inert")
	}
}
