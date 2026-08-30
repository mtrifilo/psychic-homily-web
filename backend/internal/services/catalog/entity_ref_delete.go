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
// check, with at most one hand-picked table swept beside it. PSY-1868 reports a
// production audit finding orphaned audit_logs rows attributable to no merge,
// with deletion as the leading suspect.
//
// A delete cannot re-point anything — there is no surviving entity — so each
// table needs a DIFFERENT kind of answer than the merges give it, and the answer
// is a judgement call about that table rather than about this operation. So it is
// recorded per table, once, in entityRefDeleteDispositions below, and
// deleteEntityRefs REFUSES a table with no recorded disposition rather than
// picking a default. TestEveryInventoriedRefHasADeleteDisposition and the seeding
// sweep in artist_delete_refs_test.go are what keep that list from falling behind
// the inventory.
//
// WHAT THIS FILE DOES NOT COVER. The merges enumerate THREE classes of reference
// (see artist_merge.go); this sweep handles the polymorphic class plus the two
// notification_log columns the inventory cannot see. The other two classes are
// left to the database and are called out here so the omission is recorded
// rather than implied:
//
//   - Real foreign keys to the entity. Ten of them CASCADE and a few SET NULL,
//     so the database does clean up, but silently: radio_plays.artist_id is
//     ON DELETE SET NULL, which leaves a play still marked match_state='matched'
//     with no artist, and scopePlaysForArtistRematch will never revisit it.
//     A merge rescues those rows (reassignArtistFKRefs); a delete cannot, since
//     there is nothing to rescue them onto. They also never appear in
//     logDeletedEntityRefs, so that log is a record of what the SWEEP removed,
//     not of everything the deletion took.
//   - Bare id columns with no foreign key and no discriminator. Chiefly
//     notification_filters.artist_ids / venue_ids / label_ids (bigint[]). The
//     merges rewrite these with array_replace; a delete would have to
//     array_remove, and removing the last id leaves an EMPTY array whose
//     matching semantics are a product question rather than a cleanup, so it is
//     deliberately not guessed at here. A dead id in a filter matches nothing,
//     which is inert; an empty filter might match everything, which is not.

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
)

// There is deliberately no third disposition that NULLs the entity column.
//
// It was written and removed. `requests.requested_entity_id` is nullable and
// looked like the obvious candidate: NULL means "no entity stands behind this
// request", which is true after a delete. But NULLing it alone manufactures a
// state no other code path produces. RejectFulfillment
// (services/community/request.go) clears that column as one of THREE writes,
// alongside status back to 'pending' and fulfiller_id to NULL; writing only the
// pointer leaves a row that still says fulfilled, with a fulfiller and a
// fulfilled_at, and no entity. ListRequests filters on status, so that row is
// invisible in the open bucket, can never be re-fulfilled, and still counts as
// fulfilled in the analytics the clearing was supposed to protect.
//
// The column is also dual purpose: CreateRequest sets it on a PENDING request as
// an optional link to a related entity, so the same statement would strip a
// requester's own link rather than a stale fulfillment pointer.
//
// Re-opening a fulfilled request is a workflow decision, not an orphan fix, so
// `requests` is kept untouched instead. The dangling id is inert: ResolveEntityRef
// returns (nil, nil) for a row that is gone, and the UI simply omits the link.

