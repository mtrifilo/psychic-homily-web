package catalog

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

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
type ShowDedupSummary struct {
	ClustersFound       int
	LosersMerged        int
	ShowVenuesMoved     int64
	ShowVenuesSkipped   int64
	ShowArtistsMoved    int64
	ShowArtistsSkipped  int64
	ShowReportsMoved    int64
	EnrichmentMoved     int64
	BookmarksMoved      int64
	BookmarksSkipped    int64
	CommentsRepointed   int64
	SubsRepointed       int64
	SubsSkipped         int64
	EntityTagsMoved     int64
	EntityTagsSkipped   int64
	EntityReportsMoved  int64
	PendingEditsMoved   int64
	PendingEditsSkipped int64
	RevisionsMoved      int64
	RequestsMoved       int64
	AuditLogsMoved      int64
	CollectionsMoved    int64
	CollectionsSkipped  int64
	DuplicateOfRepoint  int64
	SlugsRewritten      int
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

// MergeDuplicateShow merges loser into winner inside an existing
// transaction. All FKs (direct + polymorphic) are repointed with
// conflict-aware semantics. The loser is then deleted.
//
// Conflict policy: when a UNIQUE / PK conflict would occur on the
// winner, the loser's row is dropped (the winner's pre-existing row
// wins). This matches the tag_merge.go pattern.
func MergeDuplicateShow(tx *gorm.DB, winnerID, loserID uint, summary *ShowDedupSummary) error {
	if winnerID == 0 || loserID == 0 {
		return fmt.Errorf("winnerID and loserID must be non-zero")
	}
	if winnerID == loserID {
		return fmt.Errorf("winnerID == loserID")
	}

	// Junction tables with composite PK (show_id, otherCol).
	moved, skipped, err := movePolymorphicJunction(tx, "show_venues", "show_id", "venue_id", winnerID, loserID)
	if err != nil {
		return fmt.Errorf("show_venues: %w", err)
	}
	summary.ShowVenuesMoved += moved
	summary.ShowVenuesSkipped += skipped

	moved, skipped, err = movePolymorphicJunction(tx, "show_artists", "show_id", "artist_id", winnerID, loserID)
	if err != nil {
		return fmt.Errorf("show_artists: %w", err)
	}
	summary.ShowArtistsMoved += moved
	summary.ShowArtistsSkipped += skipped

	// Re-stamp denormalized (event_date, venue_id) on the winner's
	// show_artists rows after the merge. Moved rows carried the loser's
	// denorm; the winner may have an extra venue or a different primary
	// venue, so refresh from the canonical sources to keep the partial
	// unique index `shows_artist_venue_eventdate_uniq` aligned (PSY-576).
	if err := syncShowArtistDedupColumns(tx, winnerID); err != nil {
		return fmt.Errorf("show_artists dedup-column resync: %w", err)
	}

	// Plain FK repoints — no unique constraint to worry about.
	for _, op := range []struct {
		name string
		sql  string
		dst  *int64
	}{
		{"show_reports", `UPDATE show_reports SET show_id = ? WHERE show_id = ?`, &summary.ShowReportsMoved},
		{"enrichment_queue", `UPDATE enrichment_queue SET show_id = ? WHERE show_id = ?`, &summary.EnrichmentMoved},
		{"duplicate_of_show_id", `UPDATE shows SET duplicate_of_show_id = ? WHERE duplicate_of_show_id = ?`, &summary.DuplicateOfRepoint},
	} {
		res := tx.Exec(op.sql, winnerID, loserID)
		if res.Error != nil {
			return fmt.Errorf("%s: %w", op.name, res.Error)
		}
		*op.dst += res.RowsAffected
	}

	// Polymorphic FK repoints (entity_type='show'). Tables with no
	// uniqueness constraint on (entity_type, entity_id) just need a
	// straight UPDATE.
	for _, op := range []struct {
		name string
		sql  string
		dst  *int64
	}{
		{"comments", `UPDATE comments SET entity_id = ? WHERE entity_type = 'show' AND entity_id = ?`, &summary.CommentsRepointed},
		{"entity_reports", `UPDATE entity_reports SET entity_id = ? WHERE entity_type = 'show' AND entity_id = ?`, &summary.EntityReportsMoved},
		{"audit_logs", `UPDATE audit_logs SET entity_id = ? WHERE entity_type = 'show' AND entity_id = ?`, &summary.AuditLogsMoved},
		// requests uses requested_entity_id, not entity_id.
		{"requests", `UPDATE requests SET requested_entity_id = ? WHERE entity_type = 'show' AND requested_entity_id = ?`, &summary.RequestsMoved},
	} {
		res := tx.Exec(op.sql, winnerID, loserID)
		if res.Error != nil {
			return fmt.Errorf("%s: %w", op.name, res.Error)
		}
		*op.dst += res.RowsAffected
	}

	// pending_entity_edits: a plain re-point today, but it has to say so, for
	// the same reason revisions does below — this CLI deletes the show a
	// read-time gate would have consulted. See editHistoryCarriesNoRedaction.
	//
	// It moved with the no-uniqueness group above until PSY-1788, which was
	// wrong on the facts: idx_pending_entity_edits_unique is UNIQUE on
	// (entity_type, entity_id, submitted_by) WHERE status = 'pending', so a bare
	// UPDATE aborted the whole dedup transaction whenever one contributor had a
	// pending edit on both shows. The helper dedupes first, which is this
	// function's stated conflict policy — the winner's row wins.
	//
	// entity_edit_audit_logs is deliberately NOT re-pointed here: this merge has
	// never touched it, and adding a re-point is a separate change from routing
	// the ones that exist.
	pendingEditsMoved, pendingEditsDropped, err := repointEditHistory(
		tx, pendingEditsHistory, mergeEntityShow, winnerID, loserID, editHistoryCarriesNoRedaction)
	if err != nil {
		return err
	}
	summary.PendingEditsMoved += pendingEditsMoved
	summary.PendingEditsSkipped += pendingEditsDropped

	// revisions: stamped when the loser is gated, because this function deletes
	// the show the read-time visibility gate would have consulted moments from
	// now. See reassignShowRevisions.
	revisionsMoved, err := reassignShowRevisions(tx, winnerID, loserID)
	if err != nil {
		return err
	}
	summary.RevisionsMoved += revisionsMoved

	// Polymorphic FK repoints WITH a uniqueness constraint —
	// conflict-correlation columns vary per table.
	for _, op := range []struct {
		name        string
		correlation []string
		moved       *int64
		skipped     *int64
	}{
		{"comment_subscriptions", []string{"user_id"}, &summary.SubsRepointed, &summary.SubsSkipped},
		{"entity_tags", []string{"tag_id"}, &summary.EntityTagsMoved, &summary.EntityTagsSkipped},
		{"collection_items", []string{"collection_id"}, &summary.CollectionsMoved, &summary.CollectionsSkipped},
		{"user_bookmarks", []string{"user_id", "action"}, &summary.BookmarksMoved, &summary.BookmarksSkipped},
	} {
		moved, skipped, err = movePolymorphicEntity(tx, op.name, op.correlation, winnerID, loserID)
		if err != nil {
			return fmt.Errorf("%s: %w", op.name, err)
		}
		*op.moved += moved
		*op.skipped += skipped
	}

	// Delete the loser show. CASCADE handles anything left in
	// show_venues / show_artists / show_reports / enrichment_queue
	// (i.e. nothing — all repointed above).
	if err := tx.Delete(&catalogm.Show{}, loserID).Error; err != nil {
		return fmt.Errorf("delete loser show %d: %w", loserID, err)
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

	return repointRevisions(tx, mergeEntityShow, winnerID, loserID, provenance)
}

// movePolymorphicJunction drops conflicting rows on (show_id, otherCol)
// where the winner already has an entry, then re-points the rest to
// the winner. Used for show_venues and show_artists.
func movePolymorphicJunction(tx *gorm.DB, table, primaryCol, otherCol string, winnerID, loserID uint) (moved, skipped int64, err error) {
	// Drop loser rows whose otherCol value already exists on the winner.
	delSQL := fmt.Sprintf(`
		DELETE FROM %s
		WHERE %s = ?
		  AND EXISTS (
			SELECT 1 FROM %s w
			WHERE w.%s = ?
			  AND w.%s = %s.%s
		  )
	`, table, primaryCol, table, primaryCol, otherCol, table, otherCol)
	del := tx.Exec(delSQL, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, del.Error
	}
	skipped = del.RowsAffected

	updSQL := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ?`, table, primaryCol, primaryCol)
	upd := tx.Exec(updSQL, winnerID, loserID)
	if upd.Error != nil {
		return 0, 0, upd.Error
	}
	moved = upd.RowsAffected
	return moved, skipped, nil
}

// movePolymorphicEntity is a conflict-aware FK repoint for tables
// keyed on `(<correlation columns>, entity_type='show', entity_id)`.
// Loser rows whose `<correlation>` already collides with a winner
// row are deleted (winner wins); remaining rows are repointed to the
// winner. Used for comment_subscriptions, entity_tags,
// collection_items and user_bookmarks — `correlation` differs per
// table (e.g. `["user_id", "action"]` for user_bookmarks).
func movePolymorphicEntity(tx *gorm.DB, table string, correlation []string, winnerID, loserID uint) (moved, skipped int64, err error) {
	if len(correlation) == 0 {
		return 0, 0, fmt.Errorf("correlation must not be empty")
	}
	conds := make([]string, len(correlation))
	for i, col := range correlation {
		conds[i] = fmt.Sprintf("t2.%s = %s.%s", col, table, col)
	}
	delSQL := fmt.Sprintf(`
		DELETE FROM %s
		WHERE entity_type = 'show'
		  AND entity_id = ?
		  AND EXISTS (
			SELECT 1 FROM %s t2
			WHERE t2.entity_type = 'show'
			  AND t2.entity_id = ?
			  AND %s
		  )
	`, table, table, strings.Join(conds, " AND "))
	del := tx.Exec(delSQL, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, del.Error
	}
	skipped = del.RowsAffected

	updSQL := fmt.Sprintf(`
		UPDATE %s
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, table)
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
	if len(venues) > 0 {
		venueName = venues[0].Name
	}

	state := ""
	if show.State != nil {
		state = *show.State
	}

	canonical := utils.GenerateShowSlug(show.EventDate, headlinerName, venueName, state)
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
