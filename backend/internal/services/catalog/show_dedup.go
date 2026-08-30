package catalog

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// This file is the show dedup's reference inventory: which tables point at a
// show, and what MergeDuplicateShow does with each.
//
// It exists because this merge kept its own hand-maintained list of re-points
// with nothing guarding it. That is the drift PSY-1745 closed for the venue
// merge and PSY-1834 for the artist merge; taking the inventory here found
// seven of the seventeen polymorphic entity_type tables unhandled.
//
// A show reference comes in the same three shapes the artist merge names, and
// each has a list plus a drift guard in show_dedup_refs_test.go that fails in CI
// the moment a migration adds something this merge does not handle:
//
//   - polymorphic (entity_type, entity_id): polymorphicEntityRefs in
//     entity_ref_repoint.go, shared with the venue and artist merges because a
//     unique index is a property of the table and not of the entity pointing at
//     it. Guard: TestShowEntityRefsCoverSchema. The keys those refs declare are
//     checked against the real pg_index rows by
//     TestArtistEntityRefDedupeKeysMatchTheSchema, which needs only the one copy
//     because the inventory it reads is entity-independent.
//   - a real foreign key to shows.id: showFKColumns, guarded by
//     TestShowForeignKeysAreAllHandled.
//   - a bare show id with neither: showUnconstrainedIDColumns, guarded by
//     TestShowIDColumnsWithoutForeignKeysAreAccountedFor.

// showFKColumns is every COLUMN holding a real foreign key to shows.id, spelled
// "table.column".
//
// Column-granular rather than table-granular, matching artistFKColumns: shows
// already references itself, so a guard keyed on table names alone would stay
// green when a migration added a second show column to a table already listed.
//
// Keeping this list is not redundant with the database's own constraints. It is
// the guard for the shape that fails most quietly: four of these five CASCADE
// and the fifth sets NULL, so a column this merge does NOT handle makes no noise
// when the losing show is deleted. Its rows silently disappear, or silently lose
// the show they pointed at.
//
// What each one gets, and why:
//
//   - show_venues.show_id, show_artists.show_id, show_reports.show_id are
//     re-pointed by moveShowFKRows, which first drops the loser's row when the
//     winner already holds one with the same partner column. (show_id, venue_id)
//     and (show_id, artist_id) are primary keys, and show_reports is UNIQUE
//     (show_id, reported_by).
//
//     show_reports moved with a BARE UPDATE until PSY-1869, which could not
//     survive its own index: one user who reported BOTH duplicates aborted the
//     entire cluster transaction with a unique violation. That is a diligent
//     user rather than an exotic one, since a duplicate cluster is one event
//     listed twice and every report type ('cancelled', 'sold_out',
//     'inaccurate') describes the EVENT, so the same defect is visible on both
//     rows. The winner's report wins, matching this function's stated conflict
//     policy.
//
//     show_artists ALSO carries shows_artist_venue_eventdate_uniq, UNIQUE
//     (artist_id, venue_id, event_date) over its denormalized columns, which the
//     artist merge has to dedupe against separately (dropCollidingShowArtists).
//     This merge does not, and cannot need to: it moves a row without touching
//     any of those three columns, so the only row that could collide with the
//     moved one is a row the index would already have rejected before the merge
//     ran. syncShowArtistDedupColumns then re-stamps the winner's rows, because
//     moved rows arrive carrying the loser's denorm.
//
//   - enrichment_queue.show_id takes a bare re-point. It has no unique index at
//     all, so one show legitimately holds several queued jobs.
//
//   - shows.duplicate_of_show_id is re-pointed, EXCLUDING the winner's own row.
//     Without that exclusion a winner already flagged as a duplicate of the
//     loser comes out of the merge flagged as a duplicate of ITSELF, and that
//     column is served to clients on the show payload.
//
//   - show_notify_queue.show_id is the one column here that is DROPPED rather
//     than re-pointed, and the reason is a correctness requirement of PSY-1894
//     rather than convenience. That table's UNIQUE (show_id) means "this show has
//     already been considered for follower notification", and its emptiness at
//     deploy is what guarantees the rollout never notified anyone about the
//     pre-existing catalogue. Re-pointing a loser's still-`pending` job onto a
//     winner that has no row would manufacture exactly the thing that guarantee
//     forbids: a merge, months later, announcing a show that predates the
//     feature. Nor could the row be re-pointed onto a winner that HAS one — the
//     UNIQUE would reject it. Dropping is therefore both the only shape that
//     fits the index and the only shape that preserves the no-backfill property.
//
//     Be honest about what that costs, because it is NOT always "a duplicate
//     notification suppressed". When the winner has no queue row of its own — the
//     realistic shape during and after rollout, where the winner is one of the
//     pre-existing shows that structurally can never be announced — the loser's
//     `pending` job was the ONLY notification the event would ever get, and this
//     drop destroys it. Same when the winner's row is `skipped` or `failed`. That
//     is a silent, user-visible loss, so each drop is logged per-show through
//     logDroppedEntityRefs rather than only counted in the summary: an aggregate
//     cannot tell an operator WHICH show went quiet.
//
//   - venue_show_alert_batch.show_id is MOVED, through its own function rather
//     than through moveShowFKRows, because its unique key is the whole natural
//     key (venue_id, alert_day, show_id) and the generic helper dedupes on ONE
//     partner column. See repointVenueShowAlertBatchShows.
//
//     Moved and not dropped, which is the opposite of its neighbour above and
//     the contrast worth holding on to: show_notify_queue's row MEANS "already
//     considered for notification", so moving one can manufacture an
//     announcement. This row means "this show was part of that venue-day's
//     alert". Exactly-once for venue alerts is held by
//     uq_notification_log_venue_show_alert instead, which no re-point here
//     touches, so moving the membership can only correct which show an existing
//     alert names.
//
// TestShowForeignKeysAreAllHandled fails if a migration adds another.
var showFKColumns = []string{
	"enrichment_queue.show_id",
	"show_artists.show_id",
	"show_notify_queue.show_id",
	"show_reports.show_id",
	"show_venues.show_id",
	"shows.duplicate_of_show_id",
	"venue_show_alert_batch.show_id",
}

