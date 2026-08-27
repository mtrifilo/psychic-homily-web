package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// AuditActionMergeVenues is the action name recorded when an admin merges venues.
const AuditActionMergeVenues = "merge_venues"

// errPreviewRollback aborts the merge transaction after the work is done but
// before it commits. It never escapes mergeVenues.
//
// Preview and commit run the SAME code and differ only in whether this error is
// returned at the end. That is the point: the alternative — a second set of
// COUNT queries mirroring each mutation — is two implementations of one rule
// that drift apart the moment either side is edited, and the count that matters
// most here (which shows collide on shows_artist_venue_eventdate_uniq) is
// exactly the one whose definition must not drift from the mutation it guards.
var errPreviewRollback = errors.New("venue merge preview rollback")

// venueFKTables is every table holding a real foreign key to venues.id. Each
// one is re-pointed explicitly by the steps below.
//
// Keeping this list is not redundant with the database's own constraints: four
// of the five cascade on delete, so a table that this merge does NOT handle
// would not fail loudly when the losing venue is deleted — its rows would
// silently disappear. venue_confirmations is exactly that case, and the
// PSY-1581 one-off migration predates it.
//
// TestVenueForeignKeysAreAllHandled fails if a migration adds another.
var venueFKTables = []string{
	"festival_artists",
	"festival_venues",
	"show_artists",
	"show_venues",
	"venue_confirmations",
	// venue_show_alert_batch (PSY-1895) cascades too, and losing it is quiet in
	// a particular way: the alerts already delivered for the losing venue keep
	// their inbox rows, but those rows render their show list from THIS table at
	// read time, so the cascade would leave a user looking at "3 new shows" above
	// an empty list. Re-pointed by repointVenueShowAlertBatchVenues.
	"venue_show_alert_batch",
}

// PreviewMergeVenues reports what MergeVenues(canonicalID, mergeFromID) would
// change, without committing any of it.
func (s *VenueService) PreviewMergeVenues(canonicalID, mergeFromID uint) (*contracts.MergeVenueResult, error) {
	return s.mergeVenues(canonicalID, mergeFromID, true)
}

// MergeVenues folds mergeFromID into canonicalID: every reference to the losing
// venue is re-pointed at the canonical one, shows that would collide are
// deduplicated first, and the losing venue row is deleted.
//
// The whole merge is one transaction — a failure at any step leaves the
// database exactly as it was, never half-merged.
//
// Field data is NOT combined: the canonical venue keeps its own name, address
// and links untouched. Choosing which record's details survive is a judgment
// call, so it stays with the admin, who picks the canonical id.
//
// PRIVACY: an unverified venue's address history, and the contributor-authored
// summary prose beside it, are withheld from the anonymous revision routes, and
// this merge deletes the venue that gate reads.
// reassignVenueRevisions preserves the redaction across the re-point;
// stored history is not edited and verified-into-verified merges are
// unaffected. Merges predating that column are not backfilled.
//
// The marker carries BOTH halves without knowing about either: it routes the row
// to redactVenueRevision, which masks the private fields and drops the summary,
// so nothing here has to be taught about prose.
func (s *VenueService) MergeVenues(canonicalID, mergeFromID, actorUserID uint) (*contracts.MergeVenueResult, error) {
	result, err := s.mergeVenues(canonicalID, mergeFromID, false)
	if err != nil {
		return nil, err
	}

	// Fire-and-forget audit log after the transaction commits, matching
	// MergeTags. Errors are logged, never surfaced to the caller.
	shared.GoSafe(context.Background(), "venue_merge_audit_log", func() {
		s.writeMergeAuditLog(actorUserID, result)
	})

	return result, nil
}

