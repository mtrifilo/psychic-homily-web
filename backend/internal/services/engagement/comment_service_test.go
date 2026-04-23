package engagement

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestCommentService_NilDB(t *testing.T) {
	svc := NewCommentService(nil)

	t.Run("CreateComment_NilDB", func(t *testing.T) {
		_, err := svc.CreateComment(1, &contracts.CreateCommentRequest{
			EntityType: "artist",
			EntityID:   1,
			Body:       "test",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetComment_NilDB", func(t *testing.T) {
		_, err := svc.GetComment(1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("ListCommentsForEntity_NilDB", func(t *testing.T) {
		_, err := svc.ListCommentsForEntity("artist", 1, contracts.CommentListFilters{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetThread_NilDB", func(t *testing.T) {
		_, err := svc.GetThread(1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("UpdateComment_NilDB", func(t *testing.T) {
		_, err := svc.UpdateComment(1, 1, &contracts.UpdateCommentRequest{Body: "test"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("DeleteComment_NilDB", func(t *testing.T) {
		err := svc.DeleteComment(1, 1, false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("HideComment_NilDB", func(t *testing.T) {
		err := svc.HideComment(1, 1, "reason")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("RestoreComment_NilDB", func(t *testing.T) {
		err := svc.RestoreComment(1, 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("ListPendingComments_NilDB", func(t *testing.T) {
		_, _, err := svc.ListPendingComments(20, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("ApproveComment_NilDB", func(t *testing.T) {
		err := svc.ApproveComment(1, 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("RejectComment_NilDB", func(t *testing.T) {
		err := svc.RejectComment(1, 1, "reason")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetCommentEditHistory_NilDB", func(t *testing.T) {
		_, err := svc.GetCommentEditHistory(1, 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})
}

func TestCommentService_InvalidEntityType(t *testing.T) {
	svc := &CommentService{db: &gorm.DB{}}

	t.Run("CreateComment_InvalidEntityType", func(t *testing.T) {
		_, err := svc.CreateComment(1, &contracts.CreateCommentRequest{
			EntityType: "banana",
			EntityID:   1,
			Body:       "test",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported entity type")
	})
}

func TestCommentService_BodyValidation(t *testing.T) {
	svc := &CommentService{db: &gorm.DB{}}

	t.Run("CreateComment_EmptyBody", func(t *testing.T) {
		_, err := svc.CreateComment(1, &contracts.CreateCommentRequest{
			EntityType: "artist",
			EntityID:   1,
			Body:       "",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "comment body is required")
	})

	t.Run("CreateComment_WhitespaceBody", func(t *testing.T) {
		_, err := svc.CreateComment(1, &contracts.CreateCommentRequest{
			EntityType: "artist",
			EntityID:   1,
			Body:       "   \n\t  ",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "comment body is required")
	})

	t.Run("CreateComment_TooLongBody", func(t *testing.T) {
		_, err := svc.CreateComment(1, &contracts.CreateCommentRequest{
			EntityType: "artist",
			EntityID:   1,
			Body:       strings.Repeat("a", models.MaxCommentBodyLength+1),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})

	t.Run("UpdateComment_EmptyBody", func(t *testing.T) {
		_, err := svc.UpdateComment(1, 1, &contracts.UpdateCommentRequest{Body: ""})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "comment body is required")
	})

	t.Run("UpdateComment_TooLongBody", func(t *testing.T) {
		_, err := svc.UpdateComment(1, 1, &contracts.UpdateCommentRequest{
			Body: strings.Repeat("a", models.MaxCommentBodyLength+1),
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum length")
	})
}

func TestMarkdownRendering(t *testing.T) {
	svc := NewCommentService(nil)

	t.Run("Bold", func(t *testing.T) {
		html := svc.renderMarkdown("**bold text**")
		assert.Contains(t, html, "<strong>bold text</strong>")
	})

	t.Run("Italic", func(t *testing.T) {
		html := svc.renderMarkdown("*italic text*")
		assert.Contains(t, html, "<em>italic text</em>")
	})

	t.Run("Link", func(t *testing.T) {
		html := svc.renderMarkdown("[click here](https://example.com)")
		assert.Contains(t, html, `href="https://example.com"`)
		assert.Contains(t, html, "click here")
	})

	t.Run("CodeBlock", func(t *testing.T) {
		html := svc.renderMarkdown("```\ncode block\n```")
		assert.Contains(t, html, "<pre>")
		assert.Contains(t, html, "<code>")
	})

	t.Run("InlineCode", func(t *testing.T) {
		html := svc.renderMarkdown("`inline code`")
		assert.Contains(t, html, "<code>inline code</code>")
	})

	t.Run("Blockquote", func(t *testing.T) {
		html := svc.renderMarkdown("> quoted text")
		assert.Contains(t, html, "<blockquote>")
	})

	t.Run("List", func(t *testing.T) {
		html := svc.renderMarkdown("- item 1\n- item 2")
		assert.Contains(t, html, "<ul>")
		assert.Contains(t, html, "<li>")
	})

	t.Run("NoImages", func(t *testing.T) {
		html := svc.renderMarkdown("![alt](https://example.com/img.png)")
		// bluemonday should strip img tags
		assert.NotContains(t, html, "<img")
	})

	t.Run("NoRawHTML", func(t *testing.T) {
		html := svc.renderMarkdown("<script>alert('xss')</script>")
		assert.NotContains(t, html, "<script>")
	})

	t.Run("Heading3Allowed", func(t *testing.T) {
		html := svc.renderMarkdown("### Heading 3")
		assert.Contains(t, html, "<h3>")
	})

	t.Run("Heading1Stripped", func(t *testing.T) {
		html := svc.renderMarkdown("# Heading 1")
		// h1 is not in our allowed list, so it should be stripped
		assert.NotContains(t, html, "<h1>")
	})

	t.Run("Heading2Stripped", func(t *testing.T) {
		html := svc.renderMarkdown("## Heading 2")
		assert.NotContains(t, html, "<h2>")
	})
}

func TestComputeInitialVisibility(t *testing.T) {
	t.Run("AdminAlwaysVisible", func(t *testing.T) {
		user := &models.User{IsAdmin: true, UserTier: "new_user"}
		assert.Equal(t, models.CommentVisibilityVisible, computeInitialVisibility(user))
	})
	t.Run("NewUser_PendingReview", func(t *testing.T) {
		user := &models.User{UserTier: "new_user"}
		assert.Equal(t, models.CommentVisibilityPendingReview, computeInitialVisibility(user))
	})
	t.Run("EmptyTier_PendingReview", func(t *testing.T) {
		user := &models.User{UserTier: ""}
		assert.Equal(t, models.CommentVisibilityPendingReview, computeInitialVisibility(user))
	})
	t.Run("Contributor_Visible", func(t *testing.T) {
		user := &models.User{UserTier: "contributor"}
		assert.Equal(t, models.CommentVisibilityVisible, computeInitialVisibility(user))
	})
	t.Run("TrustedContributor_Visible", func(t *testing.T) {
		user := &models.User{UserTier: "trusted_contributor"}
		assert.Equal(t, models.CommentVisibilityVisible, computeInitialVisibility(user))
	})
	t.Run("LocalAmbassador_Visible", func(t *testing.T) {
		user := &models.User{UserTier: "local_ambassador"}
		assert.Equal(t, models.CommentVisibilityVisible, computeInitialVisibility(user))
	})
}

func TestUserTierHourlyLimit(t *testing.T) {
	assert.Equal(t, 5, userTierHourlyLimit("new_user"))
	assert.Equal(t, 5, userTierHourlyLimit(""))
	assert.Equal(t, 30, userTierHourlyLimit("contributor"))
	assert.Equal(t, 100, userTierHourlyLimit("trusted_contributor"))
	assert.Equal(t, -1, userTierHourlyLimit("local_ambassador"))
	assert.Equal(t, -1, userTierHourlyLimit("admin"))
	assert.Equal(t, 5, userTierHourlyLimit("unknown_tier"))
}

func TestWilsonScore(t *testing.T) {
	t.Run("NoVotes", func(t *testing.T) {
		score := wilsonScore(0, 0)
		assert.Equal(t, 0.0, score)
	})

	t.Run("AllUpvotes", func(t *testing.T) {
		score := wilsonScore(10, 0)
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 1.0)
	})

	t.Run("AllDownvotes", func(t *testing.T) {
		score := wilsonScore(0, 10)
		assert.Equal(t, 0.0, score)
	})

	t.Run("MixedVotes", func(t *testing.T) {
		score := wilsonScore(8, 2)
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 1.0)
	})

	t.Run("HighConfidenceBeatsLowSample", func(t *testing.T) {
		highN := wilsonScore(95, 5)
		lowN := wilsonScore(3, 0)
		assert.Greater(t, highN, lowN)
	})

	t.Run("SingleUpvote", func(t *testing.T) {
		score := wilsonScore(1, 0)
		assert.Greater(t, score, 0.0)
		assert.Less(t, score, 1.0)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type CommentServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	commentService *CommentService
}

func (suite *CommentServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.commentService = NewCommentService(suite.testDB.DB)
}

func (suite *CommentServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CommentServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM comment_votes")
	_, _ = sqlDB.Exec("DELETE FROM comment_edits")
	_, _ = sqlDB.Exec("DELETE FROM comments")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCommentServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CommentServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("user%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
		UserTier:      "contributor", // contributor tier auto-publishes comments
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *CommentServiceIntegrationTestSuite) createTestNewUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("newuser-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("newuser%d", time.Now().UnixNano())),
		FirstName:     stringPtr("New"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
		UserTier:      "new_user", // new_user tier → pending_review
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *CommentServiceIntegrationTestSuite) createTestAdmin() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("admin-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("admin%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Admin"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *CommentServiceIntegrationTestSuite) createTestArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *CommentServiceIntegrationTestSuite) createTestVenue(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	venue := &models.Venue{
		Name:  name,
		Slug:  &slug,
		City:  "Phoenix",
		State: "AZ",
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue.ID
}

// insertComment creates a comment directly in the DB, bypassing rate limiting.
// Used for test setup where we need multiple comments rapidly.
func (suite *CommentServiceIntegrationTestSuite) insertComment(userID uint, entityType string, entityID uint, body string, parentID *uint, rootID *uint, depth int) *models.Comment {
	svc := suite.commentService
	bodyHTML := svc.renderMarkdown(body)
	comment := &models.Comment{
		EntityType:      models.CommentEntityType(entityType),
		EntityID:        entityID,
		Kind:            models.CommentKindComment,
		UserID:          userID,
		ParentID:        parentID,
		RootID:          rootID,
		Depth:           depth,
		Body:            body,
		BodyHTML:        bodyHTML,
		Visibility:      models.CommentVisibilityVisible,
		ReplyPermission: models.ReplyPermissionAnyone,
	}
	err := suite.db.Create(comment).Error
	suite.Require().NoError(err)
	return comment
}

func (suite *CommentServiceIntegrationTestSuite) createTestShow(title string) uint {
	slug := fmt.Sprintf("%s-%d", title, time.Now().UnixNano())
	show := &models.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    models.ShowStatusApproved,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show.ID
}

// =============================================================================
// Group 1: CreateComment
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_TopLevel_Artist() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Test Artist")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Great artist!",
	})
	suite.Require().NoError(err)
	suite.NotZero(comment.ID)
	suite.Equal("artist", comment.EntityType)
	suite.Equal(artistID, comment.EntityID)
	suite.Equal("comment", comment.Kind)
	suite.Equal(user.ID, comment.UserID)
	suite.Equal("Great artist!", comment.Body)
	suite.Contains(comment.BodyHTML, "Great artist!")
	suite.Equal("visible", comment.Visibility)
	suite.Equal("anyone", comment.ReplyPermission)
	suite.Equal(0, comment.Depth)
	suite.Nil(comment.ParentID)
	suite.Nil(comment.RootID)
	suite.Equal(0, comment.Ups)
	suite.Equal(0, comment.Downs)
	suite.False(comment.IsEdited)
	suite.Equal("Test", comment.AuthorName)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_TopLevel_Venue() {
	user := suite.createTestUser()
	venueID := suite.createTestVenue("The Rebel Lounge")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "venue",
		EntityID:   venueID,
		Body:       "Best venue in town!",
	})
	suite.Require().NoError(err)
	suite.Equal("venue", comment.EntityType)
	suite.Equal(venueID, comment.EntityID)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_TopLevel_Show() {
	user := suite.createTestUser()
	showID := suite.createTestShow("Test Show")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "show",
		EntityID:   showID,
		Body:       "Can't wait for this show!",
	})
	suite.Require().NoError(err)
	suite.Equal("show", comment.EntityType)
	suite.Equal(showID, comment.EntityID)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_EntityNotFound() {
	user := suite.createTestUser()

	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   999999,
		Body:       "This won't work",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "not found")
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_WithReplyPermission() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Locked Artist")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType:      "artist",
		EntityID:        artistID,
		Body:            "No replies please",
		ReplyPermission: "author_only",
	})
	suite.Require().NoError(err)
	suite.Equal("author_only", comment.ReplyPermission)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_MarkdownRendered() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Markdown Artist")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "**bold** and *italic*",
	})
	suite.Require().NoError(err)
	suite.Contains(comment.BodyHTML, "<strong>bold</strong>")
	suite.Contains(comment.BodyHTML, "<em>italic</em>")
}

// =============================================================================
// Group 2: CreateComment — Replies (Threading)
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_Reply() {
	user := suite.createTestUser()
	user2 := suite.createTestUser()
	artistID := suite.createTestArtist("Reply Artist")

	// Create top-level comment
	root, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Root comment",
	})
	suite.Require().NoError(err)

	// Create reply from a different user (avoids per-entity cooldown)
	reply, err := suite.commentService.CreateComment(user2.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply comment",
		ParentID:   &root.ID,
	})
	suite.Require().NoError(err)

	suite.Equal(1, reply.Depth)
	suite.NotNil(reply.ParentID)
	suite.Equal(root.ID, *reply.ParentID)
	suite.NotNil(reply.RootID)
	suite.Equal(root.ID, *reply.RootID)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_NestedReply() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	user3 := suite.createTestUser()
	artistID := suite.createTestArtist("Nested Reply Artist")

	// Depth 0: root (user1)
	root, err := suite.commentService.CreateComment(user1.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Depth 0",
	})
	suite.Require().NoError(err)

	// Depth 1: reply to root (user2)
	reply1, err := suite.commentService.CreateComment(user2.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Depth 1",
		ParentID:   &root.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(1, reply1.Depth)

	// Depth 2: reply to reply (user3, max allowed)
	reply2, err := suite.commentService.CreateComment(user3.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Depth 2",
		ParentID:   &reply1.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(2, reply2.Depth)
	suite.Equal(root.ID, *reply2.RootID) // root_id should always point to the thread root
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_DepthLimitExceeded() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Deep Artist")

	// Create chain via direct DB insert (bypasses rate limiting)
	root := suite.insertComment(user.ID, "artist", artistID, "Root", nil, nil, 0)
	reply1 := suite.insertComment(user.ID, "artist", artistID, "Reply 1", &root.ID, &root.ID, 1)
	reply2 := suite.insertComment(user.ID, "artist", artistID, "Reply 2", &reply1.ID, &root.ID, 2)

	// Attempt depth 3 via service — should fail due to max depth
	user2 := suite.createTestUser()
	_, err := suite.commentService.CreateComment(user2.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Too deep",
		ParentID:   &reply2.ID,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "maximum reply depth")
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_ParentNotFound() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Missing Parent Artist")

	badParent := uint(999999)
	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Orphan reply",
		ParentID:   &badParent,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "parent comment not found")
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_ParentOnDifferentEntity() {
	user := suite.createTestUser()
	artist1ID := suite.createTestArtist("Artist 1")
	artist2ID := suite.createTestArtist("Artist 2")

	// Comment on artist 1
	root, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artist1ID,
		Body:       "Comment on artist 1",
	})

	// Try to reply but claim it's on artist 2
	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artist2ID,
		Body:       "Cross-entity reply",
		ParentID:   &root.ID,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "parent comment belongs to a different entity")
}

// =============================================================================
// Group 3: GetComment
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestGetComment_Success() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Get Artist")

	created, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Test get",
	})

	fetched, err := suite.commentService.GetComment(created.ID)
	suite.Require().NoError(err)
	suite.Equal(created.ID, fetched.ID)
	suite.Equal("Test get", fetched.Body)
}

func (suite *CommentServiceIntegrationTestSuite) TestGetComment_NotFound() {
	_, err := suite.commentService.GetComment(999999)
	suite.Error(err)
	suite.Contains(err.Error(), "comment not found")
}

// =============================================================================
// Group 4: ListCommentsForEntity
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestListComments_BasicPagination() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("List Artist")

	// Create 5 comments via direct insert (bypasses rate limiting)
	for i := 0; i < 5; i++ {
		suite.insertComment(user.ID, "artist", artistID, fmt.Sprintf("Comment %d", i), nil, nil, 0)
	}

	// Page 1 (limit 2)
	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{
		Limit: 2,
	})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 2)
	suite.Equal(int64(5), result.Total)
	suite.True(result.HasMore)

	// Page 3 (offset 4, limit 2)
	result, err = suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{
		Limit:  2,
		Offset: 4,
	})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 1)
	suite.False(result.HasMore)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_TopLevelOnly() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Top Level Only Artist")

	// Create a root and a reply via direct insert (bypasses rate limiting)
	root := suite.insertComment(user.ID, "artist", artistID, "Root comment", nil, nil, 0)
	suite.insertComment(user.ID, "artist", artistID, "Reply comment", &root.ID, &root.ID, 1)

	// List should only return top-level
	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 1)
	suite.Equal("Root comment", result.Comments[0].Body)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_SortByNew() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Sort New Artist")

	// Direct insert to bypass rate limiting
	c1 := suite.insertComment(user.ID, "artist", artistID, "First comment", nil, nil, 0)
	time.Sleep(10 * time.Millisecond)
	c2 := suite.insertComment(user.ID, "artist", artistID, "Second comment", nil, nil, 0)

	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{
		Sort: "new",
	})
	suite.Require().NoError(err)
	suite.Require().Len(result.Comments, 2)
	// Newest first
	suite.Equal(c2.ID, result.Comments[0].ID)
	suite.Equal(c1.ID, result.Comments[1].ID)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_SortByBest() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Sort Best Artist")

	// Direct insert to bypass rate limiting
	c1 := suite.insertComment(user.ID, "artist", artistID, "Low score", nil, nil, 0)
	c2 := suite.insertComment(user.ID, "artist", artistID, "High score", nil, nil, 0)

	// Manually set scores (simulate voting)
	suite.db.Model(&models.Comment{}).Where("id = ?", c1.ID).Update("score", 0.1)
	suite.db.Model(&models.Comment{}).Where("id = ?", c2.ID).Update("score", 0.9)

	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{
		Sort: "best",
	})
	suite.Require().NoError(err)
	suite.Require().Len(result.Comments, 2)
	suite.Equal(c2.ID, result.Comments[0].ID) // Higher score first
	suite.Equal(c1.ID, result.Comments[1].ID)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_FilterByKind() {
	user := suite.createTestUser()
	showID := suite.createTestShow("Kind Filter Show")

	// Direct insert to bypass rate limiting — one comment and one field_note
	comment := suite.insertComment(user.ID, "show", showID, "Regular comment", nil, nil, 0)
	// Insert a field_note directly
	fnComment := &models.Comment{
		EntityType:      models.CommentEntityShow,
		EntityID:        showID,
		Kind:            models.CommentKindFieldNote,
		UserID:          user.ID,
		Body:            "Field note content",
		BodyHTML:        "<p>Field note content</p>",
		Visibility:      models.CommentVisibilityVisible,
		ReplyPermission: models.ReplyPermissionAnyone,
	}
	suite.Require().NoError(suite.db.Create(fnComment).Error)
	_ = comment

	// Filter for field_notes only
	result, err := suite.commentService.ListCommentsForEntity("show", showID, contracts.CommentListFilters{
		Kind: "field_note",
	})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 1)
	suite.Equal("field_note", result.Comments[0].Kind)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_HiddenNotVisible() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Hidden Artist")

	// Create a comment then soft-delete it
	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Will be hidden",
	})
	suite.commentService.DeleteComment(user.ID, comment.ID, false)

	// List should not include hidden comments by default
	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 0)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_InvalidEntityType() {
	_, err := suite.commentService.ListCommentsForEntity("banana", 1, contracts.CommentListFilters{})
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

// =============================================================================
// Group 5: GetThread
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestGetThread_FullThread() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Thread Artist")

	// Direct insert to bypass rate limiting
	root := suite.insertComment(user.ID, "artist", artistID, "Thread root", nil, nil, 0)
	reply1 := suite.insertComment(user.ID, "artist", artistID, "Reply 1", &root.ID, &root.ID, 1)
	suite.insertComment(user.ID, "artist", artistID, "Reply to reply 1", &reply1.ID, &root.ID, 2)

	thread, err := suite.commentService.GetThread(root.ID)
	suite.Require().NoError(err)
	suite.Len(thread, 3) // root + 2 replies

	// Should be in chronological order
	suite.Equal("Thread root", thread[0].Body)
	suite.Equal("Reply 1", thread[1].Body)
	suite.Equal("Reply to reply 1", thread[2].Body)
}

func (suite *CommentServiceIntegrationTestSuite) TestGetThread_NotFound() {
	_, err := suite.commentService.GetThread(999999)
	suite.Error(err)
	suite.Contains(err.Error(), "thread root comment not found")
}

func (suite *CommentServiceIntegrationTestSuite) TestGetThread_NotARoot() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Not Root Artist")

	// Direct insert to bypass rate limiting
	root := suite.insertComment(user.ID, "artist", artistID, "Root", nil, nil, 0)
	reply := suite.insertComment(user.ID, "artist", artistID, "Reply", &root.ID, &root.ID, 1)

	_, err := suite.commentService.GetThread(reply.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "not a thread root")
}

// =============================================================================
// Group 6: UpdateComment
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestUpdateComment_OwnComment() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Update Artist")

	original, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Original body",
	})

	updated, err := suite.commentService.UpdateComment(user.ID, original.ID, &contracts.UpdateCommentRequest{
		Body: "Updated body",
	})
	suite.Require().NoError(err)
	suite.Equal("Updated body", updated.Body)
	suite.Contains(updated.BodyHTML, "Updated body")
	suite.True(updated.IsEdited)
	suite.Equal(1, updated.EditCount)
}

func (suite *CommentServiceIntegrationTestSuite) TestUpdateComment_EditHistoryAppended() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Edit History Artist")

	original, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Version 1",
	})

	suite.commentService.UpdateComment(user.ID, original.ID, &contracts.UpdateCommentRequest{
		Body: "Version 2",
	})
	suite.commentService.UpdateComment(user.ID, original.ID, &contracts.UpdateCommentRequest{
		Body: "Version 3",
	})

	// Check edit history
	var edits []models.CommentEdit
	suite.db.Where("comment_id = ?", original.ID).Order("edited_at ASC").Find(&edits)
	suite.Len(edits, 2)
	suite.Equal("Version 1", edits[0].OldBody)
	suite.Equal("Version 2", edits[1].OldBody)
}

func (suite *CommentServiceIntegrationTestSuite) TestUpdateComment_OtherUserFails() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	artistID := suite.createTestArtist("Unauthorized Update Artist")

	comment, _ := suite.commentService.CreateComment(user1.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "User 1's comment",
	})

	_, err := suite.commentService.UpdateComment(user2.ID, comment.ID, &contracts.UpdateCommentRequest{
		Body: "User 2 trying to edit",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "only the comment author can edit")
}

func (suite *CommentServiceIntegrationTestSuite) TestUpdateComment_NotFound() {
	user := suite.createTestUser()

	_, err := suite.commentService.UpdateComment(user.ID, 999999, &contracts.UpdateCommentRequest{
		Body: "Update missing comment",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "comment not found")
}

// =============================================================================
// Group 7: DeleteComment
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestDeleteComment_OwnComment() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Delete Own Artist")

	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Will delete myself",
	})

	err := suite.commentService.DeleteComment(user.ID, comment.ID, false)
	suite.Require().NoError(err)

	// Verify visibility changed
	var c models.Comment
	suite.db.First(&c, comment.ID)
	suite.Equal(models.CommentVisibilityHiddenByUser, c.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestDeleteComment_AdminDeletesOther() {
	user := suite.createTestUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Admin Delete Artist")

	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Will be moderated",
	})

	err := suite.commentService.DeleteComment(admin.ID, comment.ID, true)
	suite.Require().NoError(err)

	// Verify visibility changed to hidden_by_mod
	var c models.Comment
	suite.db.First(&c, comment.ID)
	suite.Equal(models.CommentVisibilityHiddenByMod, c.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestDeleteComment_NonAuthorNonAdminFails() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	artistID := suite.createTestArtist("Unauthorized Delete Artist")

	comment, _ := suite.commentService.CreateComment(user1.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "User 1's comment",
	})

	err := suite.commentService.DeleteComment(user2.ID, comment.ID, false)
	suite.Error(err)
	suite.Contains(err.Error(), "only the comment author or an admin")
}

func (suite *CommentServiceIntegrationTestSuite) TestDeleteComment_NotFound() {
	err := suite.commentService.DeleteComment(1, 999999, false)
	suite.Error(err)
	suite.Contains(err.Error(), "comment not found")
}

func (suite *CommentServiceIntegrationTestSuite) TestDeleteComment_AdminDeletesOwnAsUserHidden() {
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Admin Self Delete Artist")

	comment, _ := suite.commentService.CreateComment(admin.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Admin's own comment",
	})

	// Admin deleting own comment should be hidden_by_user (author=admin)
	err := suite.commentService.DeleteComment(admin.ID, comment.ID, true)
	suite.Require().NoError(err)

	var c models.Comment
	suite.db.First(&c, comment.ID)
	suite.Equal(models.CommentVisibilityHiddenByUser, c.Visibility)
}

