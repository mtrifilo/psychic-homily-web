package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// PSY-356: quality gates for public collection visibility.
//
// MinPublicCollectionItems is the minimum number of items a collection must
// contain to appear in the public /collections browse listing. Matches
// What.cd's "3 items" convention. Existing public collections that fall
// below this threshold remain `is_public=true` in the DB (URL access via
// /collections/{slug} is unaffected); they are filtered out of the browse
// list only.
//
// MinPublicCollectionDescriptionChars is the minimum CHAR_LENGTH of the
// raw description for the same gate. The collections.description column
// is NOT NULL and defaults to "" — the SQL filter below uses CHAR_LENGTH
// directly without an IS NOT NULL guard.
const (
	MinPublicCollectionItems            = 3
	MinPublicCollectionDescriptionChars = 50
)

// CollectionService handles collection-related business logic.
// md is the shared utils.MarkdownRenderer (goldmark + bluemonday) used to
// render Description and per-item Notes on read. Sanitization is applied on
// every response so existing plain-text rows are also rendered safely — the
// sanitizer is the source of truth for XSS safety, not the input pipeline.
//
// tagService is the polymorphic tag system (PSY-354). Optional — nil when
// the service is built bare (e.g. from older test paths). The
// AddTagToCollection / RemoveTagFromCollection methods require it; tag
// rendering on Get/List is gracefully no-op when nil so that older callers
// keep working.
type CollectionService struct {
	db         *gorm.DB
	md         *utils.MarkdownRenderer
	tagService contracts.TagServiceInterface
}

// NewCollectionService creates a new collection service
func NewCollectionService(database *gorm.DB) *CollectionService {
	if database == nil {
		database = db.GetDB()
	}
	return &CollectionService{
		db: database,
		md: utils.NewMarkdownRenderer(),
	}
}

// SetTagService injects the polymorphic tag service (PSY-354). Called by the
// service container after both services are constructed (avoids the
// constructor-ordering tangle that would otherwise force a TagService import
// from the services root package). Idempotent — safe to call again in tests.
func (s *CollectionService) SetTagService(tagService contracts.TagServiceInterface) {
	s.tagService = tagService
}

// renderMarkdown returns sanitized HTML for the given markdown source. Returns
// "" for empty input. Falls back to a freshly-constructed renderer when the
// service was built with the bare struct literal (older test paths).
func (s *CollectionService) renderMarkdown(src string) string {
	if src == "" {
		return ""
	}
	if s.md == nil {
		s.md = utils.NewMarkdownRenderer()
	}
	return s.md.Render(src)
}

// renderNotes is a *string-aware wrapper around renderMarkdown for the
// nullable Notes column. Returns "" when the pointer is nil or empty.
func (s *CollectionService) renderNotes(notes *string) string {
	if notes == nil {
		return ""
	}
	return s.renderMarkdown(*notes)
}

// validatePublishGate rejects a visibility transition that would make a
// collection public while it falls below the items + description thresholds
// (PSY-356). Returns nil when the gate passes. Returns an
// ErrCollectionInvalidRequest (mapped to 400) with a precise message
// enumerating only the missing pieces, so the caller can surface the same
// guidance the in-app banner shows.
func validatePublishGate(itemCount int, description string) error {
	itemsBelow := itemCount < MinPublicCollectionItems
	descBelow := len(description) < MinPublicCollectionDescriptionChars
	if !itemsBelow && !descBelow {
		return nil
	}

	itemsNeeded := MinPublicCollectionItems - itemCount
	if itemsNeeded < 0 {
		itemsNeeded = 0
	}
	switch {
	case itemsBelow && descBelow:
		return apperrors.ErrCollectionInvalidRequest(fmt.Sprintf(
			"public collections require at least %d items and a description of at least %d characters (currently %d items, %d-character description)",
			MinPublicCollectionItems, MinPublicCollectionDescriptionChars, itemCount, len(description),
		))
	case itemsBelow:
		return apperrors.ErrCollectionInvalidRequest(fmt.Sprintf(
			"public collections require at least %d items (currently %d)",
			MinPublicCollectionItems, itemCount,
		))
	default:
		return apperrors.ErrCollectionInvalidRequest(fmt.Sprintf(
			"public collections require a description of at least %d characters (currently %d)",
			MinPublicCollectionDescriptionChars, len(description),
		))
	}
}

// CreateCollection creates a new collection and auto-subscribes the creator
func (s *CollectionService) CreateCollection(creatorID uint, req *contracts.CreateCollectionRequest) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Generate unique slug from title
	baseSlug := utils.GenerateArtistSlug(req.Title)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&communitym.Collection{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	if len(description) > contracts.MaxCollectionDescriptionLength {
		return nil, fmt.Errorf("description exceeds maximum length of %d characters", contracts.MaxCollectionDescriptionLength)
	}

	// PSY-356: forward gate. New collections cannot be created public —
	// items_count is 0 at create time, so the items half of the gate
	// always rejects. The error message also enumerates the description
	// gap when applicable, mirroring the in-app banner copy.
	if req.IsPublic {
		if err := validatePublishGate(0, description); err != nil {
			return nil, err
		}
	}

	displayMode := communitym.CollectionDisplayModeUnranked
	if req.DisplayMode != nil && *req.DisplayMode != "" {
		if !communitym.IsValidCollectionDisplayMode(*req.DisplayMode) {
			return nil, apperrors.ErrCollectionInvalidRequest(
				fmt.Sprintf("display_mode must be 'ranked' or 'unranked', got %q", *req.DisplayMode),
			)
		}
		displayMode = *req.DisplayMode
	}

	collection := &communitym.Collection{
		Title:         req.Title,
		Slug:          slug,
		Description:   description,
		CreatorID:     creatorID,
		Collaborative: true, // Create with true defaults to avoid GORM zero-value skip
		CoverImageURL: req.CoverImageURL,
		IsPublic:      true,
		IsFeatured:    false,
		DisplayMode:   displayMode,
	}

	if err := s.db.Create(collection).Error; err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	// GORM bool gotcha: false is zero-value, GORM skips it on Create → DB default wins.
	// Fix: create with true, then update to false for any bool fields that should be false.
	boolUpdates := map[string]interface{}{}
	if !req.Collaborative {
		boolUpdates["collaborative"] = false
	}
	if !req.IsPublic {
		boolUpdates["is_public"] = false
	}
	if len(boolUpdates) > 0 {
		if err := s.db.Model(collection).Updates(boolUpdates).Error; err != nil {
			return nil, fmt.Errorf("failed to update collection bools: %w", err)
		}
	}

	// Auto-subscribe creator
	now := time.Now()
	subscriber := &communitym.CollectionSubscriber{
		CollectionID:  collection.ID,
		UserID:        creatorID,
		LastVisitedAt: &now,
	}
	if err := s.db.Create(subscriber).Error; err != nil {
		// Non-fatal: collection was created successfully
		_ = err
	}

	return s.GetBySlug(slug, creatorID)
}

// CloneCollection creates a new collection owned by callerID by copying
// title (suffixed with " (fork)"), description, and all items (including
// per-item notes and positions) from the source collection. Sets
// `forked_from_collection_id` to the source ID. PSY-351.
//
// Visibility: matches the existing pattern (GetBySlug). Source must be
// public OR owned by the caller. Anyone authenticated can clone any public
// collection — no trust-tier gate.
//
// Notes:
//   - The clone is created as a new public, collaborative=true collection
//     by default, mirroring CreateCollection's defaults. The cloner can
//     edit visibility/collaboration after the fact.
//   - The whole copy runs in a transaction so we never end up with a
//     half-populated fork on partial failure.
func (s *CollectionService) CloneCollection(srcSlug string, callerID uint) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if callerID == 0 {
		// Defense-in-depth: handler already enforces auth, but the service
		// must not silently accept anonymous callers.
		return nil, apperrors.ErrCollectionForbidden(srcSlug)
	}

	// Load source.
	var src communitym.Collection
	if err := s.db.Where("slug = ?", srcSlug).First(&src).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(srcSlug)
		}
		return nil, fmt.Errorf("failed to load source collection: %w", err)
	}

	// Visibility: match GetBySlug — public OR owned by caller.
	if !src.IsPublic && src.CreatorID != callerID {
		return nil, apperrors.ErrCollectionForbidden(srcSlug)
	}

	// Generate a unique slug from "<title> (fork)".
	forkTitle := src.Title + " (fork)"
	baseSlug := utils.GenerateArtistSlug(forkTitle)
	newSlug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&communitym.Collection{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	srcID := src.ID
	clone := &communitym.Collection{
		Title:                  forkTitle,
		Slug:                   newSlug,
		Description:            src.Description,
		CreatorID:              callerID,
		Collaborative:          true, // GORM bool gotcha: create with true defaults; reset below if needed.
		CoverImageURL:          src.CoverImageURL,
		IsPublic:               true, // public default — caller can flip to private after clone.
		IsFeatured:             false,
		ForkedFromCollectionID: &srcID,
	}

	now := time.Now()

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(clone).Error; err != nil {
			return fmt.Errorf("failed to create clone: %w", err)
		}

		// Copy items. We re-query the items rather than relying on a
		// preloaded slice so we can order them deterministically and
		// tolerate concurrent edits to the source.
		var srcItems []communitym.CollectionItem
		if err := tx.Where("collection_id = ?", srcID).
			Order("position ASC, created_at ASC").
			Find(&srcItems).Error; err != nil {
			return fmt.Errorf("failed to load source items: %w", err)
		}

		if len(srcItems) > 0 {
			cloned := make([]communitym.CollectionItem, 0, len(srcItems))
			for _, item := range srcItems {
				cloned = append(cloned, communitym.CollectionItem{
					CollectionID:  clone.ID,
					EntityType:    item.EntityType,
					EntityID:      item.EntityID,
					Position:      item.Position,
					AddedByUserID: callerID, // attributed to the cloner.
					Notes:         item.Notes,
				})
			}
			if err := tx.Create(&cloned).Error; err != nil {
				return fmt.Errorf("failed to copy items: %w", err)
			}
		}

		// Auto-subscribe the cloner — mirrors CreateCollection.
		sub := &communitym.CollectionSubscriber{
			CollectionID:  clone.ID,
			UserID:        callerID,
			LastVisitedAt: &now,
		}
		if err := tx.Create(sub).Error; err != nil {
			// Non-fatal: clone exists. Log but don't fail the transaction.
			_ = err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.GetBySlug(newSlug, callerID)
}

// GetBySlug retrieves a collection by slug with full detail
func (s *CollectionService) GetBySlug(slug string, viewerID uint) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Check access: private collections are only visible to the creator
	if !collection.IsPublic && collection.CreatorID != viewerID {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Load creator name
	creatorName := s.resolveUserName(collection.CreatorID)
	creatorUsername := s.resolveUserUsername(collection.CreatorID)

	// Load items
	var items []communitym.CollectionItem
	s.db.Where("collection_id = ?", collection.ID).Order("position ASC, created_at ASC").Find(&items)

	// Resolve entity names for items
	itemResponses := s.buildItemResponses(items)

	// Count subscribers
	var subscriberCount int64
	s.db.Model(&communitym.CollectionSubscriber{}).Where("collection_id = ?", collection.ID).Count(&subscriberCount)

	// Count distinct contributors
	var contributorCount int64
	s.db.Model(&communitym.CollectionItem{}).Where("collection_id = ?", collection.ID).
		Distinct("added_by_user_id").Count(&contributorCount)

	// Count forks (public social signal — PSY-351). Live COUNT mirrors
	// the existing collection counter pattern and is cheap thanks to the
	// partial index added in migration 20260427173004.
	var forksCount int64
	s.db.Model(&communitym.Collection{}).
		Where("forked_from_collection_id = ?", collection.ID).
		Count(&forksCount)

	// Resolve "forked from" attribution if applicable. PSY-351.
	// PSY-351: When the source was deleted, the FK was reset to NULL by
	// `ON DELETE SET NULL`. We never snapshot the source title at fork time
	// (per product decision); the frontend renders fallback copy in that
	// case.
	forkedFrom := s.resolveForkedFromInfo(collection.ForkedFromCollectionID)

	// Check if viewer is subscribed
	isSubscribed := false
	if viewerID > 0 {
		var subCount int64
		s.db.Model(&communitym.CollectionSubscriber{}).
			Where("collection_id = ? AND user_id = ?", collection.ID, viewerID).
			Count(&subCount)
		isSubscribed = subCount > 0
	}

	// PSY-352: aggregate like count + caller's like state. UserLikesThis
	// is always false for anonymous viewers (viewerID == 0).
	var likeCount int64
	s.db.Model(&communitym.CollectionLike{}).Where("collection_id = ?", collection.ID).Count(&likeCount)
	userLikesThis := false
	if viewerID > 0 {
		var ulCount int64
		s.db.Model(&communitym.CollectionLike{}).
			Where("collection_id = ? AND user_id = ?", collection.ID, viewerID).
			Count(&ulCount)
		userLikesThis = ulCount > 0
	}

	// PSY-350: bump last_visited_at for authenticated subscribers so the
	// library tab's "N new since last visit" badge clears. Fire-and-forget —
	// a write failure here must NOT fail the read. We do this in a goroutine
	// to avoid contention with the read path; the staleness window is one
	// page-load and that's acceptable.
	//
	// Hook point: the detail endpoint (GET /collections/{slug}) is the
	// natural "user looked at the collection" signal. Card-only views and
	// list endpoints intentionally do NOT bump the cursor.
	if isSubscribed {
		collectionID := collection.ID
		uid := viewerID
		dbHandle := s.db
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("warning: collection MarkVisited goroutine panicked for user %d collection %d: %v", uid, collectionID, r)
				}
			}()
			if err := dbHandle.Model(&communitym.CollectionSubscriber{}).
				Where("collection_id = ? AND user_id = ?", collectionID, uid).
				Update("last_visited_at", time.Now()).Error; err != nil {
				log.Printf("warning: failed to bump last_visited_at for user %d collection %d: %v", uid, collectionID, err)
			}
		}()
	}

	// PSY-354: tag chips on the detail response. Empty slice (not nil) when
	// the collection has no tags or the tag service isn't wired (older
	// test paths) — keeps the JSON shape stable and unblocks the frontend.
	tags := s.listCollectionTags(collection.ID, viewerID)

	return &contracts.CollectionDetailResponse{
		ID:                     collection.ID,
		Title:                  collection.Title,
		Slug:                   collection.Slug,
		Description:            collection.Description,
		DescriptionHTML:        s.renderMarkdown(collection.Description),
		CreatorID:              collection.CreatorID,
		CreatorName:            creatorName,
		CreatorUsername:        creatorUsername,
		Collaborative:          collection.Collaborative,
		CoverImageURL:          collection.CoverImageURL,
		IsPublic:               collection.IsPublic,
		IsFeatured:             collection.IsFeatured,
		DisplayMode:            collection.DisplayMode,
		ItemCount:              len(itemResponses),
		SubscriberCount:        int(subscriberCount),
		ContributorCount:       int(contributorCount),
		ForksCount:             int(forksCount),
		ForkedFromCollectionID: collection.ForkedFromCollectionID,
		ForkedFrom:             forkedFrom,
		Items:                  itemResponses,
		IsSubscribed:           isSubscribed,
		LikeCount:              int(likeCount),
		UserLikesThis:          userLikesThis,
		Tags:                   tags,
		CreatedAt:              collection.CreatedAt,
		UpdatedAt:              collection.UpdatedAt,
	}, nil
}