// mergeVenues is the single implementation behind both the preview and the
// commit. When preview is true the transaction is rolled back after every
// mutation has run and every count has been taken.
//
// A preview therefore costs the same work as a real merge and holds the same
// row locks for its duration. That is a deliberate trade: this is an admin-only
// path used a handful of times, and paying it buys a preview that cannot
// disagree with the merge it previews.
//
// NOTE: result is accumulated in place inside the closure (EntityRefsMoved sums
// across tables). The closure is therefore NOT safe to retry — wrapping this in
// a retry loop would double the counts. GORM's Transaction does not retry.
func (s *VenueService) mergeVenues(canonicalID, mergeFromID uint, preview bool) (*contracts.MergeVenueResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if canonicalID == 0 || mergeFromID == 0 {
		return nil, apperrors.ErrVenueMergeInvalid("canonical_venue_id and merge_from_venue_id are required")
	}
	if canonicalID == mergeFromID {
		return nil, apperrors.ErrVenueMergeInvalid("cannot merge a venue into itself")
	}

	result := &contracts.MergeVenueResult{
		CanonicalVenueID: canonicalID,
		MergedVenueID:    mergeFromID,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		canonical, mergeFrom, err := lockMergeVenues(tx, canonicalID, mergeFromID)
		if err != nil {
			return err
		}
		result.CanonicalVenueName = canonical.Name
		result.MergedVenueName = mergeFrom.Name

		if err := buildDuplicateShowMap(tx, canonicalID, mergeFromID); err != nil {
			return err
		}
		if err := tx.Raw("SELECT COUNT(*) FROM venue_merge_dup").Scan(&result.DuplicateShows).Error; err != nil {
			return fmt.Errorf("failed to count duplicate shows: %w", err)
		}

		if err := rescueSupportActs(tx, canonicalID, result); err != nil {
			return err
		}
		if err := deleteDuplicateShows(tx); err != nil {
			return err
		}
		if err := reassignShows(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}
		if err := reassignFestivals(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}
		if err := reassignConfirmations(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}
		if err := reassignNotificationFilters(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}
		if err := reassignVenueRevisions(tx, canonicalID, mergeFrom, result); err != nil {
			return err
		}
		if err := reassignVenueEditHistory(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}
		if err := reassignEntityRefs(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}
		if err := reassignVenueAlertRows(tx, canonicalID, mergeFromID, result); err != nil {
			return err
		}

		if err := tx.Delete(&catalogm.Venue{}, mergeFromID).Error; err != nil {
			return fmt.Errorf("failed to delete merged venue: %w", err)
		}

		if preview {
			return errPreviewRollback
		}
		return nil
	})

	// A preview rolled back on purpose; everything it measured is still valid.
	if errors.Is(err, errPreviewRollback) {
		return result, nil
	}
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) {
			return nil, err
		}
		return nil, fmt.Errorf("venue merge failed: %w", err)
	}

	return result, nil
}

// lockMergeVenues loads both venues under a row lock so two concurrent merges
// touching the same venue serialize instead of interleaving.
//
// The rows are locked in ascending id order regardless of which one is the
// canonical venue: two admins merging the same pair in OPPOSITE directions
// (A→B and B→A) would otherwise grab them in opposite orders and deadlock.
//
// Two explicit statements rather than one `ORDER BY id FOR UPDATE`, because
// that would leave the ordering guarantee up to the query planner — it happens
// to put LockRows above Sort today, but nothing in the contract says it must.
// Here the order is in the code, where it can be read and relied on.
func lockMergeVenues(tx *gorm.DB, canonicalID, mergeFromID uint) (*catalogm.Venue, *catalogm.Venue, error) {
	first, second := canonicalID, mergeFromID
	if second < first {
		first, second = second, first
	}

	loaded := make(map[uint]*catalogm.Venue, 2)
	for _, id := range []uint{first, second} {
		var v catalogm.Venue
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperrors.ErrVenueNotFound(id)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to lock venue %d: %w", id, err)
		}
		loaded[id] = &v
	}

	return loaded[canonicalID], loaded[mergeFromID], nil
}