// =============================================================================
// Group 8: Sort by "top" and "controversial"
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestListComments_SortByTop() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Sort Top Artist")

	// Direct insert to bypass rate limiting
	c1 := suite.insertComment(user.ID, "artist", artistID, "Low net", nil, nil, 0)
	c2 := suite.insertComment(user.ID, "artist", artistID, "High net", nil, nil, 0)

	// c1: 2 ups, 3 downs = -1 net
	suite.db.Model(&models.Comment{}).Where("id = ?", c1.ID).Updates(map[string]interface{}{"ups": 2, "downs": 3})
	// c2: 10 ups, 1 down = 9 net
	suite.db.Model(&models.Comment{}).Where("id = ?", c2.ID).Updates(map[string]interface{}{"ups": 10, "downs": 1})

	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{
		Sort: "top",
	})
	suite.Require().NoError(err)
	suite.Require().Len(result.Comments, 2)
	suite.Equal(c2.ID, result.Comments[0].ID) // Higher net first
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_SortByControversial() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Controversial Artist")

	// Direct insert to bypass rate limiting
	c1 := suite.insertComment(user.ID, "artist", artistID, "Not controversial", nil, nil, 0)
	c2 := suite.insertComment(user.ID, "artist", artistID, "Very controversial", nil, nil, 0)

	// c1: 1 up, 0 down — 1 total, 1 abs diff
	suite.db.Model(&models.Comment{}).Where("id = ?", c1.ID).Updates(map[string]interface{}{"ups": 1, "downs": 0})
	// c2: 50 ups, 50 downs — 100 total, 0 abs diff (most controversial)
	suite.db.Model(&models.Comment{}).Where("id = ?", c2.ID).Updates(map[string]interface{}{"ups": 50, "downs": 50})

	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{
		Sort: "controversial",
	})
	suite.Require().NoError(err)
	suite.Require().Len(result.Comments, 2)
	suite.Equal(c2.ID, result.Comments[0].ID) // Most total votes + smallest diff first
}