// resolveForkedFromInfo loads the minimal source-collection snapshot for
// inline attribution. Returns nil when:
//   - This collection wasn't forked (FK is nil), OR
//   - The source was deleted (FK was set to NULL by ON DELETE SET NULL).
//
// The frontend renders fallback copy ("Forked from a deleted collection")
// based on whether the FK is set but the snapshot is nil.
func (s *CollectionService) resolveForkedFromInfo(forkedFromID *uint) *contracts.ForkedFromInfo {
	if forkedFromID == nil || *forkedFromID == 0 {
		return nil
	}
	var source communitym.Collection
	err := s.db.Select("id, title, slug, creator_id").
		Where("id = ?", *forkedFromID).First(&source).Error
	if err != nil {
		// Source missing despite FK — treat as deleted.
		return nil
	}
	return &contracts.ForkedFromInfo{
		ID:          source.ID,
		Title:       source.Title,
		Slug:        source.Slug,
		CreatorID:   source.CreatorID,
		CreatorName: s.resolveUserName(source.CreatorID),
	}
}

// ListCollections retrieves collections with optional filtering
func (s *CollectionService) ListCollections(filters contracts.CollectionFilters, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&communitym.Collection{})

	if filters.PublicOnly {
		query = query.Where("is_public = ?", true)
		// PSY-356: quality gate for browse visibility. Items count is computed
		// later via batchCountItems(), so we can't filter post-fetch — use a
		// correlated subquery here. description is NOT NULL (defaults to ""),
		// so CHAR_LENGTH alone is sufficient.
		query = query.Where(
			"(SELECT COUNT(*) FROM collection_items ci WHERE ci.collection_id = collections.id) >= ?",
			MinPublicCollectionItems,
		)
		query = query.Where("CHAR_LENGTH(description) >= ?", MinPublicCollectionDescriptionChars)
	}
	if filters.CreatorID > 0 {
		query = query.Where("creator_id = ?", filters.CreatorID)
	}
	if filters.Featured {
		query = query.Where("is_featured = ?", true)
	}
	// PSY-355: expand search beyond title-only. We OR across four tiers:
	//   1. collections.title           (exact field on the row)
	//   2. collections.description     (raw markdown source on the row)
	//   3. any item's notes            (correlated EXISTS over collection_items)
	//   4. any applied tag name/alias  (correlated EXISTS over entity_tags +
	//                                  tags + tag_aliases for the polymorphic
	//                                  collection entity)
	// Whitespace-only queries are short-circuited at the handler boundary
	// (mirrors PSY-520 SearchShows). Any ILIKE pattern that is "" → "%" — also
	// handled at the handler. For safety this code still trims the value here
	// and skips the predicate when the trimmed string is empty, so direct
	// service callers (e.g. tests) don't accidentally widen the result set.
	//
	// No new indexes are added for the MVP — current corpus is small. If the
	// description / notes / tag-name predicates become hot, consider GIN
	// trigram indexes (`pg_trgm`) on `collections.description`,
	// `collection_items.notes`, `tags.name`, and `tag_aliases.alias`.
	searchTerm := strings.TrimSpace(filters.Search)
	if searchTerm != "" {
		pattern := "%" + searchTerm + "%"
		query = query.Where(`
			collections.title ILIKE ?
			OR collections.description ILIKE ?
			OR EXISTS (
				SELECT 1 FROM collection_items ci
				WHERE ci.collection_id = collections.id
				  AND ci.notes ILIKE ?
			)
			OR EXISTS (
				SELECT 1 FROM entity_tags et
				JOIN tags t ON t.id = et.tag_id
				LEFT JOIN tag_aliases ta ON ta.tag_id = t.id
				WHERE et.entity_type = ?
				  AND et.entity_id = collections.id
				  AND (t.name ILIKE ? OR ta.alias ILIKE ?)
			)
		`,
			pattern, pattern, pattern,
			catalogm.TagEntityCollection, pattern, pattern,
		)
	}
	if filters.EntityType != "" {
		query = query.Where("id IN (?)",
			s.db.Model(&communitym.CollectionItem{}).
				Select("DISTINCT collection_id").
				Where("entity_type = ?", filters.EntityType),
		)
	}
	// PSY-354: filter by a single tag slug when requested. Subquery joins
	// entity_tags → tags so callers can use the user-facing slug rather than
	// numeric tag IDs in the URL. No-op when no collection has the tag.
	if filters.Tag != "" {
		query = query.Where("collections.id IN (?)",
			s.db.Table("entity_tags").
				Select("entity_tags.entity_id").
				Joins("JOIN tags ON tags.id = entity_tags.tag_id").
				Where("entity_tags.entity_type = ? AND tags.slug = ?", catalogm.TagEntityCollection, filters.Tag),
		)
	}

	// Count total before pagination
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count collections: %w", err)
	}

	// Apply pagination and ordering
	if limit <= 0 {
		limit = 20
	}
	// PSY-355: when search is active AND the caller hasn't asked for an
	// explicit sort (e.g. ?sort=popular), lead with a tier rank that prefers
	// title matches, then description, then item notes, then tag matches.
	// `applyCollectionSort` always appends `updated_at DESC` as a fallback
	// tiebreaker. An explicit `sort=popular` wins over relevance — that's
	// the user's deliberate choice and mirrors how most browse UIs treat
	// "sort + filter" combinations.
	if searchTerm != "" && filters.Sort == "" {
		pattern := "%" + searchTerm + "%"
		query = applySearchRelevanceOrder(query, pattern).Limit(limit).Offset(offset)
	} else {
		query = applyCollectionSort(query, filters.Sort).Limit(limit).Offset(offset)
	}

	var collections []communitym.Collection
	if err := query.Find(&collections).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list collections: %w", err)
	}

	if len(collections) == 0 {
		return []*contracts.CollectionListResponse{}, total, nil
	}

	// Batch-load counts and creator names
	collectionIDs := make([]uint, len(collections))
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for i, c := range collections {
		collectionIDs[i] = c.ID
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	// Batch-load item counts
	itemCounts := s.batchCountItems(collectionIDs)

	// Batch-load subscriber counts
	subscriberCounts := s.batchCountSubscribers(collectionIDs)

	// Batch-load contributor counts
	contributorCounts := s.batchCountContributors(collectionIDs)

	// Batch-load fork counts (PSY-351)
	forkCounts := s.batchCountForks(collectionIDs)

	// Batch-load like counts and viewer's own like state (PSY-352)
	likeCounts := s.batchCountLikes(collectionIDs)
	userLikes := s.batchCheckUserLikes(filters.ViewerID, collectionIDs)

	// Batch-load entity type counts
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)

	// Batch-load tag chips (PSY-354). Returns map[collection_id][]TagSummary;
	// missing keys decode as nil — we coerce to empty slice in the per-row
	// build below so the JSON shape is always `tags: []`.
	tagsByCollection := s.batchListCollectionTagSummaries(collectionIDs)

	// Batch-load creator names
	creatorNames := s.batchResolveUserNames(creatorIDs)
	creatorUsernames := s.batchResolveUserUsernames(creatorIDs)

	// Build responses
	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		tags := tagsByCollection[c.ID]
		if tags == nil {
			tags = []contracts.TagSummary{}
		}
		responses[i] = &contracts.CollectionListResponse{
			ID:                     c.ID,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Description:            c.Description,
			DescriptionHTML:        s.renderMarkdown(c.Description),
			CreatorID:              c.CreatorID,
			CreatorName:            creatorNames[c.CreatorID],
			CreatorUsername:        creatorUsernames[c.CreatorID],
			Collaborative:          c.Collaborative,
			CoverImageURL:          c.CoverImageURL,
			IsPublic:               c.IsPublic,
			IsFeatured:             c.IsFeatured,
			DisplayMode:            c.DisplayMode,
			ItemCount:              itemCounts[c.ID],
			SubscriberCount:        subscriberCounts[c.ID],
			ContributorCount:       contributorCounts[c.ID],
			ForksCount:             forkCounts[c.ID],
			ForkedFromCollectionID: c.ForkedFromCollectionID,
			EntityTypeCounts:       entityTypeCounts[c.ID],
			LikeCount:              likeCounts[c.ID],
			UserLikesThis:          userLikes[c.ID],
			Tags:                   tags,
			CreatedAt:              c.CreatedAt,
			UpdatedAt:              c.UpdatedAt,
		}
	}

	return responses, total, nil
}

// applyCollectionSort applies the requested ordering to the list query.
// Default ("") is updated_at DESC. "popular" is HN gravity:
//
//	(like count) / POWER(age_in_hours + 2, 1.8) DESC, with updated_at DESC
//	as a tiebreaker for collections at equal gravity (rare but real).
//
// Unknown sort values fall back to the default — the handler validates
// recognized values and rejects unknowns before reaching this point.
// PSY-352.
//
// Note: when ?search is active and ?sort is unset, ListCollections calls
// applySearchRelevanceOrder instead — relevance ranks title > description >
// notes > tag matches. An explicit ?sort=popular still wins over relevance.
func applyCollectionSort(query *gorm.DB, sort string) *gorm.DB {
	if sort == contracts.CollectionSortPopular {
		return query.Order(`(
			SELECT COUNT(*) FROM collection_likes cl
			WHERE cl.collection_id = collections.id
		)::float / POWER(
			EXTRACT(EPOCH FROM (NOW() - collections.created_at))/3600 + 2, 1.8
		) DESC`).Order("collections.updated_at DESC")
	}
	return query.Order("updated_at DESC")
}

