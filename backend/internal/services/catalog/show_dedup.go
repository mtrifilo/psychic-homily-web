package catalog

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

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
	ClustersFound      int
	LosersMerged       int
	ShowVenuesMoved    int64
	ShowVenuesSkipped  int64
	ShowArtistsMoved   int64
	ShowArtistsSkipped int64
	ShowReportsMoved   int64
	EnrichmentMoved    int64
	BookmarksMoved     int64
	BookmarksSkipped   int64
	CommentsRepointed  int64
	SubsRepointed      int64
	SubsSkipped        int64
	EntityTagsMoved    int64
	EntityTagsSkipped  int64
	EntityReportsMoved int64
	PendingEditsMoved  int64
	RevisionsMoved     int64
	RequestsMoved      int64
	AuditLogsMoved     int64
	CollectionsMoved   int64
	CollectionsSkipped int64
	DuplicateOfRepoint int64
	SlugsRewritten     int
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
// conflict-aware semantics. The loser is then deleted, which cascades
// to show_venues / show_artists / show_reports / enrichment_queue
// rows that still belong to it.
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

	// 1. show_venues — copy missing rows from loser to winner, drop conflicts.
	moved, skipped, err := movePolymorphicJunction(tx, "show_venues", "show_id", "venue_id", winnerID, loserID)
	if err != nil {
		return fmt.Errorf("show_venues: %w", err)
	}
	summary.ShowVenuesMoved += moved
	summary.ShowVenuesSkipped += skipped

	// 2. show_artists — same pattern. Preserve position/set_type from
	// loser only when winner doesn't already have the artist (handled
	// by the conflict-skip below).
	moved, skipped, err = movePolymorphicJunction(tx, "show_artists", "show_id", "artist_id", winnerID, loserID)
	if err != nil {
		return fmt.Errorf("show_artists: %w", err)
	}
	summary.ShowArtistsMoved += moved
	summary.ShowArtistsSkipped += skipped

	// 3. show_reports — repoint to winner (no unique constraint).
	res := tx.Exec(`UPDATE show_reports SET show_id = ? WHERE show_id = ?`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("show_reports: %w", res.Error)
	}
	summary.ShowReportsMoved += res.RowsAffected

	// 4. enrichment_queue — repoint to winner.
	res = tx.Exec(`UPDATE enrichment_queue SET show_id = ? WHERE show_id = ?`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("enrichment_queue: %w", res.Error)
	}
	summary.EnrichmentMoved += res.RowsAffected

	// 5. shows.duplicate_of_show_id — repoint loser→winner so any
	// existing duplicate-of pointer survives the merge.
	res = tx.Exec(`UPDATE shows SET duplicate_of_show_id = ? WHERE duplicate_of_show_id = ?`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("duplicate_of_show_id: %w", res.Error)
	}
	summary.DuplicateOfRepoint += res.RowsAffected

	// 6. comments — repoint entity_id (no unique constraint on
	// (entity_type, entity_id) for comments — same comment thread
	// just gets renamed under the winner).
	res = tx.Exec(`
		UPDATE comments
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("comments: %w", res.Error)
	}
	summary.CommentsRepointed += res.RowsAffected

	// 7. comment_subscriptions — PK is (user_id, entity_type,
	// entity_id). Drop conflicts then repoint.
	moved, skipped, err = movePolymorphicEntity(tx, "comment_subscriptions",
		`(SELECT 1 FROM comment_subscriptions cs2
		   WHERE cs2.user_id = comment_subscriptions.user_id
			 AND cs2.entity_type = 'show'
			 AND cs2.entity_id = ?)`, winnerID, loserID)
	if err != nil {
		return fmt.Errorf("comment_subscriptions: %w", err)
	}
	summary.SubsRepointed += moved
	summary.SubsSkipped += skipped

	// 8. entity_tags — UNIQUE (tag_id, entity_type, entity_id) per
	// migration 000051. Drop conflicts, then move.
	moved, skipped, err = movePolymorphicEntityWithTagConflict(tx, "entity_tags", winnerID, loserID)
	if err != nil {
		return fmt.Errorf("entity_tags: %w", err)
	}
	summary.EntityTagsMoved += moved
	summary.EntityTagsSkipped += skipped

	// 9. entity_reports — repoint (no relevant unique).
	res = tx.Exec(`
		UPDATE entity_reports
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("entity_reports: %w", res.Error)
	}
	summary.EntityReportsMoved += res.RowsAffected

	// 10. pending_entity_edits — repoint.
	res = tx.Exec(`
		UPDATE pending_entity_edits
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("pending_entity_edits: %w", res.Error)
	}
	summary.PendingEditsMoved += res.RowsAffected

	// 11. revisions — repoint.
	res = tx.Exec(`
		UPDATE revisions
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("revisions: %w", res.Error)
	}
	summary.RevisionsMoved += res.RowsAffected

	// 12. requests — points via requested_entity_id, not entity_id.
	res = tx.Exec(`
		UPDATE requests
		SET requested_entity_id = ?
		WHERE entity_type = 'show' AND requested_entity_id = ?
	`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("requests: %w", res.Error)
	}
	summary.RequestsMoved += res.RowsAffected

	// 13. audit_logs — repoint.
	res = tx.Exec(`
		UPDATE audit_logs
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if res.Error != nil {
		return fmt.Errorf("audit_logs: %w", res.Error)
	}
	summary.AuditLogsMoved += res.RowsAffected

	// 14. collection_items — UNIQUE (collection_id, entity_type,
	// entity_id) handled inline; conflicts dropped.
	moved, skipped, err = moveCollectionItems(tx, winnerID, loserID)
	if err != nil {
		return fmt.Errorf("collection_items: %w", err)
	}
	summary.CollectionsMoved += moved
	summary.CollectionsSkipped += skipped

	// 15. user_bookmarks — UNIQUE (user_id, entity_type, entity_id, action).
	moved, skipped, err = moveBookmarks(tx, winnerID, loserID)
	if err != nil {
		return fmt.Errorf("user_bookmarks: %w", err)
	}
	summary.BookmarksMoved += moved
	summary.BookmarksSkipped += skipped

	// 16. Delete the loser show. CASCADE handles anything left in
	// show_venues / show_artists / show_reports / enrichment_queue
	// (i.e. nothing — all repointed above).
	if err := tx.Delete(&catalogm.Show{}, loserID).Error; err != nil {
		return fmt.Errorf("delete loser show %d: %w", loserID, err)
	}

	summary.LosersMerged++
	return nil
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

// movePolymorphicEntity is a generic conflict-aware move for tables
// keyed on (some_user_or_other_col, entity_type='show', entity_id).
// `existsClause` must use a placeholder `?` that gets bound to
// winnerID — see the comment_subscriptions call site for the shape.
func movePolymorphicEntity(tx *gorm.DB, table, existsClause string, winnerID, loserID uint) (moved, skipped int64, err error) {
	delSQL := fmt.Sprintf(`
		DELETE FROM %s
		WHERE entity_type = 'show'
		  AND entity_id = ?
		  AND EXISTS %s
	`, table, existsClause)
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

// movePolymorphicEntityWithTagConflict handles entity_tags, where the
// uniqueness key is (tag_id, entity_type, entity_id) — distinct from
// the simpler (user_id, entity_type, entity_id) shape.
func movePolymorphicEntityWithTagConflict(tx *gorm.DB, table string, winnerID, loserID uint) (moved, skipped int64, err error) {
	del := tx.Exec(`
		DELETE FROM entity_tags
		WHERE entity_type = 'show'
		  AND entity_id = ?
		  AND EXISTS (
			SELECT 1 FROM entity_tags et2
			WHERE et2.tag_id = entity_tags.tag_id
			  AND et2.entity_type = 'show'
			  AND et2.entity_id = ?
		  )
	`, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, del.Error
	}
	skipped = del.RowsAffected

	upd := tx.Exec(`
		UPDATE entity_tags
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if upd.Error != nil {
		return 0, 0, upd.Error
	}
	moved = upd.RowsAffected
	return moved, skipped, nil
}