// showFKColumnListed reports whether "table.column" carries a recorded
// disposition in showFKColumns.
func showFKColumnListed(qualified string) bool {
	for _, col := range showFKColumns {
		if col == qualified {
			return true
		}
	}
	return false
}

// showFKColumnsNeverRepointed are columns whose recorded disposition in
// showFKColumns is "DROP", not "move". moveShowFKRows refuses them.
//
// The two lists serve different masters, which is why this one exists.
// showFKColumns is the COMPLETENESS inventory — TestShowForeignKeysAreAllHandled
// asserts it equals the real set of foreign keys to shows.id, so every new column
// must appear there or CI fails. But moveShowFKRows uses membership in that same
// list as its permission check, so simply recording a column made re-pointing it a
// legal call. For show_notify_queue that is precisely the operation its own
// disposition argues at length must never happen: it would either abort the merge
// on UNIQUE (show_id), or move a `pending` job onto a winner and let a merge
// announce a show that predates the feature.
//
// A prohibition that lives only in a doc comment is one refactor away from being
// violated by someone doing the obvious thing (adding the table to the inventory
// loop). This makes it a runtime error instead.
//
// The two entries are here for DIFFERENT reasons, and both are "moveShowFKRows
// must not touch this", which is what the list actually enforces:
//
//   - show_notify_queue.show_id must not be MOVED AT ALL. Its disposition is
//     "drop"; re-pointing it would either abort on UNIQUE (show_id) or let a
//     merge announce a show that predates the feature.
//   - venue_show_alert_batch.show_id IS moved, just not by this helper. Its
//     unique key is the whole natural key (venue_id, alert_day, show_id) and
//     moveShowFKRows dedupes on exactly ONE partner column, so routing it
//     through here would miss half the key and abort the merge on a unique
//     violation. It moves through repointVenueShowAlertBatchShows instead.
var showFKColumnsNeverRepointed = []string{
	"show_notify_queue.show_id",
	"venue_show_alert_batch.show_id",
}

// showFKColumnRepointBanned reports whether "table.column" is recorded as
// drop-only.
func showFKColumnRepointBanned(qualified string) bool {
	for _, col := range showFKColumnsNeverRepointed {
		if col == qualified {
			return true
		}
	}
	return false
}

// showUnconstrainedIDColumns is the third class of show reference: a column
// holding show ids with no foreign key and no entity_type discriminator.
// Nothing in the database enforces it and nothing in the schema marks it, so the
// only way such a column stays handled is by being written down.
//
// EMPTY TODAY, which is exactly why it is asserted rather than left out: the
// artist merge found notification_filters.artist_ids hiding in this shape, and a
// show equivalent would be just as invisible to the other two guards.
// notification_filters carries artist_ids and venue_ids but no show_ids, and
// nothing else stores a bare show id.
//
// TestShowIDColumnsWithoutForeignKeysAreAccountedFor fails if a migration adds
// one. It matches on the column NAME, so an unconstrained RADIO show id would
// trip it too; recording such a column here with "checked, not a catalog show"
// is the intended outcome rather than a hole in the pattern.
var showUnconstrainedIDColumns = []string{}

// ShowDedupKey identifies a cluster of duplicate shows by the
// (artist, venue, event_date) tuple. Time-of-day is part of the key so
// matinee + evening sets at the same venue on the same day are NOT
// collapsed (PSY-559).
type ShowDedupKey struct {
	ArtistID  uint
	VenueID   uint
	EventDate time.Time
}

// ShowDedupCluster represents a group of shows that share the same
// (artist, venue, event_date). The first ID is the winner (earliest
// created_at). Remaining IDs will be merged into it.
type ShowDedupCluster struct {
	Key       ShowDedupKey
	WinnerID  uint
	LoserIDs  []uint
	ShowIDs   []uint // all IDs in cluster, sorted by created_at ASC
	CreatedAt []time.Time
}

