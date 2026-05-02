package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type CommentVoteServiceIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *CommentVoteService
}

func (suite *CommentVoteServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.service = NewCommentVoteService(suite.testDB.DB)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CommentVoteServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM comment_votes")
	_, _ = sqlDB.Exec("DELETE FROM comments")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCommentVoteServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CommentVoteServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) createTestUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *CommentVoteServiceIntegrationTestSuite) createTestComment(userID uint) *engagementm.Comment {
	comment := &engagementm.Comment{
		Kind:       engagementm.CommentKindComment,
		EntityType: "show",
		EntityID:   1,
		UserID:     userID,
		Body:       "Test comment",
		Visibility: engagementm.CommentVisibilityVisible,
	}
	err := suite.db.Create(comment).Error
	suite.Require().NoError(err)
	return comment
}

// =============================================================================
// VOTE TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestVoteUp() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	err := suite.service.Vote(user.ID, comment.ID, 1)
	suite.NoError(err)

	// Verify vote was created
	vote, err := suite.service.GetUserVote(user.ID, comment.ID)
	suite.NoError(err)
	suite.NotNil(vote)
	suite.Equal(1, *vote)

	// Verify aggregates updated
	var updated engagementm.Comment
	suite.db.First(&updated, comment.ID)
	suite.Equal(1, updated.Ups)
	suite.Equal(0, updated.Downs)
	suite.Greater(updated.Score, 0.0)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestVoteDown() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	err := suite.service.Vote(user.ID, comment.ID, -1)
	suite.NoError(err)

	vote, err := suite.service.GetUserVote(user.ID, comment.ID)
	suite.NoError(err)
	suite.NotNil(vote)
	suite.Equal(-1, *vote)

	var updated engagementm.Comment
	suite.db.First(&updated, comment.ID)
	suite.Equal(0, updated.Ups)
	suite.Equal(1, updated.Downs)
	// Wilson score for 0 ups / 1 down is effectively 0 (may have floating-point epsilon)
	suite.InDelta(0.0, updated.Score, 1e-10)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestChangeVoteDirection() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	// Vote up first
	err := suite.service.Vote(user.ID, comment.ID, 1)
	suite.NoError(err)

	var afterUp engagementm.Comment
	suite.db.First(&afterUp, comment.ID)
	suite.Equal(1, afterUp.Ups)
	suite.Equal(0, afterUp.Downs)

	// Change to down
	err = suite.service.Vote(user.ID, comment.ID, -1)
	suite.NoError(err)

	vote, err := suite.service.GetUserVote(user.ID, comment.ID)
	suite.NoError(err)
	suite.NotNil(vote)
	suite.Equal(-1, *vote)

	var afterDown engagementm.Comment
	suite.db.First(&afterDown, comment.ID)
	suite.Equal(0, afterDown.Ups)
	suite.Equal(1, afterDown.Downs)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestVoteSameDirectionIdempotent() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	// Vote up twice
	err := suite.service.Vote(user.ID, comment.ID, 1)
	suite.NoError(err)
	err = suite.service.Vote(user.ID, comment.ID, 1)
	suite.NoError(err)

	var updated engagementm.Comment
	suite.db.First(&updated, comment.ID)
	suite.Equal(1, updated.Ups)
	suite.Equal(0, updated.Downs)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestVoteInvalidDirection() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	err := suite.service.Vote(user.ID, comment.ID, 2)
	suite.Error(err)
	suite.Contains(err.Error(), "invalid vote direction")
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestVoteNonexistentComment() {
	user := suite.createTestUser()

	err := suite.service.Vote(user.ID, 99999, 1)
	suite.Error(err)
	suite.Contains(err.Error(), "comment not found")
}

// =============================================================================
// UNVOTE TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestUnvote() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	// Vote first
	err := suite.service.Vote(user.ID, comment.ID, 1)
	suite.NoError(err)

	// Verify aggregates
	var afterVote engagementm.Comment
	suite.db.First(&afterVote, comment.ID)
	suite.Equal(1, afterVote.Ups)

	// Unvote
	err = suite.service.Unvote(user.ID, comment.ID)
	suite.NoError(err)

	// Vote should be nil
	vote, err := suite.service.GetUserVote(user.ID, comment.ID)
	suite.NoError(err)
	suite.Nil(vote)

	// Aggregates should be zero
	var afterUnvote engagementm.Comment
	suite.db.First(&afterUnvote, comment.ID)
	suite.Equal(0, afterUnvote.Ups)
	suite.Equal(0, afterUnvote.Downs)
	suite.Equal(0.0, afterUnvote.Score)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestUnvoteNonexistentComment() {
	user := suite.createTestUser()

	err := suite.service.Unvote(user.ID, 99999)
	suite.Error(err)
	suite.Contains(err.Error(), "comment not found")
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestUnvoteWithoutExistingVote() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	// Unvote without having voted — should succeed (no-op delete)
	err := suite.service.Unvote(user.ID, comment.ID)
	suite.NoError(err)
}

