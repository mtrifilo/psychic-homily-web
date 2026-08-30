package catalog

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	notificationm "psychic-homily-backend/internal/models/notification"
)

// This file holds the inventory of polymorphic (entity_type, entity_id) tables
// and the mechanism that moves them, shared by all three merges in this package
// so there are not three copies to drift apart.
//
// The tables carry NO foreign key to the entity, so a row left behind does not
// fail loudly: it silently points at an id that no longer exists.
//
// Every merge that deletes an entity now reads this list. MergeVenues (PSY-1745)
// came first, MergeArtists (PSY-1834) second, and MergeDuplicateShow (PSY-1869)
// last, which had been walking its own shorter list covering ten of the
// seventeen tables. A merge that does NOT read it is the drift this file exists
// to end.

// entityRef describes one polymorphic (entity_type, entity_id) reference table.
//
// Every field describes the TABLE, not the merge using it. That is the whole
// reason one inventory serves every entity type: a unique index does not change
// depending on which kind of entity a row points at, so letting each merge
// carry its own copy could only ever produce a wrong copy.
type entityRef struct {
	// table is the SQL table name. Interpolated into SQL, so it MUST stay a
	// hardcoded literal in the inventory below — never caller input.
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
	// dedupeWhen is the WHERE clause that makes the index PARTIAL, with %s
	// standing in for the row alias; empty when the index covers every row.
	//
	// It exists so the dedupe deletes exactly what the index would have
	// rejected and not one row more. Without it the dedupe is "stricter than
	// the index needs", which sounds safe and is not: over-deleting silently
	// destroys rows the database was perfectly willing to keep.
	dedupeWhen string
}

// polymorphicEntityRefs is every table in the schema with a polymorphic
// (entity_type, entity_id) reference, minus the three in refsRepointedElsewhere,
// with the unique key that constrains each re-point.
//
// Verified against the live schema rather than copied forward: the equivalent
// list in the PSY-1581 one-off migration is missing `requests` and
// `entity_requests` (both of which name their id column something other than
// entity_id) and marks `notification_log` as having no unique key when it in
// fact has one on (user_id, filter_id, entity_type, entity_id, channel).
//
// Tables whose CHECK constraint forbids a given entity_type are listed anyway —
// source_configs is venue/label only, image_enrich_queue is artist/release only.
// Their UPDATE matches zero rows for the merges that cannot appear in them, and
// one exhaustive list is cheaper to keep correct than a list plus a set of
// per-entity exceptions that has to be re-verified every time a CHECK changes.
//
// TestVenueEntityRefsCoverSchema, TestArtistEntityRefsCoverSchema and
// TestShowEntityRefsCoverSchema fail if a migration adds an entity_type table
// that is not listed here, so this cannot silently fall behind.
var polymorphicEntityRefs = []entityRef{
	// Unique on (entity_type, entity_id) plus the listed columns.
	{table: "collection_items", idCol: "entity_id", dedupe: true, key: []string{"collection_id"}},
	{table: "comment_last_read", idCol: "entity_id", dedupe: true, key: []string{"user_id"}},
	{table: "comment_subscriptions", idCol: "entity_id", dedupe: true, key: []string{"user_id"}},
	{table: "entity_tags", idCol: "entity_id", dedupe: true, key: []string{"tag_id"}},
	{table: "notification_log", idCol: "entity_id", dedupe: true, key: []string{"user_id", "filter_id", "channel"}},
	{table: "tag_votes", idCol: "entity_id", dedupe: true, key: []string{"tag_id", "user_id"}},
	{table: "user_bookmarks", idCol: "entity_id", dedupe: true, key: []string{"user_id", "action"}},

	// Unique on (entity_type, entity_id) alone. image_enrich_queue's index is
	// PARTIAL over the active statuses, so the dedupe is scoped to match: a
	// canonical entity whose job already finished must not cause the losing
	// entity's still-queued job to be thrown away.
	{
		table: "image_enrich_queue", idCol: "entity_id", dedupe: true,
		dedupeWhen: "%s.status IN ('pending', 'processing')",
	},
	{table: "source_configs", idCol: "entity_id", dedupe: true},

	// No unique key on the entity reference: re-point in place.
	{table: "audit_logs", idCol: "entity_id"},
	{table: "comments", idCol: "entity_id"},
	{table: "entity_reports", idCol: "entity_id"},
	{table: "requests", idCol: "requested_entity_id"},
	{table: "entity_requests", idCol: "created_entity_id"},
}

