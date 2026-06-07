package catalog

import (
	"fmt"
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"

	"gorm.io/gorm"
)

// IntersectEntitiesByTags computes a cross-entity tag intersection (PSY-995):
// the entities matching the given tag intersection, grouped by entity type,
// each group carrying a full match count plus a preview (page 1 of that type's
// "show all" browse).
//
// Reuse, not new union SQL: each type's count + preview-ID lookup runs the same
// tag-filter primitive its /{type}?tags= browse uses — `ApplyTagFilter` for the
// five direct types and `ApplyTransitiveArtistTagFilter` for show/festival — and
// each preview is hydrated through the existing per-type enrich* helpers
// (enrichArtists/enrichShows/…). This guarantees the count/preview agree with the
// click-through into `/{type}?tags=<slugs>&tag_match=<match>`.
//
// Each type also replicates its browse's "active-entity" gate so the count
// matches what clicking through returns (see decision in PSY-995):
//   - artist:     no gate. /artists?tags= sets skip_active_filter=true, so a tag
//     page is an evergreen surface (every tagged artist, active or not).
//   - venue:      verified = true only (the public /venues list).
//   - show:       status = approved AND event_date >= start-of-today UTC (upcoming),
//     matching GetUpcomingShows; counted transitively via the lineup.
//   - festival:   no gate; counted transitively via the lineup.
//   - release:    no gate.
//   - label:      no gate.
//   - collection: is_public = true ONLY — private collections must never leak
//     through this public surface (load-bearing; asserted in a test).
//
// tagSlugs is assumed already resolved/validated by the caller; matchAny=false ⇒
// AND (the lineup collectively covers all tags for show/festival), true ⇒ OR.
// previewLimit bounds each group's preview slice. Groups for every valid entity
// type are returned (zero-count groups included) in canonical TagEntityTypes order.
func (s *TagService) IntersectEntitiesByTags(tagSlugs []string, matchAny bool, previewLimit int) (*contracts.TagIntersectionResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if previewLimit <= 0 {
		previewLimit = contracts.DefaultIntersectionPreviewLimit
	}

	filter := TagFilter{TagSlugs: tagSlugs, MatchAny: matchAny}

	matchMode := "all"
	if matchAny {
		matchMode = "any"
	}

	resp := &contracts.TagIntersectionResponse{
		Tags:     []contracts.TagSummary{},
		TagMatch: matchMode,
		Groups:   make([]contracts.TagIntersectionGroup, 0, len(catalogm.TagEntityTypes)),
	}

	// Echo the resolved input tags (in request order) so the chip UI can render
	// canonical names. resolveTagSummaries also validates existence: an unknown
	// slug returns *contracts.UnknownTagSlugError (mapped to 400 by the handler).
	tagSummaries, err := s.resolveTagSummaries(tagSlugs)
	if err != nil {
		return nil, err
	}
	resp.Tags = tagSummaries

	for _, et := range catalogm.TagEntityTypes {
		group, err := s.intersectGroup(et, filter, previewLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to compute %s intersection group: %w", et, err)
		}
		resp.Groups = append(resp.Groups, group)
	}

	return resp, nil
}

// resolveTagSummaries loads the TagSummary for each requested slug in a single
// batched query, preserving request order. It doubles as validation: a slug
// that doesn't resolve returns a typed *contracts.UnknownTagSlugError so the
// handler can map it to a 400 instead of yielding misleading all-zero groups.
// Returns an empty (non-nil) slice when tagSlugs is empty.
func (s *TagService) resolveTagSummaries(tagSlugs []string) ([]contracts.TagSummary, error) {
	out := make([]contracts.TagSummary, 0, len(tagSlugs))
	if len(tagSlugs) == 0 {
		return out, nil
	}
	var tags []catalogm.Tag
	if err := s.db.Where("LOWER(slug) IN ?", tagSlugs).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to load tag summaries: %w", err)
	}
	// Key by the lowercased stored slug so the lookup is robust even if a stored
	// slug ever carries uppercase; request slugs are already lowercased by
	// ParseTagFilter.
	bySlug := make(map[string]catalogm.Tag, len(tags))
	for _, t := range tags {
		bySlug[strings.ToLower(t.Slug)] = t
	}
	for _, slug := range tagSlugs {
		t, ok := bySlug[slug]
		if !ok {
			return nil, &contracts.UnknownTagSlugError{Slug: slug}
		}
		out = append(out, contracts.TagSummary{
			ID:         t.ID,
			Name:       t.Name,
			Slug:       t.Slug,
			Category:   t.Category,
			IsOfficial: t.IsOfficial,
			UsageCount: t.UsageCount,
		})
	}
	return out, nil
}

