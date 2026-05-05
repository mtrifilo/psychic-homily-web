package engagement

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"log"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// FollowChecker is the minimal FollowService surface that CommentService
// needs to enforce follower-only reply permissions. Kept as a local
// interface so tests can swap in a fake.
type FollowChecker interface {
	IsFollowing(userID uint, entityType string, entityID uint) (bool, error)
}

// commentNotifier is the subset of CommentNotificationService that CommentService
// needs to fan out fire-and-forget notifications. Declared here (not imported)
// to avoid an import cycle — the notification service lives in the same
// package but conceptually wraps CommentService.
type commentNotifier interface {
	NotifySubscribers(commentID uint) error
	NotifyMentioned(commentID uint) error
}

// CommentService implements CommentServiceInterface for comment CRUD and threading.
type CommentService struct {
	db            *gorm.DB
	md            goldmark.Markdown
	sanitize      *bluemonday.Policy
	followChecker FollowChecker
	// notifier is optional — tests often leave it nil. Production wires it
	// via SetNotifier() after both services are constructed.
	notifier commentNotifier
}

// SetNotifier wires the comment notification service (PSY-289). Optional —
// when nil, CreateComment skips the fire-and-forget notification fan-out.
// This exists as a setter (rather than a constructor arg) to avoid a
// circular dependency: CommentNotificationService and CommentService both
// live in this package but the notifier needs comment data loaded by the
// service itself.
func (s *CommentService) SetNotifier(n commentNotifier) {
	s.notifier = n
}

// NewCommentService creates a new CommentService.
func NewCommentService(db *gorm.DB) *CommentService {
	// Create sanitization policy: allow safe formatting, no images/raw HTML/tables
	policy := bluemonday.NewPolicy()
	policy.AllowStandardURLs()
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowElements("p", "br",
		"strong", "b", "em", "i",
		"code", "pre",
		"ul", "ol", "li",
		"blockquote",
		"h3", "h4", "h5", "h6",
	)
	policy.RequireNoFollowOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)

	svc := &CommentService{
		db:       db,
		md:       goldmark.New(),
		sanitize: policy,
	}
	// Default FollowChecker is a FollowService bound to the same DB. Tests
	// that construct CommentService without a DB get a nil checker; the
	// check is re-guarded at call sites.
	if db != nil {
		svc.followChecker = NewFollowService(db)
	}
	return svc
}

// SetFollowChecker overrides the follow checker. Used in tests.
func (s *CommentService) SetFollowChecker(fc FollowChecker) {
	s.followChecker = fc
}

// renderMarkdown converts markdown body to sanitized HTML.
func (s *CommentService) renderMarkdown(body string) string {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(body), &buf); err != nil {
		// Fallback: escape and wrap in <p> tag
		return "<p>" + s.sanitize.Sanitize(body) + "</p>"
	}
	return s.sanitize.Sanitize(buf.String())
}

// wilsonScore computes the Wilson score lower bound for ranking.
// Uses 90% confidence interval (z = 1.281728756502709).
func wilsonScore(ups, downs int) float64 {
	n := float64(ups + downs)
	if n == 0 {
		return 0
	}
	z := 1.281728756502709
	phat := float64(ups) / n
	return (phat + z*z/(2*n) - z*math.Sqrt((phat*(1-phat)+z*z/(4*n))/n)) / (1 + z*z/n)
}

// validateCommentEntityType checks if the entity type is supported.
func validateCommentEntityType(entityType string) (engagementm.CommentEntityType, error) {
	ct := engagementm.CommentEntityType(entityType)
	if _, ok := engagementm.ValidCommentEntityTypes[ct]; !ok {
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}
	return ct, nil
}

// validateEntityExists checks that the referenced entity actually exists in the database.
func (s *CommentService) validateEntityExists(entityType engagementm.CommentEntityType, entityID uint) error {
	tableName, ok := engagementm.ValidCommentEntityTypes[entityType]
	if !ok {
		return fmt.Errorf("unsupported entity type: %s", entityType)
	}

	var count int64
	result := s.db.Table(tableName).Where("id = ?", entityID).Count(&count)
	if result.Error != nil {
		return fmt.Errorf("failed to validate entity existence: %w", result.Error)
	}
	if count == 0 {
		return fmt.Errorf("%s with ID %d not found", entityType, entityID)
	}
	return nil
}

