package catalog

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	notificationm "psychic-homily-backend/internal/models/notification"
)

// This file is the DELETE counterpart to entity_ref_repoint.go, and it exists
// for the same reason that file does: the polymorphic (entity_type, entity_id)
// tables carry no foreign key, so a row left behind when an entity goes away
// does not fail loudly. It silently points at an id that no longer exists.
//
// PSY-1834 closed that gap for the artist MERGE and PSY-1869 for the show merge.
// Deletion was the second unguarded source and had none of it: every Delete*
// method in this package was a bare `db.Delete(&entity)` after an existence
// check, with at most one hand-picked table swept beside it. The 2026-08-19
// production audit found orphaned audit_logs rows attributable to no merge.
//
// A delete cannot re-point anything — there is no surviving entity — so each
// table needs a DIFFERENT kind of answer than the merges give it, and the answer
// is a judgement call about that table rather than about this operation. So it is
// recorded per table, once, in entityRefDeleteDispositions below, and
// deleteEntityRefs REFUSES a table with no recorded disposition rather than
// picking a default. TestEveryInventoriedRefHasADeleteDisposition and the seeding
// sweep in artist_delete_refs_test.go are what keep that list from falling behind
// the inventory.

// refDeleteDisposition is what happens to one reference table's rows when the
// entity they point at is deleted outright.
//
// There is no default. The zero value is rejected at runtime, so a table added
// to the inventory without a decision fails the first delete that runs rather
// than quietly getting whichever behaviour happened to be cheapest to write.
type refDeleteDisposition int

const (
	// deleteDispositionUndecided is the zero value, and never a legal entry. Its
	// only job is to make "a table was added to the inventory and nobody said
	// what a delete does with it" a loud failure.
	deleteDispositionUndecided refDeleteDisposition = iota

	// dropRefRows deletes the rows along with the entity.
	//
	// For tables whose rows cannot outlive the entity without becoming WRONG
	// rather than merely historical: they inflate a count somebody reads, occupy
	// a work queue that will never drain, or address a reader at an entity that
	// is not there. A saved-show tile that renders blank, a tag whose "47
	// artists" includes one that no longer exists, an enrichment job that can
	// never succeed.
	dropRefRows

	// keepRefRowsAsTombstone leaves the rows in place, still naming the deleted
	// id, on purpose.
	//
	// For tables that record what a PERSON did — an edit, a report, a comment, an
	// admin action — and are read on an axis the deleted entity does not gate:
	// by actor on the contributor profile and leaderboard, by reviewer in the
	// trusted-tier promotion counts, by time in the global admin trail. Those
	// rows become historical when the entity goes, not wrong, and deleting them
	// destroys a person's record of their own work.
	//
	// The asymmetry is deliberate and is the reason each table gets its own line:
	// DELETE /artists/{id} is open to ANY authenticated user (see
	// setupArtistRoutes), so this path is the cheapest way in the API to destroy
	// contributor history. Nothing here is recoverable — none of these tables has
	// a soft-delete column — so "leave the row" is the reversible choice and
	// "delete it" is not.
	keepRefRowsAsTombstone

	// clearRefEntityColumn keeps the row and NULLs the column that named the
	// entity.
	//
	// Only legal for a NULLABLE pointer column whose NULL already means something
	// true — both of the current entries are fulfillment pointers on a user's
	// request, where NULL means "no catalog row exists for this request", which is
	// exactly the state a delete restores.
	clearRefEntityColumn
)

// String makes a rejected disposition readable in the error rather than printing
// an integer the reader has to count out against this file.
func (d refDeleteDisposition) String() string {
	switch d {
	case dropRefRows:
		return "dropRefRows"
	case keepRefRowsAsTombstone:
		return "keepRefRowsAsTombstone"
	case clearRefEntityColumn:
		return "clearRefEntityColumn"
	case deleteDispositionUndecided:
		return "deleteDispositionUndecided"
	default:
		return fmt.Sprintf("refDeleteDisposition(%d)", int(d))
	}
}

