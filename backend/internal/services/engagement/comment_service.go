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

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

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
	db       *gorm.DB
	md       goldmark.Markdown
	sanitize *bluemonday.Policy
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
func computeInitialVisibility(user *models.User) models.CommentVisibility {
	if user.IsAdmin {
		return models.CommentVisibilityVisible
	}
	switch user.UserTier {
	case "contributor", "trusted_contributor", "local_ambassador":
		return models.CommentVisibilityVisible
	default: // "new_user" or empty
		return models.CommentVisibilityPendingReview
	}
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

	// Look up user for trust tier and rate limiting
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	// Rate limiting: per-entity cooldown (60s between comments on same entity)
	var recentEntityCount int64
	if err := s.db.Model(&models.Comment{}).
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
		if err := s.db.Model(&models.Comment{}).
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

	// Determine initial visibility based on trust tier
	visibility := computeInitialVisibility(&user)

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
		Visibility:      visibility,
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

	// PSY-289: fire-and-forget notification fan-out. Only for visible
	// comments — pending_review comments should not trigger emails until
	// approved. Added at the END of the function so parallel agents working
	// on the top of this method (e.g. reply-permission gate) don't conflict.
	if s.notifier != nil && comment.Visibility == models.CommentVisibilityVisible {
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
	if len(body) < models.MinCommentBodyLength {
		return nil, errors.New("field note body is required")
	}
	if len(body) > models.MaxCommentBodyLength {
		return nil, fmt.Errorf("field note body exceeds maximum length of %d characters", models.MaxCommentBodyLength)
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
	var show models.Show
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
		if err := s.db.Model(&models.ShowArtist{}).
			Where("show_id = ? AND artist_id = ?", req.ShowID, *req.ShowArtistID).
			Count(&count).Error; err != nil {
			return nil, fmt.Errorf("failed to validate show artist: %w", err)
		}
		if count == 0 {
			return nil, errors.New("artist is not on this show's bill")
		}
	}

	// Look up user for trust tier and rate limiting
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to look up user: %w", err)
	}

	// Rate limiting: per-entity cooldown (60s between comments on same entity)
	var recentEntityCount int64
	if err := s.db.Model(&models.Comment{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND created_at > ?",
			userID, models.CommentEntityShow, req.ShowID, time.Now().Add(-60*time.Second)).
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
		if err := s.db.Model(&models.Comment{}).
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
	var goingBookmark models.UserBookmark
	err := s.db.Where(
		"user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		userID, models.BookmarkEntityShow, req.ShowID, models.BookmarkActionGoing,
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

	comment := &models.Comment{
		EntityType:      models.CommentEntityShow,
		EntityID:        req.ShowID,
		Kind:            models.CommentKindFieldNote,
		UserID:          userID,
		Body:            body,
		BodyHTML:        bodyHTML,
		StructuredData:  &rawJSON,
		Visibility:      visibility,
		ReplyPermission: models.ReplyPermissionAnyone,
	}

	if err := s.db.Create(comment).Error; err != nil {
		return nil, fmt.Errorf("failed to create field note: %w", err)
	}

	// Auto-subscribe the user to this show's comments (fire-and-forget)
	sub := models.CommentSubscription{
		UserID:       userID,
		EntityType:   string(models.CommentEntityShow),
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
	query := s.db.Model(&models.Comment{}).
		Where("entity_type = ? AND entity_id = ? AND kind = ? AND visibility = ?",
			models.CommentEntityShow, showID, models.CommentKindFieldNote, models.CommentVisibilityVisible)

	// Count total
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count field notes: %w", err)
	}

	// Sort by song_position ASC (NULLs first), then score DESC
	// We extract song_position from the JSONB structured_data column.
	var comments []models.Comment
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

	var comment models.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	now := time.Now()
	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility":        models.CommentVisibilityHiddenByMod,
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

	var comment models.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	if comment.Visibility == models.CommentVisibilityVisible {
		return errors.New("comment is already visible")
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility":        models.CommentVisibilityVisible,
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
	if err := s.db.Model(&models.Comment{}).
		Where("visibility = ?", models.CommentVisibilityPendingReview).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count pending comments: %w", err)
	}

	var comments []models.Comment
	if err := s.db.Preload("User").
		Where("visibility = ?", models.CommentVisibilityPendingReview).
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

	var comment models.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	if comment.Visibility != models.CommentVisibilityPendingReview {
		return errors.New("comment is not pending review")
	}

	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility": models.CommentVisibilityVisible,
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

	var comment models.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("comment not found")
		}
		return fmt.Errorf("failed to fetch comment: %w", err)
	}

	if comment.Visibility != models.CommentVisibilityPendingReview {
		return errors.New("comment is not pending review")
	}

	now := time.Now()
	if err := s.db.Model(&comment).Updates(map[string]interface{}{
		"visibility":        models.CommentVisibilityHiddenByMod,
		"hidden_reason":     reason,
		"hidden_by_user_id": adminUserID,
		"hidden_at":         now,
		"updated_at":        now,
	}).Error; err != nil {
		return fmt.Errorf("failed to reject comment: %w", err)
	}

	return nil
}