// commentToResponse maps a Comment model to a CommentResponse.
func commentToResponse(c *engagementm.Comment) *contracts.CommentResponse {
	resp := &contracts.CommentResponse{
		ID:              c.ID,
		EntityType:      string(c.EntityType),
		EntityID:        c.EntityID,
		Kind:            string(c.Kind),
		UserID:          c.UserID,
		AuthorName:      resolveCommentAuthorName(&c.User),
		AuthorUsername:  resolveCommentAuthorUsername(&c.User),
		ParentID:        c.ParentID,
		RootID:          c.RootID,
		Depth:           c.Depth,
		Body:            c.Body,
		BodyHTML:        c.BodyHTML,
		StructuredData:  c.StructuredData,
		Visibility:      string(c.Visibility),
		ReplyPermission: string(c.ReplyPermission),
		Ups:             c.Ups,
		Downs:           c.Downs,
		Score:           c.Score,
		IsEdited:        c.EditCount > 0,
		EditCount:       c.EditCount,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
	return resp
}

// resolveCommentAuthorName returns the display name for a comment's author —
// never empty. Mirrors CollectionService.resolveUserName (PSY-353): prefer
// username, fall back to first/last, then to the local-part of the email,
// finally "Anonymous". Operates on the preloaded User so callers don't pay
// an extra query per comment. PSY-552.
func resolveCommentAuthorName(u *authm.User) string {
	if u == nil || u.ID == 0 {
		return "Anonymous"
	}
	if u.Username != nil && *u.Username != "" {
		return *u.Username
	}
	if u.FirstName != nil && *u.FirstName != "" {
		name := *u.FirstName
		if u.LastName != nil && *u.LastName != "" {
			name += " " + *u.LastName
		}
		return name
	}
	if u.Email != nil && *u.Email != "" {
		if idx := strings.Index(*u.Email, "@"); idx > 0 {
			return (*u.Email)[:idx]
		}
	}
	return "Anonymous"
}

// resolveCommentAuthorUsername returns the author's username for /users/:username
// links, or nil when the user has no username set. Distinct from
// resolveCommentAuthorName, which falls back to first/last/email and so cannot
// be safely used in a URL slug. Mirrors CollectionService.resolveUserUsername
// (PSY-353). PSY-552.
func resolveCommentAuthorUsername(u *authm.User) *string {
	if u == nil || u.ID == 0 {
		return nil
	}
	if u.Username == nil || *u.Username == "" {
		return nil
	}
	username := *u.Username
	return &username
}

// userTierHourlyLimit returns the hourly comment limit for a given user tier.
// Returns -1 for unlimited.
func userTierHourlyLimit(tier string) int {
	switch tier {
	case "new_user", "":
		return 5
	case "contributor":
		return 30
	case "trusted_contributor":
		return 100
	case "local_ambassador", "admin":
		return -1 // unlimited
	default:
		return 5
	}
}

// computeInitialVisibility determines the initial visibility based on user trust tier.
func computeInitialVisibility(user *authm.User) engagementm.CommentVisibility {
	if user.IsAdmin {
		return engagementm.CommentVisibilityVisible
	}
	switch user.UserTier {
	case "contributor", "trusted_contributor", "local_ambassador":
		return engagementm.CommentVisibilityVisible
	default: // "new_user" or empty
		return engagementm.CommentVisibilityPendingReview
	}
}

// CreateComment creates a new comment or reply.
func (s *CommentService) CreateComment(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// PSY-296: Reply-permission gate. When creating a reply (parent_id set),
	// enforce the parent comment's reply_permission before doing any other
	// work. This is deliberately at the top of the function to minimize
	// merge conflict with sibling PRs touching the tail of CreateComment.
	if req.ParentID != nil && *req.ParentID > 0 {
		var parentPerm engagementm.Comment
		if err := s.db.Select("id, user_id, reply_permission").
			First(&parentPerm, *req.ParentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("parent comment not found")
			}
			return nil, fmt.Errorf("failed to load parent comment: %w", err)
		}
		switch parentPerm.ReplyPermission {
		case engagementm.ReplyPermissionAuthorOnly:
			if parentPerm.UserID != userID {
				return nil, errors.New("replies to this comment are disabled")
			}
		case engagementm.ReplyPermissionFollowers:
			// Author can always reply to their own comment; otherwise the
			// replier must follow the parent author.
			if parentPerm.UserID != userID {
				if s.followChecker == nil {
					return nil, errors.New("follow state unavailable; cannot verify followers-only reply permission")
				}
				isFollowing, err := s.followChecker.IsFollowing(userID, FollowEntityUser, parentPerm.UserID)
				if err != nil {
					return nil, fmt.Errorf("failed to check follower status: %w", err)
				}
				if !isFollowing {
					return nil, errors.New("only followers of the author can reply to this comment")
				}
			}
		case engagementm.ReplyPermissionAnyone, "":
			// allow
		default:
			// Unrecognized value stored on the parent — fail closed to be safe.
			return nil, fmt.Errorf("unsupported reply_permission on parent: %s", parentPerm.ReplyPermission)
		}
	}

	// Validate body length
	body := strings.TrimSpace(req.Body)
	if len(body) < engagementm.MinCommentBodyLength {
		return nil, errors.New("comment body is required")
	}
	if len(body) > engagementm.MaxCommentBodyLength {
		return nil, fmt.Errorf("comment body exceeds maximum length of %d characters", engagementm.MaxCommentBodyLength)
	}

	// Validate entity type
	entityType, err := validateCommentEntityType(req.EntityType)
	if err != nil {
		return nil, err
	}

	// Validate entity exists
	if err := s.validateEntityExists(entityType, req.EntityID); err != nil {
		return nil, err
	}

	// Look up user for trust tier and rate limiting
	var user authm.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	// Rate limiting: per-entity cooldown (60s between comments on same entity)
	var recentEntityCount int64
	if err := s.db.Model(&engagementm.Comment{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND created_at > ?",
			userID, entityType, req.EntityID, time.Now().Add(-60*time.Second)).
		Count(&recentEntityCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check rate limit: %w", err)
	}
	if recentEntityCount > 0 {
		return nil, errors.New("please wait 60 seconds between comments on the same entity")
	}

	// Rate limiting: global hourly limit based on trust tier
	hourlyLimit := userTierHourlyLimit(user.UserTier)
	if hourlyLimit >= 0 { // -1 means unlimited
		var hourlyCount int64
		if err := s.db.Model(&engagementm.Comment{}).
			Where("user_id = ? AND created_at > ?", userID, time.Now().Add(-1*time.Hour)).
			Count(&hourlyCount).Error; err != nil {
			return nil, fmt.Errorf("failed to check hourly rate limit: %w", err)
		}
		if int(hourlyCount) >= hourlyLimit {
			return nil, fmt.Errorf("you've reached your hourly comment limit (%d/hour for %s users)",
				hourlyLimit, func() string {
					if user.UserTier == "" {
						return "new"
					}
					return strings.ReplaceAll(user.UserTier, "_", " ")
				}())
		}
	}

	// Determine kind
	kind := engagementm.CommentKindComment
	if req.Kind == string(engagementm.CommentKindFieldNote) {
		kind = engagementm.CommentKindFieldNote
	}

	// Determine reply permission.
	// Precedence:
	//  1. Explicit value on the request (validated).
	//  2. Per-user default preference (top-level comments only).
	//  3. Fallback: 'anyone'.
	replyPerm := engagementm.ReplyPermissionAnyone
	if req.ReplyPermission != "" {
		if !engagementm.IsValidReplyPermission(req.ReplyPermission) {
			return nil, fmt.Errorf("invalid reply_permission: %s", req.ReplyPermission)
		}
		replyPerm = engagementm.ReplyPermission(req.ReplyPermission)
	} else if req.ParentID == nil || *req.ParentID == 0 {
		// Top-level comment: apply the user's default preference if set.
		var prefs authm.UserPreferences
		if err := s.db.Where("user_id = ?", userID).First(&prefs).Error; err == nil {
			if prefs.DefaultReplyPermission != "" && engagementm.IsValidReplyPermission(prefs.DefaultReplyPermission) {
				replyPerm = engagementm.ReplyPermission(prefs.DefaultReplyPermission)
			}
		}
		// gorm.ErrRecordNotFound is fine — user has no prefs row yet.
	}

	// Threading: handle parent_id
	var parentID *uint
	var rootID *uint
	depth := 0

	if req.ParentID != nil && *req.ParentID > 0 {
		// Validate parent exists and belongs to the same entity
		var parent engagementm.Comment
		if err := s.db.First(&parent, *req.ParentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("parent comment not found")
			}
			return nil, fmt.Errorf("failed to fetch parent comment: %w", err)
		}

		// Parent must be on the same entity
		if parent.EntityType != entityType || parent.EntityID != req.EntityID {
			return nil, errors.New("parent comment belongs to a different entity")
		}

		// Enforce max depth
		depth = parent.Depth + 1
		if depth > engagementm.MaxCommentDepth {
			return nil, fmt.Errorf("maximum reply depth of %d exceeded", engagementm.MaxCommentDepth)
		}

		parentID = req.ParentID

		// Set root_id: if parent is top-level, root is parent; otherwise root is parent's root
		if parent.RootID != nil {
			rootID = parent.RootID
		} else {
			rootID = &parent.ID
		}
	}

	// Render markdown to HTML
	bodyHTML := s.renderMarkdown(body)

	// Determine initial visibility based on trust tier
	visibility := computeInitialVisibility(&user)

	comment := &engagementm.Comment{
		EntityType:      entityType,
		EntityID:        req.EntityID,
		Kind:            kind,
		UserID:          userID,
		ParentID:        parentID,
		RootID:          rootID,
		Depth:           depth,
		Body:            body,
		BodyHTML:        bodyHTML,
		Visibility:      visibility,
		ReplyPermission: replyPerm,
	}

	if err := s.db.Create(comment).Error; err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	// Auto-subscribe the commenter to this entity's comments (fire-and-forget)
	sub := engagementm.CommentSubscription{
		UserID:       userID,
		EntityType:   string(entityType),
		EntityID:     req.EntityID,
		SubscribedAt: time.Now().UTC(),
	}
	// ON CONFLICT DO NOTHING — idempotent, ignore if already subscribed
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&sub).Error; err != nil {
		// Log but don't fail the comment creation
		log.Printf("warning: failed to auto-subscribe user %d to %s/%d comments: %v", userID, entityType, req.EntityID, err)
	}

	// Reload with user info
	if err := s.db.Preload("User").First(comment, comment.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload comment: %w", err)
	}

	// PSY-289: fire-and-forget notification fan-out. Only for visible
	// comments — pending_review comments should not trigger emails until
	// approved. Added at the END of the function so parallel agents working
	// on the top of this method (e.g. reply-permission gate) don't conflict.
	if s.notifier != nil && comment.Visibility == engagementm.CommentVisibilityVisible {
		commentID := comment.ID
		go func() {
			if nErr := s.notifier.NotifySubscribers(commentID); nErr != nil {
				log.Printf("warning: comment notification (subscribers) failed for comment %d: %v", commentID, nErr)
			}
		}()
		go func() {
			if nErr := s.notifier.NotifyMentioned(commentID); nErr != nil {
				log.Printf("warning: comment notification (mentions) failed for comment %d: %v", commentID, nErr)
			}
		}()
	}

	return commentToResponse(comment), nil
}