// entityRefDeleteDispositions answers, for every table in the shared inventory
// (polymorphicEntityRefs plus refsRepointedElsewhere), what a delete does with
// its rows.
//
// One map for all six entity types, matching the inventory it keys on: a table's
// unique index and its read paths are properties of the TABLE, so a per-entity
// copy could only ever be a copy that drifts.
//
// TestEveryInventoriedRefHasADeleteDisposition fails when the inventory gains a
// table this map does not answer for, and when this map answers for a table the
// inventory does not carry.
var entityRefDeleteDispositions = map[string]refDeleteDisposition{
	// ── Dropped: the row is a pointer, a queue entry or a per-user cursor, and
	// outliving the entity makes it wrong rather than historical. ──

	// A crate item is the crate's contents. Left behind it renders as a card with
	// no entity behind it and still counts toward the crate's item total.
	"collection_items": dropRefRows,
	// A per-user "last read this thread at" cursor. It has no meaning without the
	// thread, and the thread is on the entity.
	"comment_last_read": dropRefRows,
	// A subscription to a thread that can no longer receive a comment.
	"comment_subscriptions": dropRefRows,
	// Tag membership. Kept, it inflates every tag count and puts a dead entity in
	// the tag's browse listing, which is the one place tags are read from.
	"entity_tags": dropRefRows,
	// A vote on a tag membership that is being dropped in the same statement
	// batch. Scoring an assertion nobody can see is not history.
	"tag_votes": dropRefRows,
	// A follow / save / confirm. The precedent is already in this package:
	// DeleteShow and DeleteRelease each swept exactly this table by hand, which is
	// how the rest of the inventory was found to be unswept.
	"user_bookmarks": dropRefRows,
	// An image-enrichment job that can never succeed. Its worker would fetch and
	// then fail to attach.
	"image_enrich_queue": dropRefRows,
	// A scraper registration. This one is not merely untidy: a source config for a
	// deleted venue or label keeps a poller pointed at a source whose entity is
	// gone. (venue/label only by CHECK; the statement matches nothing for the
	// other four types.)
	"source_configs": dropRefRows,
	// A delivered-notification record, which is a user's inbox. Left behind it is
	// an inbox row linking to an entity that 404s. Its other job — proving an
	// alert was already sent, so it is not sent twice — is moot once there is
	// nothing left to alert about.
	"notification_log": dropRefRows,

	// ── Kept as tombstones: a record of something a person did, read on an axis
	// the deleted entity does not gate. ──

	// The admin action trail. GetAuditLogs (services/admin/audit_log.go) is a
	// global, time-ordered, filterable list — not an entity lookup — and
	// contributor activity reads it by actor. These rows are the record of what
	// was done TO the entity, including by whoever deleted it, so deleting them
	// with the entity erases the only account of the deletion's context. The
	// orphaned audit_logs rows the 2026-08-19 audit found are therefore the
	// INTENDED state; what was missing was that nine other tables were orphaned
	// alongside them with nothing recording which was which.
	"audit_logs": keepRefRowsAsTombstone,
	// User-authored prose. Unreachable from the deleted entity, but reachable by
	// AUTHOR (ListFieldNotesByAuthor) and in the global moderation queue
	// (ListPendingComments), and irreversibly destroyed if dropped here — on an
	// endpoint any authenticated user can call.
	"comments": keepRefRowsAsTombstone,
	// A moderation report. Counted by reporter AND by reviewer on the contributor
	// profile, and listed on a global admin queue that is not entity-scoped.
	// Auto-resolving reports when an admin deletes the reported entity is a
	// moderation-workflow decision, not an orphan fix; it is deliberately NOT
	// smuggled in here.
	"entity_reports": keepRefRowsAsTombstone,
	// The edit-diff history. Counted per USER for contributor stats and for the
	// contributor leaderboard (services/user/contributor_profile.go), so dropping
	// these silently lowers a contributor's public standing.
	"revisions": keepRefRowsAsTombstone,
	// Contributor edit submissions. The trusted-tier auto-promotion AND demotion
	// checks (services/admin/auto_promotion.go) count these by submitter, so
	// dropping them can silently cost a contributor their tier — the same harm
	// PSY-1788 fixed on the merge path. A still-pending row does linger in the
	// admin review queue naming a deleted entity; that is the accepted cost of the
	// decision, and pruning it belongs to the review workflow rather than here.
	"pending_entity_edits": keepRefRowsAsTombstone,
	// The append-only trail beside it, served on the anonymous contributions feed
	// scoped by ACTOR (services/user/contributor_profile.go), never by entity.
	"entity_edit_audit_logs": keepRefRowsAsTombstone,

	// entity_requests looks like `requests` below and must NOT be treated like it.
	// Its created_entity_id is nullable too, but NULL is LOAD-BEARING there:
	// (decision_state = 'approved' AND created_entity_id IS NULL) is how the admin
	// rescue queue finds approvals whose catalog row was never created, and how
	// ClaimRescueFulfillment stakes its concurrency claim
	// (services/community/entityrequest_rescue.go). NULLing this would resurrect a
	// long-fulfilled request into that queue and invite an admin to create the
	// entity a second time. The stale id is inert by comparison — no read path
	// dereferences it — so the row is left exactly as it is.
	"entity_requests": keepRefRowsAsTombstone,

	// ── Cleared: the row survives, the pointer does not. ──

	// A fulfilled request on the community wishlist. The row belongs to the
	// REQUESTER — it carries their title, description and the votes it drew — so
	// dropping it would destroy a user's post and skew the request-fulfillment
	// chart in admin analytics. Only the fulfillment pointer is stale, and that
	// column is nullable with a meaning already in use: RejectFulfillment
	// (services/community/request.go) NULLs this same column to say "no entity
	// stands behind this request", which is exactly true after the delete.
	// ResolveEntityRef already degrades a missing row to "no link", so this is a
	// tidy-up rather than a repair.
	//
	// The difference from entity_requests is not cosmetic: NULL here restores a
	// wishlist item to "unfulfilled", which is honest, while NULL there manufactures
	// a work item that an admin would act on.
	"requests": clearRefEntityColumn,
}