// ShowDedupSummary summarises the work performed (or planned) by a
// dedup pass. Used by both --dry-run and --confirm flows so reviewers
// can audit the merge before live writes.
//
// The polymorphic references are counted in a MAP keyed by table rather than in
// a field per table. A field per table is the same hand-maintained list this
// merge stopped keeping: a migration that added a reference table would move
// rows the summary had no way to mention, so the report would understate what a
// destructive pass had done. The map is filled straight from the shared
// inventory, so a table added there reports itself.
type ShowDedupSummary struct {
	ClustersFound int
	LosersMerged  int

	// Direct foreign keys to shows.id. One step each; see showFKColumns.
	ShowVenuesMoved    int64
	ShowVenuesSkipped  int64
	ShowArtistsMoved   int64
	ShowArtistsSkipped int64
	ShowReportsMoved   int64
	ShowReportsSkipped int64
	EnrichmentMoved    int64
	DuplicateOfRepoint int64
	// NotifyJobsDropped counts show_notify_queue rows deleted with the loser. It
	// is a DROP rather than a move (see showFKColumns), and it is reported so a
	// reviewer can see when a merge discarded a notification that had not gone
	// out yet, rather than having that happen invisibly under the FK cascade.
	NotifyJobsDropped int64
	// AlertRowsMoved counts follow-driven alert rows in notification_log whose
	// entity_id was re-pointed onto the winner (PSY-1896). Reported because
	// those rows are a user's inbox history, and a merge silently stranding
	// them shows up as notifications that lost their title and link.
	AlertRowsMoved int64
	// VenueAlertBatchMoved counts venue_show_alert_batch memberships re-pointed
	// onto the winner (PSY-1895). Counted separately from AlertRowsMoved because
	// they are a different kind of row: those are delivered notifications, these
	// are the membership list a delivered notification RENDERS FROM. Stranding
	// these does not lose an inbox row, it silently shortens one.
	VenueAlertBatchMoved int64

	// History tables, which move through a provenance decision rather than
	// through the inventory loop. See refsRepointedElsewhere.
	PendingEditsMoved    int64
	PendingEditsSkipped  int64
	EditAuditLogsMoved   int64
	EditAuditLogsSkipped int64
	RevisionsMoved       int64

	// Every table in polymorphicEntityRefs, keyed by table name. A table that
	// matched no rows is present with a zero, so the CLI prints the full
	// coverage of the pass rather than only the tables that happened to hold
	// data. "Audited and empty" and "never looked at" are different answers.
	EntityRefsMoved   map[string]int64
	EntityRefsDropped map[string]int64

	SlugsRewritten int
}

// Add folds one cluster's counts into the pass summary.
//
// It exists so a caller can run a cluster into its OWN summary and fold that in
// only after the cluster's transaction commits. Accumulating straight into the
// pass summary reports work that was rolled back: a cluster with two losers
// whose second merge fails takes the first merge's deletions down with it, and
// without this the printed record still claims a loser merged and a bookmark
// destroyed. These counters are the only record of what a destructive pass
// destroyed, so they have to describe what committed.
//
// TestShowDedupSummaryAddFoldsEveryField fails if a field is added without
// being folded here.
func (s *ShowDedupSummary) Add(other *ShowDedupSummary) {
	if other == nil {
		return
	}
	s.ClustersFound += other.ClustersFound
	s.LosersMerged += other.LosersMerged

	s.ShowVenuesMoved += other.ShowVenuesMoved
	s.ShowVenuesSkipped += other.ShowVenuesSkipped
	s.ShowArtistsMoved += other.ShowArtistsMoved
	s.ShowArtistsSkipped += other.ShowArtistsSkipped
	s.ShowReportsMoved += other.ShowReportsMoved
	s.ShowReportsSkipped += other.ShowReportsSkipped
	s.EnrichmentMoved += other.EnrichmentMoved
	s.DuplicateOfRepoint += other.DuplicateOfRepoint
	s.NotifyJobsDropped += other.NotifyJobsDropped
	s.AlertRowsMoved += other.AlertRowsMoved
	s.VenueAlertBatchMoved += other.VenueAlertBatchMoved

	s.PendingEditsMoved += other.PendingEditsMoved
	s.PendingEditsSkipped += other.PendingEditsSkipped
	s.EditAuditLogsMoved += other.EditAuditLogsMoved
	s.EditAuditLogsSkipped += other.EditAuditLogsSkipped
	s.RevisionsMoved += other.RevisionsMoved

	s.addEntityRefCounts(other.EntityRefsMoved, other.EntityRefsDropped)

	s.SlugsRewritten += other.SlugsRewritten
}

// addEntityRefCounts folds one merge's per-table counts into the pass summary.
//
// The maps are created on demand so callers can keep building the summary as a
// bare struct literal, which all of them do.
func (s *ShowDedupSummary) addEntityRefCounts(moved, dropped map[string]int64) {
	if s.EntityRefsMoved == nil {
		s.EntityRefsMoved = make(map[string]int64, len(moved))
	}
	if s.EntityRefsDropped == nil {
		s.EntityRefsDropped = make(map[string]int64, len(dropped))
	}
	for table, count := range moved {
		s.EntityRefsMoved[table] += count
	}
	for table, count := range dropped {
		s.EntityRefsDropped[table] += count
	}
}

