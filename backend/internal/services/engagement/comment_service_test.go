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
	artistID := suite.createTestArtist("Reply Artist")

	// Create top-level comment
	root, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Root comment",
	})
	suite.Require().NoError(err)

	// Create reply
	reply, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
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
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Nested Reply Artist")

	// Depth 0: root
	root, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Depth 0",
	})
	suite.Require().NoError(err)

	// Depth 1: reply to root
	reply1, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Depth 1",
		ParentID:   &root.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(1, reply1.Depth)

	// Depth 2: reply to reply (max allowed)
	reply2, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
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

	// Create chain: depth 0 → 1 → 2
	root, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Root",
	})
	reply1, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply 1",
		ParentID:   &root.ID,
	})
	reply2, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply 2",
		ParentID:   &reply1.ID,
	})

	// Attempt depth 3 — should fail
	_, err := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
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

	// Create 5 comments
	for i := 0; i < 5; i++ {
		suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
			EntityType: "artist",
			EntityID:   artistID,
			Body:       fmt.Sprintf("Comment %d", i),
		})
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

	// Create a root and a reply
	root, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Root comment",
	})
	suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply comment",
		ParentID:   &root.ID,
	})

	// List should only return top-level
	result, err := suite.commentService.ListCommentsForEntity("artist", artistID, contracts.CommentListFilters{})
	suite.Require().NoError(err)
	suite.Len(result.Comments, 1)
	suite.Equal("Root comment", result.Comments[0].Body)
}

func (suite *CommentServiceIntegrationTestSuite) TestListComments_SortByNew() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Sort New Artist")

	c1, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "First comment",
	})
	time.Sleep(10 * time.Millisecond)
	c2, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Second comment",
	})

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

	// Create two comments, manually set different scores
	c1, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Low score",
	})
	c2, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "High score",
	})

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

	// Create one comment and one field_note
	suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "show",
		EntityID:   showID,
		Body:       "Regular comment",
		Kind:       "comment",
	})
	suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "show",
		EntityID:   showID,
		Body:       "Field note content",
		Kind:       "field_note",
	})

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

	root, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Thread root",
	})
	reply1, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply 1",
		ParentID:   &root.ID,
	})
	suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply to reply 1",
		ParentID:   &reply1.ID,
	})

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

	root, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Root",
	})
	reply, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Reply",
		ParentID:   &root.ID,
	})

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

	c1, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Low net",
	})
	c2, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "High net",
	})

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

	c1, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Not controversial",
	})
	c2, _ := suite.commentService.CreateComment(user.ID, &contracts.CreateCommentRequest{
		EntityType: "artist",
		EntityID:   artistID,
		Body:       "Very controversial",
	})

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
