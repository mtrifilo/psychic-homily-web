package catalog

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// TagService handles tag business logic.
type TagService struct {
	db *gorm.DB
	md *utils.MarkdownRenderer
}

// NewTagService creates a new tag service.
func NewTagService(database *gorm.DB) *TagService {
	if database == nil {
		database = db.GetDB()
	}
	return &TagService{
		db: database,
		md: utils.NewMarkdownRenderer(),
	}
}

// ──────────────────────────────────────────────
// CRUD
// ──────────────────────────────────────────────

// CreateTag creates a new tag. If userID is non-nil, it records who created the tag.
func (s *TagService) CreateTag(name string, description *string, parentID *uint, category string, isOfficial bool, userID *uint) (*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !catalogm.IsValidTagCategory(category) {
		return nil, fmt.Errorf("invalid tag category: %s", category)
	}

	// Check for duplicate name (case-insensitive)
	var existing catalogm.Tag
	if err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&existing).Error; err == nil {
		return nil, apperrors.ErrTagExists(name)
	}

	// Generate slug
	baseSlug := utils.GenerateSlug(name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&catalogm.Tag{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	tag := &catalogm.Tag{
		Name:            name,
		Slug:            slug,
		Description:     description,
		ParentID:        parentID,
		Category:        category,
		IsOfficial:      isOfficial,
		CreatedByUserID: userID,
	}

	if err := s.db.Create(tag).Error; err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	// Reload with relationships (CreatedBy, Parent, Children, Aliases) for response
	return s.GetTag(tag.ID)
}

// applyVisibleUsageCounts replaces each tag's UsageCount with the count of
// entity_tags rows a public reader may see, and does the same for any preloaded
// Parent and Children.
//
// Applied on the read paths that feed a response body, so no handler has to
// remember: what a caller reads off a tag is the visible count.
// services/shared/tag_usage.go is the single home for what the number means and
// for the enumeration of the places that still read the raw column; this comment
// deliberately does not restate that list.
//
// A tag with no visible rows counts zero, which is a missing key in the map.
func (s *TagService) applyVisibleUsageCounts(tags []*catalogm.Tag) error {
	ids := make([]uint, 0, len(tags)*2)
	for _, t := range tags {
		if t == nil {
			continue
		}
		ids = append(ids, t.ID)
		if t.Parent != nil {
			ids = append(ids, t.Parent.ID)
		}
		for i := range t.Children {
			ids = append(ids, t.Children[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	counts, err := shared.VisibleTagUsageCounts(s.db, ids)
	if err != nil {
		return err
	}
	for _, t := range tags {
		if t == nil {
			continue
		}
		t.UsageCount = counts[t.ID]
		if t.Parent != nil {
			t.Parent.UsageCount = counts[t.Parent.ID]
		}
		for i := range t.Children {
			t.Children[i].UsageCount = counts[t.Children[i].ID]
		}
	}
	return nil
}

// applyVisibleUsageCountsToSummaries is applyVisibleUsageCounts for the
// already-projected summary shape.
func (s *TagService) applyVisibleUsageCountsToSummaries(summaries []contracts.TagSummary) error {
	if len(summaries) == 0 {
		return nil
	}
	ids := make([]uint, len(summaries))
	for i := range summaries {
		ids[i] = summaries[i].ID
	}
	counts, err := shared.VisibleTagUsageCounts(s.db, ids)
	if err != nil {
		return err
	}
	for i := range summaries {
		summaries[i].UsageCount = counts[summaries[i].ID]
	}
	return nil
}

// GetTag retrieves a tag by ID with relationships.
func (s *TagService) GetTag(tagID uint) (*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tag catalogm.Tag
	err := s.db.Preload("Parent").Preload("Children").Preload("Aliases").Preload("CreatedBy").First(&tag, tagID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	if err := s.applyVisibleUsageCounts([]*catalogm.Tag{&tag}); err != nil {
		return nil, err
	}

	return &tag, nil
}

// GetTagBySlug retrieves a tag by slug with relationships.
func (s *TagService) GetTagBySlug(slug string) (*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tag catalogm.Tag
	err := s.db.Preload("Parent").Preload("Children").Preload("Aliases").Preload("CreatedBy").
		Where("slug = ?", slug).First(&tag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tag by slug: %w", err)
	}

	if err := s.applyVisibleUsageCounts([]*catalogm.Tag{&tag}); err != nil {
		return nil, err
	}

	return &tag, nil
}

// ListTags retrieves tags with optional filtering and sorting.
//
// When entityType is non-empty, the returned tags' UsageCount is replaced with
// the count of entity_tags rows for that specific entity type (per PSY-484).
// The persisted tag.usage_count column on the tags table is NOT mutated — it
// remains the global "applied across all entity types" count used by /tags
// browse. Sort by "usage" honours the per-entity-type count when entityType
// is set so the facet on each browse page lists the most-applicable tags
// first. entityType is validated against TagEntityTypes; an unknown value
// returns an error.
//
// cities scopes the per-tag count to a set of (city, state) pairs (PSY-982).
// It is honoured only for entityType=="show": when the /shows facet carries the
// active city filter, the count reflects shows in those cities so a non-zero
// chip can never dead-end at "0 shows". cities is ignored for every other
// entity type (no city-scoped facet exists for them today).
func (s *TagService) ListTags(category string, search string, parentID *uint, sort string, limit, offset int, entityType string, cities []contracts.CityStateFilter) ([]catalogm.Tag, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if entityType != "" && !catalogm.IsValidTagEntityType(entityType) {
		return nil, 0, fmt.Errorf("invalid entity type: %s", entityType)
	}

	query := s.db.Model(&catalogm.Tag{})

	if category != "" {
		query = query.Where("category = ?", category)
	}
	if search != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", shared.LikePattern(search))
	}
	if parentID != nil {
		query = query.Where("parent_id = ?", *parentID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count tags: %w", err)
	}

	switch sort {
	case "name":
		query = query.Order("name ASC")
	case "created":
		query = query.Order("created_at DESC")
	default: // "usage" or empty
		query = query.Order("usage_count DESC, name ASC")
	}

	if limit <= 0 {
		limit = 50
	}
	query = query.Limit(limit).Offset(offset)

	var tags []catalogm.Tag
	if err := query.Find(&tags).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list tags: %w", err)
	}

	if entityType != "" && len(tags) > 0 {
		ids := make([]uint, len(tags))
		for i, t := range tags {
			ids[i] = t.ID
		}
		countByID, err := s.computeEntityTypeTagCounts(entityType, ids, cities)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to compute per-entity-type tag counts: %w", err)
		}
		for i := range tags {
			tags[i].UsageCount = int(countByID[tags[i].ID])
		}
		// When sorting by usage, re-order in memory so the entity-scoped count
		// drives the order (the original SQL sorted by the global usage_count).
		// Stable secondary sort: name ASC (matches the SQL ORDER BY tiebreaker).
		if sort == "" || sort == "usage" {
			sortTagsByUsageDesc(tags)
		}
		return tags, total, nil
	}

	// The GLOBAL count, on the same terms the per-entity-type count above is
	// already gated: the ORDER BY above ranked on the raw column, and what the
	// caller reads is the visible count.
	//
	// NOT RE-SORTED, unlike the entity-scoped branch. That branch re-sorts a page
	// whose ordering key it has just replaced wholesale; here the SQL ORDER BY is
	// what chose WHICH rows this page holds, so re-sorting them would order the
	// page by one key and select it by another — a tag would sit at the bottom of
	// page 1 while a higher-counting tag sat on page 2. The listing is ordered by
	// raw popularity and the numbers beside it are the visible counts; see
	// services/shared/tag_usage.go, which names that as the trade.
	if err := s.applyVisibleUsageCounts(tagPointers(tags)); err != nil {
		return nil, 0, fmt.Errorf("failed to compute visible tag usage counts: %w", err)
	}

	return tags, total, nil
}

// tagPointers addresses each element of tags so a helper can write back into the
// slice rather than into a copy.
func tagPointers(tags []catalogm.Tag) []*catalogm.Tag {
	out := make([]*catalogm.Tag, len(tags))
	for i := range tags {
		out[i] = &tags[i]
	}
	return out
}

// computeEntityTypeTagCounts returns a map of tag_id → usage count scoped to
// the given entity type, for the provided tag IDs.
//
// For most entity types this is the direct count of `entity_tags` rows where
// `entity_type = ? AND tag_id IN (?)`. For `show` and `festival` the count
// is computed *transitively* through the lineup (PSY-499): a show matches a
// tag when any billed artist has it, even though no show is directly tagged
// with genre/locale tags. This keeps the facet count honest ("shoegaze: 3
// shows" instead of the misleading "0" when 3 shows have shoegaze-tagged
// artists on the bill).
//
// cities further narrows the show count to the active city filter (PSY-982).
// It applies only to entityType=="show"; it is ignored for every other type.
//
// Tags absent from the relevant tables get 0 (missing from the returned map).
func (s *TagService) computeEntityTypeTagCounts(entityType string, tagIDs []uint, cities []contracts.CityStateFilter) (map[uint]int64, error) {
	switch entityType {
	case catalogm.TagEntityShow:
		if len(cities) > 0 {
			return CountTransitiveArtistTagUsageInShowCities(s.db, tagIDs, cities)
		}
		return CountTransitiveArtistTagUsage(s.db, "show_artists", "show_id", "artist_id", tagIDs)
	case catalogm.TagEntityFestival:
		return CountTransitiveArtistTagUsage(s.db, "festival_artists", "festival_id", "artist_id", tagIDs)
	}
	// Default: direct count against entity_tags for the given entity type.
	//
	// GATED FOR COLLECTIONS. This count is rendered on the same anonymous tag
	// listing whose per-tag membership GetTagEntities now filters, so counting
	// private collections here would report, by subtraction, how many carry each
	// tag. Public tier, matching the listing it is read beside.
	type countRow struct {
		TagID uint
		Count int64
	}
	var rows []countRow
	query := s.db.Table("entity_tags").
		Select("tag_id, COUNT(*) AS count").
		Where("entity_type = ? AND tag_id IN ?", entityType, tagIDs)
	if entityType == catalogm.TagEntityCollection {
		visible, visibleArgs := shared.VisibleCollectionExistsSQL(
			"entity_tags.entity_id", contracts.ShowViewer{})
		query = query.Where(visible, visibleArgs...)
	}
	err := query.
		Group("tag_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]int64, len(rows))
	for _, r := range rows {
		out[r.TagID] = r.Count
	}
	return out, nil
}

// sortTagsByUsageDesc sorts in place by UsageCount DESC, name ASC. Used after
// per-entity-type recount so the displayed order matches the new counts.
func sortTagsByUsageDesc(tags []catalogm.Tag) {
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].UsageCount != tags[j].UsageCount {
			return tags[i].UsageCount > tags[j].UsageCount
		}
		return tags[i].Name < tags[j].Name
	})
}

// UpdateTag updates a tag's fields.
func (s *TagService) UpdateTag(tagID uint, name *string, description *string, parentID *uint, category *string, isOfficial *bool) (*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tag catalogm.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTagNotFound(tagID)
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	updates := map[string]interface{}{}
	if name != nil {
		updates["name"] = *name
		// Regenerate slug
		baseSlug := utils.GenerateSlug(*name)
		slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&catalogm.Tag{}).Where("slug = ? AND id != ?", candidate, tagID).Count(&count)
			return count > 0
		})
		updates["slug"] = slug
	}
	if description != nil {
		updates["description"] = *description
	}
	if parentID != nil {
		// Hierarchy edits go through the same cycle + category guard that
		// SetTagParent uses (PSY-311). Keep the check here so any caller
		// that sets parent_id via generic update can't bypass validation.
		if tag.Category != catalogm.TagCategoryGenre {
			return nil, apperrors.ErrTagHierarchyNotGenre(tag.Name, tag.Category)
		}
		if err := s.validateTagParent(&tag, parentID); err != nil {
			return nil, err
		}
		updates["parent_id"] = *parentID
	}
	if category != nil {
		if !catalogm.IsValidTagCategory(*category) {
			return nil, fmt.Errorf("invalid tag category: %s", *category)
		}
		updates["category"] = *category
	}
	if isOfficial != nil {
		updates["is_official"] = *isOfficial
	}

	if len(updates) > 0 {
		if err := s.db.Model(&tag).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update tag: %w", err)
		}
	}

	return s.GetTag(tagID)
}

