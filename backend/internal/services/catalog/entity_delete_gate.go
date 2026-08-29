package catalog

import (
	"fmt"

	"gorm.io/gorm"
)

// This file is the ACCESS gate in front of the sweep in entity_ref_delete.go.
//
// The sweep made an existing asymmetry dangerous. DELETE /artists/{artist_id} is
// open to any authenticated user (setupArtistRoutes puts it on rc.Protected),
// while the other five catalog deletes are admin-only. Before PSY-1868 that
// endpoint destroyed one row (the artist) and left every reference to it
// stranded. With the sweep behind it, the same unprivileged call now destroys
// OTHER people's follows, crate items, tag votes and thread subscriptions along
// the way. Fixing the orphan bug without this gate would have handed a
// destruction primitive to every logged-in account.
//
// Admin-gating the route was the obvious fix and was rejected: the show form
// offers to delete artists its own edit just orphaned (OrphanedArtistsDialog in
// ShowForm.tsx), and that flow is the endpoint's only real caller.
//
// So the answer is ORPHAN-ONLY for non-admins: an admin deletes outright, and
// everyone else may delete only an artist that is genuinely inert. A
// just-orphaned artist is inert by definition, so the dialog keeps working; an
// artist anyone has engaged with is refused with a 403.
//
// WHAT THIS GATE DOES NOT COVER, stated rather than implied:
//
//   - The tombstone tables. comments, entity_reports, requests and the rest are
//     dispositioned keepRefRowsAsTombstone, so the delete destroys nothing in
//     them, and they are deliberately NOT counted toward inertness. Counting
//     them would break the flow this gate exists to preserve: audit_logs,
//     revisions and entity_edit_audit_logs carry a row for whoever CREATED the
//     artist, which is routinely not the user the orphan dialog is asking. Those
//     rows do outlive the artist as unreachable prose, but that is pre-existing
//     behaviour of an endpoint that has always been open, unchanged by
//     PSY-1868, and widening the gate to cover it is a moderation-policy call
//     rather than a fix for the destruction this ticket introduced.
//   - The database's own foreign keys. Ten of them CASCADE (see the note at the
//     top of entity_ref_delete.go); this gate reads the polymorphic inventory
//     only, because the polymorphic tables are the ones the sweep destroys.

// engagementActorCols names, for every table the sweep DROPS rows from, the
// column that attributes a row to the person who created it.
//
// The keys are the DROPPED half of entityRefDeleteDispositions and nothing else:
// a kept table destroys nothing, so it has no bearing on what a non-admin delete
// may take from a stranger. TestEveryDroppedTableHasAnEngagementActor and
// TestNoEngagementActorForATableTheSweepDoesNotDrop keep the two halves aligned,
// and otherUsersEngagement REFUSES a dropped table that is missing here rather
// than skipping it. A skipped table is a table the gate silently answers "no
// engagement" for, which is exactly the hole this file exists to close.
var engagementActorCols = map[string]string{
	// The crate item names its adder, not the crate's owner. Both are the same
	// person today (AddItem requires the caller to own the collection), and the
	// adder is the right column either way: it is the one recording a HUMAN
	// action on this entity.
	"collection_items": "added_by_user_id",
	// A per-user read cursor on the entity's comment thread.
	"comment_last_read": "user_id",
	// A per-user subscription to that thread.
	"comment_subscriptions": "user_id",
	// Tag membership names the user who applied the tag.
	"entity_tags": "added_by_user_id",
	// A vote on someone's tag membership.
	"tag_votes": "user_id",
	// A follow / save / confirm.
	"user_bookmarks": "user_id",
	// A delivered notification, which is somebody's inbox row. Nothing writes
	// entity_type='artist' here today (see the disposition's note), so this
	// predicate matches nothing on the artist path. It is recorded anyway
	// because "matches nothing today" is a property of the WRITERS, which can
	// change without anyone revisiting this file.
	"notification_log": "user_id",

	// ── Machine-written: no person is behind the row, so it cannot make an
	// artist un-inert. ──

	// The image-enrichment outbox job. This one is not a nicety: wherever the
	// sweep is enabled (ENABLE_IMAGE_ENRICH_SWEEP=1, off by default), the artist
	// create funnel enqueues a row for EVERY artist it creates
	// (enqueueImageEnrich, called from FindOrCreateArtistTx), so counting it
	// would refuse every freshly created orphan and break the one flow the
	// non-admin path exists for.
	"image_enrich_queue": systemWrittenRows,
	// A scraper registration, written by admin tooling and reachable only for
	// venue and label by CHECK. No user column exists to consult.
	"source_configs": systemWrittenRows,
}