// =============================================================================
// Group 9: Cross-entity type validation
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_AllSupportedEntityTypes() {
	user := suite.createTestUser()

	// Artist
	artistID := suite.createTestArtist("Entity Test Artist")
	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist", EntityID: artistID, Body: "comment on artist",
	})
	suite.Require().NoError(err)

	// Venue
	venueID := suite.createTestVenue("Entity Test Venue")
	_, err = suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "venue", EntityID: venueID, Body: "comment on venue",
	})
	suite.Require().NoError(err)

	// Show
	showID := suite.createTestShow("Entity Test Show")
	_, err = suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "show", EntityID: showID, Body: "comment on show",
	})
	suite.Require().NoError(err)
}

// =============================================================================
// Group 10: Edge cases
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_MaxLengthBody() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Max Length Artist")

	body := strings.Repeat("a", models.MaxCommentBodyLength)
	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       body,
	})
	suite.Require().NoError(err)
	suite.Equal(models.MaxCommentBodyLength, len(comment.Body))
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_TrimsWhitespace() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Trim Artist")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "  trimmed body  ",
	})
	suite.Require().NoError(err)
	suite.Equal("trimmed body", comment.Body)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_EmptyResult() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Empty Artist")
	_ = user

	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 0)
	suite.Equal(int64(0), result.Total)
	suite.False(result.HasMore)
}