// buildDuplicateShowMap materializes, for each show on the losing venue, the
// show on the canonical venue that it duplicates.
//
// "Duplicate" is defined to match shows_artist_venue_eventdate_uniq exactly — a
// losing show collides if any of its (artist_id, event_date) pairs already
// exists on the canonical venue. That is precisely the condition that would
// make the venue_id reassignment below violate the index, so the mapping and
// the constraint cannot drift apart.
//
// DISTINCT ON picks one winner deterministically when the canonical venue has
// more than one show on the same date.
//
// The `wsa.show_id <> lsa.show_id` guard is belt-and-braces against the one
// catastrophic outcome available here: a show mapped as its own duplicate would
// be DELETED as the loser while also being treated as the survivor. The
// show_artists primary key (show_id, artist_id) already makes that impossible —
// one row cannot carry both venue ids — but the reasoning is subtle enough, and
// the failure destructive enough, that the invariant is stated in the query
// rather than left to be re-derived.
//
// ON COMMIT DROP ties the temp table's lifetime to this transaction, so it
// cannot leak onto the pooled connection whether the merge commits or a preview
// rolls back.
func buildDuplicateShowMap(tx *gorm.DB, canonicalID, mergeFromID uint) error {
	err := tx.Exec(`
		CREATE TEMP TABLE venue_merge_dup ON COMMIT DROP AS
		SELECT DISTINCT ON (lsa.show_id)
		       lsa.show_id AS loser_show,
		       wsa.show_id AS winner_show
		FROM show_artists lsa
		JOIN show_artists wsa
		  ON wsa.artist_id  = lsa.artist_id
		 AND wsa.event_date = lsa.event_date
		 AND wsa.venue_id   = ?
		 AND wsa.show_id   <> lsa.show_id
		WHERE lsa.venue_id = ?
		  AND lsa.event_date IS NOT NULL
		ORDER BY lsa.show_id, wsa.show_id
	`, canonicalID, mergeFromID).Error
	if err != nil {
		return fmt.Errorf("failed to build duplicate show map: %w", err)
	}
	return nil
}

// rescueSupportActs moves bill entries off each doomed duplicate show onto the
// show that survives it, BEFORE the duplicate is deleted.
//
// A duplicate pair is not always a clean copy — some duplicates list an artist
// the surviving show does not. Deleting the losing show outright would drop
// those associations silently (show_artists cascades on show_id), so any
// non-colliding artist moves across first.
//
// Both guards are load-bearing: the first keeps the move from violating
// shows_artist_venue_eventdate_uniq, the second from violating the
// (show_id, artist_id) primary key. Position is offset rather than reused so
// rescued acts append after the surviving bill instead of competing with it for
// ordering.
func rescueSupportActs(tx *gorm.DB, canonicalID uint, result *contracts.MergeVenueResult) error {
	r := tx.Exec(`
		UPDATE show_artists sa
		SET show_id  = d.winner_show,
		    venue_id = ?,
		    position = 100 + sa.position
		FROM venue_merge_dup d
		WHERE sa.show_id = d.loser_show
		  AND NOT EXISTS (
		        SELECT 1 FROM show_artists w
		        WHERE w.artist_id  = sa.artist_id
		          AND w.venue_id   = ?
		          AND w.event_date = sa.event_date
		      )
		  AND NOT EXISTS (
		        SELECT 1 FROM show_artists w2
		        WHERE w2.show_id   = d.winner_show
		          AND w2.artist_id = sa.artist_id
		      )
	`, canonicalID, canonicalID)
	if r.Error != nil {
		return fmt.Errorf("failed to rescue support acts: %w", r.Error)
	}
	result.SupportActsRescued = r.RowsAffected
	return nil
}

// deleteDuplicateShows removes the losing shows the map identified.
// show_artists and show_venues both cascade on show_id, so their rows go too —
// which is exactly why rescueSupportActs has to run first.
func deleteDuplicateShows(tx *gorm.DB) error {
	// venue_show_alert_batch memberships have to move off the losing shows FIRST.
	//
	// This delete does NOT route through MergeDuplicateShow, so none of that
	// function's re-points run for these shows — and venue_show_alert_batch's
	// foreign key CASCADES, so the memberships are destroyed here rather than
	// stranded. reassignVenueAlertRows runs later and would find nothing left to
	// move, which is exactly the outcome venueFKTables claims this table was
	// added to prevent: a delivered alert still saying "3 new shows" above a
	// list that lost one.
	//
	// Same delete-then-move shape as repointVenueShowAlertBatchShows, against the
	// same natural key. The drop is required because one event listed at both
	// venues on the same day is the ordinary case for a real duplicate pair, and
	// the winner will already hold that row.
	if err := tx.Exec(`
		DELETE FROM venue_show_alert_batch l
		USING venue_merge_dup d
		WHERE l.show_id = d.loser_show
		  AND EXISTS (
		        SELECT 1 FROM venue_show_alert_batch w
		        WHERE w.show_id = d.winner_show
		          AND w.venue_id = l.venue_id
		          AND w.alert_day = l.alert_day
		      )
	`).Error; err != nil {
		return fmt.Errorf("failed to drop conflicting venue alert batch rows: %w", err)
	}
	if err := tx.Exec(`
		UPDATE venue_show_alert_batch l
		   SET show_id = d.winner_show
		  FROM venue_merge_dup d
		 WHERE l.show_id = d.loser_show
	`).Error; err != nil {
		return fmt.Errorf("failed to move venue alert batch rows: %w", err)
	}

	if err := tx.Exec(`
		DELETE FROM shows s
		USING venue_merge_dup d
		WHERE s.id = d.loser_show
	`).Error; err != nil {
		return fmt.Errorf("failed to delete duplicate shows: %w", err)
	}
	return nil
}

