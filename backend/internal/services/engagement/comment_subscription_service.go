package engagement

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// CommentSubscriptionService implements CommentSubscriptionServiceInterface.
type CommentSubscriptionService struct {
	db *gorm.DB
}

// NewCommentSubscriptionService creates a new CommentSubscriptionService.
func NewCommentSubscriptionService(db *gorm.DB) *CommentSubscriptionService {
	return &CommentSubscriptionService{db: db}
}

// Subscribe adds a subscription for a user to an entity's comments.
// Idempotent — if already subscribed, no error (ON CONFLICT DO NOTHING).
func (s *CommentSubscriptionService) Subscribe(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return err
	}

	sub := engagementm.CommentSubscription{
		UserID:       userID,
		EntityType:   entityType,
		EntityID:     entityID,
		SubscribedAt: time.Now().UTC(),
	}

	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&sub).Error
}

// Unsubscribe removes a subscription. Idempotent — if not subscribed, no error.
func (s *CommentSubscriptionService) Unsubscribe(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return err
	}

	return s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ?",
		userID, entityType, entityID,
	).Delete(&engagementm.CommentSubscription{}).Error
}

// IsSubscribed checks whether a user is subscribed to an entity's comments.
func (s *CommentSubscriptionService) IsSubscribed(userID uint, entityType string, entityID uint) (bool, error) {
	if s.db == nil {
		return false, errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return false, err
	}

	var count int64
	err := s.db.Model(&engagementm.CommentSubscription{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", userID, entityType, entityID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check subscription: %w", err)
	}
	return count > 0, nil
}

// MarkRead updates the last-read pointer for a user on an entity to the latest comment ID.
func (s *CommentSubscriptionService) MarkRead(userID uint, entityType string, entityID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return err
	}

	// Find the max comment ID for this entity
	var maxID uint
	err := s.db.Model(&engagementm.Comment{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Select("COALESCE(MAX(id), 0)").
		Scan(&maxID).Error
	if err != nil {
		return fmt.Errorf("failed to get max comment ID: %w", err)
	}

	// Upsert the last-read record
	lastRead := engagementm.CommentLastRead{
		UserID:            userID,
		EntityType:        entityType,
		EntityID:          entityID,
		LastReadCommentID: maxID,
		UpdatedAt:         time.Now().UTC(),
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "entity_type"},
			{Name: "entity_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"last_read_comment_id", "updated_at"}),
	}).Create(&lastRead).Error
}