// moveCollectionItems handles collection_items uniqueness on
// (collection_id, entity_type, entity_id).
func moveCollectionItems(tx *gorm.DB, winnerID, loserID uint) (moved, skipped int64, err error) {
	del := tx.Exec(`
		DELETE FROM collection_items
		WHERE entity_type = 'show'
		  AND entity_id = ?
		  AND EXISTS (
			SELECT 1 FROM collection_items ci2
			WHERE ci2.collection_id = collection_items.collection_id
			  AND ci2.entity_type = 'show'
			  AND ci2.entity_id = ?
		  )
	`, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, del.Error
	}
	skipped = del.RowsAffected

	upd := tx.Exec(`
		UPDATE collection_items
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
	if upd.Error != nil {
		return 0, 0, upd.Error
	}
	moved = upd.RowsAffected
	return moved, skipped, nil
}

// moveBookmarks handles user_bookmarks uniqueness on
// (user_id, entity_type, entity_id, action).
func moveBookmarks(tx *gorm.DB, winnerID, loserID uint) (moved, skipped int64, err error) {
	del := tx.Exec(`
		DELETE FROM user_bookmarks
		WHERE entity_type = 'show'
		  AND entity_id = ?
		  AND EXISTS (
			SELECT 1 FROM user_bookmarks b2
			WHERE b2.user_id = user_bookmarks.user_id
			  AND b2.action  = user_bookmarks.action
			  AND b2.entity_type = 'show'
			  AND b2.entity_id = ?
		  )
	`, loserID, winnerID)
	if del.Error != nil {
		return 0, 0, del.Error
	}
	skipped = del.RowsAffected

	upd := tx.Exec(`
		UPDATE user_bookmarks
		SET entity_id = ?
		WHERE entity_type = 'show' AND entity_id = ?
	`, winnerID, loserID)
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