// DeleteTag deletes a tag and all associated data (cascaded by FK).
func (s *TagService) DeleteTag(tagID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&catalogm.Tag{}, tagID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete tag: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrTagNotFound(tagID)
	}

	return nil
}

// ──────────────────────────────────────────────
// Entity tagging
// ──────────────────────────────────────────────

// AddTagToEntity applies a tag to an entity. Supports tag ID or name (with alias resolution).
// If tagName is provided and no existing tag or alias matches, creates the tag inline
// for contributor+ users. The category parameter is used when creating new tags (defaults to "other").
//
// PSY-354: collections enforce a per-entity cap (MaxCollectionTags = 10). The
// cap is checked HERE (before tag resolution / inline creation) so the same
// limit applies whether the request comes through the dedicated
// /collections/{slug}/tags endpoint or the polymorphic
// /entities/collection/{id}/tags endpoint. Other entity types are uncapped
// today; the if-statement keeps the bounded knowledge to the one entity that
// has a cap rather than threading a per-entity-type limit map through.
func (s *TagService) AddTagToEntity(tagID uint, tagName string, entityType string, entityID uint, userID uint, category string) (*catalogm.EntityTag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !catalogm.IsValidTagEntityType(entityType) {
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}

	if entityType == catalogm.TagEntityCollection {
		var existingCount int64
		if err := s.db.Model(&catalogm.EntityTag{}).
			Where("entity_type = ? AND entity_id = ?", entityType, entityID).
			Count(&existingCount).Error; err != nil {
			return nil, fmt.Errorf("failed to count collection tags: %w", err)
		}
		if int(existingCount) >= contracts.MaxCollectionTags {
			return nil, apperrors.ErrCollectionTagLimitExceeded(int(existingCount), contracts.MaxCollectionTags)
		}
	}

	// Resolve tag by ID or name
	var tag *catalogm.Tag
	var createdInline bool
	if tagID > 0 {
		var t catalogm.Tag
		if err := s.db.First(&t, tagID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.ErrTagNotFound(tagID)
			}
			return nil, fmt.Errorf("failed to get tag: %w", err)
		}
		tag = &t
	} else if tagName != "" {
		// Try alias resolution first, then name lookup
		resolved, err := s.ResolveAlias(tagName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve tag: %w", err)
		}
		if resolved != nil {
			tag = resolved
		} else {
			// Try direct name match
			var t catalogm.Tag
			if err := s.db.Where("LOWER(name) = LOWER(?)", tagName).First(&t).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// Tag not found — create inline if user has permission
					newTag, createErr := s.createTagInline(tagName, category, userID)
					if createErr != nil {
						return nil, createErr
					}
					tag = newTag
					createdInline = true
				} else {
					return nil, fmt.Errorf("failed to find tag by name: %w", err)
				}
			} else {
				tag = &t
			}
		}
	} else {
		return nil, fmt.Errorf("tag_id or tag_name is required")
	}

	// Check for existing application
	var existing catalogm.EntityTag
	err := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tag.ID, entityType, entityID).
		First(&existing).Error
	if err == nil {
		return nil, apperrors.ErrEntityTagExists(tag.ID, entityType, entityID)
	}

	entityTag := &catalogm.EntityTag{
		TagID:         tag.ID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: userID,
	}

	if err := s.db.Create(entityTag).Error; err != nil {
		return nil, fmt.Errorf("failed to add tag to entity: %w", err)
	}

	// Increment usage count atomically
	s.db.Model(&catalogm.Tag{}).Where("id = ?", tag.ID).
		Update("usage_count", gorm.Expr("usage_count + 1"))

	// Auto-upvote for the creator when tag was created inline
	if createdInline {
		autoVote := catalogm.TagVote{
			TagID:      tag.ID,
			EntityType: entityType,
			EntityID:   entityID,
			UserID:     userID,
			Vote:       1,
		}
		// Fire-and-forget: don't fail the parent operation
		s.db.Create(&autoVote)
	}

	return entityTag, nil
}