// applySearchRelevanceOrder is the search-rank ORDER BY clause used when
// `filters.Search` is set and no explicit sort was requested. Tiers:
//
//	1 — title matches the query
//	2 — description matches
//	3 — any item's notes match
//	4 — any applied tag name (or alias) matches
//
// Tiebreaker is updated_at DESC so the most-recently-edited collection in
// each tier surfaces first. The same `pattern` value (already wrapped with
// %s by the caller) is reused across all four CASE branches so it lines up
// with the WHERE clause in ListCollections — adjusting one without the
// other would silently mis-rank rows. PSY-355.
func applySearchRelevanceOrder(query *gorm.DB, pattern string) *gorm.DB {
	return query.Order(gorm.Expr(`
		CASE
			WHEN collections.title ILIKE ? THEN 1
			WHEN collections.description ILIKE ? THEN 2
			WHEN EXISTS (
				SELECT 1 FROM collection_items ci
				WHERE ci.collection_id = collections.id
				  AND ci.notes ILIKE ?
			) THEN 3
			WHEN EXISTS (
				SELECT 1 FROM entity_tags et
				JOIN tags t ON t.id = et.tag_id
				LEFT JOIN tag_aliases ta ON ta.tag_id = t.id
				WHERE et.entity_type = ?
				  AND et.entity_id = collections.id
				  AND (t.name ILIKE ? OR ta.alias ILIKE ?)
			) THEN 4
			ELSE 5
		END ASC
	`, pattern, pattern, pattern, catalogm.TagEntityCollection, pattern, pattern)).
		Order("collections.updated_at DESC")
}

// UpdateCollection updates an existing collection
func (s *CollectionService) UpdateCollection(slug string, userID uint, isAdmin bool, req *contracts.UpdateCollectionRequest) (*contracts.CollectionDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Check ownership
	if collection.CreatorID != userID && !isAdmin {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	updates := map[string]interface{}{}

	if req.Title != nil {
		updates["title"] = *req.Title
		// Regenerate slug when title changes
		baseSlug := utils.GenerateArtistSlug(*req.Title)
		newSlug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&communitym.Collection{}).Where("slug = ? AND id != ?", candidate, collection.ID).Count(&count)
			return count > 0
		})
		updates["slug"] = newSlug
	}
	if req.Description != nil {
		if len(*req.Description) > contracts.MaxCollectionDescriptionLength {
			return nil, fmt.Errorf("description exceeds maximum length of %d characters", contracts.MaxCollectionDescriptionLength)
		}
		updates["description"] = *req.Description
	}
	if req.Collaborative != nil {
		updates["collaborative"] = *req.Collaborative
	}
	if req.CoverImageURL != nil {
		updates["cover_image_url"] = *req.CoverImageURL
	}
	if req.IsPublic != nil {
		// PSY-356: forward gate at the false→true visibility transition.
		// Other transitions are unchanged: keeping public is grandfathered
		// (existing public collections below the gate stay editable),
		// and going from public to private is always allowed.
		if *req.IsPublic && !collection.IsPublic {
			var itemCount int64
			if err := s.db.Model(&communitym.CollectionItem{}).
				Where("collection_id = ?", collection.ID).
				Count(&itemCount).Error; err != nil {
				return nil, fmt.Errorf("failed to count items for publish gate: %w", err)
			}
			// Use the patched description when the same request is updating
			// it, so the curator can satisfy both halves of the gate in a
			// single PATCH instead of two round-trips.
			descToCheck := collection.Description
			if req.Description != nil {
				descToCheck = *req.Description
			}
			if err := validatePublishGate(int(itemCount), descToCheck); err != nil {
				return nil, err
			}
		}
		updates["is_public"] = *req.IsPublic
	}
	if req.DisplayMode != nil {
		if !communitym.IsValidCollectionDisplayMode(*req.DisplayMode) {
			return nil, apperrors.ErrCollectionInvalidRequest(
				fmt.Sprintf("display_mode must be 'ranked' or 'unranked', got %q", *req.DisplayMode),
			)
		}
		updates["display_mode"] = *req.DisplayMode
	}

	if len(updates) > 0 {
		err = s.db.Model(&communitym.Collection{}).Where("id = ?", collection.ID).Updates(updates).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update collection: %w", err)
		}
	}

	// Determine the slug to use for retrieval (may have changed)
	retrieveSlug := slug
	if newSlug, ok := updates["slug"].(string); ok {
		retrieveSlug = newSlug
	}

	return s.GetBySlug(retrieveSlug, userID)
}

// DeleteCollection deletes a collection
func (s *CollectionService) DeleteCollection(slug string, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	// Check ownership
	if collection.CreatorID != userID && !isAdmin {
		return apperrors.ErrCollectionForbidden(slug)
	}

	// Delete collection (FK cascades handle items and subscribers)
	if err := s.db.Delete(&collection).Error; err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	return nil
}

// AddItem adds an entity to a collection
func (s *CollectionService) AddItem(slug string, userID uint, req *contracts.AddCollectionItemRequest) (*contracts.CollectionItemResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Check permission: creator or (collaborative and authenticated)
	if collection.CreatorID != userID && !collection.Collaborative {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Validate notes length on save (mirrors comment body limit).
	if req.Notes != nil && len(*req.Notes) > contracts.MaxCollectionItemNotesLength {
		return nil, fmt.Errorf("notes exceed maximum length of %d characters", contracts.MaxCollectionItemNotesLength)
	}

	// Check for duplicate
	var existingCount int64
	s.db.Model(&communitym.CollectionItem{}).
		Where("collection_id = ? AND entity_type = ? AND entity_id = ?", collection.ID, req.EntityType, req.EntityID).
		Count(&existingCount)
	if existingCount > 0 {
		return nil, apperrors.ErrCollectionItemExists(collection.ID, req.EntityType, req.EntityID)
	}

	// Get max position
	var maxPosition int
	row := s.db.Model(&communitym.CollectionItem{}).
		Where("collection_id = ?", collection.ID).
		Select("COALESCE(MAX(position), -1)").
		Row()
	if row != nil {
		_ = row.Scan(&maxPosition)
	}

	item := &communitym.CollectionItem{
		CollectionID:  collection.ID,
		EntityType:    req.EntityType,
		EntityID:      req.EntityID,
		Position:      maxPosition + 1,
		AddedByUserID: userID,
		Notes:         req.Notes,
	}

	if err := s.db.Create(item).Error; err != nil {
		return nil, fmt.Errorf("failed to add item to collection: %w", err)
	}

	// PSY-350: collection-subscription digest notifications are emitted by
	// the lazy CollectionDigestService ticker, NOT fanned out here. The
	// ticker queries collection_items.created_at against each subscriber's
	// per-row cursor, so no synchronous notification work happens during
	// AddItem. This means AddItem cannot fail or slow due to a notification
	// path — the requirement is satisfied by construction.

	// Resolve entity name and slug
	entityName, entitySlug := s.resolveEntityNameAndSlug(req.EntityType, req.EntityID)
	addedByName := s.resolveUserName(userID)

	return &contracts.CollectionItemResponse{
		ID:            item.ID,
		EntityType:    item.EntityType,
		EntityID:      item.EntityID,
		EntityName:    entityName,
		EntitySlug:    entitySlug,
		Position:      item.Position,
		AddedByUserID: item.AddedByUserID,
		AddedByName:   addedByName,
		Notes:         item.Notes,
		NotesHTML:     s.renderNotes(item.Notes),
		CreatedAt:     item.CreatedAt,
	}, nil
}

// UpdateItem updates an item's notes in a collection
func (s *CollectionService) UpdateItem(slug string, itemID uint, userID uint, isAdmin bool, req *contracts.UpdateCollectionItemRequest) (*contracts.CollectionItemResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	var item communitym.CollectionItem
	err = s.db.Where("id = ? AND collection_id = ?", itemID, collection.ID).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionItemNotFound(itemID)
		}
		return nil, fmt.Errorf("failed to get collection item: %w", err)
	}

	// Check permission: collection creator, item adder, or admin
	if collection.CreatorID != userID && item.AddedByUserID != userID && !isAdmin {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Validate notes length on save (mirrors comment body limit).
	if req.Notes != nil && len(*req.Notes) > contracts.MaxCollectionItemNotesLength {
		return nil, fmt.Errorf("notes exceed maximum length of %d characters", contracts.MaxCollectionItemNotesLength)
	}

	// Update notes
	if req.Notes != nil {
		item.Notes = req.Notes
	}

	if err := s.db.Save(&item).Error; err != nil {
		return nil, fmt.Errorf("failed to update collection item: %w", err)
	}

	// Resolve entity name and slug
	entityName, entitySlug := s.resolveEntityNameAndSlug(item.EntityType, item.EntityID)
	addedByName := s.resolveUserName(item.AddedByUserID)

	return &contracts.CollectionItemResponse{
		ID:            item.ID,
		EntityType:    item.EntityType,
		EntityID:      item.EntityID,
		EntityName:    entityName,
		EntitySlug:    entitySlug,
		Position:      item.Position,
		AddedByUserID: item.AddedByUserID,
		AddedByName:   addedByName,
		Notes:         item.Notes,
		NotesHTML:     s.renderNotes(item.Notes),
		CreatedAt:     item.CreatedAt,
	}, nil
}

// RemoveItem removes an item from a collection
func (s *CollectionService) RemoveItem(slug string, itemID uint, userID uint, isAdmin bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	var item communitym.CollectionItem
	err = s.db.Where("id = ? AND collection_id = ?", itemID, collection.ID).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionItemNotFound(itemID)
		}
		return fmt.Errorf("failed to get collection item: %w", err)
	}

	// Check permission: collection creator, item adder, or admin
	if collection.CreatorID != userID && item.AddedByUserID != userID && !isAdmin {
		return apperrors.ErrCollectionForbidden(slug)
	}

	if err := s.db.Delete(&item).Error; err != nil {
		return fmt.Errorf("failed to remove item from collection: %w", err)
	}

	return nil
}

// ReorderItems reorders items in a collection
func (s *CollectionService) ReorderItems(slug string, userID uint, req *contracts.ReorderCollectionItemsRequest) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	// Only creator can reorder
	if collection.CreatorID != userID {
		return apperrors.ErrCollectionForbidden(slug)
	}

	// Update positions in a transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Items {
			err := tx.Model(&communitym.CollectionItem{}).
				Where("id = ? AND collection_id = ?", item.ItemID, collection.ID).
				Update("position", item.Position).Error
			if err != nil {
				return fmt.Errorf("failed to update item position: %w", err)
			}
		}
		return nil
	})
}

// Subscribe subscribes a user to a collection
func (s *CollectionService) Subscribe(slug string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	// Check if collection is accessible
	if !collection.IsPublic && collection.CreatorID != userID {
		return apperrors.ErrCollectionForbidden(slug)
	}

	now := time.Now()
	subscriber := &communitym.CollectionSubscriber{
		CollectionID:  collection.ID,
		UserID:        userID,
		LastVisitedAt: &now,
	}

	// Use FirstOrCreate to handle idempotent subscribe
	result := s.db.Where("collection_id = ? AND user_id = ?", collection.ID, userID).
		FirstOrCreate(subscriber)
	if result.Error != nil {
		return fmt.Errorf("failed to subscribe to collection: %w", result.Error)
	}

	return nil
}

// Unsubscribe removes a user's subscription from a collection
func (s *CollectionService) Unsubscribe(slug string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	result := s.db.Where("collection_id = ? AND user_id = ?", collection.ID, userID).
		Delete(&communitym.CollectionSubscriber{})
	if result.Error != nil {
		return fmt.Errorf("failed to unsubscribe from collection: %w", result.Error)
	}

	return nil
}