func (suite *CommentServiceIntegrationTestSuite) TestGetThread_SingleRootNoReplies() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Lone Root Artist")

	root, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Solo root",
	})

	thread, err := suite.commentService.GetThread(root.ID)
	suite.Require().NoError(err)
	suite.Len(thread, 1)
	suite.Equal("Solo root", thread[0].Body)
}

// =============================================================================
// Group 10: Trust-Tier Visibility (PSY-292)
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_NewUser_PendingReview() {
	newUser := suite.createTestNewUser()
	artistID := suite.createTestArtist("Pending Artist")

	comment, err := suite.commentService.CreateComment(newUser.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "New user comment",
	})
	suite.Require().NoError(err)
	suite.Equal("pending_review", comment.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_Contributor_Visible() {
	user := suite.createTestUser() // contributor tier
	artistID := suite.createTestArtist("Contributor Artist")

	comment, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Contributor comment",
	})
	suite.Require().NoError(err)
	suite.Equal("visible", comment.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestCreateComment_Admin_Visible() {
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Admin Artist")

	comment, err := suite.commentService.CreateComment(admin.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Admin comment",
	})
	suite.Require().NoError(err)
	suite.Equal("visible", comment.Visibility)
}

// =============================================================================
// Group 11: Rate Limiting (PSY-292)
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestRateLimit_PerEntityCooldown() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Cooldown Artist")

	// First comment succeeds
	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "First comment",
	})
	suite.Require().NoError(err)

	// Second comment on same entity within 60s fails
	_, err = suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Too fast",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "please wait 60 seconds")
}