// String makes a rejected disposition readable in the error rather than printing
// an integer the reader has to count out against this file.
func (d refDeleteDisposition) String() string {
	switch d {
	case dropRefRows:
		return "dropRefRows"
	case keepRefRowsAsTombstone:
		return "keepRefRowsAsTombstone"
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
	// a bell entry linking to an entity that 404s, and its other job (proving an
	// alert was already sent, so it is not sent twice) is moot once there is
	// nothing left to alert about.
	//
	// Be precise about what the generic pass reaches here, because it differs by
	// entity type and only one of the six is interesting.
	//
	// This table's entity_type holds its own discriminators
	// (models/notification/notification_filter.go), and exactly one of them is
	// also a catalog entity name: 'show'. The show-filter matcher and the
	// scene-follow fanout both write entity_type='show' with a SHOW id in
	// entity_id, so a show delete really does drop those inbox rows through this
	// entry. For artist, venue, release, label and festival the generic pass
	// matches nothing, because no writer stamps those names here.
	//
	// The discriminator-keyed remainder ('artist_show_alert', 'venue_show_alert')
	// is invisible to this entry and is handled by deleteAlertRowsNamingEntity.
	"notification_log": dropRefRows,

	// ── Kept as tombstones: a record of something a person did, read on an axis
	// the deleted entity does not gate. ──

	// The admin action trail. GetAuditLogs (services/admin/audit_log.go) is a
	// global, time-ordered, filterable list with no entity_id filter at all, and
	// contributor stats read it by actor, so these rows stay readable and
	// meaningful after the entity is gone. They record who created the entity, who
	// edited it and who merged it; dropping them with the entity would erase that
	// from the trail of every actor involved.
	//
	// The orphaned audit_logs rows the 2026-08-19 audit found are therefore the
	// INTENDED state, and what was missing was that nine other tables were
	// orphaned alongside them with nothing recording which was which.
	//
	// NOT claimed: that the trail records the deletion itself. It does not.
	// DeleteArtistHandler writes no audit row (it logs to slog only), unlike the
	// create, alias and merge handlers next to it. So a deletion is attributable
	// to nobody in this table, which is a gap in the audit trail rather than
	// something this sweep introduces or can fix.
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

	// A post on the community wishlist, carrying the requester's title,
	// description and the votes it drew, plus counters in the leaderboard's
	// `requests` dimension and the admin fulfillment-rate chart. Dropping it would
	// destroy a user's post; clearing its pointer was tried and rejected for the
	// reasons recorded above the disposition constants.
	"requests": keepRefRowsAsTombstone,
}

// refsRepointedElsewhereIDCol is the entity-id column for the three tables that
// are named in refsRepointedElsewhere rather than carried in
// polymorphicEntityRefs.
//
// All three use entity_id. It is a named constant rather than a literal at the
// loop below so that a table joining that list with a different column name
// cannot be swept against the wrong column in silence.
const refsRepointedElsewhereIDCol = "entity_id"

// entityRefsWalkedOnDelete is the whole inventory as one list of (table, id
// column) pairs: polymorphicEntityRefs, then the refsRepointedElsewhere tables,
// which all use refsRepointedElsewhereIDCol.
//
// It exists so the sweep and the inertness gate in FRONT of the sweep
// (entity_delete_gate.go) cannot walk different lists. The gate's whole job is
// to answer "would this delete destroy anything of somebody else's?", and a
// table the gate does not know about is a table it silently answers "no" for.
// One function, two callers, so a new inventory entry reaches both or neither.
//
// The returned entries are copies of the inventory's, and callers must treat
// them as read-only: table and idCol are interpolated into SQL.
func entityRefsWalkedOnDelete() []entityRef {
	out := make([]entityRef, 0, len(polymorphicEntityRefs)+len(refsRepointedElsewhere))
	out = append(out, polymorphicEntityRefs...)
	for _, table := range refsRepointedElsewhere {
		out = append(out, entityRef{table: table, idCol: refsRepointedElsewhereIDCol})
	}
	return out
}

// COST, measured rather than assumed. FOUR of the nine dropped tables have no
// index supporting (entity_type, entity_id), so their DELETE is a sequential
// scan:
//
//   - notification_log — every index leads with user_id.
//   - tag_votes — the PK leads with tag_id.
//   - comment_last_read — the PK leads with user_id.
//   - image_enrich_queue — its (entity_type, entity_id) UNIQUE is PARTIAL over
//     status IN ('pending','processing'), and the sweep's DELETE carries no
//     status predicate, so the planner cannot use it. Easy to miscount as
//     covered, which is why it is spelled out.
//
// The other five are covered, source_configs included (a full UNIQUE).
//
// Left as-is deliberately. Adding three indexes is a migration, and a migration
// merged out of order is how stage died on 2026-08-02; it also belongs with a
// measurement of these tables' real size rather than with a correctness fix. It
// is recorded here instead of discovered later, and it matters more than it
// looks: DELETE /artists/{id} is reachable by any authenticated user, so this is
// scan work an unprivileged caller can trigger, unlike the merges that issue the
// same statement shape from admin-only paths.
//
// A NON-ADMIN delete pays it roughly twice: the inertness gate in
// entity_delete_gate.go reads the same tables before the sweep writes them. Its
// reads are EXISTS rather than COUNT for exactly this reason, so the planner can
// stop at the first matching row instead of scanning to the end.

// deleteEntityRefs carries out every recorded disposition for one entity that is
// about to be deleted, and reports what it did per table.
//
// Must run inside the delete's transaction. The sweep and the delete are one
// unit: a sweep that commits without the delete strips a live entity of its
// bookmarks and tags, and a delete that commits without the sweep is the bug
// this function exists to fix.
//
// Returns rows affected per table for every table in the inventory, including
// zero for the ones deliberately left alone. The zeros are there so the map is
// exhaustive over the inventory and a caller can tell "kept, so no rows" apart
// from "table not in the inventory at all"; logDeletedEntityRefs deliberately
// prints only the non-zero half.
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

	refs := entityRefsWalkedOnDelete()
	affected := make(map[string]int64, len(refs))

	// The list includes the provenance-gated tables (refsRepointedElsewhere)
	// rather than excluding them the way repointEntityRefs does. The reason that
	// exclusion exists does not apply: it forces an author to state what happens
	// to a re-pointed row's READ-TIME REDACTION, and a delete re-points nothing.
	// All three are recorded as kept, so the loop issues no statement for them
	// today — but routing them through the same lookup is what makes a future
	// change to one of those dispositions impossible to forget, and what makes
	// them appear in the affected map rather than silently missing from it.
	//
	// entity_tags is held back to the END of the loop, and the tag counter is
	// released immediately before it, so the two run as an adjacent pair.
	//
	// Order matters for contention, not correctness. releaseTagUsageCounts takes
	// FOR UPDATE on the tag rows, and Postgres holds row locks until COMMIT, so
	// running it first would hold locks on the site's most-used tags across every
	// remaining statement in the sweep — several of which are sequential scans
	// (see the COST note above). AddTagToEntity and RemoveTagFromEntity then queue
	// behind an unrelated entity's deletion for EVERY entity carrying those tags.
	// Doing it last shrinks that window to two adjacent statements.
	var tagRef *entityRef
	for i, ref := range refs {
		if ref.table == "" || ref.idCol == "" {
			return nil, fmt.Errorf("delete entity refs: table and id column are required")
		}
		if ref.table == "entity_tags" {
			tagRef = &refs[i]
			continue
		}
		n, err := applyRefDeleteDisposition(tx, ref.table, ref.idCol, entity, entityID)
		if err != nil {
			return nil, err
		}
		affected[ref.table] = n
	}

	// Last, as a pair: give back the denormalized counter, then drop the rows it
	// was counting. entity_tags must still be in the inventory for the loop above
	// to have found it, so a rename cannot silently skip the counter release.
	if tagRef == nil {
		return nil, fmt.Errorf(
			"delete entity refs: entity_tags is not in the inventory, so its " +
				"tags.usage_count release would be skipped")
	}
	if err := releaseTagUsageCounts(tx, entity, entityID); err != nil {
		return nil, err
	}
	n, err := applyRefDeleteDisposition(tx, tagRef.table, tagRef.idCol, entity, entityID)
	if err != nil {
		return nil, err
	}
	affected[tagRef.table] = n

	return affected, nil
}