// refsRepointedElsewhereIDCol is the entity-id column for the three tables that
// are named in refsRepointedElsewhere rather than carried in
// polymorphicEntityRefs.
//
// All three use entity_id. It is a named constant rather than a literal at the
// loop below so that a table joining that list with a different column name
// cannot be swept against the wrong column in silence.
const refsRepointedElsewhereIDCol = "entity_id"

// deleteEntityRefs carries out every recorded disposition for one entity that is
// about to be deleted, and reports what it did per table.
//
// Must run inside the delete's transaction. The sweep and the delete are one
// unit: a sweep that commits without the delete strips a live entity of its
// bookmarks and tags, and a delete that commits without the sweep is the bug
// this function exists to fix.
//
// Returns rows affected per table for every table in the inventory, including
// zero for the ones deliberately left alone, so a caller can log the whole
// disposition rather than only the destructive half.
func deleteEntityRefs(
	tx *gorm.DB,
	entity polymorphicEntityType,
	entityID uint,
) (map[string]int64, error) {
	if !entity.valid() {
		return nil, fmt.Errorf("delete entity refs: unknown entity type %q", string(entity))
	}
	if entityID == 0 {
		return nil, fmt.Errorf("delete entity refs: entity id is required")
	}

	affected := make(map[string]int64, len(polymorphicEntityRefs)+len(refsRepointedElsewhere))

	for _, ref := range polymorphicEntityRefs {
		if ref.table == "" || ref.idCol == "" {
			return nil, fmt.Errorf("delete entity refs: table and id column are required")
		}
		n, err := applyRefDeleteDisposition(tx, ref.table, ref.idCol, entity, entityID)
		if err != nil {
			return nil, err
		}
		affected[ref.table] = n
	}

	// The provenance-gated tables are swept here rather than being excluded the
	// way repointEntityRefs excludes them. The reason that exclusion exists does
	// not apply: it forces an author to state what happens to a re-pointed row's
	// READ-TIME REDACTION, and a delete re-points nothing. All three are recorded
	// as kept, so this loop issues no statement for them today — but routing them
	// through the same lookup is what makes a future change to one of those
	// dispositions impossible to forget, and what makes them appear in the
	// affected map rather than silently missing from it.
	for _, table := range refsRepointedElsewhere {
		n, err := applyRefDeleteDisposition(tx, table, refsRepointedElsewhereIDCol, entity, entityID)
		if err != nil {
			return nil, err
		}
		affected[table] = n
	}

	return affected, nil
}