// systemWrittenRows marks a table whose rows record no person.
//
// Deliberately distinct from "the table is missing from the map": the lookup
// below distinguishes the two, so a table recorded as machine-written is a
// DECISION with a reason written beside it, while an absent table is an
// unanswered question that fails the delete.
const systemWrittenRows = ""

// otherUsersEngagement reports which swept tables hold a row belonging to
// somebody other than the caller.
//
// The rule, stated once so nothing else has to restate it: a table counts when
// the delete would DROP its rows and those rows name a user; a row counts when
// that user is not the caller. Rows the caller made themselves are excluded
// because the gate protects OTHER people's data, and a caller destroying their
// own follow or their own tag is the same thing the un-follow and un-tag
// endpoints already let them do.
//
// It walks entityRefsWalkedOnDelete(), the same list the sweep walks, so a
// table added to the inventory reaches both or fails both.
//
// Must run inside the delete's transaction. That is not a formality: it is what
// makes the answer describe the same snapshot the sweep is about to act on. It
// does NOT make the check atomic against a concurrent engagement write, and the
// residual window is documented at the call site in DeleteArtist.
func otherUsersEngagement(
	tx *gorm.DB,
	entity polymorphicEntityType,
	entityID uint,
	callerUserID uint,
) ([]string, error) {
	if !entity.valid() {
		return nil, fmt.Errorf("other users engagement: unknown entity type %q", string(entity))
	}
	if entityID == 0 {
		return nil, fmt.Errorf("other users engagement: entity id is required")
	}
	// Without a caller there is no "other" to measure against. Refusing is the
	// fail-closed direction; the alternative (treat 0 as a user id nobody has)
	// would silently widen the count rather than narrow it, but it would also
	// mean a bug in the auth plumbing quietly changed what this function means.
	if callerUserID == 0 {
		return nil, fmt.Errorf("other users engagement: caller user id is required")
	}

	var engaged []string
	for _, ref := range entityRefsWalkedOnDelete() {
		disposition, ok := entityRefDeleteDispositions[ref.table]
		if !ok {
			return nil, fmt.Errorf(
				"other users engagement: %s is in the entity-ref inventory but has no recorded "+
					"delete disposition; add one to entityRefDeleteDispositions", ref.table)
		}
		if disposition != dropRefRows {
			continue
		}

		actorCol, ok := engagementActorCols[ref.table]
		if !ok {
			return nil, fmt.Errorf(
				"other users engagement: the delete drops %s rows but no engagement-actor "+
					"column is recorded for it, so a non-admin delete cannot tell whose rows it "+
					"would destroy; add one to engagementActorCols", ref.table)
		}
		if actorCol == systemWrittenRows {
			continue
		}

		// EXISTS rather than COUNT: four of the dropped tables have no index on
		// (entity_type, entity_id) (see the COST note in entity_ref_delete.go), so
		// this is a scan an unprivileged caller can trigger, and EXISTS lets the
		// planner stop at the first matching row.
		//
		// #nosec G201 -- table and both column names come from the hardcoded
		// inventory and the hardcoded map above, never from caller input; the
		// entity type, id and caller id are bound parameters.
		sql := fmt.Sprintf(
			"SELECT EXISTS (SELECT 1 FROM %s WHERE entity_type = ? AND %s = ? AND %s <> ?)",
			ref.table, ref.idCol, actorCol)

		var found bool
		if err := tx.Raw(sql, string(entity), entityID, callerUserID).Scan(&found).Error; err != nil {
			return nil, fmt.Errorf("failed to check %s for other users' rows: %w", ref.table, err)
		}
		if found {
			engaged = append(engaged, ref.table)
		}
	}

	return engaged, nil
}