// =============================================================================
// GET USER VOTE TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestGetUserVoteNoVote() {
	user := suite.createTestUser()
	comment := suite.createTestComment(user.ID)

	vote, err := suite.service.GetUserVote(user.ID, comment.ID)
	suite.NoError(err)
	suite.Nil(vote)
}

// =============================================================================
// BATCH GET USER VOTES TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestGetUserVotesForComments() {
	user := suite.createTestUser()
	c1 := suite.createTestComment(user.ID)
	c2 := suite.createTestComment(user.ID)
	c3 := suite.createTestComment(user.ID)

	// Vote on c1 (up) and c2 (down), skip c3
	suite.NoError(suite.service.Vote(user.ID, c1.ID, 1))
	suite.NoError(suite.service.Vote(user.ID, c2.ID, -1))

	votes, err := suite.service.GetUserVotesForComments(user.ID, []uint{c1.ID, c2.ID, c3.ID})
	suite.NoError(err)
	suite.Equal(1, votes[c1.ID])
	suite.Equal(-1, votes[c2.ID])
	_, hasC3 := votes[c3.ID]
	suite.False(hasC3)
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestGetUserVotesForCommentsEmptyList() {
	user := suite.createTestUser()

	votes, err := suite.service.GetUserVotesForComments(user.ID, []uint{})
	suite.NoError(err)
	suite.NotNil(votes)
	suite.Len(votes, 0)
}

// =============================================================================
// GET COMMENT VOTE COUNTS TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestGetCommentVoteCounts() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	comment := suite.createTestComment(user1.ID)

	// Two upvotes
	suite.NoError(suite.service.Vote(user1.ID, comment.ID, 1))
	suite.NoError(suite.service.Vote(user2.ID, comment.ID, 1))

	ups, downs, score, err := suite.service.GetCommentVoteCounts(comment.ID)
	suite.NoError(err)
	suite.Equal(2, ups)
	suite.Equal(0, downs)
	suite.Greater(score, 0.0)
}

// =============================================================================
// WILSON SCORE INTEGRATION TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestWilsonScoreComputedCorrectly() {
	creator := suite.createTestUser()
	comment := suite.createTestComment(creator.ID)

	// Create multiple users and vote
	for i := 0; i < 8; i++ {
		u := suite.createTestUser()
		suite.NoError(suite.service.Vote(u.ID, comment.ID, 1))
	}
	for i := 0; i < 2; i++ {
		u := suite.createTestUser()
		suite.NoError(suite.service.Vote(u.ID, comment.ID, -1))
	}

	var updated engagementm.Comment
	suite.db.First(&updated, comment.ID)
	suite.Equal(8, updated.Ups)
	suite.Equal(2, updated.Downs)
	// Wilson score should be between 0.5 and 1.0 for 8 up / 2 down
	suite.Greater(updated.Score, 0.5)
	suite.Less(updated.Score, 1.0)
}

// =============================================================================
// NIL DB TESTS
// =============================================================================

func (suite *CommentVoteServiceIntegrationTestSuite) TestNilDBVote() {
	svc := &CommentVoteService{db: nil}
	err := svc.Vote(1, 1, 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestNilDBUnvote() {
	svc := &CommentVoteService{db: nil}
	err := svc.Unvote(1, 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestNilDBGetUserVote() {
	svc := &CommentVoteService{db: nil}
	_, err := svc.GetUserVote(1, 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestNilDBGetUserVotesForComments() {
	svc := &CommentVoteService{db: nil}
	_, err := svc.GetUserVotesForComments(1, []uint{1, 2, 3})
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentVoteServiceIntegrationTestSuite) TestNilDBGetCommentVoteCounts() {
	svc := &CommentVoteService{db: nil}
	_, _, _, err := svc.GetCommentVoteCounts(1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}
