package catalog

import (
	"fmt"
	"sort"
	"strings"
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
		{"pending_entity_edits", `UPDATE pending_entity_edits SET entity_id = ? WHERE entity_type = 'show' AND entity_id = ?`, &summary.PendingEditsMoved},
		{"revisions", `UPDATE revisions SET entity_id = ? WHERE entity_type = 'show' AND entity_id = ?`, &summary.RevisionsMoved},
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
