package catalog

import (
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// SitemapService answers the sitemap generator's one question: which slugs are
// indexable, and when did each last change.
//
// It deliberately avoids the public list services, which hydrate joins and full
// response bodies the generator throws away. That coupling is not a style
// preference — it is the defect this service exists to fix; see
// contracts.SitemapEntry for the incident. Keep the queries here projections:
// the moment this starts Preloading it inherits the same runaway payload.
type SitemapService struct {
	db *gorm.DB
}

func NewSitemapService(database *gorm.DB) *SitemapService {
	if database == nil {
		database = db.GetDB()
	}
	return &SitemapService{db: database}
}

// Entries returns the indexable slug set for every URL family the sitemap
// currently covers. A failure in any one family fails the whole call — the
// generator must never be handed a partial result.
func (s *SitemapService) Entries() (*contracts.SitemapEntries, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Only approved shows are publicly reachable: GetShowHandler 404s any other
	// status for an anonymous visitor, so advertising them would fill the index
	// with dead URLs. This is the third place that rule is written — see the
	// note on ShowStatus in the models package.
	shows, err := s.entriesFor(
		s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusApproved),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to collect show sitemap entries: %w", err)
	}

	// Artists and venues have no visibility column — every row with a slug has a
	// reachable page. (Venue.Verified redacts address fields; it does not gate
	// the page.)
	artists, err := s.entriesFor(s.db.Model(&catalogm.Artist{}))
	if err != nil {
		return nil, fmt.Errorf("failed to collect artist sitemap entries: %w", err)
	}

	venues, err := s.entriesFor(s.db.Model(&catalogm.Venue{}))
	if err != nil {
		return nil, fmt.Errorf("failed to collect venue sitemap entries: %w", err)
	}

	return &contracts.SitemapEntries{
		Shows:   shows,
		Artists: artists,
		Venues:  venues,
	}, nil
}

// entriesFor projects slug + updated_at from an already-scoped query, skipping
// rows with no slug: slug is nullable on all three models, and a row without
// one has no canonical URL to index.
//
// Taking a scope rather than a model plus filter arguments keeps the caller's
// WHERE clause compiler-checked, and lets a family whose query does not reduce
// to "one table, one predicate" reuse this unchanged.
func (s *SitemapService) entriesFor(scope *gorm.DB) ([]contracts.SitemapEntry, error) {
	// Deterministic order, so two fetches of an unchanged catalogue diff
	// cleanly. Ordered by slug rather than recency because the partial unique
	// index on slug (migration 000013) can supply that order, while no index on
	// updated_at exists for any of these tables — sorting on it would buy a
	// guaranteed sort node for an ordering no consumer reads.
	entries := []contracts.SitemapEntry{}
	err := scope.
		Where("slug IS NOT NULL AND slug <> ''").
		Order("slug ASC").
		Select("slug", "updated_at").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}