// GetComment returns a single comment by ID.
func (s *CommentService) GetComment(commentID uint) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	var comment engagementm.Comment
	if err := s.db.Preload("User").First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("comment not found")
		}
		return nil, fmt.Errorf("failed to fetch comment: %w", err)
	}

	return commentToResponse(&comment), nil
}

// ListCommentsForEntity returns paginated top-level comments for an entity with sort options.
func (s *CommentService) ListCommentsForEntity(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Validate entity type
	et, err := validateCommentEntityType(entityType)
	if err != nil {
		return nil, err
	}

	// Default pagination
	limit := filters.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	// Build base query for top-level comments only
	query := s.db.Model(&engagementm.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND parent_id IS NULL", et, entityID)

	// Filter by visibility (default: visible only)
	if filters.Visibility != "" {
		query = query.Where("visibility = ?", filters.Visibility)
	} else {
		query = query.Where("visibility = ?", engagementm.CommentVisibilityVisible)
	}

	// Filter by kind (default: regular comments only — field notes have a
	// dedicated /shows/{id}/field-notes endpoint and must not leak into
	// the discussion list, PSY-588).
	if filters.Kind != "" {
		query = query.Where("kind = ?", filters.Kind)
	} else {
		query = query.Where("kind = ?", engagementm.CommentKindComment)
	}

	// Count total matching comments
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count comments: %w", err)
	}

	// Apply sort order
	switch filters.Sort {
	case "new":
		query = query.Order("created_at DESC")
	case "top":
		query = query.Order("(ups - downs) DESC, created_at DESC")
	case "controversial":
		query = query.Order("(ups + downs) DESC, ABS(ups - downs) ASC, created_at DESC")
	default: // "best" or empty
		query = query.Order("score DESC, created_at DESC")
	}

	// Fetch comments with user preload
	var comments []engagementm.Comment
	if err := query.Preload("User").Limit(limit).Offset(offset).Find(&comments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}

	// PSY-514: count visible replies per top-level comment so the frontend can
	// suppress the "Show replies" button on zero-reply comments. We only count
	// visible direct children — hidden/removed replies don't render anything
	// expandable. One round-trip via GROUP BY keeps this off the per-row N+1.
	replyCounts := make(map[uint]int, len(comments))
	if len(comments) > 0 {
		parentIDs := make([]uint, 0, len(comments))
		for i := range comments {
			parentIDs = append(parentIDs, comments[i].ID)
		}
		type replyCountRow struct {
			ParentID uint
			Count    int
		}
		var rows []replyCountRow
		if err := s.db.Model(&engagementm.Comment{}).
			Select("parent_id, COUNT(*) AS count").
			Where("parent_id IN ? AND visibility = ?", parentIDs, engagementm.CommentVisibilityVisible).
			Group("parent_id").
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to count replies: %w", err)
		}
		for _, r := range rows {
			replyCounts[r.ParentID] = r.Count
		}
	}

	// Map to response
	responses := make([]*contracts.CommentResponse, len(comments))
	for i := range comments {
		responses[i] = commentToResponse(&comments[i])
		responses[i].ReplyCount = replyCounts[comments[i].ID]
	}

	return &contracts.CommentListResponse{
		Comments: responses,
		Total:    total,
		HasMore:  int64(offset+limit) < total,
	}, nil
}