// reassignShows re-points the surviving (non-duplicate) shows at the canonical
// venue. The show_venues delete covers the case where one show was somehow
// attached to BOTH venue records, which would collide on the
// (show_id, venue_id) primary key.
func reassignShows(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	if err := tx.Exec(`
		DELETE FROM show_venues sv
		WHERE sv.venue_id = ?
		  AND EXISTS (
		        SELECT 1 FROM show_venues w
		        WHERE w.show_id = sv.show_id AND w.venue_id = ?
		      )
	`, mergeFromID, canonicalID).Error; err != nil {
		return fmt.Errorf("failed to drop conflicting show_venues: %w", err)
	}

	r := tx.Exec("UPDATE show_venues SET venue_id = ? WHERE venue_id = ?", canonicalID, mergeFromID)
	if r.Error != nil {
		return fmt.Errorf("failed to move show_venues: %w", r.Error)
	}
	result.ShowVenuesMoved = r.RowsAffected

	r = tx.Exec("UPDATE show_artists SET venue_id = ? WHERE venue_id = ?", canonicalID, mergeFromID)
	if r.Error != nil {
		return fmt.Errorf("failed to move show_artists: %w", r.Error)
	}
	result.ShowArtistsMoved = r.RowsAffected
	return nil
}

// reassignFestivals moves festival references. festival_venues is unique on
// (festival_id, venue_id); festival_artists.venue_id is a plain nullable FK
// with no uniqueness, so it just takes the update.
func reassignFestivals(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	if err := tx.Exec(`
		DELETE FROM festival_venues fv
		WHERE fv.venue_id = ?
		  AND EXISTS (
		        SELECT 1 FROM festival_venues w
		        WHERE w.festival_id = fv.festival_id AND w.venue_id = ?
		      )
	`, mergeFromID, canonicalID).Error; err != nil {
		return fmt.Errorf("failed to drop conflicting festival_venues: %w", err)
	}

	r := tx.Exec("UPDATE festival_venues SET venue_id = ? WHERE venue_id = ?", canonicalID, mergeFromID)
	if r.Error != nil {
		return fmt.Errorf("failed to move festival_venues: %w", r.Error)
	}
	result.FestivalVenuesMoved = r.RowsAffected

	r = tx.Exec("UPDATE festival_artists SET venue_id = ? WHERE venue_id = ?", canonicalID, mergeFromID)
	if r.Error != nil {
		return fmt.Errorf("failed to move festival_artists: %w", r.Error)
	}
	result.FestivalArtistsMoved = r.RowsAffected
	return nil
}

// reassignConfirmations moves venue_confirmations, which is keyed
// (user_id, venue_id) — a user who vouched for BOTH records keeps one row.
//
// This table has ON DELETE CASCADE, so skipping it would not orphan anything:
// it would silently DESTROY the losing venue's confirmations when the venue row
// goes. Those are user contributions, so they move.
func reassignConfirmations(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	if err := tx.Exec(`
		DELETE FROM venue_confirmations l
		WHERE l.venue_id = ?
		  AND EXISTS (
		        SELECT 1 FROM venue_confirmations w
		        WHERE w.venue_id = ? AND w.user_id = l.user_id
		      )
	`, mergeFromID, canonicalID).Error; err != nil {
		return fmt.Errorf("failed to drop conflicting venue_confirmations: %w", err)
	}

	r := tx.Exec("UPDATE venue_confirmations SET venue_id = ? WHERE venue_id = ?", canonicalID, mergeFromID)
	if r.Error != nil {
		return fmt.Errorf("failed to move venue_confirmations: %w", r.Error)
	}
	result.ConfirmationsMoved = r.RowsAffected
	return nil
}