// createTagInline creates a new tag as part of the AddTagToEntity flow.
// Only contributor+ users can create tags inline; new_user gets a 403.
func (s *TagService) createTagInline(tagName string, category string, userID uint) (*catalogm.Tag, error) {
	// Look up user to check trust tier
	var user authm.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	// Gate on trust tier: new_user cannot create tags
	if user.UserTier == "new_user" && !user.IsAdmin {
		return nil, apperrors.ErrTagCreationForbidden()
	}

	// Normalize the tag name
	normalized := NormalizeTagName(tagName)
	if len(normalized) < 2 {
		return nil, apperrors.ErrTagNameInvalid("must be at least 2 characters after normalization")
	}
	if len(normalized) > 50 {
		return nil, apperrors.ErrTagNameInvalid("must be 50 characters or fewer after normalization")
	}

	// Default category
	if category == "" {
		category = "other"
	}
	if !catalogm.IsValidTagCategory(category) {
		return nil, fmt.Errorf("invalid tag category: %s", category)
	}

	// Check for duplicate after normalization (case-insensitive)
	var existing catalogm.Tag
	if err := s.db.Where("LOWER(name) = LOWER(?)", normalized).First(&existing).Error; err == nil {
		// Already exists after normalization — use existing
		return &existing, nil
	}

	// Generate slug
	baseSlug := utils.GenerateSlug(normalized)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&catalogm.Tag{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	tag := &catalogm.Tag{
		Name:       normalized,
		Slug:       slug,
		Category:   category,
		IsOfficial: false,
	}

	if err := s.db.Create(tag).Error; err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return tag, nil
}

// NormalizeTagName normalizes a tag name for consistent storage.
// Lowercases, replaces whitespace with hyphens, strips non-alphanumeric
// except hyphens, collapses multiple hyphens, and trims.
func NormalizeTagName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Replace whitespace with hyphens
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, "-")
	// Strip non-alphanumeric except hyphens
	name = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(name, "")
	// Collapse multiple hyphens
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
	// Trim hyphens from edges
	name = strings.Trim(name, "-")
	return name
}

// RemoveTagFromEntity removes a tag from an entity.
func (s *TagService) RemoveTagFromEntity(tagID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tagID, entityType, entityID).
		Delete(&catalogm.EntityTag{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove tag from entity: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrEntityTagNotFound(tagID, entityType, entityID)
	}

	// Decrement usage count atomically (floor at 0)
	s.db.Model(&catalogm.Tag{}).Where("id = ? AND usage_count > 0", tagID).
		Update("usage_count", gorm.Expr("usage_count - 1"))

	// Clean up votes for this tag-entity pair
	s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tagID, entityType, entityID).
		Delete(&catalogm.TagVote{})

	return nil
}

// ListEntityTags lists all tags on an entity with vote counts.
// Pass userID=0 for unauthenticated requests.
func (s *TagService) ListEntityTags(entityType string, entityID uint, userID uint) ([]contracts.EntityTagResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get entity tags with tag info and added-by user
	var entityTags []catalogm.EntityTag
	err := s.db.Preload("Tag").Preload("AddedBy").
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Find(&entityTags).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list entity tags: %w", err)
	}

	responses := make([]contracts.EntityTagResponse, len(entityTags))
	for i, et := range entityTags {
		// Count votes
		var upvotes, downvotes int64
		s.db.Model(&catalogm.TagVote{}).
			Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND vote = 1", et.TagID, entityType, entityID).
			Count(&upvotes)
		s.db.Model(&catalogm.TagVote{}).
			Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND vote = -1", et.TagID, entityType, entityID).
			Count(&downvotes)

		resp := contracts.EntityTagResponse{
			TagID:       et.TagID,
			Name:        et.Tag.Name,
			Slug:        et.Tag.Slug,
			Category:    et.Tag.Category,
			IsOfficial:  et.Tag.IsOfficial,
			Upvotes:     int(upvotes),
			Downvotes:   int(downvotes),
			WilsonScore: wilsonScore(int(upvotes), int(downvotes)),
		}

		// Attribution (PSY-479): always surface the FK + timestamp so the
		// frontend can render a provenance line on the hover card. AddedBy
		// is preloaded above; copy the loaded fields by value so the response
		// owns its memory and the loop variable can be reused. Username is a
		// pointer because some legacy/seed users have username=null — let the
		// frontend distinguish that ("Source: system seed") from a real user.
		addedByID := et.AddedByUserID
		resp.AddedByUserID = &addedByID
		addedAt := et.CreatedAt
		resp.AddedAt = &addedAt
		if et.AddedBy.Username != nil && *et.AddedBy.Username != "" {
			username := *et.AddedBy.Username
			resp.AddedByUsername = &username
		}

		// Include user's vote if authenticated
		if userID > 0 {
			var vote catalogm.TagVote
			err := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND user_id = ?",
				et.TagID, entityType, entityID, userID).First(&vote).Error
			if err == nil {
				resp.UserVote = &vote.Vote
			}
		}

		responses[i] = resp
	}

	return responses, nil
}

// ──────────────────────────────────────────────
// Voting
// ──────────────────────────────────────────────

// VoteOnTag adds or updates a vote on a tag-entity pair.
func (s *TagService) VoteOnTag(tagID uint, entityType string, entityID uint, userID uint, isUpvote bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Verify the tag is actually applied to this entity
	var entityTag catalogm.EntityTag
	err := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tagID, entityType, entityID).
		First(&entityTag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrEntityTagNotFound(tagID, entityType, entityID)
		}
		return fmt.Errorf("failed to verify entity tag: %w", err)
	}

	voteValue := -1
	if isUpvote {
		voteValue = 1
	}

	// Upsert vote
	var existing catalogm.TagVote
	err = s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND user_id = ?",
		tagID, entityType, entityID, userID).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		vote := catalogm.TagVote{
			TagID:      tagID,
			EntityType: entityType,
			EntityID:   entityID,
			UserID:     userID,
			Vote:       voteValue,
		}
		if err := s.db.Create(&vote).Error; err != nil {
			return fmt.Errorf("failed to create vote: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check existing vote: %w", err)
	} else {
		if err := s.db.Model(&existing).Update("vote", voteValue).Error; err != nil {
			return fmt.Errorf("failed to update vote: %w", err)
		}
	}

	return nil
}

// RemoveTagVote removes a user's vote on a tag-entity pair.
func (s *TagService) RemoveTagVote(tagID uint, entityType string, entityID uint, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND user_id = ?",
		tagID, entityType, entityID, userID).Delete(&catalogm.TagVote{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove vote: %w", result.Error)
	}

	return nil
}

// ──────────────────────────────────────────────
// Aliases
// ──────────────────────────────────────────────

// CreateAlias creates a new alias for a tag.
func (s *TagService) CreateAlias(tagID uint, alias string) (*catalogm.TagAlias, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify tag exists
	var tag catalogm.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTagNotFound(tagID)
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	// Check for duplicate alias (case-insensitive)
	var existing catalogm.TagAlias
	if err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&existing).Error; err == nil {
		return nil, apperrors.ErrTagAliasExists(alias)
	}

	// Also check if alias matches an existing tag name
	var existingTag catalogm.Tag
	if err := s.db.Where("LOWER(name) = LOWER(?)", alias).First(&existingTag).Error; err == nil {
		return nil, apperrors.ErrTagAliasExists(alias)
	}

	tagAlias := &catalogm.TagAlias{
		TagID: tagID,
		Alias: alias,
	}

	if err := s.db.Create(tagAlias).Error; err != nil {
		return nil, fmt.Errorf("failed to create alias: %w", err)
	}

	return tagAlias, nil
}

// DeleteAlias removes a tag alias.
func (s *TagService) DeleteAlias(aliasID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&catalogm.TagAlias{}, aliasID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete alias: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("alias not found")
	}

	return nil
}

// ListAliases lists all aliases for a tag.
func (s *TagService) ListAliases(tagID uint) ([]catalogm.TagAlias, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var aliases []catalogm.TagAlias
	if err := s.db.Where("tag_id = ?", tagID).Order("alias ASC").Find(&aliases).Error; err != nil {
		return nil, fmt.Errorf("failed to list aliases: %w", err)
	}

	return aliases, nil
}