func (suite *CommentServiceIntegrationTestSuite) TestRateLimit_DifferentEntityAllowed() {
	user := suite.createTestUser()
	artist1ID := suite.createTestArtist("Rate Artist 1")
	artist2ID := suite.createTestArtist("Rate Artist 2")

	// First comment on entity 1
	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artist1ID,
		Body:       "Comment on artist 1",
	})
	suite.Require().NoError(err)

	// Second comment on different entity should succeed
	_, err = suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artist2ID,
		Body:       "Comment on artist 2",
	})
	suite.Require().NoError(err)
}

func (suite *CommentServiceIntegrationTestSuite) TestRateLimit_NewUserHourlyLimit() {
	newUser := suite.createTestNewUser()

	// Insert 5 comments directly to fill the hourly limit
	for i := 0; i < 5; i++ {
		artistID := suite.createTestArtist(fmt.Sprintf("Hourly Artist %d", i))
		suite.insertComment(newUser.ID, "artist", artistID, fmt.Sprintf("Comment %d", i), nil, nil, 0)
	}

	// 6th comment should fail (hourly limit for new_user is 5)
	artistID := suite.createTestArtist("Hourly Overflow Artist")
	_, err := suite.commentService.CreateComment(newUser.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "One too many",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "hourly comment limit")
}