// entityRefIDColumns is the entity-id column each inventoried table uses, built
// from the inventory itself so the two cannot disagree.
//
// It exists to pin the table/column PAIR rather than the table alone. Both go
// into the statement by interpolation, and a fence on the table only would let a
// listed table be swept against any column name a caller supplied. Same shape,
// and the same reason, as the show merge's showFKColumns.
func entityRefIDColumns() map[string]string {
	out := make(map[string]string, len(polymorphicEntityRefs)+len(refsRepointedElsewhere))
	for _, ref := range polymorphicEntityRefs {
		out[ref.table] = ref.idCol
	}
	for _, table := range refsRepointedElsewhere {
		out[table] = refsRepointedElsewhereIDCol
	}
	return out
}

// applyRefDeleteDisposition runs one table's recorded decision.
//
// The table and column are interpolated into SQL, so the two lookups below are
// not only completeness checks: together they pin the pair to the hardcoded
// inventory rather than to "every call site happens to pass a literal today".
func applyRefDeleteDisposition(
	tx *gorm.DB,
	table, idCol string,
	entity polymorphicEntityType,
	entityID uint,
) (int64, error) {
	disposition, ok := entityRefDeleteDispositions[table]
	if !ok {
		return 0, fmt.Errorf(
			"delete entity refs: %s is in the entity-ref inventory but has no recorded delete "+
				"disposition; add one to entityRefDeleteDispositions", table)
	}
	if want, ok := entityRefIDColumns()[table]; !ok || want != idCol {
		return 0, fmt.Errorf(
			"delete entity refs: %s's entity id column is %q in the inventory, not %q",
			table, want, idCol)
	}

	switch disposition {
	case keepRefRowsAsTombstone:
		return 0, nil

	case dropRefRows:
		// #nosec G201 -- table and column come from the hardcoded inventory,
		// never from caller input; the id and entity type are bound parameters.
		sql := fmt.Sprintf("DELETE FROM %s WHERE entity_type = ? AND %s = ?", table, idCol)
		r := tx.Exec(sql, string(entity), entityID)
		if r.Error != nil {
			return 0, fmt.Errorf("failed to delete %s rows: %w", table, r.Error)
		}
		return r.RowsAffected, nil

	case clearRefEntityColumn:
		// #nosec G201 -- see above.
		sql := fmt.Sprintf(
			"UPDATE %[1]s SET %[2]s = NULL WHERE entity_type = ? AND %[2]s = ?", table, idCol)
		r := tx.Exec(sql, string(entity), entityID)
		if r.Error != nil {
			return 0, fmt.Errorf("failed to clear %s entity reference: %w", table, r.Error)
		}
		return r.RowsAffected, nil

	default:
		return 0, fmt.Errorf(
			"delete entity refs: %s has disposition %s, which is not a decision; "+
				"record dropRefRows, keepRefRowsAsTombstone or clearRefEntityColumn",
			table, disposition)
	}
}