// applyRefDeleteDisposition runs one table's recorded decision.
//
// The table and the column are interpolated into SQL. What keeps that safe is
// the call graph rather than a runtime check: this function is unexported, has a
// single caller, and that caller passes `table` and `idCol` straight out of the
// hardcoded inventory. Nothing caller-controlled reaches the statement.
//
// An earlier version also validated the table/column PAIR against a map derived
// from that same inventory. It was removed rather than kept: the map was built
// by iterating exactly what the caller iterates, so the check compared a value
// against itself and could never fire. A guard that cannot fail is worse than no
// guard, because it stops the next reader from looking. The disposition lookup
// below DOES fail for an unknown table, and that one earns its place.
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

	default:
		return 0, fmt.Errorf(
			"delete entity refs: %s has disposition %s, which is not a decision; "+
				"record dropRefRows or keepRefRowsAsTombstone", table, disposition)
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
//
// A row found here is dropped because the entity in its entity_id IS the one
// going away, so the row announces something that no longer exists.
//
// It deliberately sweeps the entity axis ONLY. subject_entity_id is not swept,
// and that is the whole subtlety of this function: see alertTypesKeyedOnSubject.
func deleteAlertRowsNamingEntity(tx *gorm.DB, entity polymorphicEntityType, entityID uint) (int64, error) {
	if !entity.valid() {
		return 0, fmt.Errorf("delete alert rows: unknown entity type %q", string(entity))
	}
	if entityID == 0 {
		return 0, fmt.Errorf("delete alert rows: entity id is required")
	}

	types := alertTypesKeyedOnEntity[entity]
	if len(types) == 0 {
		return 0, nil
	}

	r := tx.Exec(
		"DELETE FROM notification_log WHERE entity_type IN ? AND entity_id = ?", types, entityID)
	if r.Error != nil {
		return 0, fmt.Errorf("failed to delete alert rows naming the entity: %w", r.Error)
	}
	return r.RowsAffected, nil
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

// There is deliberately NO sweep on notification_log.subject_entity_id, and this
// is the one place a delete must NOT copy what the merge path does.
//
// A merge re-points that column (repointAlertSubjectEntity) because the row
// survives either way. A delete has no id to re-point to, and the obvious move
// — delete the rows whose subject is the departing artist — is actively wrong on
// two counts:
//
//   - The row is keyed on the SHOW, not the artist. Its unique index is
//     uq_notification_log_artist_show_alert (user_id, entity_id, channel), and
//     claimArtistAlertRow (services/notification/artist_follow_notify.go) relies
//     on that row's existence via ON CONFLICT DO NOTHING as the exactly-once
//     guard. Deleting it re-arms a duplicate EMAIL for a show the user was
//     already mailed about, the next time that show's outbox row is reprocessed.
//   - subject_entity_id is only the row's LABEL: which followed artist the alert
//     is attributed to. resolveArtistAlertRecipients picks the first qualifying
//     follow in bill order, so one row can stand for a user's follow of several
//     artists on the same bill. Deleting it because ONE of them was deleted
//     takes away a bell entry for a show that still exists and that the user
//     still follows another act on.
//
// So the stale subject id is left in place. It is a label pointing at a deleted
// artist, and the read path already degrades a missing entity to a generic name
// rather than failing, which is a far smaller harm than a duplicate email plus a
// vanished inbox row.

// sweepEntityRefsForDelete is the one call every Delete* method makes: it runs
// the recorded dispositions plus the alert rows the inventory cannot see, and
// logs what it destroyed.
//
// Wrapped in one function rather than left as two calls per delete site so that
// a seventh delete path cannot pick up half of it, which is precisely how
// DeleteShow and DeleteRelease ended up sweeping user_bookmarks and nothing
// else.
func sweepEntityRefsForDelete(tx *gorm.DB, entity polymorphicEntityType, entityID uint) error {
	// releaseTagUsageCounts is called from inside deleteEntityRefs, immediately
	// before the entity_tags delete, rather than here: see the ordering note there.
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
// tag_service.go), and nothing recomputes it except MergeTags, which recounts
// only its merge target. So the generic `DELETE FROM entity_tags` in the sweep
// leaves the counter permanently overstated by however many tags the deleted
// entity carried.
//
// That counter is not decorative. GetLowQualityTagQueue
// (services/catalog/tag_low_quality.go) selects candidates on `usage_count = 0`
// and on `usage_count < N AND created_at < cutoff`, and stamps the
// "orphaned" / "aging unused" reasons from the same value, so an overstated
// counter keeps a tag whose last real use was deleted out of the moderation
// queue entirely. It also orders the tag hierarchy listing
// (tag_hierarchy.go) and the ListTags sort.
//
// Note this is a REVIEW queue an admin acts on, not an automatic reclaim;
// nothing in the repo deletes a tag on usage_count alone.
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

	// Lock the tag rows in ascending id order BEFORE updating them.
	//
	// The UPDATE below joins a HashAggregate, so the order it takes row locks in
	// is up to the planner. Two concurrent deletes of two entities sharing two or
	// more tags could take those locks in opposite orders and deadlock, which
	// Postgres resolves by aborting one of them: no corruption, but an
	// intermittent 500 on a user-facing endpoint. Ordering the lock acquisition
	// here removes the cycle, and it is the same ascending-id discipline
	// lockMergeArtists uses for the same reason.
	if err := tx.Exec(`
		SELECT 1 FROM tags
		 WHERE id IN (SELECT tag_id FROM entity_tags WHERE entity_type = ? AND entity_id = ?)
		 ORDER BY id
		   FOR UPDATE
	`, string(entity), entityID).Error; err != nil {
		return fmt.Errorf("failed to lock tags for usage-count release: %w", err)
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
		// Labelled distinctly rather than as a second bare "notification_log"
		// line, so the two records for one delete are separable by their table
		// field alone. Matches how show_dedup.go and venue_merge.go already label
		// their discriminator-keyed rows on the merge side.
		slog.Default().Info("entity delete swept references",
			"entity_type", string(entity),
			"entity_id", entityID,
			"table", "notification_log (alert discriminators)",
			"rows_affected", alertRows,
		)
	}
}