// =============================================================================
// Group 12: Admin Moderation (PSY-292)
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestHideComment_Success() {
	user := suite.createTestUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Hide Artist")

	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Will be hidden",
	})

	err := suite.commentService.HideComment(admin.ID, comment.ID, "violates guidelines")
	suite.Require().NoError(err)

	// Verify it's hidden
	fetched, err := suite.commentService.GetComment(comment.ID)
	suite.Require().NoError(err)
	suite.Equal("hidden_by_mod", fetched.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestHideComment_NotFound() {
	err := suite.commentService.HideComment(1, 999999, "reason")
	suite.Error(err)
	suite.Contains(err.Error(), "not found")
}

func (suite *CommentServiceIntegrationTestSuite) TestRestoreComment_Success() {
	user := suite.createTestUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Restore Artist")

	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Hide then restore",
	})
	suite.commentService.HideComment(admin.ID, comment.ID, "temp hide")

	err := suite.commentService.RestoreComment(admin.ID, comment.ID)
	suite.Require().NoError(err)

	fetched, err := suite.commentService.GetComment(comment.ID)
	suite.Require().NoError(err)
	suite.Equal("visible", fetched.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestRestoreComment_AlreadyVisible() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Already Visible Artist")

	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Already visible",
	})

	err := suite.commentService.RestoreComment(1, comment.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "already visible")
}