// GetThread loads a root comment and all its descendants as a flat list ordered by created_at.
func (s *CommentService) GetThread(rootID uint) ([]*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Verify the root comment exists
	var root engagementm.Comment
	if err := s.db.First(&root, rootID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("thread root comment not found")
		}
		return nil, fmt.Errorf("failed to fetch thread root: %w", err)
	}

	// The root comment must be a top-level comment (no parent)
	if root.ParentID != nil {
		return nil, errors.New("comment is not a thread root")
	}

	// Load root + all descendants
	var comments []engagementm.Comment
	if err := s.db.Preload("User").
		Where("id = ? OR root_id = ?", rootID, rootID).
		Order("created_at ASC").
		Find(&comments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch thread: %w", err)
	}

	responses := make([]*contracts.CommentResponse, len(comments))
	for i := range comments {
		responses[i] = commentToResponse(&comments[i])
	}

	return responses, nil
}

// UpdateComment updates a comment's body. Only the author can update their own comment.
// Writes the prior body to comment_edits (with the editor's user ID) and bumps the
// edit counter — all in a single transaction so the edit log and body update are atomic.
func (s *CommentService) UpdateComment(userID uint, commentID uint, req *contracts.UpdateCommentRequest) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Validate body
	body := strings.TrimSpace(req.Body)
	if len(body) < engagementm.MinCommentBodyLength {
		return nil, errors.New("comment body is required")
	}
	if len(body) > engagementm.MaxCommentBodyLength {
		return nil, fmt.Errorf("comment body exceeds maximum length of %d characters", engagementm.MaxCommentBodyLength)
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("comment not found")
		}
		return nil, fmt.Errorf("failed to fetch comment: %w", err)
	}

	// Only the author can edit
	if comment.UserID != userID {
		return nil, errors.New("only the comment author can edit this comment")
	}

	bodyHTML := s.renderMarkdown(body)
	now := time.Now()

	// Wrap the edit log write + body update in a single transaction so a partial
	// failure can't leave an orphaned comment_edits row (or vice versa).
	editorID := userID
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Append old body to comment_edits before we overwrite it.
		edit := &engagementm.CommentEdit{
			CommentID:    comment.ID,
			OldBody:      comment.Body,
			EditedAt:     now,
			EditorUserID: &editorID,
		}
		if err := tx.Create(edit).Error; err != nil {
			return fmt.Errorf("failed to save edit history: %w", err)
		}

		// Update the comment body and bump edit_count/updated_at.
		if err := tx.Model(&comment).Updates(map[string]interface{}{
			"body":       body,
			"body_html":  bodyHTML,
			"edit_count": gorm.Expr("edit_count + 1"),
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("failed to update comment: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	// Reload with user info (outside the transaction; read-only)
	if err := s.db.Preload("User").First(&comment, commentID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload comment: %w", err)
	}

	return commentToResponse(&comment), nil
}

// UpdateReplyPermission changes who can reply to a comment. Only the
// author of the comment may change this setting.
//
// PSY-296. `permission` must be one of {anyone, followers, author_only}.
func (s *CommentService) UpdateReplyPermission(userID uint, commentID uint, permission string) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}
	if !engagementm.IsValidReplyPermission(permission) {
		return nil, fmt.Errorf("invalid reply_permission: %s", permission)
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("comment not found")
		}
		return nil, fmt.Errorf("failed to fetch comment: %w", err)
	}
	if comment.UserID != userID {
		return nil, errors.New("only the comment author can change reply permission")
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"reply_permission": permission,
		"updated_at":       time.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update reply permission: %w", err)
	}

	if err := s.db.Preload("User").First(&comment, commentID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload comment: %w", err)
	}
	return commentToResponse(&comment), nil
}