// Like records a user's like on the collection. Idempotent — likes are
// composite-PK rows so an INSERT ... ON CONFLICT DO NOTHING is sufficient.
// Returns the post-mutation aggregate count and the caller's like state
// so the handler can return them without a follow-up query.
//
// Visibility: anyone authenticated can like any public collection (or
// their own private collection). Liking a private collection you don't
// own is rejected (404 / 403 mapped at the handler layer). PSY-352.
func (s *CollectionService) Like(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	if !collection.IsPublic && collection.CreatorID != userID {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// Idempotent insert. ON CONFLICT DO NOTHING is the canonical pattern
	// for composite-PK toggles — no transaction needed and no error on
	// re-like.
	if err := s.db.Exec(
		"INSERT INTO collection_likes (user_id, collection_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
		userID, collection.ID,
	).Error; err != nil {
		return nil, fmt.Errorf("failed to like collection: %w", err)
	}

	return s.buildLikeResponse(collection.ID, userID)
}

// Unlike removes a user's like on the collection. Idempotent — DELETE on
// a row that doesn't exist is a no-op. Returns the post-mutation aggregate.
// PSY-352.
func (s *CollectionService) Unlike(slug string, userID uint) (*contracts.CollectionLikeResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	if !collection.IsPublic && collection.CreatorID != userID {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	if err := s.db.Where("user_id = ? AND collection_id = ?", userID, collection.ID).
		Delete(&communitym.CollectionLike{}).Error; err != nil {
		return nil, fmt.Errorf("failed to unlike collection: %w", err)
	}

	return s.buildLikeResponse(collection.ID, userID)
}

// buildLikeResponse loads the post-mutation aggregate count and the caller's
// like state for the given collection. PSY-352.
func (s *CollectionService) buildLikeResponse(collectionID uint, userID uint) (*contracts.CollectionLikeResponse, error) {
	var likeCount int64
	if err := s.db.Model(&communitym.CollectionLike{}).
		Where("collection_id = ?", collectionID).
		Count(&likeCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count likes: %w", err)
	}

	userLikes := false
	if userID > 0 {
		var ulCount int64
		s.db.Model(&communitym.CollectionLike{}).
			Where("collection_id = ? AND user_id = ?", collectionID, userID).
			Count(&ulCount)
		userLikes = ulCount > 0
	}

	return &contracts.CollectionLikeResponse{
		LikeCount:     int(likeCount),
		UserLikesThis: userLikes,
	}, nil
}

// MarkVisited updates the last_visited_at timestamp for a subscriber
func (s *CollectionService) MarkVisited(slug string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	now := time.Now()
	result := s.db.Model(&communitym.CollectionSubscriber{}).
		Where("collection_id = ? AND user_id = ?", collection.ID, userID).
		Update("last_visited_at", now)
	if result.Error != nil {
		return fmt.Errorf("failed to mark collection visited: %w", result.Error)
	}

	return nil
}

// GetStats retrieves statistics for a collection
func (s *CollectionService) GetStats(slug string) (*contracts.CollectionStatsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Item count
	var itemCount int64
	s.db.Model(&communitym.CollectionItem{}).Where("collection_id = ?", collection.ID).Count(&itemCount)

	// Subscriber count
	var subscriberCount int64
	s.db.Model(&communitym.CollectionSubscriber{}).Where("collection_id = ?", collection.ID).Count(&subscriberCount)

	// Contributor count (distinct users who added items)
	var contributorCount int64
	s.db.Model(&communitym.CollectionItem{}).Where("collection_id = ?", collection.ID).
		Distinct("added_by_user_id").Count(&contributorCount)

	// Entity type counts
	type TypeCount struct {
		EntityType string
		Count      int
	}
	var typeCounts []TypeCount
	s.db.Model(&communitym.CollectionItem{}).
		Select("entity_type, COUNT(*) as count").
		Where("collection_id = ?", collection.ID).
		Group("entity_type").
		Find(&typeCounts)

	entityTypeCounts := make(map[string]int)
	for _, tc := range typeCounts {
		entityTypeCounts[tc.EntityType] = tc.Count
	}

	return &contracts.CollectionStatsResponse{
		ItemCount:        int(itemCount),
		SubscriberCount:  int(subscriberCount),
		ContributorCount: int(contributorCount),
		EntityTypeCounts: entityTypeCounts,
	}, nil
}

// GetUserCollections retrieves collections created by or subscribed to by a user
func (s *CollectionService) GetUserCollections(userID uint, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	// Get collection IDs the user created or is subscribed to
	subQuery := s.db.Model(&communitym.CollectionSubscriber{}).
		Select("collection_id").
		Where("user_id = ?", userID)

	query := s.db.Model(&communitym.Collection{}).
		Where("creator_id = ? OR id IN (?)", userID, subQuery)

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user collections: %w", err)
	}

	var collections []communitym.Collection
	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&collections).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get user collections: %w", err)
	}

	if len(collections) == 0 {
		return []*contracts.CollectionListResponse{}, total, nil
	}

	// Batch-load counts and creator names
	collectionIDs := make([]uint, len(collections))
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for i, c := range collections {
		collectionIDs[i] = c.ID
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	itemCounts := s.batchCountItems(collectionIDs)
	subscriberCounts := s.batchCountSubscribers(collectionIDs)
	contributorCounts := s.batchCountContributors(collectionIDs)
	forkCounts := s.batchCountForks(collectionIDs)
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)
	creatorNames := s.batchResolveUserNames(creatorIDs)
	creatorUsernames := s.batchResolveUserUsernames(creatorIDs)
	// PSY-350: per-(user, collection) "new since last visit" counts so the
	// library tab can render a "N new" badge per subscribed collection.
	newCounts := s.batchCountNewSinceLastVisit(userID, collectionIDs)
	// PSY-352: like aggregates and viewer's own like state.
	likeCounts := s.batchCountLikes(collectionIDs)
	userLikes := s.batchCheckUserLikes(userID, collectionIDs)
	// PSY-354: tag chips on library cards.
	tagsByCollection := s.batchListCollectionTagSummaries(collectionIDs)

	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		tags := tagsByCollection[c.ID]
		if tags == nil {
			tags = []contracts.TagSummary{}
		}
		responses[i] = &contracts.CollectionListResponse{
			ID:                     c.ID,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Description:            c.Description,
			DescriptionHTML:        s.renderMarkdown(c.Description),
			CreatorID:              c.CreatorID,
			CreatorName:            creatorNames[c.CreatorID],
			CreatorUsername:        creatorUsernames[c.CreatorID],
			Collaborative:          c.Collaborative,
			CoverImageURL:          c.CoverImageURL,
			IsPublic:               c.IsPublic,
			IsFeatured:             c.IsFeatured,
			DisplayMode:            c.DisplayMode,
			ItemCount:              itemCounts[c.ID],
			SubscriberCount:        subscriberCounts[c.ID],
			ContributorCount:       contributorCounts[c.ID],
			ForksCount:             forkCounts[c.ID],
			ForkedFromCollectionID: c.ForkedFromCollectionID,
			EntityTypeCounts:       entityTypeCounts[c.ID],
			NewSinceLastVisit:      newCounts[c.ID],
			LikeCount:              likeCounts[c.ID],
			UserLikesThis:          userLikes[c.ID],
			Tags:                   tags,
			CreatedAt:              c.CreatedAt,
			UpdatedAt:              c.UpdatedAt,
		}
	}

	return responses, total, nil
}

// GetUserCollectionsContainingEntity returns the IDs of the user's editable
// collections (creator OR collaborative-and-subscribed) that already contain
// the supplied entity. PSY-359 — backs the multi-select Add-to-Collection
// popover so it can pre-check rows the user has already added the entity to.
//
// Single round-trip; one indexed lookup. Caller scopes the result to the
// candidate collections shown in the popover by intersecting with the user's
// own collection list (already cached). Returns an empty slice for an
// unauthenticated caller (userID == 0) — not an error.
func (s *CollectionService) GetUserCollectionsContainingEntity(userID uint, entityType string, entityID uint) ([]uint, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if userID == 0 {
		return []uint{}, nil
	}

	// Mirror GetUserCollections's scope: collections the user CREATED or
	// is SUBSCRIBED to. The popover only adds to creator-owned collections
	// today, but collaborative collections shown in the user's library
	// should also pre-check correctly.
	subQuery := s.db.Model(&communitym.CollectionSubscriber{}).
		Select("collection_id").
		Where("user_id = ?", userID)

	var ids []uint
	err := s.db.Model(&communitym.CollectionItem{}).
		Distinct("collection_items.collection_id").
		Joins("JOIN collections ON collections.id = collection_items.collection_id").
		Where("collection_items.entity_type = ? AND collection_items.entity_id = ?", entityType, entityID).
		Where("collections.creator_id = ? OR collections.id IN (?)", userID, subQuery).
		Pluck("collection_items.collection_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("failed to look up containing collections: %w", err)
	}
	if ids == nil {
		ids = []uint{}
	}
	return ids, nil
}

// GetEntityCollections returns public collections that contain the given entity
func (s *CollectionService) GetEntityCollections(entityType string, entityID uint, limit int) ([]*contracts.CollectionListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	// Find collection IDs that contain this entity (public collections only)
	var collectionIDs []uint
	err := s.db.Model(&communitym.CollectionItem{}).
		Select("DISTINCT collection_items.collection_id").
		Joins("JOIN collections ON collections.id = collection_items.collection_id").
		Where("collection_items.entity_type = ? AND collection_items.entity_id = ? AND collections.is_public = ?", entityType, entityID, true).
		Limit(limit).
		Pluck("collection_items.collection_id", &collectionIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find entity collections: %w", err)
	}

	if len(collectionIDs) == 0 {
		return []*contracts.CollectionListResponse{}, nil
	}

	var collections []communitym.Collection
	if err := s.db.Where("id IN ?", collectionIDs).Order("updated_at DESC").Find(&collections).Error; err != nil {
		return nil, fmt.Errorf("failed to load entity collections: %w", err)
	}

	// Batch-load counts and creator names
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for _, c := range collections {
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	itemCounts := s.batchCountItems(collectionIDs)
	subscriberCounts := s.batchCountSubscribers(collectionIDs)
	contributorCounts := s.batchCountContributors(collectionIDs)
	forkCounts := s.batchCountForks(collectionIDs)
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)
	creatorNames := s.batchResolveUserNames(creatorIDs)
	creatorUsernames := s.batchResolveUserUsernames(creatorIDs)
	// PSY-352: like aggregate; viewer ID isn't threaded through this call,
	// so UserLikesThis is left false here (clients that need it should
	// use the detail endpoint).
	likeCounts := s.batchCountLikes(collectionIDs)
	// PSY-354: tag chips on entity-collection cards.
	tagsByCollection := s.batchListCollectionTagSummaries(collectionIDs)

	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		tags := tagsByCollection[c.ID]
		if tags == nil {
			tags = []contracts.TagSummary{}
		}
		responses[i] = &contracts.CollectionListResponse{
			ID:                     c.ID,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Description:            c.Description,
			DescriptionHTML:        s.renderMarkdown(c.Description),
			CreatorID:              c.CreatorID,
			CreatorName:            creatorNames[c.CreatorID],
			CreatorUsername:        creatorUsernames[c.CreatorID],
			Collaborative:          c.Collaborative,
			CoverImageURL:          c.CoverImageURL,
			IsPublic:               c.IsPublic,
			IsFeatured:             c.IsFeatured,
			DisplayMode:            c.DisplayMode,
			ItemCount:              itemCounts[c.ID],
			SubscriberCount:        subscriberCounts[c.ID],
			ContributorCount:       contributorCounts[c.ID],
			ForksCount:             forkCounts[c.ID],
			ForkedFromCollectionID: c.ForkedFromCollectionID,
			EntityTypeCounts:       entityTypeCounts[c.ID],
			LikeCount:              likeCounts[c.ID],
			Tags:                   tags,
			CreatedAt:              c.CreatedAt,
			UpdatedAt:              c.UpdatedAt,
		}
	}

	return responses, nil
}