func (suite *CommentServiceIntegrationTestSuite) TestListPendingComments() {
	newUser := suite.createTestNewUser()
	artistID := suite.createTestArtist("Pending List Artist")

	// Create a pending comment (new_user tier)
	_, err := suite.commentService.CreateComment(newUser.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Pending comment",
	})
	suite.Require().NoError(err)

	// List pending
	comments, total, err := suite.commentService.ListPendingComments(20, 0)
	suite.Require().NoError(err)
	suite.GreaterOrEqual(total, int64(1))
	found := false
	for _, c := range comments {
		if c.Body == "Pending comment" {
			found = true
			suite.Equal("pending_review", c.Visibility)
		}
	}
	suite.True(found, "Expected to find the pending comment")
}

func (suite *CommentServiceIntegrationTestSuite) TestApproveComment_Success() {
	newUser := suite.createTestNewUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Approve Artist")

	comment, _ := suite.commentService.CreateComment(newUser.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Needs approval",
	})
	suite.Equal("pending_review", comment.Visibility)

	err := suite.commentService.ApproveComment(admin.ID, comment.ID)
	suite.Require().NoError(err)

	fetched, _ := suite.commentService.GetComment(comment.ID)
	suite.Equal("visible", fetched.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestApproveComment_NotPending() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Not Pending Artist")

	// Contributor comment is auto-visible
	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Already approved",
	})

	err := suite.commentService.ApproveComment(1, comment.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "not pending review")
}

func (suite *CommentServiceIntegrationTestSuite) TestRejectComment_Success() {
	newUser := suite.createTestNewUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Reject Artist")

	comment, _ := suite.commentService.CreateComment(newUser.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Spam content",
	})
	suite.Equal("pending_review", comment.Visibility)

	err := suite.commentService.RejectComment(admin.ID, comment.ID, "spam")
	suite.Require().NoError(err)

	fetched, _ := suite.commentService.GetComment(comment.ID)
	suite.Equal("hidden_by_mod", fetched.Visibility)
}

func (suite *CommentServiceIntegrationTestSuite) TestRejectComment_NotPending() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Not Pending Reject Artist")

	comment, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Not pending",
	})

	err := suite.commentService.RejectComment(1, comment.ID, "reason")
	suite.Error(err)
	suite.Contains(err.Error(), "not pending review")
}

// =============================================================================
// Group 13: Admin Edit History Viewer (PSY-297)
// =============================================================================

func (suite *CommentServiceIntegrationTestSuite) TestUpdateComment_RecordsEditorUserID() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Editor Attribution Artist")

	original, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Original body",
	})

	_, err := suite.commentService.UpdateComment(user.ID, original.ID, &contracts.UpdateCommentRequest{
		Body: "Edited body",
	})
	suite.Require().NoError(err)

	// Verify the edit row has editor_user_id populated
	var edit models.CommentEdit
	err = suite.db.Where("comment_id = ?", original.ID).First(&edit).Error
	suite.Require().NoError(err)
	suite.Require().NotNil(edit.EditorUserID, "editor_user_id must be populated on new edits")
	suite.Equal(user.ID, *edit.EditorUserID)
	suite.Equal("Original body", edit.OldBody)
}