// GetCommentEditHistory returns the chronological edit history for a comment.
// Admin-only: returns "admin access required" for non-admin requesters. Entries
// are ordered by edited_at ASC (oldest first) so a viewer can walk forward from
// the original body to the current one. The current body is included separately.
func (s *CommentService) GetCommentEditHistory(requesterID uint, commentID uint) (*contracts.CommentEditHistoryResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Gate on admin. We do this in the service so the handler doesn't need to
	// double-check, and so any future internal callers inherit the same gate.
	var requester authm.User
	if err := s.db.First(&requester, requesterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("admin access required")
		}
		return nil, fmt.Errorf("failed to look up requester: %w", err)
	}
	if !requester.IsAdmin {
		return nil, errors.New("admin access required")
	}

	// Fetch the comment (we need the current body + to verify it exists).
	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("comment not found")
		}
		return nil, fmt.Errorf("failed to fetch comment: %w", err)
	}

	// Fetch edits oldest-first, preloading the editor so we can include display info.
	var edits []engagementm.CommentEdit
	if err := s.db.Preload("Editor").
		Where("comment_id = ?", commentID).
		Order("edited_at ASC, id ASC").
		Find(&edits).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch edit history: %w", err)
	}

	entries := make([]contracts.CommentEditHistoryEntry, len(edits))
	for i := range edits {
		e := &edits[i]
		entry := contracts.CommentEditHistoryEntry{
			ID:           e.ID,
			CommentID:    e.CommentID,
			OldBody:      e.OldBody,
			EditedAt:     e.EditedAt,
			EditorUserID: e.EditorUserID,
		}
		if e.Editor != nil && e.Editor.ID != 0 {
			if e.Editor.Username != nil {
				entry.EditorUsername = *e.Editor.Username
			}
			if e.Editor.FirstName != nil {
				entry.EditorName = *e.Editor.FirstName
			}
		}
		entries[i] = entry
	}

	return &contracts.CommentEditHistoryResponse{
		CommentID:   comment.ID,
		CurrentBody: comment.Body,
		Edits:       entries,
	}, nil
}