// GetUnreadCount returns the number of visible comments newer than the user's last-read pointer.
func (s *CommentSubscriptionService) GetUnreadCount(userID uint, entityType string, entityID uint) (int, error) {
	if s.db == nil {
		return 0, errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return 0, err
	}

	// Get the last-read comment ID (0 if never read)
	var lastReadID uint
	err := s.db.Model(&engagementm.CommentLastRead{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", userID, entityType, entityID).
		Select("last_read_comment_id").
		Scan(&lastReadID).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get last read: %w", err)
	}

	// Count visible comments with ID > lastReadID
	var count int64
	err = s.db.Model(&engagementm.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND visibility = ? AND id > ?",
			entityType, entityID, engagementm.CommentVisibilityVisible, lastReadID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count unread comments: %w", err)
	}

	return int(count), nil
}

// watchingRow is the scan target for the ListWatching aggregate query.
type watchingRow struct {
	EntityType    string
	EntityID      uint
	SubscribedAt  time.Time
	CommentCount  int
	LastCommentAt *time.Time
	LastCommentID *uint
	UnreadCount   int
}

// ListWatching returns the user's subscriptions enriched with entity
// context and last comment activity, ordered by last activity (newest
// first; threads without comments last, by subscription recency).
//
// Aggregates (count / last activity / unread-vs-last-read) come from one
// LATERAL query; entity names and last-commenter names are then resolved
// in one batch query per distinct entity table plus two for users — no
// per-row queries.
func (s *CommentSubscriptionService) ListWatching(userID uint, limit, offset int) ([]contracts.WatchingItem, int64, error) {
	if s.db == nil {
		return nil, 0, errors.New("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	err := s.db.Model(&engagementm.CommentSubscription{}).
		Where("user_id = ?", userID).
		Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}
	if total == 0 {
		return []contracts.WatchingItem{}, 0, nil
	}

	// Display aggregates (comment_count, last comment) cover only
	// kind='comment' — field notes are a separate surface. unread_count
	// deliberately spans ALL visible kinds so it agrees with
	// GetUnreadCount, which backs the per-entity subscribe/status badge.
	// last_comment_id is the id of the latest comment BY created_at
	// (ties broken by id), so LastCommentAt and LastCommenterName always
	// describe the same comment. The trailing (entity_type, entity_id)
	// sort keys make pagination deterministic under timestamp ties.
	var rows []watchingRow
	err = s.db.Raw(`
		SELECT cs.entity_type,
		       cs.entity_id,
		       cs.subscribed_at,
		       COALESCE(agg.comment_count, 0)  AS comment_count,
		       agg.last_comment_at,
		       agg.last_comment_id,
		       COALESCE(agg.unread_count, 0)   AS unread_count
		FROM comment_subscriptions cs
		LEFT JOIN comment_last_read clr
		       ON clr.user_id = cs.user_id
		      AND clr.entity_type = cs.entity_type
		      AND clr.entity_id = cs.entity_id
		LEFT JOIN LATERAL (
			SELECT COUNT(*)          FILTER (WHERE c.kind = ?) AS comment_count,
			       MAX(c.created_at) FILTER (WHERE c.kind = ?) AS last_comment_at,
			       (ARRAY_AGG(c.id ORDER BY c.created_at DESC, c.id DESC)
			            FILTER (WHERE c.kind = ?))[1]           AS last_comment_id,
			       COUNT(*) FILTER (WHERE c.id > COALESCE(clr.last_read_comment_id, 0)) AS unread_count
			FROM comments c
			WHERE c.entity_type = cs.entity_type
			  AND c.entity_id = cs.entity_id
			  AND c.visibility = ?
		) agg ON true
		WHERE cs.user_id = ?
		ORDER BY agg.last_comment_at DESC NULLS LAST, cs.subscribed_at DESC,
		         cs.entity_type ASC, cs.entity_id ASC
		LIMIT ? OFFSET ?`,
		engagementm.CommentKindComment, engagementm.CommentKindComment, engagementm.CommentKindComment,
		engagementm.CommentVisibilityVisible,
		userID, limit, offset,
	).Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch watching list: %w", err)
	}

	entities := s.loadWatchingEntities(rows)
	commenterNames := s.loadLastCommenterNames(rows)

	items := make([]contracts.WatchingItem, len(rows))
	for i, row := range rows {
		item := contracts.WatchingItem{
			EntityType:    row.EntityType,
			EntityID:      row.EntityID,
			SubscribedAt:  row.SubscribedAt,
			CommentCount:  row.CommentCount,
			LastCommentAt: row.LastCommentAt,
			UnreadCount:   row.UnreadCount,
		}
		item.EntityName, item.EntitySlug, item.EntityURL = resolveWatchingEntity(row.EntityType, row.EntityID, entities)
		if row.LastCommentID != nil {
			item.LastCommenterName = commenterNames[*row.LastCommentID]
		}
		items[i] = item
	}

	return items, total, nil
}

// loadWatchingEntities batch-loads (id, name, slug) for the page's
// entities, one SELECT per distinct entity table.
func (s *CommentSubscriptionService) loadWatchingEntities(rows []watchingRow) map[string]map[uint]shared.EntityNameRow {
	idsByType := make(map[string][]uint)
	seen := make(map[string]map[uint]struct{})
	for _, r := range rows {
		set, exists := seen[r.EntityType]
		if !exists {
			set = make(map[uint]struct{})
			seen[r.EntityType] = set
		}
		if _, dup := set[r.EntityID]; dup {
			continue
		}
		set[r.EntityID] = struct{}{}
		idsByType[r.EntityType] = append(idsByType[r.EntityType], r.EntityID)
	}
	return shared.LoadCommentEntityNames(s.db, idsByType)
}

// loadLastCommenterNames batch-resolves the display name of each
// last-comment author, keyed by comment ID.
func (s *CommentSubscriptionService) loadLastCommenterNames(rows []watchingRow) map[uint]string {
	commentIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		if r.LastCommentID != nil {
			commentIDs = append(commentIDs, *r.LastCommentID)
		}
	}
	if len(commentIDs) == 0 {
		return map[uint]string{}
	}

	type commentAuthorRow struct {
		ID     uint
		UserID uint
	}
	var authors []commentAuthorRow
	err := s.db.Table("comments").
		Select("id, user_id").
		Where("id IN ?", commentIDs).
		Scan(&authors).Error
	if err != nil {
		log.Printf("warning: failed to load last-comment authors for watching list: %v", err)
		return map[uint]string{}
	}

	userIDs := make([]uint, 0, len(authors))
	for _, a := range authors {
		userIDs = append(userIDs, a.UserID)
	}
	namesByUser, err := shared.BatchResolveUserNames(s.db, userIDs)
	if err != nil {
		log.Printf("warning: failed to resolve commenter names for watching list: %v", err)
		return map[uint]string{}
	}

	names := make(map[uint]string, len(authors))
	for _, a := range authors {
		if name, ok := namesByUser[a.UserID]; ok {
			names[a.ID] = name
		}
	}
	return names
}

// resolveWatchingEntity turns a (type, id) pair into the display name,
// slug, and root-relative URL, falling back to "<type> #<id>" + an
// ID-based URL when the entity row is missing (deleted since subscribe).
func resolveWatchingEntity(entityType string, entityID uint, entities map[string]map[uint]shared.EntityNameRow) (name, slug, url string) {
	pathSegment, _, _, ok := engagementm.CommentEntityPathAndTable(entityType)
	if !ok {
		return fmt.Sprintf("%s #%d", entityType, entityID), "", ""
	}
	row, hasRow := entities[entityType][entityID]
	if hasRow && row.Slug != "" {
		url = fmt.Sprintf("/%s/%s", pathSegment, row.Slug)
	} else {
		url = fmt.Sprintf("/%s/%d", pathSegment, entityID)
	}
	if hasRow && row.Name != "" {
		name = row.Name
	} else {
		name = fmt.Sprintf("%s #%d", entityType, entityID)
	}
	return name, row.Slug, url
}

// GetSubscribersForEntity returns user IDs of all subscribers for an entity.
func (s *CommentSubscriptionService) GetSubscribersForEntity(entityType string, entityID uint) ([]uint, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	if _, err := validateCommentEntityType(entityType); err != nil {
		return nil, err
	}

	var userIDs []uint
	err := s.db.Model(&engagementm.CommentSubscription{}).
		Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get subscribers: %w", err)
	}

	return userIDs, nil
}