// ListAllAliases returns aliases across all tags paired with their canonical
// tag info. Supports a case-insensitive substring search against either the
// alias text or the canonical tag name. Default limit 50, max 500.
func (s *TagService) ListAllAliases(search string, limit, offset int) ([]contracts.TagAliasListing, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	query := s.db.
		Table("tag_aliases AS ta").
		Joins("JOIN tags AS t ON t.id = ta.tag_id")

	if search != "" {
		pattern := shared.LikePattern(strings.ToLower(search))
		query = query.Where("LOWER(ta.alias) LIKE ? OR LOWER(t.name) LIKE ?", pattern, pattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count aliases: %w", err)
	}

	var items []contracts.TagAliasListing
	err := query.
		Select(`ta.id AS id,
			ta.alias AS alias,
			ta.tag_id AS tag_id,
			t.name AS tag_name,
			t.slug AS tag_slug,
			t.category AS tag_category,
			t.is_official AS tag_is_official,
			ta.created_at AS created_at`).
		Order("ta.alias ASC").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list aliases: %w", err)
	}

	if items == nil {
		items = []contracts.TagAliasListing{}
	}
	return items, total, nil
}

// BulkImportAliases imports a list of `{alias, canonical}` pairs in one call.
// `canonical` is resolved by slug or exact name (case-insensitive). Rows with
// unknown canonicals, empty fields, or aliases that collide with existing
// aliases/tag-names are skipped and reported so the admin can see what went
// wrong. Within the same batch, earlier rows take precedence over later rows
// that would collide with them.
func (s *TagService) BulkImportAliases(items []contracts.BulkAliasImportItem) (*contracts.BulkAliasImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	result := &contracts.BulkAliasImportResult{Skipped: []contracts.BulkAliasImportSkipped{}}
	seenInBatch := map[string]struct{}{}

	for i, item := range items {
		row := i + 1
		alias := strings.TrimSpace(item.Alias)
		canonical := strings.TrimSpace(item.Canonical)

		if alias == "" || canonical == "" {
			result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
				Row:       row,
				Alias:     alias,
				Canonical: canonical,
				Reason:    "alias and canonical are required",
			})
			continue
		}

		aliasLower := strings.ToLower(alias)
		if _, dup := seenInBatch[aliasLower]; dup {
			result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
				Row:       row,
				Alias:     alias,
				Canonical: canonical,
				Reason:    "duplicate alias in batch",
			})
			continue
		}

		var tag catalogm.Tag
		err := s.db.Where("LOWER(slug) = LOWER(?) OR LOWER(name) = LOWER(?)", canonical, canonical).
			First(&tag).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
					Row:       row,
					Alias:     alias,
					Canonical: canonical,
					Reason:    fmt.Sprintf("canonical tag '%s' not found", canonical),
				})
				continue
			}
			return nil, fmt.Errorf("failed to resolve canonical tag: %w", err)
		}

		var existingAlias catalogm.TagAlias
		if err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&existingAlias).Error; err == nil {
			reason := fmt.Sprintf("alias already maps to tag ID %d", existingAlias.TagID)
			if existingAlias.TagID == tag.ID {
				reason = "alias already exists for this tag"
			}
			result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
				Row:       row,
				Alias:     alias,
				Canonical: canonical,
				Reason:    reason,
			})
			continue
		}

		var nameConflict catalogm.Tag
		if err := s.db.Where("LOWER(name) = LOWER(?)", alias).First(&nameConflict).Error; err == nil {
			result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
				Row:       row,
				Alias:     alias,
				Canonical: canonical,
				Reason:    "alias collides with existing tag name",
			})
			continue
		}

		if err := s.db.Create(&catalogm.TagAlias{TagID: tag.ID, Alias: alias}).Error; err != nil {
			result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
				Row:       row,
				Alias:     alias,
				Canonical: canonical,
				Reason:    fmt.Sprintf("create failed: %v", err),
			})
			continue
		}

		seenInBatch[aliasLower] = struct{}{}
		result.Imported++
	}

	return result, nil
}

// ResolveAlias resolves an alias to its canonical tag.
func (s *TagService) ResolveAlias(alias string) (*catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tagAlias catalogm.TagAlias
	err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&tagAlias).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to resolve alias: %w", err)
	}

	var tag catalogm.Tag
	if err := s.db.First(&tag, tagAlias.TagID).Error; err != nil {
		return nil, fmt.Errorf("failed to get canonical tag: %w", err)
	}

	return &tag, nil
}

// ──────────────────────────────────────────────
// Utility
// ──────────────────────────────────────────────

// SearchTags performs a case-insensitive search on tag names and aliases.
// If category is non-empty, results are filtered to that category.
//
// For each returned tag, MatchedAlias is populated with the specific alias
// that matched the query when the match came through `tag_aliases` rather
// than `tags.name`. This lets the autocomplete UI show the canonical form
// alongside the typed alias for transparency ("matched `punk-rock`"). If
// the tag's name matches directly, MatchedAlias is left empty even when
// an alias also happens to match — the canonical form is the signal.
func (s *TagService) SearchTags(query string, limit int, category string) ([]contracts.TagSearchResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	q := strings.ToLower(query)

	// Search tags by name and by alias, dedup by tag ID.
	// Group the OR conditions to ensure category filter applies to both.
	pattern := shared.LikePattern(q)
	db := s.db.Where(
		s.db.Where("LOWER(name) LIKE ?", pattern).
			Or("id IN (?)",
				s.db.Model(&catalogm.TagAlias{}).Select("tag_id").Where("LOWER(alias) LIKE ?", pattern),
			),
	)

	if category != "" {
		db = db.Where("category = ?", category)
	}

	var tags []catalogm.Tag
	err := db.Order("usage_count DESC").
		Limit(limit).
		Find(&tags).Error
	if err != nil {
		return nil, fmt.Errorf("failed to search tags: %w", err)
	}

	// The count a caller reads is the visible one. The ORDER BY above still ranks
	// on the raw column, which is popularity rather than a published number.
	if err := s.applyVisibleUsageCounts(tagPointers(tags)); err != nil {
		return nil, fmt.Errorf("failed to compute visible tag usage counts: %w", err)
	}

	// Build the result set. For tags whose name does NOT match the query,
	// look up which alias on that tag matched so the UI can surface the
	// provenance. A single batch query avoids N+1.
	results := make([]contracts.TagSearchResult, len(tags))

	// Collect the tag IDs whose name did not match the query — those are
	// the ones that were returned only because of an alias match.
	aliasMatchIDs := make([]uint, 0, len(tags))
	for i, tag := range tags {
		results[i] = contracts.TagSearchResult{Tag: tag}
		if !strings.Contains(strings.ToLower(tag.Name), q) {
			aliasMatchIDs = append(aliasMatchIDs, tag.ID)
		}
	}

	if len(aliasMatchIDs) > 0 {
		// Fetch matching aliases in one query, ordered so the first (lexicographic)
		// match wins deterministically when a tag has multiple matching aliases.
		var aliases []catalogm.TagAlias
		err := s.db.Where("tag_id IN ? AND LOWER(alias) LIKE ?", aliasMatchIDs, pattern).
			Order("tag_id, alias ASC").
			Find(&aliases).Error
		if err != nil {
			return nil, fmt.Errorf("failed to load matching aliases: %w", err)
		}

		matchedByTag := make(map[uint]string, len(aliases))
		for _, a := range aliases {
			if _, seen := matchedByTag[a.TagID]; seen {
				continue
			}
			matchedByTag[a.TagID] = a.Alias
		}

		for i := range results {
			if alias, ok := matchedByTag[results[i].Tag.ID]; ok {
				results[i].MatchedAlias = alias
			}
		}
	}

	return results, nil
}

// GetTrendingTags returns the most used tags, optionally filtered by category.
func (s *TagService) GetTrendingTags(limit int, category string) ([]catalogm.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	query := s.db.Model(&catalogm.Tag{}).Where("usage_count > 0")
	if category != "" {
		query = query.Where("category = ?", category)
	}

	var tags []catalogm.Tag
	if err := query.Order("usage_count DESC").Limit(limit).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to get trending tags: %w", err)
	}

	// The count a caller reads is the visible one, as on every other tag read.
	// The ORDER BY above still ranks on the raw column.
	if err := s.applyVisibleUsageCounts(tagPointers(tags)); err != nil {
		return nil, fmt.Errorf("failed to compute visible tag usage counts: %w", err)
	}

	return tags, nil
}