// DeleteComment performs a soft delete by setting visibility.
// Authors set hidden_by_user; admins set hidden_by_mod.
func (s *CommentService) DeleteComment(userID uint, commentID uint, isAdmin bool) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	// Non-admin users can only delete their own comments
	if !isAdmin && comment.UserID != userID {
		return errors.New("only the comment author or an admin can delete this comment")
	}

	visibility := engagementm.CommentVisibilityHiddenByUser
	if isAdmin && comment.UserID != userID {
		visibility = engagementm.CommentVisibilityHiddenByMod
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility": visibility,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	return nil
}

// ============================================================================
// Field Note methods
// ============================================================================

// CreateFieldNote creates a field note (specialized comment) on a show.
// Field notes must target past shows and store structured data (sound quality, crowd energy, etc.).
func (s *CommentService) CreateFieldNote(userID uint, req *contracts.CreateFieldNoteRequest) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Validate body
	body := strings.TrimSpace(req.Body)
	if len(body) < engagementm.MinCommentBodyLength {
		return nil, errors.New("field note body is required")
	}
	if len(body) > engagementm.MaxCommentBodyLength {
		return nil, fmt.Errorf("field note body exceeds maximum length of %d characters", engagementm.MaxCommentBodyLength)
	}

	// Validate sound_quality range (1-5)
	if req.SoundQuality != nil && (*req.SoundQuality < 1 || *req.SoundQuality > 5) {
		return nil, errors.New("sound_quality must be between 1 and 5")
	}

	// Validate crowd_energy range (1-5)
	if req.CrowdEnergy != nil && (*req.CrowdEnergy < 1 || *req.CrowdEnergy > 5) {
		return nil, errors.New("crowd_energy must be between 1 and 5")
	}

	// Look up the show and verify it's in the past
	var show catalogm.Show
	if err := s.db.First(&show, req.ShowID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("show not found")
		}
		return nil, fmt.Errorf("failed to look up show: %w", err)
	}

	if show.EventDate.After(time.Now()) {
		return nil, errors.New("field notes can only be added to past shows")
	}

	// Validate show_artist_id belongs to this show (if provided)
	if req.ShowArtistID != nil {
		var count int64
		if err := s.db.Model(&catalogm.ShowArtist{}).
			Where("show_id = ? AND artist_id = ?", req.ShowID, *req.ShowArtistID).
			Count(&count).Error; err != nil {
			return nil, fmt.Errorf("failed to validate show artist: %w", err)
		}
		if count == 0 {
			return nil, errors.New("artist is not on this show's bill")
		}
	}

	// Look up user for trust tier and rate limiting
	var user authm.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	// Rate limiting: per-entity cooldown (60s between comments on same entity)
	var recentEntityCount int64
	if err := s.db.Model(&engagementm.Comment{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND created_at > ?",
			userID, engagementm.CommentEntityShow, req.ShowID, time.Now().Add(-60*time.Second)).
		Count(&recentEntityCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check rate limit: %w", err)
	}
	if recentEntityCount > 0 {
		return nil, errors.New("please wait 60 seconds between comments on the same entity")
	}

	// Rate limiting: global hourly limit based on trust tier
	hourlyLimit := userTierHourlyLimit(user.UserTier)
	if hourlyLimit >= 0 {
		var hourlyCount int64
		if err := s.db.Model(&engagementm.Comment{}).
			Where("user_id = ? AND created_at > ?", userID, time.Now().Add(-1*time.Hour)).
			Count(&hourlyCount).Error; err != nil {
			return nil, fmt.Errorf("failed to check hourly rate limit: %w", err)
		}
		if int(hourlyCount) >= hourlyLimit {
			return nil, fmt.Errorf("you've reached your hourly comment limit (%d/hour for %s users)",
				hourlyLimit, func() string {
					if user.UserTier == "" {
						return "new"
					}
					return strings.ReplaceAll(user.UserTier, "_", " ")
				}())
		}
	}

	// Compute verified attendee: user marked "going" before the show date
	isVerifiedAttendee := false
	var goingBookmark engagementm.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		userID, engagementm.BookmarkEntityShow, req.ShowID, engagementm.BookmarkActionGoing,
	).First(&goingBookmark).Error
	if err == nil {
		// User has a "going" bookmark — verified if created before the show date
		if goingBookmark.CreatedAt.Before(show.EventDate) {
			isVerifiedAttendee = true
		}
	}

	// Build structured data
	structuredData := contracts.FieldNoteStructuredData{
		ShowArtistID:       req.ShowArtistID,
		SongPosition:       req.SongPosition,
		SoundQuality:       req.SoundQuality,
		CrowdEnergy:        req.CrowdEnergy,
		NotableMoments:     req.NotableMoments,
		SetlistSpoiler:     req.SetlistSpoiler,
		IsVerifiedAttendee: isVerifiedAttendee,
	}
	sdJSON, err := json.Marshal(structuredData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal structured data: %w", err)
	}
	rawJSON := json.RawMessage(sdJSON)

	// Render markdown to HTML
	bodyHTML := s.renderMarkdown(body)

	// Determine initial visibility based on trust tier
	visibility := computeInitialVisibility(&user)

	comment := &engagementm.Comment{
		EntityType:      engagementm.CommentEntityShow,
		EntityID:        req.ShowID,
		Kind:            engagementm.CommentKindFieldNote,
		UserID:          userID,
		Body:            body,
		BodyHTML:        bodyHTML,
		StructuredData:  &rawJSON,
		Visibility:      visibility,
		ReplyPermission: engagementm.ReplyPermissionAnyone,
	}

	if err := s.db.Create(comment).Error; err != nil {
		return nil, fmt.Errorf("failed to create field note: %w", err)
	}

	// Auto-subscribe the user to this show's comments (fire-and-forget)
	sub := engagementm.CommentSubscription{
		UserID:       userID,
		EntityType:   string(engagementm.CommentEntityShow),
		EntityID:     req.ShowID,
		SubscribedAt: time.Now().UTC(),
	}
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&sub).Error; err != nil {
		log.Printf("warning: failed to auto-subscribe user %d to show/%d comments: %v", userID, req.ShowID, err)
	}

	// Reload with user info
	if err := s.db.Preload("User").First(comment, comment.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload field note: %w", err)
	}

	return commentToResponse(comment), nil
}