// FindShowDedupClusters finds groups of shows that share the same
// (artist_id, venue_id, event_date). Returns one cluster per group of
// 2+ shows. The dedup key includes the FULL event_date timestamp so
// matinee/evening shows at the same venue on the same day are
// preserved.
//
// Implementation note: we join shows with show_artists and show_venues
// and group by (artist_id, venue_id, event_date). A show with multiple
// headliners or multiple venues will appear in multiple clusters, but
// each cluster is processed independently and idempotently.
func FindShowDedupClusters(db *gorm.DB) ([]ShowDedupCluster, error) {
	type row struct {
		ArtistID  uint      `gorm:"column:artist_id"`
		VenueID   uint      `gorm:"column:venue_id"`
		EventDate time.Time `gorm:"column:event_date"`
		ShowID    uint      `gorm:"column:show_id"`
		CreatedAt time.Time `gorm:"column:created_at"`
	}

	// Pull all (artist, venue, event_date) tuples that have 2+ shows.
	// Filter approved+private only — pending/rejected duplicates are
	// admin-review concerns, not user-facing duplication.
	var rows []row
	err := db.Raw(`
		SELECT
			sa.artist_id  AS artist_id,
			sv.venue_id   AS venue_id,
			s.event_date  AS event_date,
			s.id          AS show_id,
			s.created_at  AS created_at
		FROM shows s
		JOIN show_artists sa ON sa.show_id = s.id
		JOIN show_venues  sv ON sv.show_id = s.id
		WHERE s.status IN ('approved','private')
		ORDER BY sa.artist_id, sv.venue_id, s.event_date, s.created_at ASC, s.id ASC
	`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to scan show clusters: %w", err)
	}

	// Group rows by (artist_id, venue_id, event_date). Use a stable
	// string key so map iteration order doesn't leak into output —
	// we sort the final slice deterministically below.
	type groupKey struct {
		ArtistID  uint
		VenueID   uint
		EventDate string
	}
	groups := map[groupKey][]row{}
	for _, r := range rows {
		k := groupKey{r.ArtistID, r.VenueID, r.EventDate.UTC().Format(time.RFC3339Nano)}
		groups[k] = append(groups[k], r)
	}

	clusters := make([]ShowDedupCluster, 0)
	for _, members := range groups {
		// Deduplicate within group by show_id — a single show can
		// appear multiple times if it has multiple artists or venues
		// matching the same key. Take the first (earliest created_at).
		seen := map[uint]bool{}
		uniq := make([]row, 0, len(members))
		for _, m := range members {
			if seen[m.ShowID] {
				continue
			}
			seen[m.ShowID] = true
			uniq = append(uniq, m)
		}
		if len(uniq) < 2 {
			continue
		}

		// Sort by created_at ASC, then ID ASC as a tiebreaker.
		sort.Slice(uniq, func(i, j int) bool {
			if !uniq[i].CreatedAt.Equal(uniq[j].CreatedAt) {
				return uniq[i].CreatedAt.Before(uniq[j].CreatedAt)
			}
			return uniq[i].ShowID < uniq[j].ShowID
		})

		ids := make([]uint, len(uniq))
		createds := make([]time.Time, len(uniq))
		for i, m := range uniq {
			ids[i] = m.ShowID
			createds[i] = m.CreatedAt
		}

		clusters = append(clusters, ShowDedupCluster{
			Key: ShowDedupKey{
				ArtistID:  uniq[0].ArtistID,
				VenueID:   uniq[0].VenueID,
				EventDate: uniq[0].EventDate,
			},
			WinnerID:  ids[0],
			LoserIDs:  ids[1:],
			ShowIDs:   ids,
			CreatedAt: createds,
		})
	}

	// Stable order across runs: sort clusters by (artist_id, venue_id, event_date).
	sort.Slice(clusters, func(i, j int) bool {
		a, b := clusters[i].Key, clusters[j].Key
		if a.ArtistID != b.ArtistID {
			return a.ArtistID < b.ArtistID
		}
		if a.VenueID != b.VenueID {
			return a.VenueID < b.VenueID
		}
		return a.EventDate.Before(b.EventDate)
	})

	return clusters, nil
}