// reassignNotificationFilters rewrites notification_filters.venue_ids, a
// bigint[] with no foreign key — a stale id there would linger silently.
// Replace, then de-duplicate in case the filter already tracked the canonical
// venue.
func reassignNotificationFilters(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	r := tx.Exec(`
		UPDATE notification_filters nf
		SET venue_ids = ARRAY(
		      SELECT DISTINCT x
		      FROM unnest(array_replace(nf.venue_ids, ?::bigint, ?::bigint)) AS x
		      ORDER BY x
		    ),
		    updated_at = NOW()
		WHERE nf.venue_ids @> ARRAY[?::bigint]
	`, mergeFromID, canonicalID, mergeFromID)
	if r.Error != nil {
		return fmt.Errorf("failed to update notification filters: %w", r.Error)
	}
	result.FiltersUpdated = r.RowsAffected
	return nil
}

// reassignVenueRevisions moves the losing venue's revision history onto the
// canonical venue, stamping it when that venue was unverified so the read-time
// address redaction survives the re-point. See the privacy section of the
// revisiondiff package doc for the policy; repointRevisions is the mechanism.
//
// A verified loser gets noRedactionCarryover rather than a clear: nothing was
// being withheld, and any mark already on those rows came off an EARLIER
// unverified venue, so a chain of merges cannot launder an address that a
// single merge withholds.
//
// The whole venue is the parameter rather than an id plus a flag: both facts
// come off the row lockMergeVenues already locked, and splitting them would let
// a future edit pass one venue's id with another's verification state.
//
// The count joins EntityRefsMoved, which is what it counted toward when
// revisions were re-pointed inside the reassignEntityRefs loop.
//
// This does not scrub anything. The stored diff keeps the real address, which
// is what rollback and the moderation surfaces read; only the anonymous read
// path masks it.
//
// KNOWN BOUNDARY, admin-triggered: a marked row's value can still reach an
// anonymous reader through Rollback, which writes the stored address onto the
// venue the revision NOW points at, and records the rollback as a fresh
// (unmarked) revision carrying that value. Before the merge that was an
// unverified venue and the live payload withheld it; after it, the canonical
// venue is verified and publishes it, so the new revision is consistent with
// the address the venue now serves rather than a second leak. Keeping the
// stored value is what makes rollback possible at all, so this stays an
// explicit admin action rather than another gate.
func reassignVenueRevisions(tx *gorm.DB, canonicalID uint, mergeFrom *catalogm.Venue, result *contracts.MergeVenueResult) error {
	provenance := stampFromUnverifiedVenue
	if mergeFrom.Verified {
		provenance = noRedactionCarryover
	}

	moved, err := repointRevisions(tx, mergeEntityVenue, canonicalID, mergeFrom.ID, provenance)
	if err != nil {
		return err
	}
	result.EntityRefsMoved += moved
	return nil
}

// reassignVenueEditHistory moves the losing venue's contributor edit history
// onto the canonical venue.
//
// Both tables move with editHistoryCarriesNoRedaction, which for venues is the
// claim that most deserves checking, since venues are the entity that HAS a
// redaction gate. It holds because that gate is on the revision routes: edit
// content here reaches only its own submitter and admins, and the anonymous
// reader sees only venueEditCounts' aggregate. A count carries no address.
//
// Split out of reassignEntityRefs rather than left in the loop so the decision
// is made once, in the open, instead of being implied by a table's presence in
// a list. Both stay named in refsRepointedElsewhere so the schema-coverage
// guard still counts them as handled.
//
// The counts join EntityRefsMoved, which is what they counted toward inside the
// loop. Rows dropped as duplicates are not added, also matching the loop, which
// only ever counted the UPDATE.
func reassignVenueEditHistory(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	for _, table := range []editHistoryTable{pendingEditsHistory, entityEditAuditHistory} {
		moved, _, err := repointEditHistory(
			tx, table, mergeEntityVenue, canonicalID, mergeFromID, editHistoryCarriesNoRedaction)
		if err != nil {
			return err
		}
		result.EntityRefsMoved += moved
	}
	return nil
}