// ListFieldNotesForShow returns field notes for a show, sorted by song_position ASC (NULLs first),
// then by score DESC within the same position.
func (s *CommentService) ListFieldNotesForShow(showID uint, limit, offset int) (*contracts.CommentListResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// Base query: field notes on this show that are visible
	query := s.db.Model(&engagementm.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND kind = ? AND visibility = ?",
			engagementm.CommentEntityShow, showID, engagementm.CommentKindFieldNote, engagementm.CommentVisibilityVisible)

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count field notes: %w", err)
	}

	// Sort by song_position ASC (NULLs first), then score DESC
	// We extract song_position from the JSONB structured_data column.
	var comments []engagementm.Comment
	if err := query.Preload("User").
		Order("(structured_data->>'song_position')::int ASC NULLS FIRST, score DESC").
		Limit(limit).
		Offset(offset).
		Find(&comments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch field notes: %w", err)
	}

	responses := make([]*contracts.CommentResponse, len(comments))
	for i := range comments {
		responses[i] = commentToResponse(&comments[i])
	}

	return &contracts.CommentListResponse{
		Comments: responses,
		Total:    total,
		HasMore:  int64(offset+limit) < total,
	}, nil
}

// ============================================================================
// Admin moderation methods
// ============================================================================

// HideComment hides a comment with a reason (admin action).
func (s *CommentService) HideComment(adminUserID uint, commentID uint, reason string) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	now := time.Now()
	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility":        engagementm.CommentVisibilityHiddenByMod,
		"hidden_reason":     reason,
		"hidden_by_user_id": adminUserID,
		"hidden_at":         now,
		"updated_at":        now,
	}).Error; err != nil {
		return fmt.Errorf("failed to hide comment: %w", err)
	}

	return nil
}