// PruneDownvotedTags removes entity_tags where the community has voted them irrelevant.
// A tag application is pruned when: downvotes > upvotes AND total votes >= 2.
// Official tags are immune from pruning.
func (s *TagService) PruneDownvotedTags() (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	// Find entity_tags that should be pruned
	// Subquery: tag-entity pairs with more downvotes than upvotes and at least 2 total votes
	type pruneCandidate struct {
		TagID      uint
		EntityType string
		EntityID   uint
	}

	var candidates []pruneCandidate
	err := s.db.Raw(`
		SELECT tv.tag_id, tv.entity_type, tv.entity_id
		FROM tag_votes tv
		JOIN tags t ON t.id = tv.tag_id
		WHERE t.is_official = false
		GROUP BY tv.tag_id, tv.entity_type, tv.entity_id
		HAVING COUNT(*) >= 2
		   AND SUM(CASE WHEN tv.vote = -1 THEN 1 ELSE 0 END) > SUM(CASE WHEN tv.vote = 1 THEN 1 ELSE 0 END)
	`).Scan(&candidates).Error
	if err != nil {
		return 0, fmt.Errorf("failed to find prune candidates: %w", err)
	}

	if len(candidates) == 0 {
		return 0, nil
	}

	var totalPruned int64
	for _, c := range candidates {
		result := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?",
			c.TagID, c.EntityType, c.EntityID).Delete(&catalogm.EntityTag{})
		if result.Error == nil {
			totalPruned += result.RowsAffected
			// Decrement usage count
			if result.RowsAffected > 0 {
				s.db.Model(&catalogm.Tag{}).Where("id = ? AND usage_count > 0", c.TagID).
					Update("usage_count", gorm.Expr("usage_count - 1"))
			}
		}
		// Also clean up the votes
		s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?",
			c.TagID, c.EntityType, c.EntityID).Delete(&catalogm.TagVote{})
	}

	return totalPruned, nil
}

// ──────────────────────────────────────────────
// Tag entities
// ──────────────────────────────────────────────

// entityTableMap maps entity type strings to their DB table name and name column.
var entityTableMap = map[string]struct {
	table   string
	nameCol string
}{
	"artist":   {table: "artists", nameCol: "name"},
	"venue":    {table: "venues", nameCol: "name"},
	"label":    {table: "labels", nameCol: "name"},
	"show":     {table: "shows", nameCol: "title"},
	"release":  {table: "releases", nameCol: "title"},
	"festival": {table: "festivals", nameCol: "name"},
}

// GetTagEntities returns entities tagged with a given tag, optionally filtered by entity type.
//
// PSY-485: in addition to (entity_type, entity_id, name, slug), this populates
// per-type fields (city/state, verified, upcoming_show_count, edition_year,
// release_type, etc.) so the tag detail page can render proper entity cards
// instead of bare links. Each entity type has its own enrichment query;
// failures fall back to bare info so we never lose the link.
func (s *TagService) GetTagEntities(tagID uint, entityType string, limit, offset int) ([]contracts.TaggedEntityItem, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&catalogm.EntityTag{}).Where("tag_id = ?", tagID)
	if entityType != "" {
		if !catalogm.IsValidTagEntityType(entityType) {
			return nil, 0, fmt.Errorf("invalid entity type: %s", entityType)
		}
		query = query.Where("entity_type = ?", entityType)
	}

	// Gated entities leave this listing in the QUERY, not in the assembly loop
	// below, so the total, the page and the offset all describe the same rows.
	// Dropping them later would answer a short page beside a total that counts
	// them, which is the withheld count published as arithmetic and is exactly
	// what a gate on this listing exists to prevent.
	//
	// PUBLIC tier for both arms: this route is anonymous, and a tag's membership
	// is a shared listing whose contents must not vary by credential. The
	// anonymous ShowViewer is what makes VisibleCollectionExistsSQL collapse to
	// `is_public = TRUE`, which is the rule enrichCollections applies below.
	visibleShows, visibleShowsArgs := shared.VisibleShowExistsSQL(
		"entity_tags.entity_id", contracts.ShowViewer{})
	query = query.Where(
		"(entity_tags.entity_type <> ? OR "+visibleShows+")",
		append([]interface{}{catalogm.TagEntityShow}, visibleShowsArgs...)...)
	visibleCollections, visibleCollectionsArgs := shared.VisibleCollectionExistsSQL(
		"entity_tags.entity_id", contracts.ShowViewer{})
	query = query.Where(
		"(entity_tags.entity_type <> ? OR "+visibleCollections+")",
		append([]interface{}{catalogm.TagEntityCollection}, visibleCollectionsArgs...)...)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count tagged entities: %w", err)
	}

	if total == 0 {
		return []contracts.TaggedEntityItem{}, 0, nil
	}

	// Fetch entity_tag rows
	if limit <= 0 {
		limit = 50
	}

	var entityTags []catalogm.EntityTag
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&entityTags).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list tagged entities: %w", err)
	}

	// Group entity IDs by type for batch resolution
	byType := make(map[string][]uint)
	for _, et := range entityTags {
		byType[et.EntityType] = append(byType[et.EntityType], et.EntityID)
	}

	// Per-type enrichment: builds an index keyed by entityID with all the
	// optional card fields populated. Each branch swallows query errors and
	// falls back to bare info so the response is robust if a downstream
	// table is missing or a JOIN fails.
	enrichedByType := make(map[string]map[uint]contracts.TaggedEntityItem)
	for eType, ids := range byType {
		switch eType {
		case "artist":
			enrichedByType[eType] = s.enrichArtists(ids)
		case "venue":
			enrichedByType[eType] = s.enrichVenues(ids)
		case "festival":
			enrichedByType[eType] = s.enrichFestivals(ids)
		case "label":
			enrichedByType[eType] = s.enrichLabels(ids)
		case "release":
			enrichedByType[eType] = s.enrichReleases(ids)
		case "show":
			enrichedByType[eType] = s.enrichShows(ids)
		case "collection":
			enrichedByType[eType] = s.enrichCollections(ids)
		default:
			enrichedByType[eType] = s.enrichBare(eType, ids)
		}
	}

	// Build response in the same order as entityTags (preserves created_at DESC).
	items := make([]contracts.TaggedEntityItem, 0, len(entityTags))
	for _, et := range entityTags {
		item := contracts.TaggedEntityItem{
			EntityType: et.EntityType,
			EntityID:   et.EntityID,
		}
		if m, ok := enrichedByType[et.EntityType]; ok {
			if enriched, ok := m[et.EntityID]; ok {
				// Carry enriched fields, but preserve canonical EntityType / EntityID
				// from the entity_tags row.
				enriched.EntityType = et.EntityType
				enriched.EntityID = et.EntityID
				item = enriched
			} else if et.EntityType == catalogm.TagEntityCollection || et.EntityType == catalogm.TagEntityShow {
				// DEFENCE IN DEPTH, not the gate. Neither type reaches this
				// branch: the query above excludes gated shows, private
				// collections and rows whose entity no longer exists, which is
				// what keeps the total describing the same rows as the page. The
				// arm stays so that an edit loosening the query cannot emit a
				// nameless card asserting the entity is there.
				continue
			}
		}
		items = append(items, item)
	}

	return items, total, nil
}