// intersectGroup computes one entity type's (count, preview) for the tag
// intersection. It builds the type's gated base query, applies the shared
// tag-filter primitive, counts, then fetches the preview IDs and hydrates them
// through the existing enrich* helper for that type.
func (s *TagService) intersectGroup(entityType string, filter TagFilter, previewLimit int) (contracts.TagIntersectionGroup, error) {
	group := contracts.TagIntersectionGroup{
		EntityType: entityType,
		Count:      0,
		Preview:    []contracts.TaggedEntityItem{},
	}

	idColumn := intersectIDColumn(entityType)

	// Build the gated + tag-filtered query fresh for each step. GORM mutates the
	// builder in place (Count injects SELECT count(*), Select/Order/Limit append
	// clauses), so reusing one *gorm.DB across Count and the preview Scan would
	// bleed state. A small closure keeps the gate + filter defined once.
	filtered := func() *gorm.DB {
		q := s.intersectBaseQuery(entityType)
		return s.applyIntersectionTagFilter(q, entityType, idColumn, filter)
	}

	// Count distinct matching entities. The tag-filter primitives use an
	// `idColumn IN (subquery)` shape, so the base rows are already distinct by
	// primary key — a plain Count is correct.
	var total int64
	if err := filtered().Count(&total).Error; err != nil {
		return group, fmt.Errorf("count failed: %w", err)
	}
	group.Count = total
	if total == 0 {
		return group, nil
	}

	// Preview IDs: page 1 in the same default sort the per-type browse uses.
	var ids []uint
	if err := filtered().
		Select(idColumn).
		Order(s.intersectPreviewOrder(entityType)).
		Limit(previewLimit).
		Scan(&ids).Error; err != nil {
		return group, fmt.Errorf("preview id scan failed: %w", err)
	}
	if len(ids) == 0 {
		return group, nil
	}

	enriched := s.enrichForType(entityType, ids)
	preview := make([]contracts.TaggedEntityItem, 0, len(ids))
	for _, id := range ids {
		item, ok := enriched[id]
		if !ok {
			// Collections drop private/deleted rows in enrichCollections; skip
			// rather than emit an empty-name placeholder. (Belt-and-suspenders:
			// the collection base query already filters is_public = true.)
			continue
		}
		item.EntityType = entityType
		item.EntityID = id
		preview = append(preview, item)
	}
	group.Preview = preview
	return group, nil
}

// intersectIDColumn returns the fully-qualified primary-key column the
// tag-filter primitives constrain for a given entity type's base table.
func intersectIDColumn(entityType string) string {
	switch entityType {
	case catalogm.TagEntityArtist:
		return "artists.id"
	case catalogm.TagEntityVenue:
		return "venues.id"
	case catalogm.TagEntityRelease:
		return "releases.id"
	case catalogm.TagEntityLabel:
		return "labels.id"
	case catalogm.TagEntityShow:
		return "shows.id"
	case catalogm.TagEntityFestival:
		return "festivals.id"
	case catalogm.TagEntityCollection:
		return "collections.id"
	default:
		return "entity_tags.entity_id"
	}
}

