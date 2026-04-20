package catalog

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
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
func (s *TagService) CreateTag(name string, description *string, parentID *uint, category string, isOfficial bool, userID *uint) (*models.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !models.IsValidTagCategory(category) {
		return nil, fmt.Errorf("invalid tag category: %s", category)
	}

	// Check for duplicate name (case-insensitive)
	var existing models.Tag
	if err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&existing).Error; err == nil {
		return nil, apperrors.ErrTagExists(name)
	}

	// Generate slug
	baseSlug := utils.GenerateSlug(name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Tag{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	tag := &models.Tag{
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

// GetTag retrieves a tag by ID with relationships.
func (s *TagService) GetTag(tagID uint) (*models.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tag models.Tag
	err := s.db.Preload("Parent").Preload("Children").Preload("Aliases").Preload("CreatedBy").First(&tag, tagID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	return &tag, nil
}

// GetTagBySlug retrieves a tag by slug with relationships.
func (s *TagService) GetTagBySlug(slug string) (*models.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tag models.Tag
	err := s.db.Preload("Parent").Preload("Children").Preload("Aliases").Preload("CreatedBy").
		Where("slug = ?", slug).First(&tag).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tag by slug: %w", err)
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
func (s *TagService) ListTags(category string, search string, parentID *uint, sort string, limit, offset int, entityType string) ([]models.Tag, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	if entityType != "" && !models.IsValidTagEntityType(entityType) {
		return nil, 0, fmt.Errorf("invalid entity type: %s", entityType)
	}

	query := s.db.Model(&models.Tag{})

	if category != "" {
		query = query.Where("category = ?", category)
	}
	if search != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+search+"%")
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

	var tags []models.Tag
	if err := query.Find(&tags).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list tags: %w", err)
	}

	if entityType != "" && len(tags) > 0 {
		// Single GROUP BY to fetch per-entity-type counts for the returned page.
		// Cheap: one indexed scan over (entity_type, tag_id) for at most `limit`
		// tag IDs. Tags absent from entity_tags for this type get 0.
		ids := make([]uint, len(tags))
		for i, t := range tags {
			ids[i] = t.ID
		}
		type countRow struct {
			TagID uint
			Count int64
		}
		var rows []countRow
		err := s.db.Table("entity_tags").
			Select("tag_id, COUNT(*) AS count").
			Where("entity_type = ? AND tag_id IN ?", entityType, ids).
			Group("tag_id").
			Scan(&rows).Error
		if err != nil {
			return nil, 0, fmt.Errorf("failed to compute per-entity-type tag counts: %w", err)
		}
		countByID := make(map[uint]int64, len(rows))
		for _, r := range rows {
			countByID[r.TagID] = r.Count
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
	}

	return tags, total, nil
}

// sortTagsByUsageDesc sorts in place by UsageCount DESC, name ASC. Used after
// per-entity-type recount so the displayed order matches the new counts.
func sortTagsByUsageDesc(tags []models.Tag) {
	sort.SliceStable(tags, func(i, j int) bool {
		if tags[i].UsageCount != tags[j].UsageCount {
			return tags[i].UsageCount > tags[j].UsageCount
		}
		return tags[i].Name < tags[j].Name
	})
}

// UpdateTag updates a tag's fields.
func (s *TagService) UpdateTag(tagID uint, name *string, description *string, parentID *uint, category *string, isOfficial *bool) (*models.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tag models.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
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
			s.db.Model(&models.Tag{}).Where("slug = ? AND id != ?", candidate, tagID).Count(&count)
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
		if tag.Category != models.TagCategoryGenre {
			return nil, apperrors.ErrTagHierarchyNotGenre(tag.Name, tag.Category)
		}
		if err := s.validateTagParent(&tag, parentID); err != nil {
			return nil, err
		}
		updates["parent_id"] = *parentID
	}
	if category != nil {
		if !models.IsValidTagCategory(*category) {
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

	result := s.db.Delete(&models.Tag{}, tagID)
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
func (s *TagService) AddTagToEntity(tagID uint, tagName string, entityType string, entityID uint, userID uint, category string) (*models.EntityTag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !models.IsValidTagEntityType(entityType) {
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}

	// Resolve tag by ID or name
	var tag *models.Tag
	var createdInline bool
	if tagID > 0 {
		var t models.Tag
		if err := s.db.First(&t, tagID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
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
			var t models.Tag
			if err := s.db.Where("LOWER(name) = LOWER(?)", tagName).First(&t).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
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
	var existing models.EntityTag
	err := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tag.ID, entityType, entityID).
		First(&existing).Error
	if err == nil {
		return nil, apperrors.ErrEntityTagExists(tag.ID, entityType, entityID)
	}

	entityTag := &models.EntityTag{
		TagID:         tag.ID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: userID,
	}

	if err := s.db.Create(entityTag).Error; err != nil {
		return nil, fmt.Errorf("failed to add tag to entity: %w", err)
	}

	// Increment usage count atomically
	s.db.Model(&models.Tag{}).Where("id = ?", tag.ID).
		Update("usage_count", gorm.Expr("usage_count + 1"))

	// Auto-upvote for the creator when tag was created inline
	if createdInline {
		autoVote := models.TagVote{
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
func (s *TagService) createTagInline(tagName string, category string, userID uint) (*models.Tag, error) {
	// Look up user to check trust tier
	var user models.User
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
	if !models.IsValidTagCategory(category) {
		return nil, fmt.Errorf("invalid tag category: %s", category)
	}

	// Check for duplicate after normalization (case-insensitive)
	var existing models.Tag
	if err := s.db.Where("LOWER(name) = LOWER(?)", normalized).First(&existing).Error; err == nil {
		// Already exists after normalization — use existing
		return &existing, nil
	}

	// Generate slug
	baseSlug := utils.GenerateSlug(normalized)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.Tag{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	tag := &models.Tag{
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
		Delete(&models.EntityTag{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove tag from entity: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrEntityTagNotFound(tagID, entityType, entityID)
	}

	// Decrement usage count atomically (floor at 0)
	s.db.Model(&models.Tag{}).Where("id = ? AND usage_count > 0", tagID).
		Update("usage_count", gorm.Expr("usage_count - 1"))

	// Clean up votes for this tag-entity pair
	s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tagID, entityType, entityID).
		Delete(&models.TagVote{})

	return nil
}

// ListEntityTags lists all tags on an entity with vote counts.
// Pass userID=0 for unauthenticated requests.
func (s *TagService) ListEntityTags(entityType string, entityID uint, userID uint) ([]contracts.EntityTagResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get entity tags with tag info and added-by user
	var entityTags []models.EntityTag
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
		s.db.Model(&models.TagVote{}).
			Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND vote = 1", et.TagID, entityType, entityID).
			Count(&upvotes)
		s.db.Model(&models.TagVote{}).
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

		// Resolve username
		if et.AddedBy.Username != nil && *et.AddedBy.Username != "" {
			resp.AddedByUsername = *et.AddedBy.Username
		}

		// Include user's vote if authenticated
		if userID > 0 {
			var vote models.TagVote
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
	var entityTag models.EntityTag
	err := s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?", tagID, entityType, entityID).
		First(&entityTag).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrEntityTagNotFound(tagID, entityType, entityID)
		}
		return fmt.Errorf("failed to verify entity tag: %w", err)
	}

	voteValue := -1
	if isUpvote {
		voteValue = 1
	}

	// Upsert vote
	var existing models.TagVote
	err = s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ? AND user_id = ?",
		tagID, entityType, entityID, userID).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		vote := models.TagVote{
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
		tagID, entityType, entityID, userID).Delete(&models.TagVote{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove vote: %w", result.Error)
	}

	return nil
}

// ──────────────────────────────────────────────
// Aliases
// ──────────────────────────────────────────────

// CreateAlias creates a new alias for a tag.
func (s *TagService) CreateAlias(tagID uint, alias string) (*models.TagAlias, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify tag exists
	var tag models.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrTagNotFound(tagID)
		}
		return nil, fmt.Errorf("failed to get tag: %w", err)
	}

	// Check for duplicate alias (case-insensitive)
	var existing models.TagAlias
	if err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&existing).Error; err == nil {
		return nil, apperrors.ErrTagAliasExists(alias)
	}

	// Also check if alias matches an existing tag name
	var existingTag models.Tag
	if err := s.db.Where("LOWER(name) = LOWER(?)", alias).First(&existingTag).Error; err == nil {
		return nil, apperrors.ErrTagAliasExists(alias)
	}

	tagAlias := &models.TagAlias{
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

	result := s.db.Delete(&models.TagAlias{}, aliasID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete alias: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("alias not found")
	}

	return nil
}

// ListAliases lists all aliases for a tag.
func (s *TagService) ListAliases(tagID uint) ([]models.TagAlias, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var aliases []models.TagAlias
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
		pattern := "%" + strings.ToLower(search) + "%"
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

		var tag models.Tag
		err := s.db.Where("LOWER(slug) = LOWER(?) OR LOWER(name) = LOWER(?)", canonical, canonical).
			First(&tag).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
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

		var existingAlias models.TagAlias
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

		var nameConflict models.Tag
		if err := s.db.Where("LOWER(name) = LOWER(?)", alias).First(&nameConflict).Error; err == nil {
			result.Skipped = append(result.Skipped, contracts.BulkAliasImportSkipped{
				Row:       row,
				Alias:     alias,
				Canonical: canonical,
				Reason:    "alias collides with existing tag name",
			})
			continue
		}

		if err := s.db.Create(&models.TagAlias{TagID: tag.ID, Alias: alias}).Error; err != nil {
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
func (s *TagService) ResolveAlias(alias string) (*models.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tagAlias models.TagAlias
	err := s.db.Where("LOWER(alias) = LOWER(?)", alias).First(&tagAlias).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to resolve alias: %w", err)
	}

	var tag models.Tag
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
	db := s.db.Where(
		s.db.Where("LOWER(name) LIKE ?", "%"+q+"%").
			Or("id IN (?)",
				s.db.Model(&models.TagAlias{}).Select("tag_id").Where("LOWER(alias) LIKE ?", "%"+q+"%"),
			),
	)

	if category != "" {
		db = db.Where("category = ?", category)
	}

	var tags []models.Tag
	err := db.Order("usage_count DESC").
		Limit(limit).
		Find(&tags).Error
	if err != nil {
		return nil, fmt.Errorf("failed to search tags: %w", err)
	}

	// Build the result set. For tags whose name does NOT match the query,
	// look up which alias on that tag matched so the UI can surface the
	// provenance. A single batch query avoids N+1.
	results := make([]contracts.TagSearchResult, len(tags))
	pattern := "%" + q + "%"

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
		var aliases []models.TagAlias
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
func (s *TagService) GetTrendingTags(limit int, category string) ([]models.Tag, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	query := s.db.Model(&models.Tag{}).Where("usage_count > 0")
	if category != "" {
		query = query.Where("category = ?", category)
	}

	var tags []models.Tag
	if err := query.Order("usage_count DESC").Limit(limit).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to get trending tags: %w", err)
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
			c.TagID, c.EntityType, c.EntityID).Delete(&models.EntityTag{})
		if result.Error == nil {
			totalPruned += result.RowsAffected
			// Decrement usage count
			if result.RowsAffected > 0 {
				s.db.Model(&models.Tag{}).Where("id = ? AND usage_count > 0", c.TagID).
					Update("usage_count", gorm.Expr("usage_count - 1"))
			}
		}
		// Also clean up the votes
		s.db.Where("tag_id = ? AND entity_type = ? AND entity_id = ?",
			c.TagID, c.EntityType, c.EntityID).Delete(&models.TagVote{})
	}

	return totalPruned, nil
}

// ──────────────────────────────────────────────
// Tag entities
// ──────────────────────────────────────────────

// entityTableMap maps entity type strings to their DB table name and name column.
var entityTableMap = map[string]struct {
	table     string
	nameCol   string
}{
	"artist":   {table: "artists", nameCol: "name"},
	"venue":    {table: "venues", nameCol: "name"},
	"label":    {table: "labels", nameCol: "name"},
	"show":     {table: "shows", nameCol: "title"},
	"release":  {table: "releases", nameCol: "title"},
	"festival": {table: "festivals", nameCol: "name"},
}

// GetTagEntities returns entities tagged with a given tag, optionally filtered by entity type.
func (s *TagService) GetTagEntities(tagID uint, entityType string, limit, offset int) ([]contracts.TaggedEntityItem, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&models.EntityTag{}).Where("tag_id = ?", tagID)
	if entityType != "" {
		if !models.IsValidTagEntityType(entityType) {
			return nil, 0, fmt.Errorf("invalid entity type: %s", entityType)
		}
		query = query.Where("entity_type = ?", entityType)
	}

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

	var entityTags []models.EntityTag
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&entityTags).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list tagged entities: %w", err)
	}

	// Group entity IDs by type for batch resolution
	byType := make(map[string][]uint)
	for _, et := range entityTags {
		byType[et.EntityType] = append(byType[et.EntityType], et.EntityID)
	}

	// Resolve names and slugs per entity type
	type entityInfo struct {
		ID   uint
		Name string
		Slug string
	}
	infoMap := make(map[string]map[uint]entityInfo) // entityType -> entityID -> info

	for eType, ids := range byType {
		meta, ok := entityTableMap[eType]
		if !ok {
			continue
		}

		var results []entityInfo
		err := s.db.Raw(
			fmt.Sprintf("SELECT id, %s AS name, COALESCE(slug, '') AS slug FROM %s WHERE id IN ?", meta.nameCol, meta.table),
			ids,
		).Scan(&results).Error
		if err != nil {
			continue // skip if table doesn't exist or query fails
		}

		m := make(map[uint]entityInfo, len(results))
		for _, r := range results {
			m[r.ID] = r
		}
		infoMap[eType] = m
	}

	// Build response
	items := make([]contracts.TaggedEntityItem, 0, len(entityTags))
	for _, et := range entityTags {
		info := entityInfo{}
		if m, ok := infoMap[et.EntityType]; ok {
			if i, ok := m[et.EntityID]; ok {
				info = i
			}
		}
		items = append(items, contracts.TaggedEntityItem{
			EntityType: et.EntityType,
			EntityID:   et.EntityID,
			Name:       info.Name,
			Slug:       info.Slug,
		})
	}

	return items, total, nil
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
	for _, et := range models.TagEntityTypes {
		resp.UsageBreakdown[et] = 0
	}
	type breakdownRow struct {
		EntityType string
		Count      int64
	}
	var rows []breakdownRow
	if err := s.db.Model(&models.EntityTag{}).
		Select("entity_type, COUNT(*) AS count").
		Where("tag_id = ?", tagID).
		Group("entity_type").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to compute usage breakdown: %w", err)
	}
	for _, r := range rows {
		if _, ok := resp.UsageBreakdown[r.EntityType]; ok {
			resp.UsageBreakdown[r.EntityType] = r.Count
		}
	}

	// Top contributors: top 5 users by tag application count.
	type contribRow struct {
		UserID   uint
		Username *string
		Count    int64
	}
	var contribRows []contribRow
	if err := s.db.Table("entity_tags AS et").
		Select("et.added_by_user_id AS user_id, u.username AS username, COUNT(*) AS count").
		Joins("LEFT JOIN users u ON u.id = et.added_by_user_id").
		Where("et.tag_id = ?", tagID).
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
	var relatedRows []relatedRow
	err = s.db.Raw(`
		SELECT t.id, t.name, t.slug, t.category, t.is_official, t.usage_count
		FROM entity_tags et_other
		JOIN entity_tags et_self
		  ON et_self.entity_type = et_other.entity_type
		 AND et_self.entity_id   = et_other.entity_id
		JOIN tags t ON t.id = et_other.tag_id
		WHERE et_self.tag_id = ?
		  AND et_other.tag_id <> ?
		GROUP BY t.id, t.name, t.slug, t.category, t.is_official, t.usage_count
		ORDER BY COUNT(*) DESC, t.usage_count DESC, t.name ASC
		LIMIT 5
	`, tagID, tagID).Scan(&relatedRows).Error
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