// enrichBare is a fallback for entity types we don't have a richer query for.
// It only resolves the name and slug, leaving all other optional fields zero.
func (s *TagService) enrichBare(entityType string, ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	meta, ok := entityTableMap[entityType]
	if !ok {
		return out
	}
	type row struct {
		ID   uint
		Name string
		Slug string
	}
	// The shows table carries a read-time visibility gate, so the bare spelling
	// of a show row carries it too: enrichShows falls back here when its own
	// query fails, and that path may not publish a gated show's title either.
	// Public tier for every caller, for the reason enrichShows gives (PSY-1939).
	where := "id IN ?"
	args := []interface{}{ids}
	if meta.table == "shows" {
		visible, visibleArgs := shared.VisibleShowPredicateSQL("shows", contracts.ShowViewer{})
		where += " AND " + visible
		args = append(args, visibleArgs...)
	}

	var rows []row
	if err := s.db.Raw(
		fmt.Sprintf("SELECT id, %s AS name, COALESCE(slug, '') AS slug FROM %s WHERE %s", meta.nameCol, meta.table, where),
		args...,
	).Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.ID] = contracts.TaggedEntityItem{Name: r.Name, Slug: r.Slug}
	}
	return out
}

// venueLocalUpcomingCountSQL renders "approved upcoming shows per entity" as a
// derived table projecting (<entityFK>, cnt), partitioned on each show's OWN
// venue-local calendar day via shared.VenueTZJoin + VenueLocalDateCondition.
//
// junction is the show⇄entity join table ("show_artists", "show_venues") and
// entityFK the column on it naming the entity ("artist_id", "venue_id"). Both
// must be compile-time literals, NEVER runtime input — they are interpolated,
// not bound, exactly like the fragments in services/shared this builds on.
//
// ONE renderer, because a tag page prints this number on an entity card AND
// sorts the cards by it AND links through to the /shows list it summarizes.
// Those three read from here and from shared.VenueLocalDateCondition, so they
// cannot answer "is this show upcoming" three different ways — which is what
// the page did before PSY-1760 (start-of-today UTC for the entity gate,
// event_date >= NOW() with no status filter for the card counts, venue-local
// for the list).
//
// entityRestriction is an optional extra predicate on the junction row, and is
// the difference between aggregating the whole catalogue and aggregating the
// handful of entities a caller is about to render. Measured on a
// production-shaped fixture (670 upcoming shows, 240 venues, 3000 artists), the
// enrichment count runs 17.5ms unrestricted and 0.4ms with `j.artist_id IN ?`
// pushed down, because the restriction plans INSIDE the aggregate: the junction
// scan becomes an index probe on the requested ids, and the venue lateral then
// evaluates over their shows instead of every upcoming show in the catalogue.
// Callers that render a bounded id set MUST pass it. It may carry `?` placeholders, which bind BEFORE
// any in the enclosing statement — the fragment is spliced into the FROM clause.
// Pass "" for none, which keeps the fragment parameter-free.
//
// `shows` is deliberately UNALIASED: shared.VenueTZJoin's lateral correlates on
// `shows.id`, so aliasing the table would leave the correlation dangling.
//
// The visibility predicate is the shared rule's PUBLIC tier in its inlined form,
// so the status literal is interpolated from the Go constant rather than bound
// and the fragment stays composable into GORM's Joins and Raw alike. Safe by
// construction (a `const ShowStatus = "approved"`), pinned by
// TestShowStatusApproved_IsSafeToInterpolate.
func venueLocalUpcomingCountSQL(junction, entityFK, entityRestriction string) string {
	if entityRestriction != "" {
		entityRestriction = "\n\t      AND " + entityRestriction
	}
	return `(
	    SELECT j.` + entityFK + `, COUNT(DISTINCT shows.id) AS cnt
	    FROM ` + junction + ` j
	    JOIN shows ON shows.id = j.show_id
	    ` + shared.VenueTZJoin + `
	    WHERE ` + shared.PublicShowPredicateSQL("shows") + `
	      AND ` + shared.VenueLocalDateCondition("upcoming") + entityRestriction + `
	    GROUP BY j.` + entityFK + `
	)`
}

// enrichArtists adds city/state and an upcoming-show count to each artist row.
// Upcoming = approved shows on their own venue-local calendar day
// (venueLocalUpcomingCountSQL), joined via show_artists.
func (s *TagService) enrichArtists(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID                uint
		Name              string
		Slug              string
		City              string
		State             string
		UpcomingShowCount int
	}
	var rows []row
	err := s.db.Raw(`
		SELECT a.id,
		       a.name,
		       COALESCE(a.slug, '') AS slug,
		       COALESCE(a.city, '') AS city,
		       COALESCE(a.state, '') AS state,
		       COALESCE(c.cnt, 0)::int AS upcoming_show_count
		FROM artists a
		LEFT JOIN `+venueLocalUpcomingCountSQL("show_artists", "artist_id", "j.artist_id IN ?")+` c
		       ON c.artist_id = a.id
		WHERE a.id IN ?
	`, ids, ids).Scan(&rows).Error
	if err != nil {
		// Fallback: at least name + slug so the link still works.
		return s.enrichBare("artist", ids)
	}
	for _, r := range rows {
		count := r.UpcomingShowCount
		out[r.ID] = contracts.TaggedEntityItem{
			Name:              r.Name,
			Slug:              r.Slug,
			City:              r.City,
			State:             r.State,
			UpcomingShowCount: &count,
		}
	}
	return out
}

// enrichVenues adds city/state, the verified flag, and an upcoming-show count
// drawn from the same venue-local partition as enrichArtists'.
func (s *TagService) enrichVenues(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID                uint
		Name              string
		Slug              string
		City              string
		State             string
		Verified          bool
		UpcomingShowCount int
	}
	var rows []row
	err := s.db.Raw(`
		SELECT v.id,
		       v.name,
		       COALESCE(v.slug, '') AS slug,
		       COALESCE(v.city, '') AS city,
		       COALESCE(v.state, '') AS state,
		       v.verified,
		       COALESCE(c.cnt, 0)::int AS upcoming_show_count
		FROM venues v
		LEFT JOIN `+venueLocalUpcomingCountSQL("show_venues", "venue_id", "j.venue_id IN ?")+` c
		       ON c.venue_id = v.id
		WHERE v.id IN ?
	`, ids, ids).Scan(&rows).Error
	if err != nil {
		return s.enrichBare("venue", ids)
	}
	for _, r := range rows {
		verified := r.Verified
		count := r.UpcomingShowCount
		out[r.ID] = contracts.TaggedEntityItem{
			Name:              r.Name,
			Slug:              r.Slug,
			City:              r.City,
			State:             r.State,
			Verified:          &verified,
			UpcomingShowCount: &count,
		}
	}
	return out
}

// enrichFestivals adds edition year, location, status, dates, and counts.
func (s *TagService) enrichFestivals(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID          uint
		Name        string
		Slug        string
		EditionYear int
		City        string
		State       string
		Status      string
		StartDate   time.Time
		EndDate     time.Time
		ArtistCount int
		VenueCount  int
	}
	var rows []row
	err := s.db.Raw(`
		SELECT f.id,
		       f.name,
		       COALESCE(f.slug, '') AS slug,
		       f.edition_year,
		       COALESCE(f.city, '') AS city,
		       COALESCE(f.state, '') AS state,
		       f.status,
		       f.start_date,
		       f.end_date,
		       COALESCE(ac.cnt, 0)::int AS artist_count,
		       COALESCE(vc.cnt, 0)::int AS venue_count
		FROM festivals f
		LEFT JOIN (
		    SELECT festival_id, COUNT(*) AS cnt FROM festival_artists GROUP BY festival_id
		) ac ON ac.festival_id = f.id
		LEFT JOIN (
		    SELECT festival_id, COUNT(*) AS cnt FROM festival_venues GROUP BY festival_id
		) vc ON vc.festival_id = f.id
		WHERE f.id IN ?
	`, ids).Scan(&rows).Error
	if err != nil {
		return s.enrichBare("festival", ids)
	}
	for _, r := range rows {
		year := r.EditionYear
		ac := r.ArtistCount
		vc := r.VenueCount
		out[r.ID] = contracts.TaggedEntityItem{
			Name:        r.Name,
			Slug:        r.Slug,
			EditionYear: &year,
			City:        r.City,
			State:       r.State,
			Status:      r.Status,
			StartDate:   r.StartDate.Format("2006-01-02"),
			EndDate:     r.EndDate.Format("2006-01-02"),
			ArtistCount: &ac,
			VenueCount:  &vc,
		}
	}
	return out
}