// refsRepointedElsewhere names entity_type tables the merges DO handle but not
// through the loop above, so the schema-coverage guards count them as covered
// without repointEntityRefs trying to re-point them a second time.
//
// Every table here is one whose move is inseparable from a provenance decision —
// see repointRevisions and repointEditHistory, and the per-merge steps that give
// each merge's answer.
var refsRepointedElsewhere = []string{
	"revisions",
	"pending_entity_edits",
	"entity_edit_audit_logs",
}

// repointEntityRefs walks refs, dropping rows that would collide on a unique key
// before re-pointing the rest at the canonical entity.
//
// Must run inside the merge's transaction, which is where the rest of the
// merge's atomicity comes from.
//
// Returns moved and dropped row counts keyed by table rather than one total,
// because callers report different slices of it and because a merge that
// DELETES a user's bookmark, crate item or tag vote should be able to say how
// many — same reasoning as repointEditHistory's separate dropped count.
//
// A self-merge is rejected: the dedupe's EXISTS correlation would match every
// row against itself and delete the surviving entity's rows before the no-op
// move ran.
//
// The three tables in refsRepointedElsewhere are rejected outright. They are not
// merely absent from the inventory — routing one through here would assemble its
// UPDATE with fmt.Sprintf, which is invisible to the source-scanning guards
// (TestNoRevisionRepointOutsideTheHelper, TestNoEditHistoryRepointOutsideTheHelper)
// that force a provenance decision. This check is the part of that fence a list
// edit cannot step around.
func repointEntityRefs(
	tx *gorm.DB,
	refs []entityRef,
	entity polymorphicEntityType,
	canonicalID, mergeFromID uint,
) (moved, dropped map[string]int64, err error) {
	if !entity.mergeable() {
		return nil, nil, fmt.Errorf("repoint entity refs: %q has no merge path", string(entity))
	}
	if canonicalID == 0 || mergeFromID == 0 {
		return nil, nil, fmt.Errorf("repoint entity refs: canonical and merge-from ids are required")
	}
	if canonicalID == mergeFromID {
		return nil, nil, fmt.Errorf(
			"repoint entity refs: cannot re-point %s %d onto itself", entity, canonicalID)
	}

	moved = make(map[string]int64, len(refs))
	dropped = make(map[string]int64, len(refs))
	for _, ref := range refs {
		if ref.table == "" || ref.idCol == "" {
			return nil, nil, fmt.Errorf("repoint entity refs: table and id column are required")
		}
		if provenanceGatedTable(ref.table) {
			return nil, nil, fmt.Errorf(
				"repoint entity refs: %s must move through repointRevisions or "+
					"repointEditHistory, which require a provenance decision", ref.table)
		}

		if ref.dedupe {
			joinPred := ""
			for _, col := range ref.key {
				joinPred += fmt.Sprintf(" AND w.%s = l.%s", col, col)
			}
			loserScope, winnerScope := "", ""
			if ref.dedupeWhen != "" {
				// #nosec G201 -- dedupeWhen is a hardcoded predicate from the
				// inventory above; only the row alias is substituted.
				loserScope = " AND " + fmt.Sprintf(ref.dedupeWhen, "l")
				// #nosec G201 -- see above.
				winnerScope = " AND " + fmt.Sprintf(ref.dedupeWhen, "w")
			}
			// #nosec G201 -- table/column names come from the hardcoded
			// inventory, never from caller input; the ids and the entity type
			// are bound parameters.
			del := fmt.Sprintf(`
				DELETE FROM %[1]s l
				WHERE l.entity_type = ?
				  AND l.%[2]s = ?%[4]s
				  AND EXISTS (
				        SELECT 1 FROM %[1]s w
				        WHERE w.entity_type = ?
				          AND w.%[2]s = ?%[3]s%[5]s
				      )
			`, ref.table, ref.idCol, joinPred, loserScope, winnerScope)
			r := tx.Exec(del, string(entity), mergeFromID, string(entity), canonicalID)
			if r.Error != nil {
				return nil, nil, fmt.Errorf("failed to drop conflicting %s rows: %w", ref.table, r.Error)
			}
			dropped[ref.table] = r.RowsAffected
		}

		// #nosec G201 -- see above.
		upd := fmt.Sprintf(
			"UPDATE %[1]s SET %[2]s = ? WHERE entity_type = ? AND %[2]s = ?",
			ref.table, ref.idCol,
		)
		r := tx.Exec(upd, canonicalID, string(entity), mergeFromID)
		if r.Error != nil {
			return nil, nil, fmt.Errorf("failed to move %s rows: %w", ref.table, r.Error)
		}
		moved[ref.table] = r.RowsAffected
	}
	return moved, dropped, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Follow-driven alert rows in notification_log (PSY-1896)
//
// These two functions exist because notification_log carries entity ids the
// inventory above CANNOT see, and a merge that misses them corrupts a user's
// inbox rather than failing.
//
// The inventory keys every re-point on `entity_type = 'show' | 'artist' |
// 'venue'`. An artist show-alert row has entity_type = 'artist_show_alert', so
// the loop's UPDATE matches none of them even though its entity_id IS a show id.
// And subject_entity_id is a second, non-polymorphic entity reference the
// inventory has no concept of at all: its column name is not entity_id and its
// type is implied by the discriminator.
//
// Neither column has a foreign key, so nothing raises when a merge leaves them
// behind. The row simply points at a deleted id, and the inbox renders it
// stripped of its title, link and artist name.
// ─────────────────────────────────────────────────────────────────────────────

// repointArtistShowAlertShows moves artist show-alert rows from a losing show
// onto the winner, dropping the ones the winner already has.
//
// The drop is required by uq_notification_log_artist_show_alert, the partial
// UNIQUE on (user_id, entity_id, channel). A user who was alerted about BOTH
// shows before they were found to be duplicates has a row for each, and moving
// the loser's onto the winner would violate it and abort the whole merge. The
// dropped row is the redundant half of a duplicate notification the user already
// received, so nothing they can see is lost.
func repointArtistShowAlertShows(tx *gorm.DB, winnerID, loserID uint) (moved, dropped int64, err error) {
	// Self-merge would make the correlated EXISTS match every row against itself
	// and the DELETE would wipe the show's alerts entirely. repointEntityRefs
	// rejects this earlier in every current call path, so this guard is about not
	// depending on call order: the check is free and the failure is destructive.
	if winnerID == 0 || loserID == 0 || winnerID == loserID {
		return 0, 0, fmt.Errorf(
			"repoint artist show alerts: winner and loser must be two distinct shows (got %d, %d)",
			winnerID, loserID)
	}

	del := tx.Exec(`
		DELETE FROM notification_log l
		WHERE l.entity_type = ?
		  AND l.entity_id = ?
		  AND EXISTS (
		        SELECT 1 FROM notification_log w
		        WHERE w.entity_type = l.entity_type
		          AND w.entity_id = ?
		          AND w.user_id = l.user_id
		          AND w.channel = l.channel
		      )
	`, notificationm.NotificationEntityArtistShowAlert, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, fmt.Errorf("failed to drop conflicting artist show-alert rows: %w", del.Error)
	}

	upd := tx.Exec(`
		UPDATE notification_log SET entity_id = ?
		WHERE entity_type = ? AND entity_id = ?
	`, winnerID, notificationm.NotificationEntityArtistShowAlert, loserID)
	if upd.Error != nil {
		return 0, 0, fmt.Errorf("failed to move artist show-alert rows: %w", upd.Error)
	}
	return upd.RowsAffected, del.RowsAffected, nil
}

// repointAlertSubjectEntity moves notification_log.subject_entity_id from a
// losing entity onto the winner, for the alert types whose subject is that kind
// of entity.
//
// A plain UPDATE with no dedupe, because subject_entity_id is in no unique
// index: it is the row's LABEL (which band the alert is about), not part of its
// identity. Two rows for one user that end up naming the same artist are two
// alerts about two different shows, which is correct.
func repointAlertSubjectEntity(tx *gorm.DB, entityTypes []string, canonicalID, mergeFromID uint) (int64, error) {
	if len(entityTypes) == 0 {
		return 0, nil
	}
	r := tx.Exec(`
		UPDATE notification_log SET subject_entity_id = ?
		WHERE subject_entity_id = ? AND entity_type IN ?
	`, canonicalID, mergeFromID, entityTypes)
	if r.Error != nil {
		return 0, fmt.Errorf("failed to move alert subject references: %w", r.Error)
	}
	return r.RowsAffected, nil
}

// artistSubjectAlertTypes are the notification_log entity types whose
// subject_entity_id holds an ARTIST id. A new artist-subject alert type must be
// added here or an artist merge will strand its rows' labels.
//
// Venue show alerts (PSY-1895) are deliberately absent, and NOT by oversight:
// their subject_entity_id is NULL. The followed entity and the row's subject are
// the same venue, so the venue lives in entity_id and there is no second
// reference to move. repointVenueShowAlertLogs is what handles them.
var artistSubjectAlertTypes = []string{
	notificationm.NotificationEntityArtistShowAlert,
}

// ─────────────────────────────────────────────────────────────────────────────
// Coalesced venue show alerts (PSY-1895)
//
// Three more references the inventory loop cannot see, for two different
// reasons:
//
//   - notification_log rows of entity_type 'venue_show_alert' key on their own
//     discriminator, so the loop's `entity_type = 'venue'` UPDATE misses them —
//     the same blind spot artist show alerts have. Their entity_id is a VENUE id
//     (a venue alert is coalesced over a day and has no single show to point
//     at), so a venue merge is what strands them.
//   - venue_show_alert_batch is not polymorphic at all. It carries two real
//     foreign keys, one to venues and one to shows, so BOTH a venue merge and a
//     show dedup have to move it, and its unique key is the whole natural key
//     (venue_id, alert_day, show_id) — two columns to dedupe on, which is one
//     more than the generic helpers take.
//
// Every one of these is MOVED rather than dropped, which is the opposite of
// show_notify_queue's disposition next door and worth being explicit about.
// That table's UNIQUE means "already considered for notification", so moving a
// row could manufacture an announcement. These rows mean nothing of the kind:
// what guarantees exactly-once here is uq_notification_log_venue_show_alert, and
// re-pointing a batch membership onto the canonical show or venue only corrects
// which entity an existing alert names.
// ─────────────────────────────────────────────────────────────────────────────

// repointVenueShowAlertLogs moves venue show-alert inbox rows from a losing
// venue onto the winner, dropping the ones the winner already has.
//
// The drop is required by uq_notification_log_venue_show_alert, the partial
// UNIQUE on (user_id, entity_id, alert_bucket, channel). A user who follows both
// halves of a duplicate pair and was alerted about each on the same day holds a
// row for each, and moving the loser's would violate it and abort the merge. The
// dropped row is the redundant half of a duplicate notification the user already
// received.
//
// alert_bucket is compared with IS NOT DISTINCT FROM rather than `=` so the
// dedupe matches what the INDEX matches. A NULL bucket is forbidden on these
// rows by a CHECK, so this should never differ in practice — but if it ever did,
// `=` would silently fail to detect the collision and the merge would abort with
// a unique violation instead of dropping the row.
func repointVenueShowAlertLogs(tx *gorm.DB, canonicalID, mergeFromID uint) (moved, dropped int64, err error) {
	if canonicalID == 0 || mergeFromID == 0 || canonicalID == mergeFromID {
		return 0, 0, fmt.Errorf(
			"repoint venue show alerts: winner and loser must be two distinct venues (got %d, %d)",
			canonicalID, mergeFromID)
	}

	del := tx.Exec(`
		DELETE FROM notification_log l
		WHERE l.entity_type = ?
		  AND l.entity_id = ?
		  AND EXISTS (
		        SELECT 1 FROM notification_log w
		        WHERE w.entity_type = l.entity_type
		          AND w.entity_id = ?
		          AND w.user_id = l.user_id
		          AND w.channel = l.channel
		          AND w.alert_bucket IS NOT DISTINCT FROM l.alert_bucket
		      )
	`, notificationm.NotificationEntityVenueShowAlert, mergeFromID, canonicalID)
	if del.Error != nil {
		return 0, 0, fmt.Errorf("failed to drop conflicting venue show-alert rows: %w", del.Error)
	}

	upd := tx.Exec(`
		UPDATE notification_log SET entity_id = ?
		WHERE entity_type = ? AND entity_id = ?
	`, canonicalID, notificationm.NotificationEntityVenueShowAlert, mergeFromID)
	if upd.Error != nil {
		return 0, 0, fmt.Errorf("failed to move venue show-alert rows: %w", upd.Error)
	}
	return upd.RowsAffected, del.RowsAffected, nil
}

// repointVenueShowAlertBatchVenues moves batch memberships from a losing venue
// onto the winner, dropping the ones that would collide on the winner.
//
// A collision here is the ordinary case for a real duplicate pair: the same show
// listed at both venues on the same day is exactly what makes them duplicates.
// The dropped row is redundant — the winner already records that show in that
// day's batch.
func repointVenueShowAlertBatchVenues(tx *gorm.DB, canonicalID, mergeFromID uint) (moved, dropped int64, err error) {
	return repointVenueShowAlertBatch(tx, "venue_id",
		[]string{"alert_day", "show_id"}, canonicalID, mergeFromID)
}

// repointVenueShowAlertBatchShows moves batch memberships from a losing show
// onto the winner, dropping the ones that would collide.
//
// Unlike show_notify_queue, which the same merge DROPS, this is a plain move.
// The row records that a show was part of a venue-day's alert, not that a show
// has been considered for notification, so re-pointing it cannot cause an
// announcement — it only makes an existing alert name the canonical show. What
// prevents a second delivery is uq_notification_log_venue_show_alert, which is
// untouched by any of this.
//
// A moved row arriving still-undispatched into a group that has already
// dispatched is CORRECT and expected: the next flush re-resolves that group, the
// delivery claim no-ops, and the show simply joins the inbox row. That is the
// same path a show announced late in the day takes.
func repointVenueShowAlertBatchShows(tx *gorm.DB, winnerID, loserID uint) (moved, dropped int64, err error) {
	return repointVenueShowAlertBatch(tx, "show_id",
		[]string{"venue_id", "alert_day"}, winnerID, loserID)
}

// repointVenueShowAlertBatch is the shared body of the two functions above:
// drop the losing rows that would collide on the rest of the natural key, then
// move the remainder.
//
// idCol and keyCols are hardcoded at both call sites and never reach here from
// caller input, which is what makes the interpolation below safe.
func repointVenueShowAlertBatch(
	tx *gorm.DB,
	idCol string,
	keyCols []string,
	canonicalID, mergeFromID uint,
) (moved, dropped int64, err error) {
	// A self-merge would correlate every row against itself, so the DELETE would
	// empty the surviving entity's batches before the no-op move ran. Every
	// current call path rejects it earlier; the check is free and the failure is
	// destructive.
	if canonicalID == 0 || mergeFromID == 0 || canonicalID == mergeFromID {
		return 0, 0, fmt.Errorf(
			"repoint venue show alert batch: canonical and merge-from must be two distinct ids (got %d, %d)",
			canonicalID, mergeFromID)
	}

	joinPred := ""
	for _, col := range keyCols {
		joinPred += fmt.Sprintf(" AND w.%[1]s = l.%[1]s", col)
	}

	// #nosec G201 -- idCol and keyCols are hardcoded at the two call sites in
	// this file, never caller input; the ids are bound parameters.
	delSQL := fmt.Sprintf(`
		DELETE FROM venue_show_alert_batch l
		WHERE l.%[1]s = ?
		  AND EXISTS (
		        SELECT 1 FROM venue_show_alert_batch w
		        WHERE w.%[1]s = ?%[2]s
		      )
	`, idCol, joinPred)
	del := tx.Exec(delSQL, mergeFromID, canonicalID)
	if del.Error != nil {
		return 0, 0, fmt.Errorf("failed to drop conflicting venue_show_alert_batch rows: %w", del.Error)
	}

	// #nosec G201 -- see above.
	updSQL := fmt.Sprintf(
		"UPDATE venue_show_alert_batch SET %[1]s = ? WHERE %[1]s = ?", idCol)
	upd := tx.Exec(updSQL, canonicalID, mergeFromID)
	if upd.Error != nil {
		return 0, 0, fmt.Errorf("failed to move venue_show_alert_batch rows: %w", upd.Error)
	}
	return upd.RowsAffected, del.RowsAffected, nil
}

// logDroppedEntityRefs records the rows a merge DELETED rather than moved.
//
// These are hard deletes of user-authored rows — a crate item, a bookmark, a tag
// vote, a comment-thread subscription — in tables with no soft-delete column, so
// without this line nobody can answer "what did that merge destroy?" afterwards.
// The merge summary types have no field for it, and adding one is an API-contract
// change; a log entry is the part that can land with the fix itself.
func logDroppedEntityRefs(entity polymorphicEntityType, canonicalID, mergeFromID uint, dropped map[string]int64) {
	for table, count := range dropped {
		if count == 0 {
			continue
		}
		slog.Default().Info("merge dropped duplicate entity references",
			"entity_type", string(entity),
			"canonical_id", canonicalID,
			"merged_id", mergeFromID,
			"table", table,
			"rows_deleted", count,
		)
	}
}

// provenanceGatedTable reports whether a table may only be re-pointed through
// the helpers that demand a provenance decision.
func provenanceGatedTable(table string) bool {
	for _, gated := range refsRepointedElsewhere {
		if table == gated {
			return true
		}
	}
	return false
}

// movedCount reads one table's moved-row count, failing loudly when the table is
// absent from the inventory.
//
// A bare map lookup would return zero, which is indistinguishable from "the
// table had no rows" — so a merge whose summary field silently reported 0
// forever after a table rename would pass every test in this package.
func movedCount(moved map[string]int64, table string) (int64, error) {
	count, ok := moved[table]
	if !ok {
		return 0, fmt.Errorf(
			"merge summary wants %s, which is not in the entity-ref inventory", table)
	}
	return count, nil
}

// unhandledEntityRefTables names the schema's entity_type tables that no merge
// handles: absent from polymorphicEntityRefs AND from refsRepointedElsewhere.
//
// Split out of the suite guards that call it so the guard's own logic can be
// exercised without a database, against a deliberately incomplete inventory
// (TestUnhandledEntityRefTablesNamesTheGap). A drift guard nobody has ever
// watched fail is a guard nobody knows works.
func unhandledEntityRefTables(schemaTables []string, covered map[string]bool) []string {
	var out []string
	for _, table := range schemaTables {
		if !covered[table] {
			out = append(out, table)
		}
	}
	return out
}

// entityRefTables is every entity_type table the merges handle, for the
// schema-drift tests: the ones re-pointed by the loop, plus the ones re-pointed
// by a dedicated step.
func entityRefTables() map[string]bool {
	out := make(map[string]bool, len(polymorphicEntityRefs)+len(refsRepointedElsewhere))
	for _, ref := range polymorphicEntityRefs {
		out[ref.table] = true
	}
	for _, table := range refsRepointedElsewhere {
		out[table] = true
	}
	return out
}