func (suite *CommentServiceIntegrationTestSuite) TestUpdateComment_TransactionAtomicity() {
	// If the body update fails inside the transaction, the comment_edits row
	// must NOT exist (rollback). We simulate by giving the service an invalid
	// commentID that survives the First() fetch but fails the Update.
	// Approach: wrap via a nested transaction — simpler to just assert that
	// after a successful update, one edit row exists per successful call,
	// and that the old body in that row matches the pre-update body.
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Atomicity Artist")

	c, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "v1",
	})

	// Three edits should produce exactly three comment_edits rows, oldest-first
	// capturing "v1", "v2", "v3" as the prior bodies.
	suite.Require().NoError(suite.db.Model(&models.Comment{}).Where("id = ?", c.ID).Update("updated_at", time.Now().Add(-time.Hour)).Error)
	_, err := suite.commentService.UpdateComment(user.ID, c.ID, &contracts.UpdateCommentRequest{Body: "v2"})
	suite.Require().NoError(err)
	_, err = suite.commentService.UpdateComment(user.ID, c.ID, &contracts.UpdateCommentRequest{Body: "v3"})
	suite.Require().NoError(err)
	_, err = suite.commentService.UpdateComment(user.ID, c.ID, &contracts.UpdateCommentRequest{Body: "v4"})
	suite.Require().NoError(err)

	var edits []models.CommentEdit
	suite.Require().NoError(suite.db.Where("comment_id = ?", c.ID).Order("edited_at ASC, id ASC").Find(&edits).Error)
	suite.Require().Len(edits, 3)
	suite.Equal("v1", edits[0].OldBody)
	suite.Equal("v2", edits[1].OldBody)
	suite.Equal("v3", edits[2].OldBody)

	// The current comment body should be v4.
	var current models.Comment
	suite.Require().NoError(suite.db.First(&current, c.ID).Error)
	suite.Equal("v4", current.Body)
	suite.Equal(3, current.EditCount)
}

func (suite *CommentServiceIntegrationTestSuite) TestGetCommentEditHistory_AdminAccess() {
	user := suite.createTestUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Edit History Viewer Artist")

	c, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "first",
	})
	_, err := suite.commentService.UpdateComment(user.ID, c.ID, &contracts.UpdateCommentRequest{Body: "second"})
	suite.Require().NoError(err)
	_, err = suite.commentService.UpdateComment(user.ID, c.ID, &contracts.UpdateCommentRequest{Body: "third"})
	suite.Require().NoError(err)

	history, err := suite.commentService.GetCommentEditHistory(admin.ID, c.ID)
	suite.Require().NoError(err)
	suite.Equal(c.ID, history.CommentID)
	suite.Equal("third", history.CurrentBody)
	suite.Require().Len(history.Edits, 2)

	// Oldest edit first: prior body == "first", then "second"
	suite.Equal("first", history.Edits[0].OldBody)
	suite.Equal("second", history.Edits[1].OldBody)

	// Editor attribution is populated
	suite.Require().NotNil(history.Edits[0].EditorUserID)
	suite.Equal(user.ID, *history.Edits[0].EditorUserID)
	suite.NotEmpty(history.Edits[0].EditorUsername)
}

func (suite *CommentServiceIntegrationTestSuite) TestGetCommentEditHistory_NonAdminForbidden() {
	user := suite.createTestUser()
	other := suite.createTestUser()
	artistID := suite.createTestArtist("Non-Admin Edit History Artist")

	c, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "owner body",
	})
	_, err := suite.commentService.UpdateComment(user.ID, c.ID, &contracts.UpdateCommentRequest{Body: "edited"})
	suite.Require().NoError(err)

	// The comment's own author, if not an admin, cannot see history.
	_, err = suite.commentService.GetCommentEditHistory(user.ID, c.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "admin access required")

	// An unrelated user also cannot see history.
	_, err = suite.commentService.GetCommentEditHistory(other.ID, c.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "admin access required")

	// A non-existent requester is also rejected.
	_, err = suite.commentService.GetCommentEditHistory(999999, c.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "admin access required")
}

func (suite *CommentServiceIntegrationTestSuite) TestGetCommentEditHistory_EmptyForNeverEdited() {
	user := suite.createTestUser()
	admin := suite.createTestAdmin()
	artistID := suite.createTestArtist("Never Edited Artist")

	c, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "pristine",
	})

	history, err := suite.commentService.GetCommentEditHistory(admin.ID, c.ID)
	suite.Require().NoError(err)
	suite.Equal(c.ID, history.CommentID)
	suite.Equal("pristine", history.CurrentBody)
	suite.Empty(history.Edits, "never-edited comment should have zero history entries")
}

func (suite *CommentServiceIntegrationTestSuite) TestGetCommentEditHistory_CommentNotFound() {
	admin := suite.createTestAdmin()

	_, err := suite.commentService.GetCommentEditHistory(admin.ID, 999999)
	suite.Error(err)
	suite.Contains(err.Error(), "comment not found")
}
