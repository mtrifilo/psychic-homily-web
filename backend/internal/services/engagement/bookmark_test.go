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

type BookmarkServiceIntegrationTestSuite struct {
	suite.Suite
	testDB          *testutil.TestDatabase
	db              *gorm.DB
	bookmarkService *BookmarkService
}

func (suite *BookmarkServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.bookmarkService = &BookmarkService{db: suite.testDB.DB}
}

func (suite *BookmarkServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *BookmarkServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestBookmarkServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(BookmarkServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) createTestUser() *authm.User {
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

// =============================================================================
// Group 1: CreateBookmark
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestCreateBookmark_Success() {
	user := suite.createTestUser()

	err := suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)

	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestCreateBookmark_Idempotent() {
	user := suite.createTestUser()

	err := suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.Require().NoError(err)

	// Create again - should not error
	err = suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestCreateBookmark_DifferentActions() {
	user := suite.createTestUser()

	// Same entity, different actions should create separate records
	err := suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.Require().NoError(err)

	err = suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionGoing)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?",
			user.ID, engagementm.BookmarkEntityShow, 1).
		Count(&count)
	suite.Equal(int64(2), count)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestCreateBookmark_DifferentEntityTypes() {
	user := suite.createTestUser()

	err := suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.Require().NoError(err)

	err = suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityVenue, 1, engagementm.BookmarkActionFollow)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).Where("user_id = ?", user.ID).Count(&count)
	suite.Equal(int64(2), count)
}

// =============================================================================
// Group 2: DeleteBookmark
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestDeleteBookmark_Success() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)

	err := suite.bookmarkService.DeleteBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)

	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestDeleteBookmark_NotFound() {
	user := suite.createTestUser()

	err := suite.bookmarkService.DeleteBookmark(user.ID, engagementm.BookmarkEntityShow, 99999, engagementm.BookmarkActionSave)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "bookmark not found")
}

// =============================================================================
// Group 3: IsBookmarked
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestIsBookmarked_True() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)

	result, err := suite.bookmarkService.IsBookmarked(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)

	suite.Require().NoError(err)
	suite.True(result)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestIsBookmarked_False() {
	user := suite.createTestUser()

	result, err := suite.bookmarkService.IsBookmarked(user.ID, engagementm.BookmarkEntityShow, 99999, engagementm.BookmarkActionSave)

	suite.Require().NoError(err)
	suite.False(result)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestIsBookmarked_WrongAction() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)

	result, err := suite.bookmarkService.IsBookmarked(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionGoing)

	suite.Require().NoError(err)
	suite.False(result)
}

// =============================================================================
// Group 4: GetBookmarkedEntityIDs
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestGetBookmarkedEntityIDs_Success() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 3, engagementm.BookmarkActionSave)

	result, err := suite.bookmarkService.GetBookmarkedEntityIDs(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, []uint{1, 2, 3})

	suite.Require().NoError(err)
	suite.True(result[1])
	suite.False(result[2])
	suite.True(result[3])
}

func (suite *BookmarkServiceIntegrationTestSuite) TestGetBookmarkedEntityIDs_EmptyInput() {
	user := suite.createTestUser()

	result, err := suite.bookmarkService.GetBookmarkedEntityIDs(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, []uint{})

	suite.Require().NoError(err)
	suite.Empty(result)
}

// =============================================================================
// Group 5: GetUserBookmarks
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestGetUserBookmarks_Success() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	time.Sleep(10 * time.Millisecond)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 2, engagementm.BookmarkActionSave)

	bookmarks, total, err := suite.bookmarkService.GetUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(bookmarks, 2)
	// Most recently created first
	suite.Equal(uint(2), bookmarks[0].EntityID)
	suite.Equal(uint(1), bookmarks[1].EntityID)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestGetUserBookmarks_Empty() {
	user := suite.createTestUser()

	bookmarks, total, err := suite.bookmarkService.GetUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(bookmarks)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestGetUserBookmarks_Pagination() {
	user := suite.createTestUser()

	for i := uint(1); i <= 5; i++ {
		suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, i, engagementm.BookmarkActionSave)
		time.Sleep(5 * time.Millisecond)
	}

	page1, total, err := suite.bookmarkService.GetUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(page1, 2)

	page2, _, err := suite.bookmarkService.GetUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 2, 2)
	suite.Require().NoError(err)
	suite.Len(page2, 2)

	page3, _, err := suite.bookmarkService.GetUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 2, 4)
	suite.Require().NoError(err)
	suite.Len(page3, 1)

	// No overlap
	suite.NotEqual(page1[0].EntityID, page2[0].EntityID)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestGetUserBookmarks_OnlyOwnBookmarks() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user1.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.bookmarkService.CreateBookmark(user2.ID, engagementm.BookmarkEntityShow, 2, engagementm.BookmarkActionSave)

	bookmarks, total, err := suite.bookmarkService.GetUserBookmarks(user1.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(bookmarks, 1)
	suite.Equal(uint(1), bookmarks[0].EntityID)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestGetUserBookmarks_FiltersByEntityTypeAndAction() {
	user := suite.createTestUser()

	// Create different types of bookmarks
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityVenue, 1, engagementm.BookmarkActionFollow)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 2, engagementm.BookmarkActionGoing)

	// Should only return show saves
	bookmarks, total, err := suite.bookmarkService.GetUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(bookmarks, 1)
	suite.Equal(uint(1), bookmarks[0].EntityID)
	suite.Equal(engagementm.BookmarkEntityShow, bookmarks[0].EntityType)
	suite.Equal(engagementm.BookmarkActionSave, bookmarks[0].Action)
}

// =============================================================================
// Group 6: GetUserBookmarksByEntityType
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestGetUserBookmarksByEntityType_Success() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityVenue, 1, engagementm.BookmarkActionFollow)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityVenue, 2, engagementm.BookmarkActionFollow)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave) // Different type

	bookmarks, err := suite.bookmarkService.GetUserBookmarksByEntityType(user.ID, engagementm.BookmarkEntityVenue, engagementm.BookmarkActionFollow)

	suite.Require().NoError(err)
	suite.Len(bookmarks, 2)
}

// =============================================================================
// Group 7: CountUserBookmarks
// =============================================================================

func (suite *BookmarkServiceIntegrationTestSuite) TestCountUserBookmarks_Success() {
	user := suite.createTestUser()

	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 1, engagementm.BookmarkActionSave)
	suite.bookmarkService.CreateBookmark(user.ID, engagementm.BookmarkEntityShow, 2, engagementm.BookmarkActionSave)

	count, err := suite.bookmarkService.CountUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave)

	suite.Require().NoError(err)
	suite.Equal(int64(2), count)
}

func (suite *BookmarkServiceIntegrationTestSuite) TestCountUserBookmarks_Zero() {
	user := suite.createTestUser()

	count, err := suite.bookmarkService.CountUserBookmarks(user.ID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave)

	suite.Require().NoError(err)
	suite.Equal(int64(0), count)
}