// GetUserPublicCollections returns public collections created by a specific user
func (s *CollectionService) GetUserPublicCollections(userID uint, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	query := s.db.Model(&communitym.Collection{}).
		Where("creator_id = ? AND is_public = ?", userID, true)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count user public collections: %w", err)
	}

	var collections []communitym.Collection
	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&collections).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get user public collections: %w", err)
	}

	if len(collections) == 0 {
		return []*contracts.CollectionListResponse{}, total, nil
	}

	collectionIDs := make([]uint, len(collections))
	creatorIDs := make([]uint, 0)
	creatorIDSet := make(map[uint]bool)
	for i, c := range collections {
		collectionIDs[i] = c.ID
		if !creatorIDSet[c.CreatorID] {
			creatorIDs = append(creatorIDs, c.CreatorID)
			creatorIDSet[c.CreatorID] = true
		}
	}

	itemCounts := s.batchCountItems(collectionIDs)
	subscriberCounts := s.batchCountSubscribers(collectionIDs)
	contributorCounts := s.batchCountContributors(collectionIDs)
	forkCounts := s.batchCountForks(collectionIDs)
	entityTypeCounts := s.batchEntityTypeCounts(collectionIDs)
	creatorNames := s.batchResolveUserNames(creatorIDs)
	creatorUsernames := s.batchResolveUserUsernames(creatorIDs)
	// PSY-352: like aggregate; viewer ID is not threaded through this call,
	// so UserLikesThis is left false here.
	likeCounts := s.batchCountLikes(collectionIDs)
	// PSY-354: tag chips on profile-page cards.
	tagsByCollection := s.batchListCollectionTagSummaries(collectionIDs)

	responses := make([]*contracts.CollectionListResponse, len(collections))
	for i, c := range collections {
		tags := tagsByCollection[c.ID]
		if tags == nil {
			tags = []contracts.TagSummary{}
		}
		responses[i] = &contracts.CollectionListResponse{
			ID:                     c.ID,
			Title:                  c.Title,
			Slug:                   c.Slug,
			Description:            c.Description,
			DescriptionHTML:        s.renderMarkdown(c.Description),
			CreatorID:              c.CreatorID,
			CreatorName:            creatorNames[c.CreatorID],
			CreatorUsername:        creatorUsernames[c.CreatorID],
			Collaborative:          c.Collaborative,
			CoverImageURL:          c.CoverImageURL,
			IsPublic:               c.IsPublic,
			IsFeatured:             c.IsFeatured,
			DisplayMode:            c.DisplayMode,
			ItemCount:              itemCounts[c.ID],
			SubscriberCount:        subscriberCounts[c.ID],
			ContributorCount:       contributorCounts[c.ID],
			ForksCount:             forkCounts[c.ID],
			ForkedFromCollectionID: c.ForkedFromCollectionID,
			EntityTypeCounts:       entityTypeCounts[c.ID],
			LikeCount:              likeCounts[c.ID],
			Tags:                   tags,
			CreatedAt:              c.CreatedAt,
			UpdatedAt:              c.UpdatedAt,
		}
	}

	return responses, total, nil
}

// GetUserPublicCollectionsByUsername returns public collections by username lookup
func (s *CollectionService) GetUserPublicCollectionsByUsername(username string, limit, offset int) ([]*contracts.CollectionListResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Look up user by username
	var user authm.User
	err := s.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		// User not found - return empty
		return []*contracts.CollectionListResponse{}, 0, nil
	}

	return s.GetUserPublicCollections(user.ID, limit, offset)
}