// deleteAlertRowsNamingEntity removes the notification_log rows that name a
// deleted entity through a column the inventory above cannot see.
//
// Two blind spots, both inherited from the merge path that documented them
// (PSY-1895, PSY-1896) and both invisible to the loop for a structural reason
// rather than an oversight:
//
//   - An alert row keys entity_type on its OWN discriminator, not on the kind of
//     entity in entity_id. An 'artist_show_alert' row's entity_id is a SHOW id
//     and a 'venue_show_alert' row's is a VENUE id, so the loop's
//     `entity_type = 'show' | 'venue'` DELETE matches none of them.
//   - subject_entity_id is a second entity reference whose column is not
//     entity_id and whose type is implied by the row's discriminator, so the
//     inventory has no concept of it at all.
//
// Every row found here is DROPPED, matching notification_log's disposition
// above: an inbox row is only worth keeping while the thing it announces still
// exists, and "a show by an artist that is no longer in the catalog" is not an
// announcement anyone can act on.
func deleteAlertRowsNamingEntity(tx *gorm.DB, entity polymorphicEntityType, entityID uint) (int64, error) {
	if !entity.valid() {
		return 0, fmt.Errorf("delete alert rows: unknown entity type %q", string(entity))
	}
	if entityID == 0 {
		return 0, fmt.Errorf("delete alert rows: entity id is required")
	}

	var total int64

	// Alert rows whose entity_id holds this kind of entity.
	if types := alertTypesKeyedOnEntity[entity]; len(types) > 0 {
		r := tx.Exec(
			"DELETE FROM notification_log WHERE entity_type IN ? AND entity_id = ?", types, entityID)
		if r.Error != nil {
			return 0, fmt.Errorf("failed to delete alert rows naming the entity: %w", r.Error)
		}
		total += r.RowsAffected
	}

	// Alert rows whose SUBJECT is this kind of entity. The row's own entity_id
	// still points at a live show, but the alert is about a band that no longer
	// exists, so the row goes with it rather than losing its name.
	if types := alertTypesKeyedOnSubject[entity]; len(types) > 0 {
		r := tx.Exec(
			"DELETE FROM notification_log WHERE entity_type IN ? AND subject_entity_id = ?",
			types, entityID)
		if r.Error != nil {
			return 0, fmt.Errorf("failed to delete alert rows naming the subject: %w", r.Error)
		}
		total += r.RowsAffected
	}

	return total, nil
}

// alertTypesKeyedOnEntity maps an entity type to the notification_log
// discriminators whose entity_id holds that kind of id.
//
// Artist show alerts name a SHOW (the alert is about one specific show) and
// venue show alerts name a VENUE (the alert is coalesced over a day and has no
// single show to point at), which is why neither appears under the entity type
// its NAME suggests.
var alertTypesKeyedOnEntity = map[polymorphicEntityType][]string{
	entityTypeShow:  {notificationm.NotificationEntityArtistShowAlert},
	entityTypeVenue: {notificationm.NotificationEntityVenueShowAlert},
}

// alertTypesKeyedOnSubject maps an entity type to the discriminators whose
// subject_entity_id holds that kind of id.
//
// Venue show alerts are deliberately absent and not by oversight: their subject
// and their followed entity are the same venue, so the venue lives in entity_id
// and subject_entity_id stays NULL. artistSubjectAlertTypes is the merge path's
// list of the same thing, reused so the two cannot disagree.
var alertTypesKeyedOnSubject = map[polymorphicEntityType][]string{
	entityTypeArtist: artistSubjectAlertTypes,
}

// sweepEntityRefsForDelete is the one call every Delete* method makes: it runs
// the recorded dispositions plus the alert rows the inventory cannot see, and
// logs what it destroyed.
//
// Wrapped in one function rather than left as two calls per delete site so that
// a seventh delete path cannot pick up half of it, which is precisely how
// DeleteShow and DeleteRelease ended up sweeping user_bookmarks and nothing
// else.
func sweepEntityRefsForDelete(tx *gorm.DB, entity polymorphicEntityType, entityID uint) error {
	// Strictly before the sweep: it counts rows the sweep is about to remove.
	if err := releaseTagUsageCounts(tx, entity, entityID); err != nil {
		return err
	}
	affected, err := deleteEntityRefs(tx, entity, entityID)
	if err != nil {
		return err
	}
	alerts, err := deleteAlertRowsNamingEntity(tx, entity, entityID)
	if err != nil {
		return err
	}
	logDeletedEntityRefs(entity, entityID, affected, alerts)
	return nil
}