// RestoreComment restores a hidden comment to visible (admin action).
func (s *CommentService) RestoreComment(adminUserID uint, commentID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	if comment.Visibility == engagementm.CommentVisibilityVisible {
		return errors.New("comment is already visible")
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility":        engagementm.CommentVisibilityVisible,
		"hidden_reason":     nil,
		"hidden_by_user_id": nil,
		"hidden_at":         nil,
		"updated_at":        time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("failed to restore comment: %w", err)
	}

	return nil
}

// ListPendingComments returns comments with pending_review visibility.
func (s *CommentService) ListPendingComments(limit, offset int) ([]*contracts.CommentResponse, int64, error) {
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
	if err := s.db.Model(&engagementm.Comment{}).
		Where("visibility = ?", engagementm.CommentVisibilityPendingReview).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending comments: %w", err)
	}

	var comments []engagementm.Comment
	if err := s.db.Preload("User").
		Where("visibility = ?", engagementm.CommentVisibilityPendingReview).
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&comments).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch pending comments: %w", err)
	}

	responses := make([]*contracts.CommentResponse, len(comments))
	for i := range comments {
		responses[i] = commentToResponse(&comments[i])
	}

	return responses, total, nil
}

// ApproveComment approves a pending comment (sets visibility to visible).
func (s *CommentService) ApproveComment(adminUserID uint, commentID uint) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	if comment.Visibility != engagementm.CommentVisibilityPendingReview {
		return errors.New("comment is not pending review")
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility": engagementm.CommentVisibilityVisible,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("failed to approve comment: %w", err)
	}

	return nil
}

// RejectComment rejects a pending comment (sets visibility to hidden_by_mod).
func (s *CommentService) RejectComment(adminUserID uint, commentID uint, reason string) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment engagementm.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	if comment.Visibility != engagementm.CommentVisibilityPendingReview {
		return errors.New("comment is not pending review")
	}

	now := time.Now()
	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility":        engagementm.CommentVisibilityHiddenByMod,
		"hidden_reason":     reason,
		"hidden_by_user_id": adminUserID,
		"hidden_at":         now,
		"updated_at":        now,
	}).Error; err != nil {
		return fmt.Errorf("failed to reject comment: %w", err)
	}

	return nil
}
