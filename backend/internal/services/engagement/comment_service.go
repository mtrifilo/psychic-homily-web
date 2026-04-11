package engagement

import (
	"bytes"
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

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// CommentService implements CommentServiceInterface for comment CRUD and threading.
type CommentService struct {
	db       *gorm.DB
	md       goldmark.Markdown
	sanitize *bluemonday.Policy
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

	return &CommentService{
		db:       db,
		md:       goldmark.New(),
		sanitize: policy,
	}
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
func validateCommentEntityType(entityType string) (models.CommentEntityType, error) {
	ct := models.CommentEntityType(entityType)
	if _, ok := models.ValidCommentEntityTypes[ct]; !ok {
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}
	return ct, nil
}

// validateEntityExists checks that the referenced entity actually exists in the database.
func (s *CommentService) validateEntityExists(entityType models.CommentEntityType, entityID uint) error {
	tableName, ok := models.ValidCommentEntityTypes[entityType]
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
func commentToResponse(c *models.Comment) *contracts.CommentResponse {
	resp := &contracts.CommentResponse{
		ID:              c.ID,
		EntityType:      string(c.EntityType),
		EntityID:        c.EntityID,
		Kind:            string(c.Kind),
		UserID:          c.UserID,
		ParentID:        c.ParentID,
		RootID:          c.RootID,
		Depth:           c.Depth,
		Body:            c.Body,
		BodyHTML:        c.BodyHTML,
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

	// Populate author info from preloaded User
	if c.User.ID != 0 {
		if c.User.Username != nil {
			resp.AuthorUsername = *c.User.Username
		}
		if c.User.FirstName != nil {
			resp.AuthorName = *c.User.FirstName
		}
	}

	return resp
}

// CreateComment creates a new comment or reply.
func (s *CommentService) CreateComment(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Validate body length
	body := strings.TrimSpace(req.Body)
	if len(body) < models.MinCommentBodyLength {
		return nil, errors.New("comment body is required")
	}
	if len(body) > models.MaxCommentBodyLength {
		return nil, fmt.Errorf("comment body exceeds maximum length of %d characters", models.MaxCommentBodyLength)
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

	// Determine kind
	kind := models.CommentKindComment
	if req.Kind == string(models.CommentKindFieldNote) {
		kind = models.CommentKindFieldNote
	}

	// Determine reply permission
	replyPerm := models.ReplyPermissionAnyone
	if req.ReplyPermission == string(models.ReplyPermissionAuthorOnly) {
		replyPerm = models.ReplyPermissionAuthorOnly
	}

	// Threading: handle parent_id
	var parentID *uint
	var rootID *uint
	depth := 0

	if req.ParentID != nil && *req.ParentID > 0 {
		// Validate parent exists and belongs to the same entity
		var parent models.Comment
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
		if depth > models.MaxCommentDepth {
			return nil, fmt.Errorf("maximum reply depth of %d exceeded", models.MaxCommentDepth)
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

	comment := &models.Comment{
		EntityType:      entityType,
		EntityID:        req.EntityID,
		Kind:            kind,
		UserID:          userID,
		ParentID:        parentID,
		RootID:          rootID,
		Depth:           depth,
		Body:            body,
		BodyHTML:        bodyHTML,
		Visibility:      models.CommentVisibilityVisible,
		ReplyPermission: replyPerm,
	}

	if err := s.db.Create(comment).Error; err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	// Auto-subscribe the commenter to this entity's comments (fire-and-forget)
	sub := models.CommentSubscription{
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

	return commentToResponse(comment), nil
}

// GetComment returns a single comment by ID.
func (s *CommentService) GetComment(commentID uint) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	var comment models.Comment
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
	query := s.db.Model(&models.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND parent_id IS NULL", et, entityID)

	// Filter by visibility (default: visible only)
	if filters.Visibility != "" {
		query = query.Where("visibility = ?", filters.Visibility)
	} else {
		query = query.Where("visibility = ?", models.CommentVisibilityVisible)
	}

	// Filter by kind
	if filters.Kind != "" {
		query = query.Where("kind = ?", filters.Kind)
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
	var comments []models.Comment
	if err := query.Preload("User").Limit(limit).Offset(offset).Find(&comments).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}

	// Map to response
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

// GetThread loads a root comment and all its descendants as a flat list ordered by created_at.
func (s *CommentService) GetThread(rootID uint) ([]*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Verify the root comment exists
	var root models.Comment
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
	var comments []models.Comment
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
func (s *CommentService) UpdateComment(userID uint, commentID uint, req *contracts.UpdateCommentRequest) (*contracts.CommentResponse, error) {
	if s.db == nil {
		return nil, errors.New("database not initialized")
	}

	// Validate body
	body := strings.TrimSpace(req.Body)
	if len(body) < models.MinCommentBodyLength {
		return nil, errors.New("comment body is required")
	}
	if len(body) > models.MaxCommentBodyLength {
		return nil, fmt.Errorf("comment body exceeds maximum length of %d characters", models.MaxCommentBodyLength)
	}

	var comment models.Comment
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

	// Store old body in comment_edits (append-only edit history)
	edit := &models.CommentEdit{
		CommentID: comment.ID,
		OldBody:   comment.Body,
		EditedAt:  time.Now(),
	}
	if err := s.db.Create(edit).Error; err != nil {
		return nil, fmt.Errorf("failed to save edit history: %w", err)
	}

	// Update the comment
	bodyHTML := s.renderMarkdown(body)
	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"body":       body,
		"body_html":  bodyHTML,
		"edit_count": gorm.Expr("edit_count + 1"),
		"updated_at": time.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update comment: %w", err)
	}

	// Reload with user info
	if err := s.db.Preload("User").First(&comment, commentID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload comment: %w", err)
	}

	return commentToResponse(&comment), nil
}

// DeleteComment performs a soft delete by setting visibility.
// Authors set hidden_by_user; admins set hidden_by_mod.
func (s *CommentService) DeleteComment(userID uint, commentID uint, isAdmin bool) error {
	if s.db == nil {
		return errors.New("database not initialized")
	}

	var comment models.Comment
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

	visibility := models.CommentVisibilityHiddenByUser
	if isAdmin && comment.UserID != userID {
		visibility = models.CommentVisibilityHiddenByMod
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility": visibility,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	return nil
}