// releaseTagUsageCounts gives back the tags.usage_count this entity was holding,
// and MUST run before the entity_tags rows are deleted.
//
// tags.usage_count is DENORMALIZED and hand-maintained: AddTagToEntity
// increments it and RemoveTagFromEntity decrements it (services/catalog/
// tag_service.go), and nothing recomputes it. So the generic
// `DELETE FROM entity_tags` in the sweep leaves the counter permanently
// overstated by however many tags the deleted entity carried — and that counter
// is not decorative. It orders ListTags, feeds the tag hierarchy, and is the
// whole predicate of PruneLowQualityTags (usage_count = 0), which would stop
// reclaiming a tag whose last real use has been deleted.
//
// GREATEST(...,0) mirrors RemoveTagFromEntity's `usage_count > 0` floor: the
// counter is already known to drift (nothing recomputes it), and a delete is not
// the place to turn accumulated drift into a negative number.
func releaseTagUsageCounts(tx *gorm.DB, entity polymorphicEntityType, entityID uint) error {
	if !entity.valid() {
		return fmt.Errorf("release tag usage counts: unknown entity type %q", string(entity))
	}
	if entityID == 0 {
		return fmt.Errorf("release tag usage counts: entity id is required")
	}
	// The decrement is only correct because the rows are about to go. If
	// entity_tags is ever re-dispositioned to keep its rows, this becomes a
	// silent double-count against every tag on the entity, so the assumption is
	// asserted rather than left to a comment.
	if d := entityRefDeleteDispositions["entity_tags"]; d != dropRefRows {
		return fmt.Errorf(
			"release tag usage counts: entity_tags is dispositioned %s, so releasing its "+
				"usage_count would decrement a counter for rows that are staying", d)
	}

	if err := tx.Exec(`
		UPDATE tags t
		   SET usage_count = GREATEST(t.usage_count - held.n, 0)
		  FROM (
		        SELECT tag_id, COUNT(*) AS n
		          FROM entity_tags
		         WHERE entity_type = ? AND entity_id = ?
		      GROUP BY tag_id
		       ) held
		 WHERE t.id = held.tag_id
	`, string(entity), entityID).Error; err != nil {
		return fmt.Errorf("failed to release tag usage counts: %w", err)
	}
	return nil
}

// logDeletedEntityRefs records the rows a delete destroyed.
//
// Same reasoning as logDroppedEntityRefs on the merge side: these are hard
// deletes of user-authored rows — a crate item, a bookmark, a tag vote, a
// comment-thread subscription — in tables with no soft-delete column, and the
// delete endpoints return no body, so without this line nobody can answer "what
// did that deletion take with it?" afterwards.
//
// Written INSIDE the caller's transaction, so a delete that is rolled back
// afterwards leaves a line describing rows that were never actually destroyed.
// That is the same trade logDroppedEntityRefs makes, and it is the right way
// round: the alternative is threading counts back out of every transaction
// closure to log after commit, and a missing line about a deletion that DID
// happen is the more expensive of the two failures.
func logDeletedEntityRefs(
	entity polymorphicEntityType,
	entityID uint,
	affected map[string]int64,
	alertRows int64,
) {
	for table, count := range affected {
		if count == 0 {
			continue
		}
		slog.Default().Info("entity delete swept references",
			"entity_type", string(entity),
			"entity_id", entityID,
			"table", table,
			"rows_affected", count,
		)
	}
	if alertRows > 0 {
		slog.Default().Info("entity delete swept references",
			"entity_type", string(entity),
			"entity_id", entityID,
			"table", "notification_log",
			"rows_affected", alertRows,
			"note", "alert rows keyed on their own discriminator",
		)
	}
}
