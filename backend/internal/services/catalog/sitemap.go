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
// preference — it is the defect this service exists to fix. Keep the queries
// here projections; the moment this starts Preloading it inherits the same
// runaway payload.
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
// currently covers.
//
// A failure in any one family fails the whole call. The generator must not be
// handed a partial result: a response missing an entity family is
// indistinguishable from that family being legitimately empty, and publishing
// it silently drops thousands of URLs out of the index.
func (s *SitemapService) Entries() (*contracts.SitemapEntries, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Only approved shows are publicly reachable — pending, rejected and private
	// shows 404 for anonymous visitors, so advertising them would fill the index
	// with dead URLs. Mirrors the status gate in ShowService.GetShows.
	shows, err := s.entriesFor(&catalogm.Show{}, "status = ?", catalogm.ShowStatusApproved)
	if err != nil {
		return nil, fmt.Errorf("failed to collect show sitemap entries: %w", err)
	}

	artists, err := s.entriesFor(&catalogm.Artist{})
	if err != nil {
		return nil, fmt.Errorf("failed to collect artist sitemap entries: %w", err)
	}

	venues, err := s.entriesFor(&catalogm.Venue{})
	if err != nil {
		return nil, fmt.Errorf("failed to collect venue sitemap entries: %w", err)
	}

	return &contracts.SitemapEntries{
		Shows:   shows,
		Artists: artists,
		Venues:  venues,
	}, nil
}

// entriesFor projects slug + updated_at for one model, skipping rows with no
// slug. Slug is a nullable column on all three models, and a row without one
// has no canonical URL to index.
func (s *SitemapService) entriesFor(model any, extraWhere ...any) ([]contracts.SitemapEntry, error) {
	query := s.db.Model(model).Where("slug IS NOT NULL AND slug <> ''")

	if len(extraWhere) > 0 {
		where, ok := extraWhere[0].(string)
		if !ok {
			return nil, fmt.Errorf("extra where clause must be a string")
		}
		query = query.Where(where, extraWhere[1:]...)
	}

	// Freshest first, slug as the tiebreak so the ordering is total: equal
	// timestamps are common after a bulk ingest, and a stable order is what lets
	// the freshness monitor diff two responses meaningfully.
	query = query.Order("updated_at DESC, slug ASC")

	// Non-nil empty slice, so an empty family serialises as [] rather than null
	// and the generator can iterate it without a nil check.
	entries := []contracts.SitemapEntry{}
	if err := query.Select("slug", "updated_at").Scan(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}
