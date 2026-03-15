package engagement

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewFollowService(t *testing.T) {
	svc := NewFollowService(nil)
	assert.NotNil(t, svc)
}

func TestFollowService_NilDatabase(t *testing.T) {
	svc := &FollowService{db: nil}

	t.Run("Follow", func(t *testing.T) {
		err := svc.Follow(1, "artist", 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("Unfollow", func(t *testing.T) {
		err := svc.Unfollow(1, "artist", 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("IsFollowing", func(t *testing.T) {
		result, err := svc.IsFollowing(1, "artist", 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.False(t, result)
	})

	t.Run("GetFollowerCount", func(t *testing.T) {
		count, err := svc.GetFollowerCount("artist", 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Zero(t, count)
	})

	t.Run("GetBatchFollowerCounts", func(t *testing.T) {
		result, err := svc.GetBatchFollowerCounts("artist", []uint{1, 2})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, result)
	})

	t.Run("GetBatchUserFollowing", func(t *testing.T) {
		result, err := svc.GetBatchUserFollowing(1, "artist", []uint{1, 2})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, result)
	})

	t.Run("GetUserFollowing", func(t *testing.T) {
		following, total, err := svc.GetUserFollowing(1, "artist", 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, following)
		assert.Zero(t, total)
	})

	t.Run("GetFollowers", func(t *testing.T) {
		followers, total, err := svc.GetFollowers("artist", 1, 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, followers)
		assert.Zero(t, total)
	})
}

func TestFollowService_InvalidEntityType(t *testing.T) {
	svc := &FollowService{db: &gorm.DB{}}

	t.Run("Follow_InvalidType", func(t *testing.T) {
		err := svc.Follow(1, "show", 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("Follow_InvalidType_Unknown", func(t *testing.T) {
		err := svc.Follow(1, "banana", 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("Unfollow_InvalidType", func(t *testing.T) {
		err := svc.Unfollow(1, "show", 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("IsFollowing_InvalidType", func(t *testing.T) {
		_, err := svc.IsFollowing(1, "release", 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("GetFollowerCount_InvalidType", func(t *testing.T) {
		_, err := svc.GetFollowerCount("show", 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("GetBatchFollowerCounts_InvalidType", func(t *testing.T) {
		_, err := svc.GetBatchFollowerCounts("show", []uint{1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("GetBatchUserFollowing_InvalidType", func(t *testing.T) {
		_, err := svc.GetBatchUserFollowing(1, "show", []uint{1})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("GetUserFollowing_InvalidType", func(t *testing.T) {
		_, _, err := svc.GetUserFollowing(1, "show", 10, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})

	t.Run("GetFollowers_InvalidType", func(t *testing.T) {
		_, _, err := svc.GetFollowers("show", 1, 10, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type FollowServiceIntegrationTestSuite struct {
	suite.Suite
	container     testcontainers.Container
	db            *gorm.DB
	followService *FollowService
	ctx           context.Context
}

func (suite *FollowServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	testutil.RunAllMigrations(suite.T(), sqlDB, filepath.Join("..", "..", "..", "db", "migrations"))

	suite.followService = NewFollowService(db)
}

func (suite *FollowServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *FollowServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestFollowServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FollowServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *FollowServiceIntegrationTestSuite) createTestUserWithUsername(username string) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("%s@test.com", username)),
		Username:      stringPtr(username),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *FollowServiceIntegrationTestSuite) createTestArtist(name string) uint {
	slug := name
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *FollowServiceIntegrationTestSuite) createTestVenue(name string) uint {
	slug := name
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

func (suite *FollowServiceIntegrationTestSuite) createTestLabel(name string) uint {
	slug := name
	label := &models.Label{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label.ID
}

func (suite *FollowServiceIntegrationTestSuite) createTestFestival(name string) uint {
	festival := &models.Festival{
		Name:        name,
		Slug:        name,
		SeriesSlug:  name,
		EditionYear: 2026,
		StartDate:   "2026-07-01",
		EndDate:     "2026-07-03",
	}
	err := suite.db.Create(festival).Error
	suite.Require().NoError(err)
	return festival.ID
}

// =============================================================================
// Group 1: Follow + Unfollow
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Artist() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Test Artist")

	err := suite.followService.Follow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	// Verify bookmark exists
	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityArtist, artistID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Idempotent() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Idempotent Artist")

	err := suite.followService.Follow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	// Follow again — should not error or create duplicate
	err = suite.followService.Follow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityArtist, artistID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Venue() {
	user := suite.createTestUser()
	venueID := suite.createTestVenue("Test Venue")

	err := suite.followService.Follow(user.ID, "venue", venueID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityVenue, venueID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Label() {
	user := suite.createTestUser()
	labelID := suite.createTestLabel("Test Label")

	err := suite.followService.Follow(user.ID, "label", labelID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityLabel, labelID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Festival() {
	user := suite.createTestUser()
	festivalID := suite.createTestFestival("test-festival")

	err := suite.followService.Follow(user.ID, "festival", festivalID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityFestival, festivalID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestUnfollow_Success() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Unfollow Artist")

	suite.followService.Follow(user.ID, "artist", artistID)

	err := suite.followService.Unfollow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityArtist, artistID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestUnfollow_Idempotent() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Never Followed Artist")

	// Unfollow something we never followed — should not error
	err := suite.followService.Unfollow(user.ID, "artist", artistID)
	suite.Require().NoError(err)
}

// =============================================================================
// Group 2: IsFollowing
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestIsFollowing_True() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Following Artist")

	suite.followService.Follow(user.ID, "artist", artistID)

	result, err := suite.followService.IsFollowing(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.True(result)
}

func (suite *FollowServiceIntegrationTestSuite) TestIsFollowing_False() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Not Following Artist")

	result, err := suite.followService.IsFollowing(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.False(result)
}

// =============================================================================
// Group 3: GetFollowerCount
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowerCount_MultipleFollowers() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	user3 := suite.createTestUser()
	artistID := suite.createTestArtist("Popular Artist")

	suite.followService.Follow(user1.ID, "artist", artistID)
	suite.followService.Follow(user2.ID, "artist", artistID)
	suite.followService.Follow(user3.ID, "artist", artistID)

	count, err := suite.followService.GetFollowerCount("artist", artistID)
	suite.Require().NoError(err)
	suite.Equal(int64(3), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowerCount_NoFollowers() {
	artistID := suite.createTestArtist("Unpopular Artist")

	count, err := suite.followService.GetFollowerCount("artist", artistID)
	suite.Require().NoError(err)
	suite.Equal(int64(0), count)
}

// =============================================================================
// Group 4: GetBatchFollowerCounts
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetBatchFollowerCounts_MultipleEntities() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	artist1ID := suite.createTestArtist("Batch Artist 1")
	artist2ID := suite.createTestArtist("Batch Artist 2")
	artist3ID := suite.createTestArtist("Batch Artist 3")

	suite.followService.Follow(user1.ID, "artist", artist1ID)
	suite.followService.Follow(user2.ID, "artist", artist1ID)
	suite.followService.Follow(user1.ID, "artist", artist2ID)

	result, err := suite.followService.GetBatchFollowerCounts("artist", []uint{artist1ID, artist2ID, artist3ID})
	suite.Require().NoError(err)
	suite.Len(result, 3)
	suite.Equal(int64(2), result[artist1ID])
	suite.Equal(int64(1), result[artist2ID])
	suite.Equal(int64(0), result[artist3ID])
}

func (suite *FollowServiceIntegrationTestSuite) TestGetBatchFollowerCounts_EmptyList() {
	result, err := suite.followService.GetBatchFollowerCounts("artist", []uint{})
	suite.Require().NoError(err)
	suite.Empty(result)
}

// =============================================================================
// Group 5: GetBatchUserFollowing
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetBatchUserFollowing_MixedFollowing() {
	user := suite.createTestUser()
	artist1ID := suite.createTestArtist("Batch User Artist 1")
	artist2ID := suite.createTestArtist("Batch User Artist 2")
	artist3ID := suite.createTestArtist("Batch User Artist 3")

	suite.followService.Follow(user.ID, "artist", artist1ID)
	suite.followService.Follow(user.ID, "artist", artist3ID)

	result, err := suite.followService.GetBatchUserFollowing(user.ID, "artist", []uint{artist1ID, artist2ID, artist3ID})
	suite.Require().NoError(err)

	suite.True(result[artist1ID])
	suite.False(result[artist2ID]) // not present in map or false
	suite.True(result[artist3ID])
}

func (suite *FollowServiceIntegrationTestSuite) TestGetBatchUserFollowing_EmptyList() {
	user := suite.createTestUser()

	result, err := suite.followService.GetBatchUserFollowing(user.ID, "artist", []uint{})
	suite.Require().NoError(err)
	suite.Empty(result)
}

// =============================================================================
// Group 6: GetUserFollowing
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_ByType() {
	user := suite.createTestUser()
	artist1ID := suite.createTestArtist("Following Artist 1")
	artist2ID := suite.createTestArtist("Following Artist 2")
	venueID := suite.createTestVenue("Following Venue")

	suite.followService.Follow(user.ID, "artist", artist1ID)
	suite.followService.Follow(user.ID, "artist", artist2ID)
	suite.followService.Follow(user.ID, "venue", venueID)

	// Filter by artist
	following, total, err := suite.followService.GetUserFollowing(user.ID, "artist", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(following, 2)
	for _, f := range following {
		suite.Equal("artist", f.EntityType)
	}
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_AllTypes() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("All Types Artist")
	venueID := suite.createTestVenue("All Types Venue")
	labelID := suite.createTestLabel("All Types Label")

	suite.followService.Follow(user.ID, "artist", artistID)
	suite.followService.Follow(user.ID, "venue", venueID)
	suite.followService.Follow(user.ID, "label", labelID)

	following, total, err := suite.followService.GetUserFollowing(user.ID, "", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(following, 3)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		artistID := suite.createTestArtist(fmt.Sprintf("Paginated Artist %d", i))
		suite.followService.Follow(user.ID, "artist", artistID)
	}

	// First page
	page1, total, err := suite.followService.GetUserFollowing(user.ID, "artist", 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(page1, 2)

	// Second page
	page2, _, err := suite.followService.GetUserFollowing(user.ID, "artist", 2, 2)
	suite.Require().NoError(err)
	suite.Len(page2, 2)

	// Third page
	page3, _, err := suite.followService.GetUserFollowing(user.ID, "artist", 2, 4)
	suite.Require().NoError(err)
	suite.Len(page3, 1)

	// Check no overlap between pages
	suite.NotEqual(page1[0].EntityID, page2[0].EntityID)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_OrderByFollowDateDesc() {
	user := suite.createTestUser()
	artist1ID := suite.createTestArtist("First Followed")
	artist2ID := suite.createTestArtist("Second Followed")

	suite.followService.Follow(user.ID, "artist", artist1ID)
	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)
	suite.followService.Follow(user.ID, "artist", artist2ID)

	following, _, err := suite.followService.GetUserFollowing(user.ID, "artist", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(following, 2)

	// Most recent follow should be first
	suite.Equal(artist2ID, following[0].EntityID)
	suite.Equal(artist1ID, following[1].EntityID)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_IncludesNameAndSlug() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Named Artist")

	suite.followService.Follow(user.ID, "artist", artistID)

	following, _, err := suite.followService.GetUserFollowing(user.ID, "artist", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(following, 1)
	suite.Equal("Named Artist", following[0].Name)
	suite.Equal("Named Artist", following[0].Slug) // slug was set to name in helper
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_Empty() {
	user := suite.createTestUser()

	following, total, err := suite.followService.GetUserFollowing(user.ID, "artist", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(following)
}

// =============================================================================
// Group 7: GetFollowers
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_ReturnsUserInfo() {
	user := suite.createTestUserWithUsername("follower-user")
	artistID := suite.createTestArtist("Followed Artist")

	suite.followService.Follow(user.ID, "artist", artistID)

	followers, total, err := suite.followService.GetFollowers("artist", artistID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(followers, 1)
	suite.Equal(user.ID, followers[0].UserID)
	suite.Equal("follower-user", followers[0].Username)
	suite.Equal("Test", followers[0].DisplayName)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_Pagination() {
	artistID := suite.createTestArtist("Paginated Followers Artist")
	for i := 0; i < 5; i++ {
		user := suite.createTestUser()
		suite.followService.Follow(user.ID, "artist", artistID)
	}

	page1, total, err := suite.followService.GetFollowers("artist", artistID, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(page1, 2)

	page2, _, err := suite.followService.GetFollowers("artist", artistID, 2, 2)
	suite.Require().NoError(err)
	suite.Len(page2, 2)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_Empty() {
	artistID := suite.createTestArtist("No Followers Artist")

	followers, total, err := suite.followService.GetFollowers("artist", artistID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(followers)
}

// =============================================================================
// Group 8: Cross-entity type coverage
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_VenueNameSlug() {
	user := suite.createTestUser()
	venueID := suite.createTestVenue("The Rebel Lounge")

	suite.followService.Follow(user.ID, "venue", venueID)

	following, total, err := suite.followService.GetUserFollowing(user.ID, "venue", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)
	suite.Equal("venue", following[0].EntityType)
	suite.Equal("The Rebel Lounge", following[0].Name)
	suite.Equal("The Rebel Lounge", following[0].Slug) // slug set to name in helper
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_FestivalNameSlug() {
	user := suite.createTestUser()
	festivalID := suite.createTestFestival("summer-fest")

	suite.followService.Follow(user.ID, "festival", festivalID)

	following, total, err := suite.followService.GetUserFollowing(user.ID, "festival", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)
	suite.Equal("festival", following[0].EntityType)
	suite.Equal("summer-fest", following[0].Name)
	suite.Equal("summer-fest", following[0].Slug)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_LabelNameSlug() {
	user := suite.createTestUser()
	labelID := suite.createTestLabel("Sub Pop")

	suite.followService.Follow(user.ID, "label", labelID)

	following, total, err := suite.followService.GetUserFollowing(user.ID, "label", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)
	suite.Equal("label", following[0].EntityType)
	suite.Equal("Sub Pop", following[0].Name)
	suite.Equal("Sub Pop", following[0].Slug)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_VenueEntityType() {
	user1 := suite.createTestUserWithUsername("venue-follower-1")
	user2 := suite.createTestUserWithUsername("venue-follower-2")
	venueID := suite.createTestVenue("Crescent Ballroom")

	suite.followService.Follow(user1.ID, "venue", venueID)
	suite.followService.Follow(user2.ID, "venue", venueID)

	followers, total, err := suite.followService.GetFollowers("venue", venueID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(followers, 2)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_FestivalEntityType() {
	user := suite.createTestUserWithUsername("fest-follower")
	festivalID := suite.createTestFestival("m3f-fest")

	suite.followService.Follow(user.ID, "festival", festivalID)

	followers, total, err := suite.followService.GetFollowers("festival", festivalID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(followers, 1)
	suite.Equal("fest-follower", followers[0].Username)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetBatchFollowerCounts_Venues() {
	user := suite.createTestUser()
	venue1ID := suite.createTestVenue("Batch Venue 1")
	venue2ID := suite.createTestVenue("Batch Venue 2")

	suite.followService.Follow(user.ID, "venue", venue1ID)

	result, err := suite.followService.GetBatchFollowerCounts("venue", []uint{venue1ID, venue2ID})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result[venue1ID])
	suite.Equal(int64(0), result[venue2ID])
}

// =============================================================================
// Group 9: Follow → Unfollow → Verify cycle
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestFollowUnfollowCycle() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Cycle Artist")

	// Follow
	err := suite.followService.Follow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	isFollowing, err := suite.followService.IsFollowing(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.True(isFollowing)

	count, err := suite.followService.GetFollowerCount("artist", artistID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), count)

	// Unfollow
	err = suite.followService.Unfollow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	isFollowing, err = suite.followService.IsFollowing(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.False(isFollowing)

	count, err = suite.followService.GetFollowerCount("artist", artistID)
	suite.Require().NoError(err)
	suite.Equal(int64(0), count)

	// Re-follow
	err = suite.followService.Follow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	isFollowing, err = suite.followService.IsFollowing(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.True(isFollowing)
}
