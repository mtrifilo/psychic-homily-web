package catalog

import (
	"context"
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
//
// ctx is threaded to the queries so an abandoned request (the generator gives
// up at 30 s) does not leave three unbounded scans running to completion.
func (s *SitemapService) Entries(ctx context.Context) (*contracts.SitemapEntries, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Only approved shows are publicly reachable: GetShowHandler
	// (internal/api/handlers/catalog/show.go — the access-control branch that
	// 404s unless the caller is an admin or the submitter) rejects every other
	// status, so advertising them would fill the index with dead URLs. That
	// reachability rule is enforced in many places across the catalog services;
	// this is one more, and they can drift — if another status is ever added,
	// grep for ShowStatusApproved rather than trusting any single site to be
	// authoritative.
	shows, err := s.entriesFor(
		ctx,
		s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusApproved),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to collect show sitemap entries: %w", err)
	}

	// Artists and venues have no visibility column — every row with a slug has a
	// reachable page. (Venue.Verified redacts address fields; it does not gate
	// the page.)
	artists, err := s.entriesFor(ctx, s.db.Model(&catalogm.Artist{}))
	if err != nil {
		return nil, fmt.Errorf("failed to collect artist sitemap entries: %w", err)
	}

	venues, err := s.entriesFor(ctx, s.db.Model(&catalogm.Venue{}))
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
// one has no canonical URL to index. (GenerateSlug returns "" for an all-
// non-ASCII name, hence the empty-string check as well as the NULL one.)
//
// Taking a scope keeps each family's visibility predicate at the call site
// where it can be read next to the model it applies to.
//
// CONSTRAINT: the scope must resolve to a SINGLE table. slug and updated_at are
// referenced unqualified here, so a joined scope will either error with
// "column reference is ambiguous" or silently bind the wrong table's
// updated_at. A family needing a join wants its own projection, not this.
func (s *SitemapService) entriesFor(ctx context.Context, scope *gorm.DB) ([]contracts.SitemapEntry, error) {
	// Deterministic order, so two fetches of an unchanged catalogue diff
	// cleanly. Ordered by slug rather than recency because the partial unique
	// index on slug (migration 000013) can supply that order, while no index on
	// updated_at exists for any of these tables — sorting on it would buy a
	// guaranteed sort node for an ordering no consumer reads.
	entries := []contracts.SitemapEntry{}
	err := scope.
		WithContext(ctx).
		Where("slug IS NOT NULL AND slug <> ''").
		Order("slug ASC").
		Select("slug", "updated_at").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}