// SetFeatured sets or unsets the featured flag on a collection
func (s *CollectionService) SetFeatured(slug string, featured bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	err := s.db.Where("slug = ?", slug).First(&collection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	if err := s.db.Model(&collection).Update("is_featured", featured).Error; err != nil {
		return fmt.Errorf("failed to update featured status: %w", err)
	}

	return nil
}

// ============================================================================
// Helper methods
// ============================================================================

// resolveUserUsername returns the user's username (for /users/:username
// links) or nil when the user has no username set. Distinct from
// resolveUserName, which falls back to first/last/email so it can never be
// safely used in a URL slug. PSY-353.
func (s *CollectionService) resolveUserUsername(userID uint) *string {
	var user authm.User
	if err := s.db.Select("id, username").First(&user, userID).Error; err != nil {
		return nil
	}
	if user.Username == nil || *user.Username == "" {
		return nil
	}
	username := *user.Username
	return &username
}

// batchResolveUserUsernames resolves usernames for multiple user IDs.
// Map values are nil-pointer when the user has no username — callers should
// treat that as "render unlinked". PSY-353.
func (s *CollectionService) batchResolveUserUsernames(userIDs []uint) map[uint]*string {
	result := make(map[uint]*string)
	if len(userIDs) == 0 {
		return result
	}
	var users []authm.User
	s.db.Select("id, username").Where("id IN ?", userIDs).Find(&users)
	for _, user := range users {
		if user.Username != nil && *user.Username != "" {
			username := *user.Username
			result[user.ID] = &username
		} else {
			result[user.ID] = nil
		}
	}
	return result
}

// resolveUserName returns the display name for a user ID
func (s *CollectionService) resolveUserName(userID uint) string {
	var user authm.User
	if err := s.db.Select("id, username, first_name, last_name, email").First(&user, userID).Error; err != nil {
		return "Anonymous"
	}
	if user.Username != nil && *user.Username != "" {
		return *user.Username
	}
	if user.FirstName != nil && *user.FirstName != "" {
		name := *user.FirstName
		if user.LastName != nil && *user.LastName != "" {
			name += " " + *user.LastName
		}
		return name
	}
	if user.Email != nil && *user.Email != "" {
		if idx := strings.Index(*user.Email, "@"); idx > 0 {
			return (*user.Email)[:idx]
		}
	}
	return "Anonymous"
}

// batchResolveUserNames resolves user names for multiple user IDs
func (s *CollectionService) batchResolveUserNames(userIDs []uint) map[uint]string {
	result := make(map[uint]string)
	if len(userIDs) == 0 {
		return result
	}

	var users []authm.User
	s.db.Select("id, username, first_name, last_name, email").Where("id IN ?", userIDs).Find(&users)

	for _, user := range users {
		if user.Username != nil && *user.Username != "" {
			result[user.ID] = *user.Username
		} else if user.FirstName != nil && *user.FirstName != "" {
			name := *user.FirstName
			if user.LastName != nil && *user.LastName != "" {
				name += " " + *user.LastName
			}
			result[user.ID] = name
		} else if user.Email != nil && *user.Email != "" {
			if idx := strings.Index(*user.Email, "@"); idx > 0 {
				result[user.ID] = (*user.Email)[:idx]
			} else {
				result[user.ID] = "Anonymous"
			}
		} else {
			result[user.ID] = "Anonymous"
		}
	}

	return result
}

// resolveEntityNameAndSlug resolves the name and slug for a single entity
func (s *CollectionService) resolveEntityNameAndSlug(entityType string, entityID uint) (string, string) {
	switch entityType {
	case communitym.CollectionEntityArtist:
		var artist catalogm.Artist
		if err := s.db.Select("id, name, slug").First(&artist, entityID).Error; err == nil {
			slug := ""
			if artist.Slug != nil {
				slug = *artist.Slug
			}
			return artist.Name, slug
		}
	case communitym.CollectionEntityVenue:
		var venue catalogm.Venue
		if err := s.db.Select("id, name, slug").First(&venue, entityID).Error; err == nil {
			slug := ""
			if venue.Slug != nil {
				slug = *venue.Slug
			}
			return venue.Name, slug
		}
	case communitym.CollectionEntityShow:
		var show catalogm.Show
		if err := s.db.Select("id, title, slug").First(&show, entityID).Error; err == nil {
			slug := ""
			if show.Slug != nil {
				slug = *show.Slug
			} else {
				slug = strconv.FormatUint(uint64(show.ID), 10)
			}
			return show.Title, slug
		}
	case communitym.CollectionEntityRelease:
		var release catalogm.Release
		if err := s.db.Select("id, title, slug").First(&release, entityID).Error; err == nil {
			slug := ""
			if release.Slug != nil {
				slug = *release.Slug
			}
			return release.Title, slug
		}
	case communitym.CollectionEntityLabel:
		var label catalogm.Label
		if err := s.db.Select("id, name, slug").First(&label, entityID).Error; err == nil {
			slug := ""
			if label.Slug != nil {
				slug = *label.Slug
			}
			return label.Name, slug
		}
	case communitym.CollectionEntityFestival:
		var festival catalogm.Festival
		if err := s.db.Select("id, name, slug").First(&festival, entityID).Error; err == nil {
			return festival.Name, festival.Slug
		}
	}
	return "Unknown", ""
}

// buildItemResponses converts model items to response items with resolved entity names
func (s *CollectionService) buildItemResponses(items []communitym.CollectionItem) []contracts.CollectionItemResponse {
	if len(items) == 0 {
		return []contracts.CollectionItemResponse{}
	}

	// Group entity IDs by type for batch resolution
	entityIDsByType := make(map[string][]uint)
	userIDs := make([]uint, 0)
	userIDSet := make(map[uint]bool)

	for _, item := range items {
		entityIDsByType[item.EntityType] = append(entityIDsByType[item.EntityType], item.EntityID)
		if !userIDSet[item.AddedByUserID] {
			userIDs = append(userIDs, item.AddedByUserID)
			userIDSet[item.AddedByUserID] = true
		}
	}

	// Batch-resolve entity names, slugs, and images. Images are returned as a
	// separate map (rather than folded into the names/slugs tuple) because
	// only release + festival populate it today, and a separate map keeps the
	// nil-vs-empty distinction clean per row (PSY-360).
	entityNames, entitySlugs, entityImages := s.batchResolveEntityNames(entityIDsByType)

	// Batch-resolve user names
	userNames := s.batchResolveUserNames(userIDs)

	// Build responses
	responses := make([]contracts.CollectionItemResponse, len(items))
	for i, item := range items {
		key := fmt.Sprintf("%s:%d", item.EntityType, item.EntityID)
		responses[i] = contracts.CollectionItemResponse{
			ID:            item.ID,
			EntityType:    item.EntityType,
			EntityID:      item.EntityID,
			EntityName:    entityNames[key],
			EntitySlug:    entitySlugs[key],
			ImageURL:      entityImages[key],
			Position:      item.Position,
			AddedByUserID: item.AddedByUserID,
			AddedByName:   userNames[item.AddedByUserID],
			Notes:         item.Notes,
			NotesHTML:     s.renderNotes(item.Notes),
			CreatedAt:     item.CreatedAt,
		}
	}

	return responses
}

// batchResolveEntityNames resolves names, slugs, and image URLs for groups of
// entities by type. Each entity table stores its image in a column whose name
// matches the entity's domain language: release.cover_art_url, festival.flyer_url,
// and artist/venue/show/label.image_url (PSY-521). The batch resolver
// normalizes them all into the same `images` map for the collection grid.
func (s *CollectionService) batchResolveEntityNames(entityIDsByType map[string][]uint) (map[string]string, map[string]string, map[string]*string) {
	names := make(map[string]string)
	slugs := make(map[string]string)
	images := make(map[string]*string)

	for entityType, ids := range entityIDsByType {
		if len(ids) == 0 {
			continue
		}

		switch entityType {
		case communitym.CollectionEntityArtist:
			var artists []catalogm.Artist
			s.db.Select("id, name, slug, image_url").Where("id IN ?", ids).Find(&artists)
			for _, a := range artists {
				key := fmt.Sprintf("%s:%d", entityType, a.ID)
				names[key] = a.Name
				if a.Slug != nil {
					slugs[key] = *a.Slug
				}
				images[key] = nonEmptyImageURL(a.ImageURL)
			}

		case communitym.CollectionEntityVenue:
			var venues []catalogm.Venue
			s.db.Select("id, name, slug, image_url").Where("id IN ?", ids).Find(&venues)
			for _, v := range venues {
				key := fmt.Sprintf("%s:%d", entityType, v.ID)
				names[key] = v.Name
				if v.Slug != nil {
					slugs[key] = *v.Slug
				}
				images[key] = nonEmptyImageURL(v.ImageURL)
			}

		case communitym.CollectionEntityShow:
			var shows []catalogm.Show
			s.db.Select("id, title, slug, image_url").Where("id IN ?", ids).Find(&shows)
			for _, sh := range shows {
				key := fmt.Sprintf("%s:%d", entityType, sh.ID)
				names[key] = sh.Title
				if sh.Slug != nil {
					slugs[key] = *sh.Slug
				} else {
					slugs[key] = strconv.FormatUint(uint64(sh.ID), 10)
				}
				images[key] = nonEmptyImageURL(sh.ImageURL)
			}

		case communitym.CollectionEntityRelease:
			var releases []catalogm.Release
			s.db.Select("id, title, slug, cover_art_url").Where("id IN ?", ids).Find(&releases)
			for _, r := range releases {
				key := fmt.Sprintf("%s:%d", entityType, r.ID)
				names[key] = r.Title
				if r.Slug != nil {
					slugs[key] = *r.Slug
				}
				images[key] = nonEmptyImageURL(r.CoverArtURL)
			}

		case communitym.CollectionEntityLabel:
			var labels []catalogm.Label
			s.db.Select("id, name, slug, image_url").Where("id IN ?", ids).Find(&labels)
			for _, l := range labels {
				key := fmt.Sprintf("%s:%d", entityType, l.ID)
				names[key] = l.Name
				if l.Slug != nil {
					slugs[key] = *l.Slug
				}
				images[key] = nonEmptyImageURL(l.ImageURL)
			}

		case communitym.CollectionEntityFestival:
			var festivals []catalogm.Festival
			s.db.Select("id, name, slug, flyer_url").Where("id IN ?", ids).Find(&festivals)
			for _, f := range festivals {
				key := fmt.Sprintf("%s:%d", entityType, f.ID)
				names[key] = f.Name
				slugs[key] = f.Slug
				images[key] = nonEmptyImageURL(f.FlyerURL)
			}
		}
	}

	return names, slugs, images
}

// nonEmptyImageURL normalizes an entity's nullable image column. Returns nil
// when the source is nil OR when the stored value is whitespace-only — both
// cases mean "no image" to the frontend grid (PSY-360). Without this, an
// empty string would render an `<img src="">` tag in the browser, which most
// browsers turn into a broken-image icon.
func nonEmptyImageURL(src *string) *string {
	if src == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*src)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// batchCountItems returns item counts per collection ID
func (s *CollectionService) batchCountItems(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&communitym.CollectionItem{}).
		Select("collection_id, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchEntityTypeCounts returns a breakdown of item counts by entity type per collection ID.
// Used to surface entity type badges on collection cards.
func (s *CollectionService) batchEntityTypeCounts(collectionIDs []uint) map[uint]map[string]int {
	result := make(map[uint]map[string]int)
	if len(collectionIDs) == 0 {
		return result
	}

	type Row struct {
		CollectionID uint
		EntityType   string
		Count        int
	}
	var rows []Row
	s.db.Model(&communitym.CollectionItem{}).
		Select("collection_id, entity_type, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id, entity_type").
		Find(&rows)

	for _, r := range rows {
		if _, ok := result[r.CollectionID]; !ok {
			result[r.CollectionID] = make(map[string]int)
		}
		result[r.CollectionID][r.EntityType] = r.Count
	}
	return result
}

// batchCountSubscribers returns subscriber counts per collection ID
func (s *CollectionService) batchCountSubscribers(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&communitym.CollectionSubscriber{}).
		Select("collection_id, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchCountNewSinceLastVisit returns, per collection in `collectionIDs`, the
// number of items added after the user's `last_visited_at` cursor on the
// subscription. PSY-350.
//
// Only populated for collections the user is actually subscribed to —
// non-subscribed collection IDs simply don't appear in the result map (zero
// values when looked up). Collections never visited (last_visited_at IS
// NULL) fall back to "all items added by other users since subscribed".
func (s *CollectionService) batchCountNewSinceLastVisit(userID uint, collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 || userID == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult

	// Items added by *other* users since the viewer last visited (or, when
	// the viewer has never visited, since they subscribed). We exclude
	// the viewer's own additions because seeing your own work as "new" is
	// noise.
	err := s.db.Raw(`
		SELECT cs.collection_id, COUNT(*) AS count
		FROM collection_subscribers cs
		JOIN collection_items ci
			ON ci.collection_id = cs.collection_id
			AND ci.added_by_user_id <> cs.user_id
			AND ci.created_at > COALESCE(cs.last_visited_at, cs.created_at)
		WHERE cs.user_id = ?
			AND cs.collection_id IN ?
		GROUP BY cs.collection_id
	`, userID, collectionIDs).Scan(&results).Error
	if err != nil {
		// Non-fatal — surface as zero counts in the response.
		return counts
	}

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchCountContributors returns distinct contributor counts per collection ID
func (s *CollectionService) batchCountContributors(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&communitym.CollectionItem{}).
		Select("collection_id, COUNT(DISTINCT added_by_user_id) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchCountForks returns fork counts (number of cloned children) keyed by
// source collection ID. Live COUNT mirrors the existing collection counter
// pattern. PSY-351.
func (s *CollectionService) batchCountForks(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type Row struct {
		ForkedFromCollectionID uint
		Count                  int
	}
	var rows []Row
	s.db.Model(&communitym.Collection{}).
		Select("forked_from_collection_id, COUNT(*) as count").
		Where("forked_from_collection_id IN ?", collectionIDs).
		Group("forked_from_collection_id").
		Find(&rows)

	for _, r := range rows {
		counts[r.ForkedFromCollectionID] = r.Count
	}
	return counts
}

// batchCountLikes returns like counts keyed by collection ID. Live COUNT —
// mirrors batchCountSubscribers / batchCountForks. PSY-352.
func (s *CollectionService) batchCountLikes(collectionIDs []uint) map[uint]int {
	counts := make(map[uint]int)
	if len(collectionIDs) == 0 {
		return counts
	}

	type CountResult struct {
		CollectionID uint
		Count        int
	}
	var results []CountResult
	s.db.Model(&communitym.CollectionLike{}).
		Select("collection_id, COUNT(*) as count").
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&results)

	for _, r := range results {
		counts[r.CollectionID] = r.Count
	}
	return counts
}

// batchCheckUserLikes returns a set (as map) of collection IDs the user has
// liked, drawn from the supplied candidate IDs. Empty for unauthenticated
// viewers (userID == 0). PSY-352.
func (s *CollectionService) batchCheckUserLikes(userID uint, collectionIDs []uint) map[uint]bool {
	result := make(map[uint]bool)
	if userID == 0 || len(collectionIDs) == 0 {
		return result
	}

	var rows []communitym.CollectionLike
	s.db.Select("collection_id").
		Where("user_id = ? AND collection_id IN ?", userID, collectionIDs).
		Find(&rows)

	for _, r := range rows {
		result[r.CollectionID] = true
	}
	return result
}

// ============================================================================
// Collection tags (PSY-354)
// ============================================================================

// canEditCollectionTags returns true when the user has permission to
// add/remove tags on the given collection. Mirrors the AddItem rule:
// the creator can always edit, plus any authenticated user when the
// collection is collaborative. Anonymous callers (userID == 0) are
// always rejected.
func (s *CollectionService) canEditCollectionTags(collection *communitym.Collection, userID uint) bool {
	if userID == 0 {
		return false
	}
	if collection.CreatorID == userID {
		return true
	}
	return collection.Collaborative
}

// AddTagToCollection applies a tag to a collection (PSY-354). Reuses the
// polymorphic tag service for the tag/alias resolution + inline-creation
// path, then enforces:
//   - max-10 tags per collection (rejects 11th with 400),
//   - edit-access (creator OR collaborative-and-authenticated),
//   - default category "other" when creating a new tag inline.
//
// Returns the post-mutation tag list so the frontend can refresh the chip
// row from a single round-trip.
func (s *CollectionService) AddTagToCollection(slug string, userID uint, req *contracts.AddCollectionTagRequest) (*contracts.AddCollectionTagResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if s.tagService == nil {
		return nil, fmt.Errorf("tag service not initialized")
	}
	if req == nil {
		return nil, apperrors.ErrCollectionInvalidRequest("request body is required")
	}
	if req.TagID == 0 && strings.TrimSpace(req.TagName) == "" {
		return nil, apperrors.ErrCollectionInvalidRequest("tag_id or tag_name is required")
	}

	var collection communitym.Collection
	if err := s.db.Where("slug = ?", slug).First(&collection).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	if !s.canEditCollectionTags(&collection, userID) {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	// PSY-354: the per-collection cap (MaxCollectionTags) is now enforced
	// inside catalog.TagService.AddTagToEntity so the same limit applies
	// regardless of which endpoint the caller used. This wrapper still
	// validates auth + edit-access and returns the post-mutation tag list
	// for the dedicated endpoint's response shape.

	category := req.Category
	if category == "" {
		// Collection meta-tags rarely fit "genre" or "locale"; default to
		// "other" so the autocomplete doesn't accidentally pollute the genre
		// taxonomy when the curator types a freeform term.
		category = catalogm.TagCategoryOther
	}

	if _, err := s.tagService.AddTagToEntity(req.TagID, req.TagName, catalogm.TagEntityCollection, collection.ID, userID, category); err != nil {
		return nil, err
	}

	// Re-list and return the post-mutation set.
	tags, err := s.tagService.ListEntityTags(catalogm.TagEntityCollection, collection.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list collection tags: %w", err)
	}
	if tags == nil {
		tags = []contracts.EntityTagResponse{}
	}
	return &contracts.AddCollectionTagResponse{Tags: tags}, nil
}

// RemoveTagFromCollection removes a tag from a collection (PSY-354). Same
// edit-access rule as AddTagToCollection. Idempotency is delegated to the
// tag service — removing a non-existent application returns ErrEntityTagNotFound.
func (s *CollectionService) RemoveTagFromCollection(slug string, tagID uint, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if s.tagService == nil {
		return fmt.Errorf("tag service not initialized")
	}
	if tagID == 0 {
		return apperrors.ErrCollectionInvalidRequest("tag_id is required")
	}

	var collection communitym.Collection
	if err := s.db.Where("slug = ?", slug).First(&collection).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrCollectionNotFound(slug)
		}
		return fmt.Errorf("failed to get collection: %w", err)
	}

	if !s.canEditCollectionTags(&collection, userID) {
		return apperrors.ErrCollectionForbidden(slug)
	}

	return s.tagService.RemoveTagFromEntity(tagID, catalogm.TagEntityCollection, collection.ID)
}

// listCollectionTags returns the EntityTagResponse list for a single
// collection. Returns an empty slice (never nil) so the JSON shape is
// stable. Tag service unavailability is a no-op (older test paths build
// CollectionService bare); callers always get an empty array, never an
// error. Live errors from the tag service are logged and swallowed for
// the same reason — a list/get must not fail because the tag side-channel
// hiccupped.
func (s *CollectionService) listCollectionTags(collectionID uint, viewerID uint) []contracts.EntityTagResponse {
	if s.tagService == nil {
		return []contracts.EntityTagResponse{}
	}
	tags, err := s.tagService.ListEntityTags(catalogm.TagEntityCollection, collectionID, viewerID)
	if err != nil {
		log.Printf("warning: failed to list tags for collection %d: %v", collectionID, err)
		return []contracts.EntityTagResponse{}
	}
	if tags == nil {
		return []contracts.EntityTagResponse{}
	}
	return tags
}

// batchListCollectionTagSummaries fetches lightweight tag summaries for
// many collections in one query. Used by list endpoints (cards) where the
// per-tag vote/upvote/wilson_score detail isn't needed. Mirrors the
// batchCount* / batchCheck* helpers the rest of the service uses.
//
// SQL shape:
//
//	SELECT et.entity_id AS collection_id,
//	       t.id, t.name, t.slug, t.category, t.is_official, t.usage_count
//	FROM entity_tags et
//	JOIN tags t ON t.id = et.tag_id
//	WHERE et.entity_type = 'collection' AND et.entity_id IN (...)
//	ORDER BY t.is_official DESC, t.usage_count DESC, t.name ASC
//
// Ordering keeps the most-curated chips first on the card.
func (s *CollectionService) batchListCollectionTagSummaries(collectionIDs []uint) map[uint][]contracts.TagSummary {
	result := make(map[uint][]contracts.TagSummary)
	if len(collectionIDs) == 0 {
		return result
	}

	type Row struct {
		CollectionID uint
		ID           uint
		Name         string
		Slug         string
		Category     string
		IsOfficial   bool
		UsageCount   int
	}
	var rows []Row
	err := s.db.Table("entity_tags").
		Select(`entity_tags.entity_id AS collection_id,
		        tags.id, tags.name, tags.slug, tags.category,
		        tags.is_official, tags.usage_count`).
		Joins("JOIN tags ON tags.id = entity_tags.tag_id").
		Where("entity_tags.entity_type = ? AND entity_tags.entity_id IN ?", catalogm.TagEntityCollection, collectionIDs).
		Order("tags.is_official DESC, tags.usage_count DESC, tags.name ASC").
		Scan(&rows).Error
	if err != nil {
		log.Printf("warning: failed to batch list collection tags: %v", err)
		return result
	}

	for _, r := range rows {
		result[r.CollectionID] = append(result[r.CollectionID], contracts.TagSummary{
			ID:         r.ID,
			Name:       r.Name,
			Slug:       r.Slug,
			Category:   r.Category,
			IsOfficial: r.IsOfficial,
			UsageCount: r.UsageCount,
		})
	}
	return result
}

// ──────────────────────────────────────────────
// PSY-366: Collection graph
// ──────────────────────────────────────────────

// allowedCollectionEdgeTypes whitelists relationship types the collection
// graph surfaces. Broader than scene graph's allowlist because collections
// are explicitly user-curated — every relationship signal is informative in
// the context of a deliberate set. Excludes festival_cobill (query-time-derived
// only for per-artist graphs, not stored in artist_relationships).
var allowedCollectionEdgeTypes = map[string]bool{
	"shared_bills":       true,
	"shared_label":       true,
	"member_of":          true,
	"side_project":       true,
	"similar":            true,
	"radio_cooccurrence": true,
}

// resolveCollectionEdgeTypes filters the caller's requested types against the
// collection-graph allowlist and returns a deterministic slice. Empty input
// means "all allowed types"; a non-empty input that resolves to nothing must
// short-circuit to zero edges (never silently fall back to "all types").
// Mirrors resolveSceneEdgeTypes in services/catalog/scene.go.
func resolveCollectionEdgeTypes(requested []string) []string {
	if len(requested) == 0 {
		out := make([]string, 0, len(allowedCollectionEdgeTypes))
		for t := range allowedCollectionEdgeTypes {
			out = append(out, t)
		}
		sortCollectionStringsAsc(out)
		return out
	}
	seen := make(map[string]bool, len(requested))
	out := make([]string, 0, len(requested))
	for _, t := range requested {
		if !allowedCollectionEdgeTypes[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sortCollectionStringsAsc(out)
	return out
}

// sortCollectionStringsAsc is a tiny insertion sort. Local to avoid pulling
// `sort` for one call site (mirrors the same pattern in scene.go).
func sortCollectionStringsAsc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// collectionRelationshipRow mirrors sceneRelationshipRow shape; kept private
// so divergent service evolution doesn't pull on a shared type.
type collectionRelationshipRow struct {
	SourceArtistID   uint            `gorm:"column:source_artist_id"`
	TargetArtistID   uint            `gorm:"column:target_artist_id"`
	RelationshipType string          `gorm:"column:relationship_type"`
	Score            float32         `gorm:"column:score"`
	Detail           json.RawMessage `gorm:"column:detail"`
}

// queryCollectionRelationships fetches all stored relationships where BOTH
// source and target artist IDs are in the collection's artist set, optionally
// filtered to the resolved type list. Mirrors querySceneRelationships.
func (s *CollectionService) queryCollectionRelationships(artistIDs []uint, types []string) ([]collectionRelationshipRow, error) {
	if len(artistIDs) < 2 {
		return nil, nil
	}
	var rows []collectionRelationshipRow
	q := s.db.Table("artist_relationships").
		Select("source_artist_id, target_artist_id, relationship_type, score, detail").
		Where("source_artist_id IN ? AND target_artist_id IN ?", artistIDs, artistIDs)
	if len(types) > 0 {
		q = q.Where("relationship_type IN ?", types)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// batchUpcomingShowCountForArtists returns artist_id → count of upcoming
// approved shows. Mirrors SceneService.batchUpcomingShowCount.
func (s *CollectionService) batchUpcomingShowCountForArtists(artistIDs []uint) map[uint]int {
	out := make(map[uint]int, len(artistIDs))
	if len(artistIDs) == 0 {
		return out
	}
	type row struct {
		ArtistID  uint
		ShowCount int64
	}
	var rows []row
	s.db.Table("show_artists").
		Select("show_artists.artist_id, COUNT(DISTINCT shows.id) AS show_count").
		Joins("JOIN shows ON shows.id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND shows.status = ? AND shows.event_date > NOW()",
			artistIDs, catalogm.ShowStatusApproved).
		Group("show_artists.artist_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.ArtistID] = int(r.ShowCount)
	}
	return out
}

// Derived edge types for the multi-type collection graph (PSY-555).
// These are NOT stored in artist_relationships — they're derived at query
// time from existing junction tables (show_artists, artist_releases, etc.).
const (
	CollectionEdgePlayedAt    = "played_at"   // artist ↔ venue (via shows the artist played at the venue)
	CollectionEdgeDiscography = "discography" // artist ↔ release (artist made the release)
	CollectionEdgeSignedTo    = "signed_to"   // artist ↔ label (artist signed to the label)
	CollectionEdgeLineup      = "lineup"      // artist ↔ festival (artist played the festival)
	CollectionEdgeShowLineup  = "show_lineup" // show ↔ artist (the show's billed acts)
	CollectionEdgeShowVenue   = "show_venue"  // show ↔ venue (the show's location)
)

// GetCollectionGraph returns the multi-type knowledge subgraph for the
// collection's items. PSY-366 (artist-only origin), PSY-555 (Option B —
// every collection item becomes a node).
//
// Visibility gate mirrors GetBySlug: private collections return
// ErrCollectionForbidden unless viewer is the creator. Collections that exist
// but contain no items return a 200 response with empty nodes/links — the
// collection is valid, just non-graph-able.
//
// Edge derivation rules:
//   - artist ↔ artist  : stored artist_relationships rows (subject to type
//     filter via the original PSY-366 allowlist)
//   - artist ↔ venue   : artist played the venue (via show_artists ⋈ show_venues)
//   - artist ↔ release : artist made the release (via artist_releases)
//   - artist ↔ label   : artist signed to the label (via artist_labels)
//   - artist ↔ festival: artist played the festival (via festival_artists)
//   - show   ↔ artist  : show's lineup (via show_artists)
//   - show   ↔ venue   : show's location (via show_venues)
//
// Both endpoints must be in the collection — we never invent phantom nodes.
// Edges between non-artist nodes (venue↔festival etc.) are out of scope.
func (s *CollectionService) GetCollectionGraph(slug string, viewerID uint, types []string) (*contracts.CollectionGraphResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var collection communitym.Collection
	if err := s.db.Where("slug = ?", slug).First(&collection).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrCollectionNotFound(slug)
		}
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	if !collection.IsPublic && collection.CreatorID != viewerID {
		return nil, apperrors.ErrCollectionForbidden(slug)
	}

	resolvedTypes := resolveCollectionEdgeTypes(types)
	// Type filter only applies to the stored artist_relationships allowlist.
	// noStoredEdges short-circuits artist↔artist edges when the caller passed
	// an explicit but all-rejected filter (mirrors PSY-366 behaviour).
	// Derived edge types (played_at etc.) are not part of the user-facing
	// type filter today — they're an "always-on" side of the graph.
	noStoredEdges := len(types) > 0 && len(resolvedTypes) == 0

	resp := &contracts.CollectionGraphResponse{
		Collection: contracts.CollectionGraphInfo{
			Slug:         collection.Slug,
			Name:         collection.Title,
			EntityCounts: map[string]int{},
		},
		Nodes: []contracts.CollectionGraphNode{},
		Links: []contracts.CollectionGraphLink{},
	}

	// Load all collection items, regardless of entity type.
	var items []communitym.CollectionItem
	if err := s.db.
		Where("collection_id = ?", collection.ID).
		Order("position ASC, created_at ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("failed to load collection items: %w", err)
	}
	if len(items) == 0 {
		return resp, nil
	}

	// Bucket entity IDs by type so each detail-load query stays narrow.
	idsByType := bucketCollectionItemIDs(items)

	// Build nodes for each entity type. Returned in the order: artist,
	// venue, show, release, label, festival — stable per type, sorted by
	// name within type.
	nodes, nodeIDByEntity, err := s.buildCollectionGraphNodes(items, idsByType)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		// Items existed but every detail row was missing (deleted entity).
		return resp, nil
	}

	// Build edges. nodeIDByEntity maps (entityType, entityID) → node ID
	// (currently the collection_item row id) so links can reference nodes
	// uniquely even when entity DB IDs collide across types.
	links, err := s.buildCollectionGraphLinks(idsByType, nodeIDByEntity, resolvedTypes, noStoredEdges)
	if err != nil {
		return nil, err
	}

	// Mark isolates (no in-set edges, post type-filter).
	connected := make(map[uint]bool, len(nodes))
	for _, l := range links {
		connected[l.SourceID] = true
		connected[l.TargetID] = true
	}
	for i := range nodes {
		nodes[i].IsIsolate = !connected[nodes[i].ID]
		resp.Collection.EntityCounts[nodes[i].EntityType]++
	}

	resp.Nodes = nodes
	resp.Links = links
	resp.Collection.ArtistCount = resp.Collection.EntityCounts[communitym.CollectionEntityArtist]
	resp.Collection.EdgeCount = len(resp.Links)
	return resp, nil
}

// bucketCollectionItemIDs groups item entity IDs by type. Returns a map
// keyed by communitym.CollectionEntity* constants. Empty buckets are
// omitted (callers should range over the map directly).
func bucketCollectionItemIDs(items []communitym.CollectionItem) map[string][]uint {
	out := make(map[string][]uint, 6)
	for _, it := range items {
		out[it.EntityType] = append(out[it.EntityType], it.EntityID)
	}
	return out
}

// entityNodeKey is the lookup key for nodeIDByEntity. Two different entity
// types can share a numeric DB ID; the type qualifier disambiguates.
type entityNodeKey struct {
	EntityType string
	EntityID   uint
}

// buildCollectionGraphNodes loads detail rows for every entity type in the
// collection and emits one node per item. Returns nodes ordered by type
// (artist, venue, show, release, label, festival) then by name within
// type, plus a (entity_type, entity_id) → node ID map used for edge
// construction.
//
// Node ID == collection_item.id. This is naturally unique within the
// response (composite primary key (collection_id, entity_type, entity_id)
// ensures one item row per (entity_type, entity_id) in a collection) and
// avoids the cross-type DB-ID collision the artist-only design didn't have
// to worry about.
func (s *CollectionService) buildCollectionGraphNodes(
	items []communitym.CollectionItem,
	idsByType map[string][]uint,
) ([]contracts.CollectionGraphNode, map[entityNodeKey]uint, error) {
	// Index items by (entity_type, entity_id) so we can recover the
	// collection_item.id in node-emission order.
	itemByKey := make(map[entityNodeKey]communitym.CollectionItem, len(items))
	for _, it := range items {
		itemByKey[entityNodeKey{EntityType: it.EntityType, EntityID: it.EntityID}] = it
	}

	nodes := make([]contracts.CollectionGraphNode, 0, len(items))
	nodeIDByEntity := make(map[entityNodeKey]uint, len(items))

	// Iterate in a stable type order so the response ordering is deterministic.
	typeOrder := []string{
		communitym.CollectionEntityArtist,
		communitym.CollectionEntityVenue,
		communitym.CollectionEntityShow,
		communitym.CollectionEntityRelease,
		communitym.CollectionEntityLabel,
		communitym.CollectionEntityFestival,
	}

	upcomingByArtist := s.batchUpcomingShowCountForArtists(idsByType[communitym.CollectionEntityArtist])

	for _, et := range typeOrder {
		ids := idsByType[et]
		if len(ids) == 0 {
			continue
		}
		details, err := s.loadEntityDetailsForGraph(et, ids)
		if err != nil {
			return nil, nil, err
		}
		for _, d := range details {
			key := entityNodeKey{EntityType: et, EntityID: d.ID}
			item, ok := itemByKey[key]
			if !ok {
				// Detail row exists but the collection_item it came from
				// was deleted between queries. Skip.
				continue
			}
			node := contracts.CollectionGraphNode{
				ID:         item.ID,
				EntityType: et,
				Name:       d.Name,
				Slug:       d.Slug,
				City:       d.City,
				State:      d.State,
			}
			if et == communitym.CollectionEntityArtist {
				node.UpcomingShowCount = upcomingByArtist[d.ID]
			}
			nodes = append(nodes, node)
			nodeIDByEntity[key] = item.ID
		}
	}

	return nodes, nodeIDByEntity, nil
}

// graphEntityDetail is the per-type detail-row DTO used by node building.
// City/State are blank for entity types that don't have them (releases,
// labels can have city/state but we don't surface them on the node tooltip;
// shows store city/state directly on the row).
type graphEntityDetail struct {
	ID    uint
	Name  string
	Slug  string
	City  string
	State string
}

// loadEntityDetailsForGraph fetches the rows needed to render nodes for one
// entity type. Each branch below is a tight 5-column SELECT on the entity
// table — no joins. The intermediate `row` shape uses *string for slug,
// city, state because most entity tables have nullable columns; the loop
// at the bottom collapses nil → "".
func (s *CollectionService) loadEntityDetailsForGraph(entityType string, ids []uint) ([]graphEntityDetail, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	type row struct {
		ID    uint
		Name  string
		Slug  *string
		City  *string
		State *string
	}
	var raws []row

	switch entityType {
	case communitym.CollectionEntityArtist:
		if err := s.db.Table("artists").
			Select("id, name, slug, city, state").
			Where("id IN ?", ids).
			Order("name ASC").Scan(&raws).Error; err != nil {
			return nil, fmt.Errorf("failed to load artist details: %w", err)
		}
	case communitym.CollectionEntityVenue:
		if err := s.db.Table("venues").
			Select("id, name, slug, city, state").
			Where("id IN ?", ids).
			Order("name ASC").Scan(&raws).Error; err != nil {
			return nil, fmt.Errorf("failed to load venue details: %w", err)
		}
	case communitym.CollectionEntityShow:
		if err := s.db.Table("shows").
			Select("id, title AS name, slug, city, state").
			Where("id IN ?", ids).
			Order("title ASC").Scan(&raws).Error; err != nil {
			return nil, fmt.Errorf("failed to load show details: %w", err)
		}
	case communitym.CollectionEntityRelease:
		// Releases don't have city/state — `Select` gives the row blank
		// values so the shape stays uniform.
		if err := s.db.Table("releases").
			Select("id, title AS name, slug, NULL AS city, NULL AS state").
			Where("id IN ?", ids).
			Order("title ASC").Scan(&raws).Error; err != nil {
			return nil, fmt.Errorf("failed to load release details: %w", err)
		}
	case communitym.CollectionEntityLabel:
		if err := s.db.Table("labels").
			Select("id, name, slug, city, state").
			Where("id IN ?", ids).
			Order("name ASC").Scan(&raws).Error; err != nil {
			return nil, fmt.Errorf("failed to load label details: %w", err)
		}
	case communitym.CollectionEntityFestival:
		if err := s.db.Table("festivals").
			Select("id, name, slug, city, state").
			Where("id IN ?", ids).
			Order("name ASC").Scan(&raws).Error; err != nil {
			return nil, fmt.Errorf("failed to load festival details: %w", err)
		}
	default:
		// Unknown entity type — silently skip. No new types should appear
		// without updating CollectionEntity* and this switch.
		return nil, nil
	}

	out := make([]graphEntityDetail, 0, len(raws))
	for _, r := range raws {
		d := graphEntityDetail{ID: r.ID, Name: r.Name}
		if r.Slug != nil {
			d.Slug = *r.Slug
		}
		if r.City != nil {
			d.City = *r.City
		}
		if r.State != nil {
			d.State = *r.State
		}
		out = append(out, d)
	}
	return out, nil
}

// buildCollectionGraphLinks emits all in-set edges. Returns links with
// source_id/target_id pointing at NODE IDs (collection_item.id), not
// underlying entity IDs.
func (s *CollectionService) buildCollectionGraphLinks(
	idsByType map[string][]uint,
	nodeIDByEntity map[entityNodeKey]uint,
	resolvedArtistTypes []string,
	noStoredEdges bool,
) ([]contracts.CollectionGraphLink, error) {
	links := make([]contracts.CollectionGraphLink, 0)

	// 1. Stored artist↔artist edges (PSY-366 origin path).
	artistIDs := idsByType[communitym.CollectionEntityArtist]
	if !noStoredEdges && len(artistIDs) >= 2 {
		rels, err := s.queryCollectionRelationships(artistIDs, resolvedArtistTypes)
		if err != nil {
			return nil, fmt.Errorf("failed to query collection relationships: %w", err)
		}
		for _, r := range rels {
			srcNodeID, tgtNodeID, ok := lookupNodeIDs(nodeIDByEntity,
				communitym.CollectionEntityArtist, r.SourceArtistID,
				communitym.CollectionEntityArtist, r.TargetArtistID)
			if !ok {
				continue
			}
			var detail any
			if len(r.Detail) > 0 {
				_ = json.Unmarshal(r.Detail, &detail)
			}
			links = append(links, contracts.CollectionGraphLink{
				SourceID: srcNodeID,
				TargetID: tgtNodeID,
				Type:     r.RelationshipType,
				Score:    float64(r.Score),
				Detail:   detail,
			})
		}
	}

	// 2. Derived multi-type edges. Each spec declares the source/target
	// entity types it joins, the edge type emitted, and the pair-query
	// func. The unified loop below skips empty sides, runs the query, and
	// appends one link per resolved pair.
	derivedSpecs := []derivedEdgeSpec{
		{
			srcType: communitym.CollectionEntityArtist,
			tgtType: communitym.CollectionEntityVenue,
			edge:    CollectionEdgePlayedAt,
			query:   s.queryArtistVenuePairs,
		},
		{
			srcType: communitym.CollectionEntityArtist,
			tgtType: communitym.CollectionEntityRelease,
			edge:    CollectionEdgeDiscography,
			query:   s.queryArtistReleasePairs,
		},
		{
			srcType: communitym.CollectionEntityArtist,
			tgtType: communitym.CollectionEntityLabel,
			edge:    CollectionEdgeSignedTo,
			query:   s.queryArtistLabelPairs,
		},
		{
			srcType: communitym.CollectionEntityArtist,
			tgtType: communitym.CollectionEntityFestival,
			edge:    CollectionEdgeLineup,
			query:   s.queryArtistFestivalPairs,
		},
		{
			srcType: communitym.CollectionEntityShow,
			tgtType: communitym.CollectionEntityArtist,
			edge:    CollectionEdgeShowLineup,
			query:   s.queryShowArtistPairs,
		},
		{
			srcType: communitym.CollectionEntityShow,
			tgtType: communitym.CollectionEntityVenue,
			edge:    CollectionEdgeShowVenue,
			query:   s.queryShowVenuePairs,
		},
	}
	for _, spec := range derivedSpecs {
		srcIDs := idsByType[spec.srcType]
		tgtIDs := idsByType[spec.tgtType]
		if len(srcIDs) == 0 || len(tgtIDs) == 0 {
			continue
		}
		pairs, err := spec.query(srcIDs, tgtIDs)
		if err != nil {
			return nil, err
		}
		for _, p := range pairs {
			srcNodeID, tgtNodeID, ok := lookupNodeIDs(nodeIDByEntity,
				spec.srcType, p.SrcID, spec.tgtType, p.TgtID)
			if !ok {
				continue
			}
			links = append(links, contracts.CollectionGraphLink{
				SourceID: srcNodeID,
				TargetID: tgtNodeID,
				Type:     spec.edge,
			})
		}
	}

	return links, nil
}

// derivedEdgePair is the uniform return shape for every multi-type pair
// query. SrcID and TgtID align with derivedEdgeSpec.srcType / tgtType.
type derivedEdgePair struct {
	SrcID uint
	TgtID uint
}

// derivedEdgeSpec is one row in the derived-edge dispatch table. The
// query func returns pairs already filtered to (srcIDs ∩ tgtIDs); the
// loop appends one edge per pair after resolving node IDs.
type derivedEdgeSpec struct {
	srcType string
	tgtType string
	edge    string
	query   func(srcIDs, tgtIDs []uint) ([]derivedEdgePair, error)
}

// lookupNodeIDs is a tiny helper that resolves a (type, id) pair to its
// node IDs in one call. Returns ok=false when either side is missing —
// only happens when a row was deleted between queries.
func lookupNodeIDs(
	nodeIDByEntity map[entityNodeKey]uint,
	srcType string, srcID uint,
	tgtType string, tgtID uint,
) (uint, uint, bool) {
	srcNodeID, srcOK := nodeIDByEntity[entityNodeKey{EntityType: srcType, EntityID: srcID}]
	tgtNodeID, tgtOK := nodeIDByEntity[entityNodeKey{EntityType: tgtType, EntityID: tgtID}]
	if !srcOK || !tgtOK {
		return 0, 0, false
	}
	return srcNodeID, tgtNodeID, true
}

// ──────────────────────────────────────────────
// Multi-type relationship lookups (PSY-555)
// ──────────────────────────────────────────────
//
// Each query returns distinct (src, tgt) ID pairs that exist in the
// underlying junction table AND have both sides in the caller-supplied
// ID set. Returned shape is uniform (derivedEdgePair) so the dispatch
// table in buildCollectionGraphLinks can iterate generically.
//
// Most queries hit a single junction table directly; queryArtistVenuePairs
// is the only one that needs a join (artists are linked to venues via the
// shows they played).

func (s *CollectionService) queryArtistVenuePairs(artistIDs, venueIDs []uint) ([]derivedEdgePair, error) {
	var rows []derivedEdgePair
	if err := s.db.Table("show_artists").
		Select("DISTINCT show_artists.artist_id AS src_id, show_venues.venue_id AS tgt_id").
		Joins("JOIN show_venues ON show_venues.show_id = show_artists.show_id").
		Where("show_artists.artist_id IN ? AND show_venues.venue_id IN ?", artistIDs, venueIDs).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query artist-venue pairs: %w", err)
	}
	return rows, nil
}

func (s *CollectionService) queryArtistReleasePairs(artistIDs, releaseIDs []uint) ([]derivedEdgePair, error) {
	return s.queryEdgePairs("artist_releases", "artist_id", "release_id",
		artistIDs, releaseIDs, "artist-release")
}

func (s *CollectionService) queryArtistLabelPairs(artistIDs, labelIDs []uint) ([]derivedEdgePair, error) {
	return s.queryEdgePairs("artist_labels", "artist_id", "label_id",
		artistIDs, labelIDs, "artist-label")
}

func (s *CollectionService) queryArtistFestivalPairs(artistIDs, festivalIDs []uint) ([]derivedEdgePair, error) {
	return s.queryEdgePairs("festival_artists", "artist_id", "festival_id",
		artistIDs, festivalIDs, "artist-festival")
}

func (s *CollectionService) queryShowArtistPairs(showIDs, artistIDs []uint) ([]derivedEdgePair, error) {
	return s.queryEdgePairs("show_artists", "show_id", "artist_id",
		showIDs, artistIDs, "show-artist")
}

func (s *CollectionService) queryShowVenuePairs(showIDs, venueIDs []uint) ([]derivedEdgePair, error) {
	return s.queryEdgePairs("show_venues", "show_id", "venue_id",
		showIDs, venueIDs, "show-venue")
}

// queryEdgePairs is the single-junction-table variant: SELECT DISTINCT
// (srcCol, tgtCol) FROM table WHERE srcCol IN srcIDs AND tgtCol IN tgtIDs.
// label is used to namespace error messages.
func (s *CollectionService) queryEdgePairs(table, srcCol, tgtCol string, srcIDs, tgtIDs []uint, label string) ([]derivedEdgePair, error) {
	var rows []derivedEdgePair
	selectExpr := fmt.Sprintf("DISTINCT %s AS src_id, %s AS tgt_id", srcCol, tgtCol)
	whereExpr := fmt.Sprintf("%s IN ? AND %s IN ?", srcCol, tgtCol)
	if err := s.db.Table(table).
		Select(selectExpr).
		Where(whereExpr, srcIDs, tgtIDs).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query %s pairs: %w", label, err)
	}
	return rows, nil
}