// enrichLabels adds location, status, and roster/catalog counts.
func (s *TagService) enrichLabels(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID           uint
		Name         string
		Slug         string
		City         string
		State        string
		Status       string
		ArtistCount  int
		ReleaseCount int
	}
	var rows []row
	err := s.db.Raw(`
		SELECT l.id,
		       l.name,
		       COALESCE(l.slug, '') AS slug,
		       COALESCE(l.city, '') AS city,
		       COALESCE(l.state, '') AS state,
		       l.status,
		       COALESCE(ac.cnt, 0)::int AS artist_count,
		       COALESCE(rc.cnt, 0)::int AS release_count
		FROM labels l
		LEFT JOIN (
		    SELECT label_id, COUNT(*) AS cnt FROM artist_labels GROUP BY label_id
		) ac ON ac.label_id = l.id
		LEFT JOIN (
		    SELECT label_id, COUNT(*) AS cnt FROM release_labels GROUP BY label_id
		) rc ON rc.label_id = l.id
		WHERE l.id IN ?
	`, ids).Scan(&rows).Error
	if err != nil {
		return s.enrichBare("label", ids)
	}
	for _, r := range rows {
		ac := r.ArtistCount
		rc := r.ReleaseCount
		out[r.ID] = contracts.TaggedEntityItem{
			Name:         r.Name,
			Slug:         r.Slug,
			City:         r.City,
			State:        r.State,
			Status:       r.Status,
			ArtistCount:  &ac,
			ReleaseCount: &rc,
		}
	}
	return out
}

// enrichReleases adds release type, year, and cover art URL.
func (s *TagService) enrichReleases(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID          uint
		Title       string
		Slug        string
		ReleaseType string
		ReleaseYear *int
		CoverArtURL *string
	}
	var rows []row
	err := s.db.Raw(`
		SELECT id,
		       title,
		       COALESCE(slug, '') AS slug,
		       release_type,
		       release_year,
		       cover_art_url
		FROM releases
		WHERE id IN ?
	`, ids).Scan(&rows).Error
	if err != nil {
		return s.enrichBare("release", ids)
	}
	for _, r := range rows {
		item := contracts.TaggedEntityItem{
			Name:        r.Title,
			Slug:        r.Slug,
			ReleaseType: r.ReleaseType,
		}
		if r.ReleaseYear != nil {
			y := *r.ReleaseYear
			item.ReleaseYear = &y
		}
		if r.CoverArtURL != nil {
			item.CoverArtURL = *r.CoverArtURL
		}
		out[r.ID] = item
	}
	return out
}

// enrichShows adds event_date, the primary venue, and the headliner artist.
// This RESOLVES the one act to name for a show, so it always returns a row and
// is deliberately not catalog/headline_slot.go's classification rule, which
// may find no headline slot at all on a curated bill.
//
// The ordering is rank, then position, then a stable id. Do not shorten the
// rank to `(sa.set_type = 'headliner') DESC` and do not drop the trailing
// artist_id; headline_slot.go explains why each part is required and lists the
// other sites that resolve this same slot.
func (s *TagService) enrichShows(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID            uint
		Title         string
		Slug          string
		EventDate     time.Time
		City          string
		State         string
		VenueName     string
		VenueSlug     string
		HeadlinerName string
		HeadlinerSlug string
	}
	// A show whose status is not approved is not published by this listing, and
	// the tier is PUBLIC for every caller, admins included: /tags/{tag_id}/entities
	// is an anonymous, shareable, cacheable listing whose contents must not vary
	// by credential. Its siblings answer the same way (tag_intersection.go,
	// entity_existence.go, engagement's venue field-note rollup), and
	// venueLocalUpcomingCountSQL above spells this same rule for its own count.
	// The predicate comes from services/shared so there stays ONE definition of
	// it (PSY-1939).
	visible, visibleArgs := shared.VisibleShowPredicateSQL("s", contracts.ShowViewer{})
	args := append([]interface{}{ids}, visibleArgs...)

	var rows []row
	err := s.db.Raw(`
		SELECT s.id,
		       COALESCE(s.title, '') AS title,
		       COALESCE(s.slug, '') AS slug,
		       s.event_date,
		       COALESCE(s.city, '') AS city,
		       COALESCE(s.state, '') AS state,
		       COALESCE(v.name, '') AS venue_name,
		       COALESCE(v.slug, '') AS venue_slug,
		       COALESCE(a.name, '') AS headliner_name,
		       COALESCE(a.slug, '') AS headliner_slug
		FROM shows s
		LEFT JOIN LATERAL (
		    SELECT vv.id, vv.name, vv.slug
		    FROM show_venues sv
		    JOIN venues vv ON vv.id = sv.venue_id
		    WHERE sv.show_id = s.id
		    LIMIT 1
		) v ON true
		LEFT JOIN LATERAL (
		    SELECT aa.id, aa.name, aa.slug
		    FROM show_artists sa
		    JOIN artists aa ON aa.id = sa.artist_id
		    WHERE sa.show_id = s.id
		    ORDER BY CASE WHEN sa.set_type = 'headliner' THEN 0 ELSE 1 END, sa.position ASC, sa.artist_id ASC
		    LIMIT 1
		) a ON true
		WHERE s.id IN ? AND `+visible+`
	`, args...).Scan(&rows).Error
	if err != nil {
		return s.enrichBare("show", ids)
	}
	for _, r := range rows {
		out[r.ID] = contracts.TaggedEntityItem{
			Name:          r.Title,
			Slug:          r.Slug,
			EventDate:     r.EventDate.Format(time.RFC3339),
			City:          r.City,
			State:         r.State,
			VenueName:     r.VenueName,
			VenueSlug:     r.VenueSlug,
			HeadlinerName: r.HeadlinerName,
			HeadlinerSlug: r.HeadlinerSlug,
		}
	}
	return out
}

// enrichCollections resolves tagged collections to (title → name, slug).
//
// PSY-553: tag detail is public, so private collections must NOT leak
// through this surface even when tagged — the `is_public = true` filter is
// load-bearing. Excluded collections (private OR deleted) are dropped from
// the response in the build loop above via the matching collection-type
// continue branch. On query error we return an empty map (drop all) rather
// than fall back to `enrichBare` because `entityTableMap` has no collection
// entry and a fallthrough would surface every tagged collection's id with
// an empty name.
func (s *TagService) enrichCollections(ids []uint) map[uint]contracts.TaggedEntityItem {
	out := make(map[uint]contracts.TaggedEntityItem, len(ids))
	type row struct {
		ID    uint
		Title string
		Slug  string
	}
	var rows []row
	if err := s.db.Raw(`
		SELECT id,
		       title,
		       COALESCE(slug, '') AS slug
		FROM collections
		WHERE id IN ?
		  AND is_public = true
	`, ids).Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.ID] = contracts.TaggedEntityItem{
			Name: r.Title,
			Slug: r.Slug,
		}
	}
	return out
}

// ──────────────────────────────────────────────
// Tag detail enrichment
// ──────────────────────────────────────────────

