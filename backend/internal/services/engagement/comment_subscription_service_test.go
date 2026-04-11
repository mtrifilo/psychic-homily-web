package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type CommentSubscriptionServiceIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *CommentSubscriptionService
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.service = NewCommentSubscriptionService(suite.testDB.DB)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM comment_last_read")
	_, _ = sqlDB.Exec("DELETE FROM comment_subscriptions")
	_, _ = sqlDB.Exec("DELETE FROM comments")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCommentSubscriptionServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CommentSubscriptionServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
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

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestComment(userID uint, entityType string, entityID uint) *models.Comment {
	comment := &models.Comment{
		Kind:       models.CommentKindComment,
		EntityType: models.CommentEntityType(entityType),
		EntityID:   entityID,
		UserID:     userID,
		Body:       "Test comment",
		BodyHTML:   "<p>Test comment</p>",
		Visibility: models.CommentVisibilityVisible,
	}
	err := suite.db.Create(comment).Error
	suite.Require().NoError(err)
	return comment
}

// =============================================================================
// SUBSCRIBE TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestSubscribeSuccess() {
	user := suite.createTestUser()

	err := suite.service.Subscribe(user.ID, "show", 1)
	suite.NoError(err)

	// Verify subscription exists
	subscribed, err := suite.service.IsSubscribed(user.ID, "show", 1)
	suite.NoError(err)
	suite.True(subscribed)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestSubscribeIdempotent() {
	user := suite.createTestUser()

	// Subscribe twice — should not error
	err := suite.service.Subscribe(user.ID, "show", 1)
	suite.NoError(err)
	err = suite.service.Subscribe(user.ID, "show", 1)
	suite.NoError(err)

	subscribed, err := suite.service.IsSubscribed(user.ID, "show", 1)
	suite.NoError(err)
	suite.True(subscribed)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestSubscribeInvalidEntityType() {
	user := suite.createTestUser()

	err := suite.service.Subscribe(user.ID, "invalid_type", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestSubscribeMultipleEntities() {
	user := suite.createTestUser()

	err := suite.service.Subscribe(user.ID, "show", 1)
	suite.NoError(err)
	err = suite.service.Subscribe(user.ID, "artist", 2)
	suite.NoError(err)

	sub1, err := suite.service.IsSubscribed(user.ID, "show", 1)
	suite.NoError(err)
	suite.True(sub1)

	sub2, err := suite.service.IsSubscribed(user.ID, "artist", 2)
	suite.NoError(err)
	suite.True(sub2)
}

// =============================================================================
// UNSUBSCRIBE TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestUnsubscribeSuccess() {
	user := suite.createTestUser()

	err := suite.service.Subscribe(user.ID, "show", 1)
	suite.NoError(err)

	err = suite.service.Unsubscribe(user.ID, "show", 1)
	suite.NoError(err)

	subscribed, err := suite.service.IsSubscribed(user.ID, "show", 1)
	suite.NoError(err)
	suite.False(subscribed)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestUnsubscribeIdempotent() {
	user := suite.createTestUser()

	// Unsubscribe without subscribing — should not error
	err := suite.service.Unsubscribe(user.ID, "show", 1)
	suite.NoError(err)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestUnsubscribeInvalidEntityType() {
	user := suite.createTestUser()

	err := suite.service.Unsubscribe(user.ID, "invalid_type", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

// =============================================================================
// IS_SUBSCRIBED TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestIsSubscribedFalseWhenNotSubscribed() {
	user := suite.createTestUser()

	subscribed, err := suite.service.IsSubscribed(user.ID, "show", 1)
	suite.NoError(err)
	suite.False(subscribed)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestIsSubscribedInvalidEntityType() {
	user := suite.createTestUser()

	_, err := suite.service.IsSubscribed(user.ID, "invalid_type", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

// =============================================================================
// MARK READ TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestMarkReadUpdatesLastReadPointer() {
	user := suite.createTestUser()
	c1 := suite.createTestComment(user.ID, "show", 1)

	err := suite.service.MarkRead(user.ID, "show", 1)
	suite.NoError(err)

	// Unread count should be 0 since we just marked read
	count, err := suite.service.GetUnreadCount(user.ID, "show", 1)
	suite.NoError(err)
	suite.Equal(0, count)

	// Add another comment — unread count should be 1
	_ = c1 // use c1 to suppress lint
	suite.createTestComment(user.ID, "show", 1)

	count, err = suite.service.GetUnreadCount(user.ID, "show", 1)
	suite.NoError(err)
	suite.Equal(1, count)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestMarkReadWithNoComments() {
	user := suite.createTestUser()

	// MarkRead on entity with no comments — should succeed with last_read_comment_id = 0
	err := suite.service.MarkRead(user.ID, "show", 1)
	suite.NoError(err)

	count, err := suite.service.GetUnreadCount(user.ID, "show", 1)
	suite.NoError(err)
	suite.Equal(0, count)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestMarkReadInvalidEntityType() {
	user := suite.createTestUser()

	err := suite.service.MarkRead(user.ID, "invalid_type", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestMarkReadIsIdempotent() {
	user := suite.createTestUser()
	suite.createTestComment(user.ID, "show", 1)

	err := suite.service.MarkRead(user.ID, "show", 1)
	suite.NoError(err)
	err = suite.service.MarkRead(user.ID, "show", 1)
	suite.NoError(err)

	count, err := suite.service.GetUnreadCount(user.ID, "show", 1)
	suite.NoError(err)
	suite.Equal(0, count)
}

// =============================================================================
// GET UNREAD COUNT TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetUnreadCountWithNoLastRead() {
	user := suite.createTestUser()
	suite.createTestComment(user.ID, "show", 1)
	suite.createTestComment(user.ID, "show", 1)

	// Without marking read, all visible comments should be unread
	count, err := suite.service.GetUnreadCount(user.ID, "show", 1)
	suite.NoError(err)
	suite.Equal(2, count)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetUnreadCountExcludesHiddenComments() {
	user := suite.createTestUser()
	suite.createTestComment(user.ID, "show", 1) // visible

	// Create a hidden comment
	hidden := &models.Comment{
		Kind:       models.CommentKindComment,
		EntityType: "show",
		EntityID:   1,
		UserID:     user.ID,
		Body:       "Hidden comment",
		BodyHTML:   "<p>Hidden comment</p>",
		Visibility: models.CommentVisibilityHiddenByMod,
	}
	suite.Require().NoError(suite.db.Create(hidden).Error)

	count, err := suite.service.GetUnreadCount(user.ID, "show", 1)
	suite.NoError(err)
	suite.Equal(1, count) // only visible comment counted
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetUnreadCountInvalidEntityType() {
	user := suite.createTestUser()

	_, err := suite.service.GetUnreadCount(user.ID, "invalid_type", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetUnreadCountZeroForEmptyEntity() {
	user := suite.createTestUser()

	count, err := suite.service.GetUnreadCount(user.ID, "show", 999)
	suite.NoError(err)
	suite.Equal(0, count)
}

// =============================================================================
// GET SUBSCRIPTIONS FOR USER TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscriptionsForUserWithResults() {
	user := suite.createTestUser()

	suite.NoError(suite.service.Subscribe(user.ID, "show", 1))
	suite.NoError(suite.service.Subscribe(user.ID, "artist", 2))

	subs, total, err := suite.service.GetSubscriptionsForUser(user.ID, 20, 0)
	suite.NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(subs, 2)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscriptionsForUserEmpty() {
	user := suite.createTestUser()

	subs, total, err := suite.service.GetSubscriptionsForUser(user.ID, 20, 0)
	suite.NoError(err)
	suite.Equal(int64(0), total)
	suite.Len(subs, 0)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscriptionsForUserPagination() {
	user := suite.createTestUser()

	for i := 1; i <= 5; i++ {
		suite.NoError(suite.service.Subscribe(user.ID, "show", uint(i)))
	}

	// First page
	subs, total, err := suite.service.GetSubscriptionsForUser(user.ID, 2, 0)
	suite.NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(subs, 2)

	// Second page
	subs2, _, err := suite.service.GetSubscriptionsForUser(user.ID, 2, 2)
	suite.NoError(err)
	suite.Len(subs2, 2)

	// Third page (only 1 remaining)
	subs3, _, err := suite.service.GetSubscriptionsForUser(user.ID, 2, 4)
	suite.NoError(err)
	suite.Len(subs3, 1)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscriptionsForUserIncludesUnreadCount() {
	user := suite.createTestUser()

	suite.NoError(suite.service.Subscribe(user.ID, "show", 1))
	suite.createTestComment(user.ID, "show", 1)
	suite.createTestComment(user.ID, "show", 1)

	subs, _, err := suite.service.GetSubscriptionsForUser(user.ID, 20, 0)
	suite.NoError(err)
	suite.Len(subs, 1)
	suite.Equal(2, subs[0].UnreadCount)
}

// =============================================================================
// GET SUBSCRIBERS FOR ENTITY TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscribersForEntityMultipleUsers() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	user3 := suite.createTestUser()

	suite.NoError(suite.service.Subscribe(user1.ID, "show", 1))
	suite.NoError(suite.service.Subscribe(user2.ID, "show", 1))
	// user3 subscribes to different entity
	suite.NoError(suite.service.Subscribe(user3.ID, "show", 2))

	subs, err := suite.service.GetSubscribersForEntity("show", 1)
	suite.NoError(err)
	suite.Len(subs, 2)
	suite.Contains(subs, user1.ID)
	suite.Contains(subs, user2.ID)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscribersForEntityNone() {
	subs, err := suite.service.GetSubscribersForEntity("show", 999)
	suite.NoError(err)
	suite.Len(subs, 0)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestGetSubscribersForEntityInvalidType() {
	_, err := suite.service.GetSubscribersForEntity("invalid_type", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "unsupported entity type")
}

// =============================================================================
// ALL ENTITY TYPES TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestSubscribeAllEntityTypes() {
	user := suite.createTestUser()

	entityTypes := []string{"artist", "venue", "show", "release", "label", "festival", "collection"}
	for _, et := range entityTypes {
		err := suite.service.Subscribe(user.ID, et, 1)
		suite.NoError(err, "Subscribe should work for entity type: %s", et)

		subscribed, err := suite.service.IsSubscribed(user.ID, et, 1)
		suite.NoError(err)
		suite.True(subscribed, "Should be subscribed to %s", et)
	}
}

// =============================================================================
// NIL DB TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBSubscribe() {
	svc := &CommentSubscriptionService{db: nil}
	err := svc.Subscribe(1, "show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBUnsubscribe() {
	svc := &CommentSubscriptionService{db: nil}
	err := svc.Unsubscribe(1, "show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBIsSubscribed() {
	svc := &CommentSubscriptionService{db: nil}
	_, err := svc.IsSubscribed(1, "show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBMarkRead() {
	svc := &CommentSubscriptionService{db: nil}
	err := svc.MarkRead(1, "show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBGetUnreadCount() {
	svc := &CommentSubscriptionService{db: nil}
	_, err := svc.GetUnreadCount(1, "show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBGetSubscriptionsForUser() {
	svc := &CommentSubscriptionService{db: nil}
	_, _, err := svc.GetSubscriptionsForUser(1, 20, 0)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBGetSubscribersForEntity() {
	svc := &CommentSubscriptionService{db: nil}
	_, err := svc.GetSubscribersForEntity("show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}
