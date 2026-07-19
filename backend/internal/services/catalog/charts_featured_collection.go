package catalog

import (
	"fmt"
	"time"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// PSY-1500: public reads over collection_feature_runs. These are the only
// read paths for the Broadsheet's live "Featured Collection" card and the
// /charts/featured-collection/history archive; the write side lives in
// community.CollectionService.SetFeatured. They live on ChartsService (rather
// than CollectionService) so the live pick can fold into the existing charts
// masthead cache tier — the card renders on the charts masthead, and adding a
// second uncached hot path was explicitly ruled out.

// featuredRunRow is the flat scan target for the feature-run + collection join.
// Creator name/username are resolved in a second batch query (the house
// user-attribution pattern) rather than joined, so the display logic in
// shared.ResolveUserName stays the single source of truth.
type featuredRunRow struct {
	RunID               uint    `gorm:"column:run_id"`
	CollectionID        uint    `gorm:"column:collection_id"`
	Title               string  `gorm:"column:title"`
	Slug                string  `gorm:"column:slug"`
	Description         string  `gorm:"column:description"`
	CoverImageURL       *string `gorm:"column:cover_image_url"`
	CreatorID           uint       `gorm:"column:creator_id"`
	ItemCount           int        `gorm:"column:item_count"`
	FeaturedAt          time.Time  `gorm:"column:featured_at"`
	UnfeaturedAt        *time.Time `gorm:"column:unfeatured_at"`
	FeaturedAtEstimated bool       `gorm:"column:featured_at_estimated"`
}

// GetFeaturedCollection returns the single live featured-collection pick: the
// open run (unfeatured_at IS NULL) with the newest featured_at, or nil when
// nothing is currently featured (PSY-1500). Cached in the masthead tier (60s)
// like the rest of the Broadsheet masthead — a nil pick caches too, so the
// "nothing featured" answer costs one query per TTL, not one per request.
func (s *ChartsService) GetFeaturedCollection() (*contracts.FeaturedCollectionRun, error) {
	return chartsCached(s.mastheadCache, "featured_collection", chartsMastheadTTL, func() (*contracts.FeaturedCollectionRun, error) {
		return s.getFeaturedCollectionUncached()
	})
}

func (s *ChartsService) getFeaturedCollectionUncached() (*contracts.FeaturedCollectionRun, error) {
	runs, err := s.queryFeaturedRuns(true, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return &runs[0], nil
}

// GetFeaturedCollectionHistory returns every featuring stint (open + closed)
// newest-first, paginated, plus the full-set total for the archive's pager
// (PSY-1500). Cached in the capped module tier because limit/offset are
// client-controlled, matching the other paginated chart pages.
func (s *ChartsService) GetFeaturedCollectionHistory(limit, offset int) ([]contracts.FeaturedCollectionRun, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	return cachedFeaturedHistory(s.cache, limit, offset, func() ([]contracts.FeaturedCollectionRun, int, error) {
		runs, err := s.queryFeaturedRuns(false, limit, offset)
		if err != nil {
			return nil, 0, err
		}
		var total int64
		if err := s.db.
			Table("collection_feature_runs AS r").
			Joins("JOIN collections c ON c.id = r.collection_id").
			Where("c.is_public = ?", true).
			Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to count feature runs: %w", err)
		}
		return runs, int(total), nil
	})
}

// queryFeaturedRuns runs the feature-run + collection join, ordered
// featured_at DESC (id DESC tiebreaker for a stable order when two runs share
// a timestamp — the backfill can stamp many rows with the same NOW()), and
// enriches each row with its curator's display name/username. openOnly narrows
// to the currently-featured set (unfeatured_at IS NULL); false returns the
// full archive.
func (s *ChartsService) queryFeaturedRuns(openOnly bool, limit, offset int) ([]contracts.FeaturedCollectionRun, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// These are PUBLIC read paths, so they only ever surface collections the
	// public can actually open (c.is_public = true) — mirroring the public
	// /collections list's PublicOnly filter. A private collection that gets
	// featured must never leak its title/description/cover through the masthead
	// card or the public archive.
	where := "WHERE c.is_public = true"
	if openOnly {
		where += " AND r.unfeatured_at IS NULL"
	}

	query := fmt.Sprintf(`
		SELECT
			r.id AS run_id,
			r.collection_id,
			c.title,
			c.slug,
			c.description,
			c.cover_image_url,
			c.creator_id,
			(SELECT COUNT(*) FROM collection_items ci WHERE ci.collection_id = c.id) AS item_count,
			r.featured_at,
			r.unfeatured_at,
			r.featured_at_estimated
		FROM collection_feature_runs r
		JOIN collections c ON c.id = r.collection_id
		%s
		ORDER BY r.featured_at DESC, r.id DESC
		LIMIT ? OFFSET ?
	`, where)

	var rows []featuredRunRow
	if err := s.db.Raw(query, limit, offset).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query featured collection runs: %w", err)
	}
	if len(rows) == 0 {
		return []contracts.FeaturedCollectionRun{}, nil
	}

	creatorIDs := make([]uint, 0, len(rows))
	seen := make(map[uint]bool, len(rows))
	for _, r := range rows {
		if !seen[r.CreatorID] {
			seen[r.CreatorID] = true
			creatorIDs = append(creatorIDs, r.CreatorID)
		}
	}
	names, err := shared.BatchResolveUserNames(s.db, creatorIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve curator names: %w", err)
	}
	usernames, err := shared.BatchResolveUserUsernames(s.db, creatorIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve curator usernames: %w", err)
	}

	results := make([]contracts.FeaturedCollectionRun, len(rows))
	for i, r := range rows {
		results[i] = contracts.FeaturedCollectionRun{
			RunID:               r.RunID,
			CollectionID:        r.CollectionID,
			Title:               r.Title,
			Slug:                r.Slug,
			Description:         r.Description,
			CoverImageURL:       r.CoverImageURL,
			CreatorID:           r.CreatorID,
			CreatorName:         names[r.CreatorID],
			CreatorUsername:     usernames[r.CreatorID],
			ItemCount:           r.ItemCount,
			FeaturedAt:          r.FeaturedAt,
			UnfeaturedAt:        r.UnfeaturedAt,
			FeaturedAtEstimated: r.FeaturedAtEstimated,
		}
	}
	return results, nil
}