// GetTagDetail returns the enriched tag detail response: description (+ rendered HTML),
// parent, direct children, usage breakdown by entity type, top contributors,
// creator attribution, and related tags computed via naive co-occurrence.
// Returns (nil, nil) if the tag does not exist.
func (s *TagService) GetTagDetail(tagID uint) (*contracts.TagDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Load the tag with its base relationships (parent, children, aliases, creator).
	tag, err := s.GetTag(tagID)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, nil
	}

	resp := &contracts.TagDetailResponse{
		TagResponse: contracts.TagResponse{
			ID:              tag.ID,
			Name:            tag.Name,
			Slug:            tag.Slug,
			Description:     tag.Description,
			ParentID:        tag.ParentID,
			Category:        tag.Category,
			IsOfficial:      tag.IsOfficial,
			UsageCount:      tag.UsageCount,
			ChildCount:      len(tag.Children),
			CreatedByUserID: tag.CreatedByUserID,
			CreatedAt:       tag.CreatedAt,
			UpdatedAt:       tag.UpdatedAt,
		},
		Children:        []contracts.TagSummary{},
		UsageBreakdown:  map[string]int64{},
		TopContributors: []contracts.TagContributor{},
		RelatedTags:     []contracts.TagSummary{},
	}

	// Parent summary
	if tag.Parent != nil {
		resp.ParentName = tag.Parent.Name
		resp.Parent = &contracts.TagSummary{
			ID:         tag.Parent.ID,
			Name:       tag.Parent.Name,
			Slug:       tag.Parent.Slug,
			Category:   tag.Parent.Category,
			IsOfficial: tag.Parent.IsOfficial,
			UsageCount: tag.Parent.UsageCount,
		}
	}

	// Children summaries
	if len(tag.Children) > 0 {
		resp.Children = make([]contracts.TagSummary, 0, len(tag.Children))
		for _, c := range tag.Children {
			resp.Children = append(resp.Children, contracts.TagSummary{
				ID:         c.ID,
				Name:       c.Name,
				Slug:       c.Slug,
				Category:   c.Category,
				IsOfficial: c.IsOfficial,
				UsageCount: c.UsageCount,
			})
		}
	}

	// Aliases
	if len(tag.Aliases) > 0 {
		resp.Aliases = make([]string, len(tag.Aliases))
		for i, a := range tag.Aliases {
			resp.Aliases[i] = a.Alias
		}
	}

	// Creator attribution
	if tag.CreatedBy != nil {
		ref := &contracts.TagUserRef{ID: tag.CreatedBy.ID}
		if tag.CreatedBy.Username != nil {
			ref.Username = *tag.CreatedBy.Username
			resp.CreatedByUsername = tag.CreatedBy.Username
		}
		resp.CreatedBy = ref
	}

	// Description HTML (markdown rendered + sanitized). Skip if no description.
	if tag.Description != nil && *tag.Description != "" {
		resp.DescriptionHTML = s.md.Render(*tag.Description)
	}

	// Usage breakdown: count entity_tags per valid entity type. Initialize every
	// valid entity type so the map always has a complete keyset (zero-counts
	// included); the frontend decides whether to show or hide them.
	for _, et := range catalogm.TagEntityTypes {
		resp.UsageBreakdown[et] = 0
	}
	type breakdownRow struct {
		EntityType string
		Count      int64
	}
	// GATED THE SAME WAY GetTagEntities IS, and for the same reason. This count
	// and that listing's total are rendered on one page, so a breakdown that
	// counted rows the listing withholds would report how many gated shows and
	// private collections carry this tag: the withheld rows published as
	// arithmetic. Public tier for both, because the tag page is anonymous.
	//
	// One predicate, from the registry, rather than an arm per gated type written
	// out here: the same condition decides the usage count on every other tag
	// surface, so a type that gains a rule reaches this query in the same edit.
	breakdownVisible, breakdownVisibleArgs := shared.VisibleEntityTagsSQL(
		"entity_tags", contracts.ShowViewer{})
	var rows []breakdownRow
	if err := s.db.Model(&catalogm.EntityTag{}).
		Select("entity_type, COUNT(*) AS count").
		Where("tag_id = ?", tagID).
		Where(breakdownVisible, breakdownVisibleArgs...).
		Group("entity_type").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to compute usage breakdown: %w", err)
	}
	var visibleUsage int64
	for _, r := range rows {
		if _, ok := resp.UsageBreakdown[r.EntityType]; ok {
			resp.UsageBreakdown[r.EntityType] = r.Count
			visibleUsage += r.Count
		}
	}
	// The count is the SUM OF THE BREAKDOWN IT IS RENDERED WITH, taken from the
	// same rows rather than from a second query, so the pair cannot disagree.
	// Every other surface that publishes this tag's count reaches the same number
	// through shared.VisibleTagUsageCounts, which applies the same predicate
	// grouped by tag instead of by entity type.
	resp.UsageCount = int(visibleUsage)

	// Top contributors: top 5 users by tag application count.
	type contribRow struct {
		UserID   uint
		Username *string
		Count    int64
	}
	// GATED ON THE SAME TERMS as the breakdown above. A contributor's count is
	// per-user, so an unfenced one attributes the withheld rows to a NAMED
	// person: "alice applied this tag five times, two are visible" says alice has
	// three gated shows or private collections carrying it.
	contribVisible, contribVisibleArgs := shared.VisibleEntityTagsSQL("et", contracts.ShowViewer{})
	var contribRows []contribRow
	if err := s.db.Table("entity_tags AS et").
		Select("et.added_by_user_id AS user_id, u.username AS username, COUNT(*) AS count").
		Joins("LEFT JOIN users u ON u.id = et.added_by_user_id").
		Where("et.tag_id = ?", tagID).
		Where(contribVisible, contribVisibleArgs...).
		Group("et.added_by_user_id, u.username").
		Order("count DESC, et.added_by_user_id ASC").
		Limit(5).
		Scan(&contribRows).Error; err != nil {
		return nil, fmt.Errorf("failed to compute top contributors: %w", err)
	}
	for _, r := range contribRows {
		ref := contracts.TagUserRef{ID: r.UserID}
		if r.Username != nil {
			ref.Username = *r.Username
		}
		resp.TopContributors = append(resp.TopContributors, contracts.TagContributor{
			User:  ref,
			Count: r.Count,
		})
	}

	// Related tags: other tags that co-occur on the same (entity_type, entity_id)
	// as this tag. Naive single-query implementation — fine for v1, optimize via
	// materialized view if profiling shows hotspots.
	type relatedRow struct {
		ID         uint
		Name       string
		Slug       string
		Category   string
		IsOfficial bool
		UsageCount int
	}
	// BOTH SIDES OF THE SELF-JOIN ARE GATED. Co-occurrence is computed over the
	// entities carrying the two tags, so an ungated join reports which tags share
	// a private collection with this one: the collection's contents restated as a
	// relationship. The self side and the other side name the same entity, so
	// judging one would be enough, and both carry the condition because the join
	// is on a pair of columns that a later edit could loosen.
	//
	// Placeholders bind by POSITION, so the arguments below are appended in
	// statement order: the self gate, the other gate, then the two tag ids.
	selfVisible, selfVisibleArgs := shared.VisibleEntityTagsSQL("et_self", contracts.ShowViewer{})
	otherVisible, otherVisibleArgs := shared.VisibleEntityTagsSQL("et_other", contracts.ShowViewer{})
	relatedArgs := append([]interface{}{}, selfVisibleArgs...)
	relatedArgs = append(relatedArgs, otherVisibleArgs...)
	relatedArgs = append(relatedArgs, tagID, tagID)
	var relatedRows []relatedRow
	err = s.db.Raw(`
		SELECT t.id, t.name, t.slug, t.category, t.is_official, t.usage_count
		FROM entity_tags et_other
		JOIN entity_tags et_self
		  ON et_self.entity_type = et_other.entity_type
		 AND et_self.entity_id   = et_other.entity_id
		JOIN tags t ON t.id = et_other.tag_id
		WHERE `+selfVisible+`
		  AND `+otherVisible+`
		  AND et_self.tag_id = ?
		  AND et_other.tag_id <> ?
		GROUP BY t.id, t.name, t.slug, t.category, t.is_official, t.usage_count
		ORDER BY COUNT(*) DESC, t.usage_count DESC, t.name ASC
		LIMIT 5
	`, relatedArgs...).Scan(&relatedRows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to compute related tags: %w", err)
	}
	for _, r := range relatedRows {
		resp.RelatedTags = append(resp.RelatedTags, contracts.TagSummary{
			ID:         r.ID,
			Name:       r.Name,
			Slug:       r.Slug,
			Category:   r.Category,
			IsOfficial: r.IsOfficial,
			UsageCount: r.UsageCount,
		})
	}
	// The related tags' own counts, on the same terms as every other published
	// count. The ORDER BY above still ranks on the raw column.
	if err := s.applyVisibleUsageCountsToSummaries(resp.RelatedTags); err != nil {
		return nil, fmt.Errorf("failed to compute visible tag usage counts: %w", err)
	}

	return resp, nil
}

// wilsonScore computes the Wilson score lower bound for a binomial proportion.
// This is the same algorithm used for request voting.
func wilsonScore(upvotes, downvotes int) float64 {
	n := float64(upvotes + downvotes)
	if n == 0 {
		return 0
	}
	p := float64(upvotes) / n
	z := 1.96 // 95% confidence
	denominator := 1 + z*z/n
	centre := p + z*z/(2*n)
	spread := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n))
	return (centre - spread) / denominator
}