// reassignEntityRefs walks the shared polymorphic inventory, dropping rows that
// would collide on a unique key before moving the rest onto the canonical venue,
// and folds the per-table counts into the merge summary.
//
// Rows dropped as duplicates are logged rather than summed: EntityRefsMoved has
// always counted only the UPDATE, and a merge that deletes a user's bookmark or
// crate item should leave a trace even though the admin-facing summary has no
// field for it.
func reassignEntityRefs(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	moved, dropped, err := repointEntityRefs(
		tx, polymorphicEntityRefs, mergeEntityVenue, canonicalID, mergeFromID)
	if err != nil {
		return err
	}
	for _, count := range moved {
		result.EntityRefsMoved += count
	}
	logDroppedEntityRefs(mergeEntityVenue, canonicalID, mergeFromID, dropped)
	return nil
}

// reassignVenueAlertRows moves the two venue-alert references the steps above
// cannot see, and it has to run BEFORE the losing venue is deleted.
//
// Neither is reachable from the inventory loop, for different reasons:
//
//   - notification_log rows of entity_type 'venue_show_alert' hold a VENUE id in
//     entity_id, but the loop keys on entity_type = 'venue', so its UPDATE
//     matches none of them. Left behind they point at a deleted venue and the
//     user's inbox row loses its name and link.
//   - venue_show_alert_batch is a real foreign key, so it does not fail loudly
//     either — it CASCADES. The alert rows survive and render their show list
//     from that table at read time, so the cascade turns "3 new shows" into a
//     heading above nothing.
//
// Both counts fold into EntityRefsMoved rather than adding fields to the
// admin-facing result. They are entity references being moved, which is what
// that counter has always meant, and a merge summary is not the place a second
// pair of numbers earns its API surface. Drops are logged, matching every other
// destructive step here.
func reassignVenueAlertRows(tx *gorm.DB, canonicalID, mergeFromID uint, result *contracts.MergeVenueResult) error {
	logMoved, logDropped, err := repointVenueShowAlertLogs(tx, canonicalID, mergeFromID)
	if err != nil {
		return err
	}
	result.EntityRefsMoved += logMoved
	if logDropped > 0 {
		logDroppedEntityRefs(mergeEntityVenue, canonicalID, mergeFromID,
			map[string]int64{"notification_log (venue_show_alert)": logDropped})
	}

	batchMoved, batchDropped, err := repointVenueShowAlertBatchVenues(tx, canonicalID, mergeFromID)
	if err != nil {
		return err
	}
	result.EntityRefsMoved += batchMoved
	if batchDropped > 0 {
		logDroppedEntityRefs(mergeEntityVenue, canonicalID, mergeFromID,
			map[string]int64{"venue_show_alert_batch": batchDropped})
	}
	return nil
}

// writeMergeAuditLog records a venue merge in the audit log.
// Fire-and-forget — errors are logged but never fail the parent operation.
func (s *VenueService) writeMergeAuditLog(actorID uint, result *contracts.MergeVenueResult) {
	if s.db == nil {
		return
	}

	metadata := map[string]interface{}{
		"merged_venue_id":        result.MergedVenueID,
		"merged_venue_name":      result.MergedVenueName,
		"canonical_venue_name":   result.CanonicalVenueName,
		"duplicate_shows":        result.DuplicateShows,
		"support_acts_rescued":   result.SupportActsRescued,
		"show_venues_moved":      result.ShowVenuesMoved,
		"show_artists_moved":     result.ShowArtistsMoved,
		"festival_venues_moved":  result.FestivalVenuesMoved,
		"festival_artists_moved": result.FestivalArtistsMoved,
		"confirmations_moved":    result.ConfirmationsMoved,
		"filters_updated":        result.FiltersUpdated,
		"entity_refs_moved":      result.EntityRefsMoved,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		slog.Default().Error("failed to marshal venue merge audit metadata", "error", err)
		return
	}

	raw := json.RawMessage(metadataJSON)
	var actor *uint
	if actorID > 0 {
		actor = &actorID
	}

	auditLog := adminm.AuditLog{
		ActorID:    actor,
		Action:     AuditActionMergeVenues,
		EntityType: "venue",
		EntityID:   result.CanonicalVenueID,
		Metadata:   &raw,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.db.Create(&auditLog).Error; err != nil {
		slog.Default().Error("failed to write venue merge audit log", "error", err)
	}
}