// MergeDuplicateShow merges loser into winner inside an existing transaction,
// re-points every reference this file's inventory names, and deletes the loser.
//
// Conflict policy: when a UNIQUE or PK conflict would occur on the winner, the
// loser's row is dropped and the winner's pre-existing row survives. This
// matches the tag_merge.go pattern.
//
// The steps run in the order of the inventory at the top of this file: foreign
// keys to shows.id first, then the two history tables that require a provenance
// decision, then the shared polymorphic inventory, then the delete. Only the
// delete depends on that order; the rest reads in inventory order so a reader
// can check the code against the list.
func MergeDuplicateShow(tx *gorm.DB, winnerID, loserID uint, summary *ShowDedupSummary) error {
	if winnerID == 0 || loserID == 0 {
		return fmt.Errorf("winnerID and loserID must be non-zero")
	}
	if winnerID == loserID {
		return fmt.Errorf("winnerID == loserID")
	}
	if summary == nil {
		return fmt.Errorf("summary is required")
	}

	// Junction tables keyed (show_id, otherCol), plus show_reports, which is
	// UNIQUE (show_id, reported_by) and needs the same collision delete.
	for _, ref := range []struct {
		table    string
		dedupeOn string
		moved    *int64
		skipped  *int64
	}{
		{"show_venues", "venue_id", &summary.ShowVenuesMoved, &summary.ShowVenuesSkipped},
		{"show_artists", "artist_id", &summary.ShowArtistsMoved, &summary.ShowArtistsSkipped},
		{"show_reports", "reported_by", &summary.ShowReportsMoved, &summary.ShowReportsSkipped},
	} {
		moved, skipped, err := moveShowFKRows(tx, ref.table, "show_id", ref.dedupeOn, winnerID, loserID)
		if err != nil {
			return fmt.Errorf("%s: %w", ref.table, err)
		}
		*ref.moved += moved
		*ref.skipped += skipped
		// Only show_reports is logged. A dropped report is a user-authored row
		// gone with no undo, which is what logDroppedEntityRefs is for. The two
		// junction drops are not: duplicate shows share an artist and a venue by
		// definition, since that IS the cluster key, so those two collapse on
		// essentially every merge. Logging them would bury each real deletion
		// under two lines of expected bookkeeping. Both counts still reach the
		// summary the CLI prints.
		if skipped > 0 && ref.table == "show_reports" {
			logDroppedEntityRefs(
				entityTypeShow, winnerID, loserID, map[string]int64{ref.table: skipped})
		}
	}

	// Re-stamp denormalized (event_date, venue_id) on the winner's
	// show_artists rows after the merge. Moved rows carried the loser's
	// denorm; the winner may have an extra venue or a different primary
	// venue, so refresh from the canonical sources to keep the partial
	// unique index `shows_artist_venue_eventdate_uniq` aligned (PSY-576).
	if err := syncShowArtistDedupColumns(tx, winnerID); err != nil {
		return fmt.Errorf("show_artists dedup-column resync: %w", err)
	}

	// enrichment_queue has no unique index over show_id, so it takes a bare
	// re-point.
	res := tx.Exec(`UPDATE enrichment_queue SET show_id = ? WHERE show_id = ?`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("enrichment_queue: %w", res.Error)
	}
	summary.EnrichmentMoved += res.RowsAffected

	// show_notify_queue is DROPPED, not re-pointed. See its entry in
	// showFKColumns for the argument: the row means "already considered for
	// follower notification", its UNIQUE is on show_id alone so it cannot be
	// moved onto a winner that has one, and moving it onto a winner that has none
	// would let a merge announce a show that predates the feature. Deleting it
	// explicitly rather than leaving it to the FK cascade keeps this function's
	// stated invariant true — every foreign key to shows.id is handled here — so
	// the delete below can go on meaning "nothing was silently destroyed".
	res = tx.Exec(`DELETE FROM show_notify_queue WHERE show_id = ?`, loserID)
	if res.Error != nil {
		return fmt.Errorf("show_notify_queue: %w", res.Error)
	}
	summary.NotifyJobsDropped += res.RowsAffected
	if res.RowsAffected > 0 {
		// Logged per-show for the same reason show_reports is: this can be the only
		// notification the event would ever have received (see showFKColumns), and
		// "which show went quiet" is not a question the aggregate counter can answer.
		logDroppedEntityRefs(
			entityTypeShow, winnerID, loserID,
			map[string]int64{"show_notify_queue": res.RowsAffected})
	}

	// shows.duplicate_of_show_id is the self-reference, so the winner's own row
	// has to be excluded: a winner already flagged as a duplicate of the loser
	// would otherwise come out of the merge flagged as a duplicate of ITSELF,
	// and that column is served to clients on the show payload.
	res = tx.Exec(
		`UPDATE shows SET duplicate_of_show_id = ? WHERE duplicate_of_show_id = ? AND id <> ?`,
		winnerID, loserID, winnerID)
	if res.Error != nil {
		return fmt.Errorf("duplicate_of_show_id: %w", res.Error)
	}
	summary.DuplicateOfRepoint += res.RowsAffected

	// The two history tables move through repointEditHistory, which will not run
	// without a provenance decision. They need one for the same reason revisions
	// does below: this CLI deletes the show a read-time gate would have
	// consulted. See editHistoryCarriesNoRedaction for why neither carries a
	// redaction today, and what has to change if either gains an entity-scoped
	// gate.
	//
	// pending_entity_edits moved as a bare UPDATE until PSY-1788, which was wrong
	// on the facts: idx_pending_entity_edits_unique is UNIQUE on
	// (entity_type, entity_id, submitted_by) WHERE status = 'pending', so one
	// contributor with a pending edit on both shows aborted the whole dedup
	// transaction.
	//
	// entity_edit_audit_logs was not re-pointed at all until PSY-1869. That has
	// orphaned nothing YET, and the reason is worth writing down rather than
	// discovering later: no writer records entity_type='show' there today
	// (LogEntityEdit is called for artist, label, release, festival and scene,
	// and the show update path does not call it). Nothing in the schema says so
	// though. There is no CHECK, the table is in the shared inventory, and the
	// day a show edit route logs one, the merge that would have orphaned it is
	// the last place anyone would think to look. It moves for the same reason the
	// CHECK-forbidden tables are still walked: the inventory decides, not a fact
	// about today's callers.
	for _, h := range []struct {
		table   editHistoryTable
		moved   *int64
		dropped *int64
	}{
		{pendingEditsHistory, &summary.PendingEditsMoved, &summary.PendingEditsSkipped},
		{entityEditAuditHistory, &summary.EditAuditLogsMoved, &summary.EditAuditLogsSkipped},
	} {
		moved, dropped, err := repointEditHistory(
			tx, h.table, entityTypeShow, winnerID, loserID, editHistoryCarriesNoRedaction)
		if err != nil {
			return err
		}
		*h.moved += moved
		*h.dropped += dropped
		if dropped > 0 {
			logDroppedEntityRefs(
				entityTypeShow, winnerID, loserID, map[string]int64{h.table.name: dropped})
		}
	}

	// revisions: stamped when the loser is gated, because this function deletes
	// the show the read-time visibility gate would have consulted moments from
	// now. See reassignShowRevisions.
	revisionsMoved, err := reassignShowRevisions(tx, winnerID, loserID)
	if err != nil {
		return err
	}
	summary.RevisionsMoved += revisionsMoved

	// Everything with a polymorphic (entity_type, entity_id) reference, from the
	// inventory the venue and artist merges read. This replaced two hand-written
	// loops that between them covered ten of the seventeen tables:
	// comment_last_read, entity_requests, image_enrich_queue, notification_log,
	// source_configs and tag_votes were never touched by a show merge
	// (entity_edit_audit_logs, the seventh, moves with the history tables above).
	//
	// Two of those six can never hold entity_type='show' because a CHECK forbids
	// it (image_enrich_queue is artist/release, source_configs is venue/label),
	// so their statements match zero rows. They are walked anyway: one exhaustive
	// list is cheaper to keep correct than a list plus a set of per-entity
	// exceptions that has to be re-verified every time a CHECK changes.
	entityRefsMoved, entityRefsDropped, err := repointEntityRefs(
		tx, polymorphicEntityRefs, entityTypeShow, winnerID, loserID)
	if err != nil {
		return err
	}
	summary.addEntityRefCounts(entityRefsMoved, entityRefsDropped)
	logDroppedEntityRefs(entityTypeShow, winnerID, loserID, entityRefsDropped)

	// notification_log rows for follow-driven show alerts (PSY-1896). Separate
	// from the loop above because they key on their own entity_type, so
	// polymorphicEntityRefs cannot see them even though their entity_id is a
	// show id. Left behind they point at a deleted show and the user's inbox row
	// loses its title and link. See repointArtistShowAlertShows.
	alertsMoved, alertsDropped, err := repointArtistShowAlertShows(tx, winnerID, loserID)
	if err != nil {
		return err
	}
	summary.AlertRowsMoved += alertsMoved
	if alertsDropped > 0 {
		logDroppedEntityRefs(entityTypeShow, winnerID, loserID,
			map[string]int64{"notification_log (artist_show_alert)": alertsDropped})
	}

	// venue_show_alert_batch memberships (PSY-1895). A real foreign key, so
	// unlike the rows above these would CASCADE away silently with the loser —
	// the venue alert that named this show would quietly render one show shorter.
	// Its own function rather than moveShowFKRows because the unique key needs
	// two dedupe columns; see repointVenueShowAlertBatchShows.
	batchMoved, batchDropped, err := repointVenueShowAlertBatchShows(tx, winnerID, loserID)
	if err != nil {
		return err
	}
	summary.VenueAlertBatchMoved += batchMoved
	if batchDropped > 0 {
		logDroppedEntityRefs(entityTypeShow, winnerID, loserID,
			map[string]int64{"venue_show_alert_batch": batchDropped})
	}

	// Delete the loser show. Nothing should be left to CASCADE: every foreign key
	// to shows.id is named in showFKColumns, and every one of them was re-pointed
	// above.
	//
	// A delete that matches no row is an ERROR, not a no-op. GORM reports no
	// error for it, so without this a merge against a show that had already been
	// deleted returned success and incremented LosersMerged, and the CLI printed
	// a merge that never happened. Every re-point above matched zero rows in that
	// case too, so failing here costs nothing real and rolls the cluster back.
	res = tx.Delete(&catalogm.Show{}, loserID)
	if res.Error != nil {
		return fmt.Errorf("delete loser show %d: %w", loserID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("loser show %d no longer exists; nothing was merged into %d",
			loserID, winnerID)
	}

	summary.LosersMerged++
	return nil
}

// reassignShowRevisions moves the losing show's revision history onto the
// winner, stamping it when that show was gated so the read-time ENTITY
// suppression survives the re-point. See the privacy section of the
// revisiondiff package doc for the policy; repointRevisions is the mechanism.
//
// Gated means any status other than approved. GET /shows/{id} 404s an
// unprivileged caller for exactly those, admin.RevisionService mirrors that
// rule, and both read shows.status for the show a revision currently points
// at — which this merge is about to delete.
//
// BOTH statuses decide the stamp, not just the loser's. The stamp is a
// permanent override: it suppresses a row for every non-admin caller whatever
// the show it now points at says, and nothing ever clears it. Paying that price
// is right when the winner is approved, because the read-time lookup would
// otherwise publish the loser's history the moment the loser row is deleted.
// It is WRONG when the winner is gated too, and that case is reachable:
// FindShowDedupClusters selects candidates from status IN ('approved','private'),
// so both members of a cluster can be private. Stamping there would suppress the
// carried history forever, including from the surviving show's own submitter,
// and the owner publishing the winner through POST /shows/{id}/publish would
// leave a fully public show whose edit history had been silently erased.
//
// So: stamp only when the loser is gated AND the winner is not known to be
// gated. An unstamped row lands on a gated winner, where the plain status lookup
// already suppresses it — and keeps tracking that winner, so publishing the
// merged show restores the history exactly as if the two rows had always been
// one. That is the behaviour the policy doc claims, and this is what makes the
// claim true.
//
// An approved loser gets noRedactionCarryover rather than a clear: nothing was
// being suppressed, and any mark already on those rows came off an EARLIER
// gated show, so a chain of merges cannot launder a private show's history.
//
// The statuses are read here rather than taken as parameters because the caller
// has only ids — unlike the venue merge, which already holds the locked row.
// The read takes FOR UPDATE for that reason: at READ COMMITTED an unlocked read
// could see 'approved', have a concurrent transaction unpublish the show and
// commit, and then delete it — leaving a gated show's history unstamped on an
// approved winner, which is exactly the leak the stamp exists to prevent. The
// lock holds until the merge transaction ends, and the loser is deleted inside
// it, so no writer can move a status out from under the decision.
//
// FAILS CLOSED on both reads, in the direction that withholds. A loser row this
// cannot read is treated as GATED: a missing row means the merge is operating on
// a show that no longer exists, and any revisions still pointing at it are
// orphans no read-time lookup could ever gate. A winner row it cannot read is
// treated as NOT gated, which is the answer that stamps — because the exemption
// above has to be earned by positively reading a gated winner, never inherited
// from a failed lookup.
//
// This does not scrub anything. The stored diff keeps the real values, which is
// what rollback reads; only the public read path suppresses the row.
//
// KNOWN BOUNDARY, admin-triggered and the same one the venue stamp carries: a
// stamped row's values can still reach a public reader through Rollback, which
// writes them onto the show the revision NOW points at and records the rollback
// as a fresh, unstamped revision. After the merge that show is approved and
// publishes those fields anyway, so the new revision is consistent with what the
// show already serves rather than a second leak. Keeping the stored values is
// what makes rollback possible at all, so this stays an explicit admin action
// rather than another gate.
func reassignShowRevisions(tx *gorm.DB, winnerID, loserID uint) (int64, error) {
	// Both rows in one locked read. Ordered by id so two merges that happen to
	// share a row cannot take the locks in opposite orders.
	var rows []struct {
		ID     uint                `gorm:"column:id"`
		Status catalogm.ShowStatus `gorm:"column:status"`
	}
	err := tx.Model(&catalogm.Show{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id, status").
		Where("id IN ?", []uint{winnerID, loserID}).
		Order("id").
		Scan(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("read show statuses for merge %d <- %d: %w", winnerID, loserID, err)
	}

	// Absent from the result means the row could not be read. The zero value is
	// not a real status, and each side's default below is the one that withholds.
	statuses := make(map[uint]catalogm.ShowStatus, len(rows))
	for _, r := range rows {
		statuses[r.ID] = r.Status
	}
	loserGated := statuses[loserID] != catalogm.ShowStatusApproved
	winnerGated := false
	if s, read := statuses[winnerID]; read {
		winnerGated = s != catalogm.ShowStatusApproved
	}

	provenance := noRedactionCarryover
	if loserGated && !winnerGated {
		provenance = stampFromGatedShow
	}

	return repointRevisions(tx, entityTypeShow, winnerID, loserID, provenance)
}

// moveShowFKRows re-points one table's foreign key to shows.id, dropping the
// loser's rows first when the winner already holds a row the unique key would
// collide with.
//
// dedupeOn is the OTHER column in that unique key: venue_id and artist_id for
// the show_venues and show_artists primary keys, reported_by for show_reports'
// UNIQUE (show_id, reported_by). It is a property of the TABLE, never of the
// call site, for the same reason editHistoryTable.dedupeOn is: letting a caller
// choose whether to dodge an index is what left show_reports moving with a bare
// UPDATE that its own index could reject.
//
// Every dedupeOn column is NOT NULL today, so the delete's `w.col = l.col`
// correlation needs no NULL handling. A nullable one would need the same care
// notification_log's does in repointEntityRefs: a unique index treats NULLs as
// distinct, so the equality that skips them matches the index rather than
// missing rows.
func moveShowFKRows(tx *gorm.DB, table, showCol, dedupeOn string, winnerID, loserID uint) (moved, skipped int64, err error) {
	if table == "" || showCol == "" || dedupeOn == "" {
		return 0, 0, fmt.Errorf("move show fk rows: table, show column and dedupe column are required")
	}
	// The column being re-pointed must be one the inventory names. This makes
	// showFKColumns load-bearing rather than documentary: a re-point of a column
	// with no recorded disposition cannot run, and the interpolated identifiers
	// are pinned to a package-level list instead of merely being literals today.
	if !showFKColumnListed(table + "." + showCol) {
		return 0, 0, fmt.Errorf(
			"move show fk rows: %s.%s is not in showFKColumns, so it has no recorded disposition",
			table, showCol)
	}
	// Being in the inventory is necessary but not sufficient: some columns are
	// recorded there BECAUSE the completeness guard demands it, while their
	// disposition is "drop", not "move". Without this second check, adding such a
	// column to the inventory silently granted permission to re-point it.
	if showFKColumnRepointBanned(table + "." + showCol) {
		return 0, 0, fmt.Errorf(
			"move show fk rows: %s.%s is recorded as drop-only in showFKColumnsNeverRepointed "+
				"and must not be re-pointed; see its entry in showFKColumns for why",
			table, showCol)
	}
	if winnerID == 0 || loserID == 0 {
		return 0, 0, fmt.Errorf("move show fk rows: winner and loser ids are required")
	}
	// A self-merge would correlate every row against itself and delete the
	// surviving show's own rows before the no-op move ran.
	if winnerID == loserID {
		return 0, 0, fmt.Errorf("move show fk rows: cannot re-point show %d onto itself", winnerID)
	}

	// Drop loser rows whose dedupeOn value already exists on the winner.
	// #nosec G201 -- table and column names come from the hardcoded call sites in
	// MergeDuplicateShow, never from caller input; the ids are bound parameters.
	delSQL := fmt.Sprintf(`
		DELETE FROM %[1]s l
		WHERE l.%[2]s = ?
		  AND EXISTS (
		        SELECT 1 FROM %[1]s w
		        WHERE w.%[2]s = ?
		          AND w.%[3]s = l.%[3]s
		      )
	`, table, showCol, dedupeOn)
	del := tx.Exec(delSQL, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, del.Error
	}
	skipped = del.RowsAffected

	// #nosec G201 -- see above.
	updSQL := fmt.Sprintf(`UPDATE %[1]s SET %[2]s = ? WHERE %[2]s = ?`, table, showCol)
	upd := tx.Exec(updSQL, winnerID, loserID)
	if upd.Error != nil {
		return 0, 0, upd.Error
	}
	moved = upd.RowsAffected
	return moved, skipped, nil
}

// RecanonicaliseShowSlug recomputes the show's slug using the canonical
// venue-timezone-aware GenerateShowSlug helper. Idempotent: if the
// computed slug already matches the stored slug, no DB write happens.
//
// Used by the dedup cmd to fix slugs left in the legacy
// migration-000019 form ("…YYYY-MM-DD" derived from raw UTC date) on
// shows that survive a merge. Returns true if the slug was rewritten.
func RecanonicaliseShowSlug(tx *gorm.DB, showID uint) (bool, error) {
	var show catalogm.Show
	if err := tx.First(&show, showID).Error; err != nil {
		return false, fmt.Errorf("load show: %w", err)
	}

	// Resolve headliner — set_type='headliner' wins, else position=0.
	var artists []catalogm.Artist
	if err := tx.Table("artists").
		Joins("JOIN show_artists ON show_artists.artist_id = artists.id").
		Where("show_artists.show_id = ?", showID).
		Order("CASE WHEN show_artists.set_type='headliner' THEN 0 ELSE 1 END, show_artists.position ASC, artists.id ASC").
		Find(&artists).Error; err != nil {
		return false, fmt.Errorf("load show artists: %w", err)
	}

	// Resolve venue — first by show_venues join.
	var venues []catalogm.Venue
	if err := tx.Table("venues").
		Joins("JOIN show_venues ON show_venues.venue_id = venues.id").
		Where("show_venues.show_id = ?", showID).
		Order("venues.id ASC").
		Find(&venues).Error; err != nil {
		return false, fmt.Errorf("load show venues: %w", err)
	}

	headlinerName := "unknown"
	if len(artists) > 0 {
		headlinerName = artists[0].Name
	}
	venueName := "unknown"
	var venueTimezone *string
	if len(venues) > 0 {
		venueName = venues[0].Name
		venueTimezone = venues[0].Timezone
	}

	state := ""
	if show.State != nil {
		state = *show.State
	}

	canonical := utils.GenerateShowSlug(show.EventDate, headlinerName, venueName, venueTimezone, state)
	current := ""
	if show.Slug != nil {
		current = *show.Slug
	}
	if canonical == current {
		return false, nil
	}

	// Ensure uniqueness — if the canonical slug already exists on
	// another show, append a numeric suffix.
	unique := utils.GenerateUniqueSlug(canonical, func(candidate string) bool {
		var count int64
		tx.Model(&catalogm.Show{}).
			Where("slug = ? AND id <> ?", candidate, showID).
			Count(&count)
		return count > 0
	})

	if err := tx.Model(&catalogm.Show{}).Where("id = ?", showID).Update("slug", unique).Error; err != nil {
		return false, fmt.Errorf("update slug: %w", err)
	}
	return true, nil
}