// intersectBaseQuery returns the gated base query for an entity type. The gate
// replicates the type's public browse so the intersection count agrees with the
// `/{type}?tags=` click-through.
func (s *TagService) intersectBaseQuery(entityType string) *gorm.DB {
	switch entityType {
	case catalogm.TagEntityArtist:
		// Evergreen: /artists?tags= drops the upcoming-show activity gate.
		return s.db.Table("artists")
	case catalogm.TagEntityVenue:
		// Public /venues lists verified venues only.
		return s.db.Table("venues").Where("venues.verified = ?", true)
	case catalogm.TagEntityRelease:
		return s.db.Table("releases")
	case catalogm.TagEntityLabel:
		return s.db.Table("labels")
	case catalogm.TagEntityShow:
		// Shows: approved + upcoming (event_date >= start of today, UTC). This is
		// a discovery surface, so past shows are excluded from the count. The
		// boundary is UTC start-of-day — intentionally coarser than ShowService's
		// timezone-aware upcoming filter, because this endpoint is city-agnostic
		// and carries no request timezone (see startOfTodayUTC). PSY-993 owns the
		// "show all shows" link target and must point it at an upcoming-scoped
		// shows surface so the linked list agrees with this count.
		return s.db.Table("shows").
			Where("shows.status = ?", catalogm.ShowStatusApproved).
			Where("shows.event_date >= ?", startOfTodayUTC())
	case catalogm.TagEntityFestival:
		return s.db.Table("festivals")
	case catalogm.TagEntityCollection:
		// Public-only: private collections must never leak through this surface.
		return s.db.Table("collections").Where("collections.is_public = ?", true)
	default:
		// Unreachable: entityType comes from TagEntityTypes. Return a query that
		// matches nothing so a future type addition fails closed (0 count).
		return s.db.Table("entity_tags").Where("1 = 0")
	}
}

// applyIntersectionTagFilter narrows the base query by the tag filter, choosing
// the direct (entity owns its tags) or transitive (lineup-cover) primitive to
// match each type's browse semantics.
func (s *TagService) applyIntersectionTagFilter(query *gorm.DB, entityType, idColumn string, filter TagFilter) *gorm.DB {
	switch entityType {
	case catalogm.TagEntityShow:
		return ApplyTransitiveArtistTagFilter(query, s.db, "show_artists", "show_id", "artist_id", idColumn, filter)
	case catalogm.TagEntityFestival:
		return ApplyTransitiveArtistTagFilter(query, s.db, "festival_artists", "festival_id", "artist_id", idColumn, filter)
	default:
		return ApplyTagFilter(query, s.db, entityType, idColumn, filter)
	}
}

// intersectPreviewOrder returns the ORDER BY clause for a type's preview so the
// preview matches page 1 of the per-type browse's default sort.
func (s *TagService) intersectPreviewOrder(entityType string) string {
	switch entityType {
	case catalogm.TagEntityShow:
		// GetUpcomingShows: soonest first.
		return "shows.event_date ASC, shows.id ASC"
	case catalogm.TagEntityFestival:
		// ListFestivals: most recent edition first.
		return "festivals.start_date DESC, festivals.name ASC"
	case catalogm.TagEntityRelease:
		// ListReleases default ("newest"): newest year first.
		return "releases.release_year DESC NULLS LAST, releases.title ASC"
	case catalogm.TagEntityArtist:
		return "artists.name ASC"
	case catalogm.TagEntityVenue:
		return "venues.name ASC"
	case catalogm.TagEntityLabel:
		return "labels.name ASC"
	case catalogm.TagEntityCollection:
		return "collections.updated_at DESC, collections.id DESC"
	default:
		return "entity_tags.entity_id ASC"
	}
}

// enrichForType dispatches to the existing per-type enrichment helper (shared
// with GetTagEntities). Reusing these keeps the intersection preview's card
// shape identical to the single-tag detail page's cards.
func (s *TagService) enrichForType(entityType string, ids []uint) map[uint]contracts.TaggedEntityItem {
	switch entityType {
	case catalogm.TagEntityArtist:
		return s.enrichArtists(ids)
	case catalogm.TagEntityVenue:
		return s.enrichVenues(ids)
	case catalogm.TagEntityFestival:
		return s.enrichFestivals(ids)
	case catalogm.TagEntityLabel:
		return s.enrichLabels(ids)
	case catalogm.TagEntityRelease:
		return s.enrichReleases(ids)
	case catalogm.TagEntityShow:
		return s.enrichShows(ids)
	case catalogm.TagEntityCollection:
		return s.enrichCollections(ids)
	default:
		return s.enrichBare(entityType, ids)
	}
}

// startOfTodayUTC returns midnight UTC for the current day — the lower bound for
// the upcoming-show gate. ShowService's upcoming filter computes start-of-day in
// the request timezone; this endpoint is city-agnostic (no request timezone), so
// it uses UTC start-of-day as the global default. The two boundaries can differ
// by up to a day near the UTC/local date line — an accepted trade-off for a
// tag-discovery count, NOT a claim of byte-parity with ShowService.
func startOfTodayUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
