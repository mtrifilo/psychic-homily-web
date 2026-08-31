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
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// watchingViewer is the ordinary non-admin subscriber reading their own list —
// the caller every test below has always meant.
//
// ListWatching takes a viewer rather than a user id (PSY-1983): the rows' owner
// and the tier they are read at are the same fact. The tests that exercise the
// gate build their own viewer; these do not, and say so by using this.
func watchingViewer(userID uint) contracts.ShowViewer {
	return contracts.ShowViewer{UserID: userID}
}

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
	_, _ = sqlDB.Exec("DELETE FROM shows")
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

// createTestShow creates an APPROVED show, for the watching-list tests whose
// subject is pagination, unread counts or user scoping rather than visibility.
//
// A real row, and an approved one, because ListWatching gates show-typed
// subscriptions on the show detail route's rule (PSY-1983): a subscription
// pointing at a show id with no row behind it is suppressed, exactly as a gated
// one is, so that a deleted show and a private one cannot be told apart. These
// fixtures used bare ids 1..5 and never created the shows, which is why the gate
// emptied every one of them.
//
// SubmittedBy is left null on purpose: approved is visible to everybody, so a
// submitter would be a second reason for these rows to appear and would mask the
// gate if the status one ever broke.
func (suite *CommentSubscriptionServiceIntegrationTestSuite) createTestShow(title string) *catalogm.Show {
	show := &catalogm.Show{
		Title:     title,
		EventDate: time.Now().UTC().AddDate(0, 0, 7),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
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

	items, total, err := suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
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
	// A RELEASE with no row, not a show. The fallback rendering is still the
	// behaviour for every entity type that has no visibility rule of its own,
	// but a show is no longer one of them: a show-typed subscription with no row
	// behind it is suppressed rather than rendered "show #999", because a
	// deleted show and a private one have to answer the same
	// (TestListWatchingSuppressesAShowThatIsNotVisible).
	suite.NoError(suite.service.Subscribe(user.ID, "release", 999)) // no release row

	base := time.Now().UTC().Add(-time.Hour)
	suite.createTestCommentAt(commenter.ID, "venue", venue.ID, base)
	suite.createTestCommentAt(commenter.ID, "artist", artist.ID, base.Add(time.Minute))
	// Backdated comment with a HIGHER id but EARLIER created_at: the last
	// commenter must be resolved from the latest comment BY TIMESTAMP
	// (DJ Spectre's), not from MAX(id).
	suite.createTestCommentAt(user.ID, "artist", artist.ID, base.Add(-time.Minute))

	items, total, err := suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
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
	suite.Equal("release", items[2].EntityType)
	suite.Equal("release #999", items[2].EntityName)
	suite.Equal("", items[2].EntitySlug)
	suite.Equal("/releases/999", items[2].EntityURL)
	suite.Equal(0, items[2].CommentCount)
	suite.Nil(items[2].LastCommentAt)
	suite.Equal("", items[2].LastCommenterName)
}

// The show-typed half of the case above, and the reason it had to move off a
// show: a subscription is suppressed, with its count, when the viewer cannot see
// the show behind it — whether that is because the show is gated or because
// there is no show. Answering differently for the two would let a caller sort
// real private shows from ids that were never used, which is the oracle the
// detail route's 404 exists to remove (PSY-1983).
func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingSuppressesAShowThatIsNotVisible() {
	user := suite.createTestUser()
	other := suite.createTestUser()

	visible := suite.createTestShow("Watching Approved Show")
	private := suite.createTestShow("Watching Private Show")
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("id = ?", private.ID).
		Update("status", catalogm.ShowStatusPrivate).Error)
	// Somebody else's private show, so the submitter branch cannot be what
	// hides it, and a submitted_by of the viewer cannot be what reveals it.
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("id = ?", private.ID).
		Update("submitted_by", other.ID).Error)

	suite.NoError(suite.service.Subscribe(user.ID, "show", visible.ID))
	suite.NoError(suite.service.Subscribe(user.ID, "show", private.ID))
	suite.NoError(suite.service.Subscribe(user.ID, "show", 99999999)) // no show row
	// The submitter subscribes to their own private show, which is the control:
	// without it, "suppressed" and "the gate refuses everybody" look the same.
	suite.NoError(suite.service.Subscribe(other.ID, "show", private.ID))

	items, total, err := suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
	suite.NoError(err)
	suite.Require().Len(items, 1)
	suite.Equal(visible.ID, items[0].EntityID)
	// The total moves with the page. Three subscriptions exist and one is
	// reportable; a total of 3 beside one row would publish the withheld two as
	// arithmetic.
	suite.Equal(int64(1), total)

	// The submitter reads their own private show, matching the detail route.
	otherItems, otherTotal, err := suite.service.ListWatching(watchingViewer(other.ID), 20, 0)
	suite.NoError(err)
	suite.Equal(int64(1), otherTotal)
	suite.Require().Len(otherItems, 1)
	suite.Equal(private.ID, otherItems[0].EntityID)

	// Suppression, not deletion: publishing the show again restores its row.
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Where("id = ?", private.ID).
		Update("status", catalogm.ShowStatusApproved).Error)
	items, total, err = suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
	suite.NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(items, 2)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingUnreadVsLastRead() {
	user := suite.createTestUser()

	showA := suite.createTestShow("Unread Show A")
	showB := suite.createTestShow("Unread Show B")

	suite.NoError(suite.service.Subscribe(user.ID, "show", showA.ID))
	suite.NoError(suite.service.Subscribe(user.ID, "show", showB.ID))

	// showA: two comments, never read → unread
	suite.createTestComment(user.ID, "show", showA.ID)
	suite.createTestComment(user.ID, "show", showA.ID)

	// showB: one comment, fully read → not unread
	suite.createTestComment(user.ID, "show", showB.ID)
	suite.NoError(suite.service.MarkRead(user.ID, "show", showB.ID))

	items, _, err := suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
	suite.NoError(err)
	suite.Require().Len(items, 2)

	byEntity := map[uint]int{items[0].EntityID: 0, items[1].EntityID: 1}
	showAItem := items[byEntity[showA.ID]]
	showBItem := items[byEntity[showB.ID]]

	suite.Equal(2, showAItem.UnreadCount)
	suite.Equal(0, showBItem.UnreadCount)

	// New comment after mark-read flips showB back to unread
	suite.createTestComment(user.ID, "show", showB.ID)
	items, _, err = suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
	suite.NoError(err)
	for _, item := range items {
		if item.EntityID == showB.ID {
			suite.Equal(1, item.UnreadCount)
		}
	}
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingPagination() {
	user := suite.createTestUser()

	base := time.Now().UTC().Add(-time.Hour)
	shows := make([]*catalogm.Show, 0, 5)
	for i := 1; i <= 5; i++ {
		show := suite.createTestShow(fmt.Sprintf("Paged Show %d", i))
		shows = append(shows, show)
		suite.NoError(suite.service.Subscribe(user.ID, "show", show.ID))
		suite.createTestCommentAt(user.ID, "show", show.ID, base.Add(time.Duration(i)*time.Minute))
	}

	// First page: newest activity first (the 5th show, then the 4th)
	items, total, err := suite.service.ListWatching(watchingViewer(user.ID), 2, 0)
	suite.NoError(err)
	suite.Equal(int64(5), total)
	suite.Require().Len(items, 2)
	suite.Equal(shows[4].ID, items[0].EntityID)
	suite.Equal(shows[3].ID, items[1].EntityID)

	// Second page
	items2, _, err := suite.service.ListWatching(watchingViewer(user.ID), 2, 2)
	suite.NoError(err)
	suite.Require().Len(items2, 2)
	suite.Equal(shows[2].ID, items2[0].EntityID)

	// Third page (only 1 remaining)
	items3, _, err := suite.service.ListWatching(watchingViewer(user.ID), 2, 4)
	suite.NoError(err)
	suite.Len(items3, 1)
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestListWatchingCountsOnlyVisibleComments() {
	user := suite.createTestUser()
	show := suite.createTestShow("Visible Counts Show")
	suite.NoError(suite.service.Subscribe(user.ID, "show", show.ID))

	suite.createTestComment(user.ID, "show", show.ID)

	hidden := &engagementm.Comment{
		Kind:       engagementm.CommentKindComment,
		EntityType: "show",
		EntityID:   show.ID,
		UserID:     user.ID,
		Body:       "Hidden",
		BodyHTML:   "<p>Hidden</p>",
		Visibility: engagementm.CommentVisibilityHiddenByMod,
	}
	suite.Require().NoError(suite.db.Create(hidden).Error)

	fieldNote := &engagementm.Comment{
		Kind:       engagementm.CommentKindFieldNote,
		EntityType: "show",
		EntityID:   show.ID,
		UserID:     user.ID,
		Body:       "Field note",
		BodyHTML:   "<p>Field note</p>",
		Visibility: engagementm.CommentVisibilityVisible,
	}
	suite.Require().NoError(suite.db.Create(fieldNote).Error)

	items, _, err := suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
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
	mine := suite.createTestShow("Scoped Mine")
	theirs := suite.createTestShow("Scoped Theirs")

	suite.NoError(suite.service.Subscribe(user.ID, "show", mine.ID))
	suite.NoError(suite.service.Subscribe(other.ID, "show", theirs.ID))

	items, total, err := suite.service.ListWatching(watchingViewer(user.ID), 20, 0)
	suite.NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Equal(mine.ID, items[0].EntityID)
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
	_, _, err := svc.ListWatching(watchingViewer(1), 20, 0)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}

func (suite *CommentSubscriptionServiceIntegrationTestSuite) TestNilDBGetSubscribersForEntity() {
	svc := &CommentSubscriptionService{db: nil}
	_, err := svc.GetSubscribersForEntity("show", 1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
}
