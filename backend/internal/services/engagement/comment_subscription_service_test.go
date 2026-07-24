package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
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
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestCommentSubscriptionServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CommentSubscriptionServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestUser() *authm.User {
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

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestComment(userID uint, entityType string, entityID uint) *engagementm.Comment {
	comment := &engagementm.Comment{
		Kind:       engagementm.CommentKindComment,
		EntityType: engagementm.CommentEntityType(entityType),
		EntityID:   entityID,
		UserID:     userID,
		Body:       "Test comment",
		BodyHTML:   "<p>Test comment</p>",
		Visibility: engagementm.CommentVisibilityVisible,
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
	hidden := &engagementm.Comment{
		Kind:       engagementm.CommentKindComment,
		EntityType: "show",
		EntityID:   1,
		UserID:     user.ID,
		Body:       "Hidden comment",
		BodyHTML:   "<p>Hidden comment</p>",
		Visibility: engagementm.CommentVisibilityHiddenByMod,
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
// LIST WATCHING TESTS
// =============================================================================

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestVenue(name string) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	venue := &catalogm.Venue{Name: name, Slug: &slug, City: "Phoenix", State: "AZ"}
	suite.Require().NoError(suite.db.Create(venue).Error)
	return venue
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestCommentAt(userID uint, entityType string, entityID uint, createdAt time.Time) *engagementm.Comment {
	comment := &engagementm.Comment{
		Kind:       engagementm.CommentKindComment,
		EntityType: engagementm.CommentEntityType(entityType),
		EntityID:   entityID,
		UserID:     userID,
		Body:       "Test comment",
		BodyHTML:   "<p>Test comment</p>",
		Visibility: engagementm.CommentVisibilityVisible,
		CreatedAt:  createdAt,
	}
	suite.Require().NoError(suite.db.Create(comment).Error)
	return comment
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingEmpty() {
	user := suite.createTestUser()

	items, total, err := suite.service.ListWatching(user.ID, 20, 0)
	suite.NoError(err)
	suite.Equal(int64(0), total)
	suite.Len(items, 0)
}

// TestListWatchingEnrichesEntityContextAcrossTypes covers the batched
// multi-entity-type resolution: artist + venue rows resolve to names,
// slugs, and slug URLs; a subscription whose entity row is missing
// falls back to "<type> #<id>" + ID URL.
func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingEnrichesEntityContextAcrossTypes() {
	user := suite.createTestUser()
	commenter := suite.createTestUser()
	displayName := "DJ Spectre"
	suite.Require().NoError(suite.db.Model(commenter).Update("display_name", displayName).Error)

	artist := suite.createTestArtist("Watch Artist")
	venue := suite.createTestVenue("Watch Venue")

	suite.NoError(suite.service.Subscribe(user.ID, "artist", artist.ID))
	suite.NoError(suite.service.Subscribe(user.ID, "venue", venue.ID))
	suite.NoError(suite.service.Subscribe(user.ID, "show", 999)) // no show row

	base := time.Now().UTC().Add(-time.Hour)
	suite.createTestCommentAt(commenter.ID, "venue", venue.ID, base)
	suite.createTestCommentAt(commenter.ID, "artist", artist.ID, base.Add(time.Minute))
	// Backdated comment with a HIGHER id but EARLIER created_at: the last
	// commenter must be resolved from the latest comment BY TIMESTAMP
	// (DJ Spectre's), not from MAX(id).
	suite.createTestCommentAt(user.ID, "artist", artist.ID, base.Add(-time.Minute))

	items, total, err := suite.service.ListWatching(user.ID, 20, 0)
	suite.NoError(err)
	suite.Equal(int64(3), total)
	suite.Require().Len(items, 3)

	// Ordered by last_comment_at DESC; no-comment sub last
	suite.Equal("artist", items[0].EntityType)
	suite.Equal("Watch Artist", items[0].EntityName)
	suite.Equal(*artist.Slug, items[0].EntitySlug)
	suite.Equal("/artists/"+*artist.Slug, items[0].EntityURL)
	suite.Equal(2, items[0].CommentCount)
	suite.Equal(displayName, items[0].LastCommenterName)
	suite.NotNil(items[0].LastCommentAt)

	suite.Equal("venue", items[1].EntityType)
	suite.Equal("Watch Venue", items[1].EntityName)
	suite.Equal("/venues/"+*venue.Slug, items[1].EntityURL)

	// Missing entity row → fallback name + ID URL, empty thread
	suite.Equal("show", items[2].EntityType)
	suite.Equal("show #999", items[2].EntityName)
	suite.Equal("", items[2].EntitySlug)
	suite.Equal("/shows/999", items[2].EntityURL)
	suite.Equal(0, items[2].CommentCount)
	suite.Nil(items[2].LastCommentAt)
	suite.Equal("", items[2].LastCommenterName)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingUnreadVsLastRead() {
	user := suite.createTestUser()

	suite.NoError(suite.service.Subscribe(user.ID, "show", 1))
	suite.NoError(suite.service.Subscribe(user.ID, "show", 2))

	// show 1: two comments, never read → unread
	suite.createTestComment(user.ID, "show", 1)
	suite.createTestComment(user.ID, "show", 1)

	// show 2: one comment, fully read → not unread
	suite.createTestComment(user.ID, "show", 2)
	suite.NoError(suite.service.MarkRead(user.ID, "show", 2))

	items, _, err := suite.service.ListWatching(user.ID, 20, 0)
	suite.NoError(err)
	suite.Require().Len(items, 2)

	byEntity := map[uint]int{items[0].EntityID: 0, items[1].EntityID: 1}
	show1 := items[byEntity[1]]
	show2 := items[byEntity[2]]

	suite.Equal(2, show1.UnreadCount)
	suite.Equal(0, show2.UnreadCount)

	// New comment after mark-read flips show 2 back to unread
	suite.createTestComment(user.ID, "show", 2)
	items, _, err = suite.service.ListWatching(user.ID, 20, 0)
	suite.NoError(err)
	for _, item := range items {
		if item.EntityID == 2 {
			suite.Equal(1, item.UnreadCount)
		}
	}
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingPagination() {
	user := suite.createTestUser()

	base := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= 5; i++ {
		suite.NoError(suite.service.Subscribe(user.ID, "show", uint(i)))
		suite.createTestCommentAt(user.ID, "show", uint(i), base.Add(time.Duration(i)*time.Minute))
	}

	// First page: newest activity first (show 5, show 4)
	items, total, err := suite.service.ListWatching(user.ID, 2, 0)
	suite.NoError(err)
	suite.Equal(int64(5), total)
	suite.Require().Len(items, 2)
	suite.Equal(uint(5), items[0].EntityID)
	suite.Equal(uint(4), items[1].EntityID)

	// Second page
	items2, _, err := suite.service.ListWatching(user.ID, 2, 2)
	suite.NoError(err)
	suite.Require().Len(items2, 2)
	suite.Equal(uint(3), items2[0].EntityID)

	// Third page (only 1 remaining)
	items3, _, err := suite.service.ListWatching(user.ID, 2, 4)
	suite.NoError(err)
	suite.Len(items3, 1)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingCountsOnlyVisibleComments() {
	user := suite.createTestUser()
	suite.NoError(suite.service.Subscribe(user.ID, "show", 1))

	suite.createTestComment(user.ID, "show", 1)

	hidden := &engagementm.Comment{
		Kind:       engagementm.CommentKindComment,
		EntityType: "show",
		EntityID:   1,
		UserID:     user.ID,
		Body:       "Hidden",
		BodyHTML:   "<p>Hidden</p>",
		Visibility: engagementm.CommentVisibilityHiddenByMod,
	}
	suite.Require().NoError(suite.db.Create(hidden).Error)

	fieldNote := &engagementm.Comment{
		Kind:       engagementm.CommentKindFieldNote,
		EntityType: "show",
		EntityID:   1,
		UserID:     user.ID,
		Body:       "Field note",
		BodyHTML:   "<p>Field note</p>",
		Visibility: engagementm.CommentVisibilityVisible,
	}
	suite.Require().NoError(suite.db.Create(fieldNote).Error)

	items, _, err := suite.service.ListWatching(user.ID, 20, 0)
	suite.NoError(err)
	suite.Require().Len(items, 1)
	// comment_count covers only visible kind='comment' rows
	suite.Equal(1, items[0].CommentCount)
	// unread_count spans all visible kinds (comment + field note), matching
	// the subscribe/status badge semantics (GetUnreadCount)
	suite.Equal(2, items[0].UnreadCount)
}

// TestListWatchingScopedToUser: another user's subscriptions must never
// appear in (or affect the total of) the requesting user's list.
func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingScopedToUser() {
	user := suite.createTestUser()
	other := suite.createTestUser()

	suite.NoError(suite.service.Subscribe(user.ID, "show", 1))
	suite.NoError(suite.service.Subscribe(other.ID, "show", 2))

	items, total, err := suite.service.ListWatching(user.ID, 20, 0)
	suite.NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Equal(uint(1), items[0].EntityID)
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

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBListWatching() {
	svc := &CommentSubscriptionService{db: nil}
	_, _, err := svc.ListWatching(1, 20, 0)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBGetSubscribersForEntity() {
	svc := &CommentSubscriptionService{db: nil}
	_, err := svc.GetSubscribersForEntity("show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}
