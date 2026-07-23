package engagement

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// PSY-296: ensure "user" is accepted as a valid follow entity type.
func TestFollowService_UserEntityTypeAccepted(t *testing.T) {
	assert.True(t, validFollowEntityTypes[FollowEntityUser])
	assert.Equal(t, "user", FollowEntityUser)
}

// PSY-1356: ensure "radio_show" is accepted as a valid follow entity type.
func TestFollowService_RadioShowEntityTypeAccepted(t *testing.T) {
	assert.True(t, validFollowEntityTypes[string(engagementm.BookmarkEntityRadioShow)])
	assert.Equal(t, "radio_show", string(engagementm.BookmarkEntityRadioShow))
}

// PSY-1064: ensure "tag" is accepted as a valid follow entity type.
func TestFollowService_TagEntityTypeAccepted(t *testing.T) {
	assert.True(t, validFollowEntityTypes[string(engagementm.BookmarkEntityTag)])
	assert.True(t, libraryFollowEntityTypes[string(engagementm.BookmarkEntityTag)])
	assert.Equal(t, "tag", string(engagementm.BookmarkEntityTag))
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

	t.Run("GetLibraryFollowing_InvalidType", func(t *testing.T) {
		_, _, err := svc.GetLibraryFollowing(1, "radio_show", 10, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not available in Library")
	})

	t.Run("GetLibraryFollowing_InvalidLimit", func(t *testing.T) {
		_, _, err := svc.GetLibraryFollowing(1, "artist", 0, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be between")
	})

	t.Run("GetFollowers_InvalidType", func(t *testing.T) {
		_, _, err := svc.GetFollowers("show", 1, 10, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid entity type for follow")
	})
}

// PSY-1466: an unrecognized scene notify mode is rejected before any DB
// access ("off" acceptance is covered by the integration round-trip below).
func TestFollowService_SetSceneNotifyMode_InvalidMode(t *testing.T) {
	svc := &FollowService{db: &gorm.DB{}}

	err := svc.SetSceneNotifyMode(1, 1, "muted")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid notify mode")
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type FollowServiceIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	followService *FollowService
}

func (suite *FollowServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	// TestFollow_ConcurrentIdempotent fires many Follow calls at once; bound
	// the pool so they queue rather than exhaust the container's connections.
	boundTestPool(suite.db)

	suite.followService = NewFollowService(suite.testDB.DB)
}

func (suite *FollowServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FollowServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM scenes")
	// FK order: episodes → shows → stations (episode.show_id, show.station_id).
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestFollowServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FollowServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) createTestUser() *authm.User {
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

func (suite *FollowServiceIntegrationTestSuite) createTestUserWithUsername(username string) *authm.User {
	user := &authm.User{
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
	artist := &catalogm.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *FollowServiceIntegrationTestSuite) createTestVenue(name string) uint {
	slug := name
	venue := &catalogm.Venue{
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
	label := &catalogm.Label{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label.ID
}

// createTestRadioShow creates a station + show (+ optional host + episodes) and
// returns the show id. host="" leaves host_name NULL; airDates are "YYYY-MM-DD".
// The station has no timezone (NULL → the enrichment query's aired gate falls
// back to UTC).
func (suite *FollowServiceIntegrationTestSuite) createTestRadioShow(
	showName, showSlug, stationName, stationSlug, host string, airDates ...string,
) uint {
	return suite.createTestRadioShowTZ(showName, showSlug, stationName, stationSlug, "", host, airDates...)
}

// createTestRadioShowTZ is createTestRadioShow with an explicit station IANA
// timezone (timezone="" leaves it NULL). Used to exercise the station-local
// aired gate in last_episode_date.
func (suite *FollowServiceIntegrationTestSuite) createTestRadioShowTZ(
	showName, showSlug, stationName, stationSlug, timezone, host string, airDates ...string,
) uint {
	station := &catalogm.RadioStation{Name: stationName, Slug: stationSlug}
	if timezone != "" {
		tz := timezone
		station.Timezone = &tz
	}
	suite.Require().NoError(suite.db.Create(station).Error)

	var hostPtr *string
	if host != "" {
		hostPtr = &host
	}
	show := &catalogm.RadioShow{
		StationID: station.ID,
		Name:      showName,
		Slug:      showSlug,
		HostName:  hostPtr,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	for _, d := range airDates {
		ep := &catalogm.RadioEpisode{ShowID: show.ID, AirDate: d}
		suite.Require().NoError(suite.db.Create(ep).Error)
	}
	return show.ID
}

func (suite *FollowServiceIntegrationTestSuite) createTestScene(city, state, slug string) uint {
	scene := &catalogm.Scene{City: city, State: state, Slug: slug}
	err := suite.db.Create(scene).Error
	suite.Require().NoError(err)
	return scene.ID
}

func (suite *FollowServiceIntegrationTestSuite) createTestFestival(name string) uint {
	festival := &catalogm.Festival{
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

func (suite *FollowServiceIntegrationTestSuite) createTestTag(name string) uint {
	// Name uniqueness is global (idx_tags_name_lower); suffix so suite cases can
	// reuse display-ish prefixes without colliding across tests.
	unique := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	tag := &catalogm.Tag{
		Name:     unique,
		Slug:     unique,
		Category: catalogm.TagCategoryGenre,
	}
	err := suite.db.Create(tag).Error
	suite.Require().NoError(err)
	return tag.ID
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityArtist, artistID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

// PSY-1496: user→user follows reuse FollowEntityUser; IsFollowing must see
// edges created the same way the username-addressed API writes them (PSY-296
// reply gating depends on this).
func (suite *FollowServiceIntegrationTestSuite) TestFollow_User_IdempotentAndIsFollowing() {
	follower := suite.createTestUser()
	target := suite.createTestUser()

	err := suite.followService.Follow(follower.ID, FollowEntityUser, target.ID)
	suite.Require().NoError(err)
	err = suite.followService.Follow(follower.ID, FollowEntityUser, target.ID)
	suite.Require().NoError(err)

	following, err := suite.followService.IsFollowing(follower.ID, FollowEntityUser, target.ID)
	suite.Require().NoError(err)
	suite.True(following)

	count, err := suite.followService.GetFollowerCount(FollowEntityUser, target.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), count)

	// Empty-type GetUserFollowing is entities-only — user edge must not appear.
	artistID := suite.createTestArtist("Also Followed")
	suite.Require().NoError(suite.followService.Follow(follower.ID, "artist", artistID))
	list, total, err := suite.followService.GetUserFollowing(follower.ID, "", 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(list, 1)
	suite.Equal("artist", list[0].EntityType)

	suite.Require().NoError(suite.followService.Unfollow(follower.ID, FollowEntityUser, target.ID))
	following, err = suite.followService.IsFollowing(follower.ID, FollowEntityUser, target.ID)
	suite.Require().NoError(err)
	suite.False(following)
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityArtist, artistID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

// TestFollow_ConcurrentIdempotent fires N parallel Follow calls for the same
// (user, entity) and asserts none returns an error and exactly one row lands.
// PSY-755: the prior FirstOrCreate (SELECT-then-INSERT) let two racing calls
// both miss the row and both INSERT, surfacing the 23505 unique violation as a
// user-visible error from a supposedly-idempotent op. ON CONFLICT DO NOTHING
// makes the INSERT itself idempotent so no caller sees a failure under
// contention.
func (suite *FollowServiceIntegrationTestSuite) TestFollow_ConcurrentIdempotent() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Concurrent Follow Artist")

	const n = 100
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // release all goroutines together to maximize contention
			errs[idx] = suite.followService.Follow(user.ID, "artist", artistID)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		suite.NoError(err, "concurrent Follow #%d must not surface a unique violation", i)
	}

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityArtist, artistID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count, "concurrent follows must collapse to exactly one row")
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Venue() {
	user := suite.createTestUser()
	venueID := suite.createTestVenue("Test Venue")

	err := suite.followService.Follow(user.ID, "venue", venueID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityVenue, venueID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Label() {
	user := suite.createTestUser()
	labelID := suite.createTestLabel("Test Label")

	err := suite.followService.Follow(user.ID, "label", labelID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityLabel, labelID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollow_Festival() {
	user := suite.createTestUser()
	festivalID := suite.createTestFestival("test-festival")

	err := suite.followService.Follow(user.ID, "festival", festivalID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityFestival, festivalID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FollowServiceIntegrationTestSuite) TestUnfollow_Success() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Unfollow Artist")

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	err := suite.followService.Unfollow(user.ID, "artist", artistID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityArtist, artistID, engagementm.BookmarkActionFollow).
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

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

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

	suite.Require().NoError(suite.followService.Follow(user1.ID, "artist", artistID))
	suite.Require().NoError(suite.followService.Follow(user2.ID, "artist", artistID))
	suite.Require().NoError(suite.followService.Follow(user3.ID, "artist", artistID))

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

	suite.Require().NoError(suite.followService.Follow(user1.ID, "artist", artist1ID))
	suite.Require().NoError(suite.followService.Follow(user2.ID, "artist", artist1ID))
	suite.Require().NoError(suite.followService.Follow(user1.ID, "artist", artist2ID))

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

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artist1ID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artist3ID))

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

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artist1ID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artist2ID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "venue", venueID))

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

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "venue", venueID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "label", labelID))

	following, total, err := suite.followService.GetUserFollowing(user.ID, "", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(following, 3)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		artistID := suite.createTestArtist(fmt.Sprintf("Paginated Artist %d", i))
		suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
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

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artist1ID))
	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artist2ID))

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

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

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

func (suite *FollowServiceIntegrationTestSuite) TestGetLibraryFollowingCounts_AllTypesInOneResult() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Count Artist")
	venueID := suite.createTestVenue("Count Venue")
	labelID := suite.createTestLabel("Count Label")
	festivalID := suite.createTestFestival("Count Festival")
	tagID := suite.createTestTag("shoegaze")
	scene := &catalogm.Scene{City: "Phoenix", State: "AZ", Slug: "phoenix-az"}
	suite.Require().NoError(suite.db.Create(scene).Error)

	for entityType, entityID := range map[string]uint{
		"artist": artistID, "venue": venueID, "scene": scene.ID,
		"label": labelID, "festival": festivalID, "tag": tagID,
	} {
		suite.Require().NoError(suite.followService.Follow(user.ID, entityType, entityID))
	}
	// Generic follows predate entity-existence enforcement. Library counts must
	// match the inner-joined list even if an orphan bookmark survives.
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", 999999))

	counts, err := suite.followService.GetLibraryFollowingCounts(user.ID)
	suite.Require().NoError(err)
	suite.Equal(&contracts.LibraryFollowingCounts{
		Artists: 1, Venues: 1, Scenes: 1, Labels: 1, Festivals: 1, Tags: 1,
	}, counts)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetLibraryFollowing_AlphabeticalStablePagination() {
	user := suite.createTestUser()
	ids := []uint{
		suite.createTestArtist("zebra"),
		suite.createTestArtist("Alpha"),
		suite.createTestArtist("aardvark"),
		suite.createTestArtist("Beta"),
	}
	for _, id := range ids {
		suite.Require().NoError(suite.followService.Follow(user.ID, "artist", id))
	}

	page1, cursor, err := suite.followService.GetLibraryFollowing(user.ID, "artist", 2, nil)
	suite.Require().NoError(err)
	suite.Require().Len(page1, 2)
	suite.Require().NotNil(cursor)
	suite.Equal([]string{"aardvark", "Alpha"}, []string{page1[0].Name, page1[1].Name})

	// A new row before the cursor must not duplicate or shift page two.
	earlyID := suite.createTestArtist("AAA")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", earlyID))

	page2, nextCursor, err := suite.followService.GetLibraryFollowing(user.ID, "artist", 2, cursor)
	suite.Require().NoError(err)
	suite.Require().Len(page2, 2)
	suite.Nil(nextCursor)
	suite.Equal([]string{"Beta", "zebra"}, []string{page2[0].Name, page2[1].Name})

	legacy, _, err := suite.followService.GetUserFollowing(user.ID, "artist", 4, 0)
	suite.Require().NoError(err)
	suite.NotEqual([]string{"aardvark", "Alpha", "Beta", "zebra"}, []string{
		legacy[0].Name, legacy[1].Name, legacy[2].Name, legacy[3].Name,
	})
}

// =============================================================================
// Group 6b: Scene follow notify mode (PSY-1341, +off in PSY-1466)
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestSceneNotifyMode_DefaultsToAllWhenUnset() {
	user := suite.createTestUser()
	sceneID := suite.createTestScene("Phoenix", "AZ", "phoenix-az")
	suite.Require().NoError(suite.followService.Follow(user.ID, "scene", sceneID))

	mode, err := suite.followService.SceneNotifyMode(user.ID, sceneID)
	suite.Require().NoError(err)
	suite.Equal(SceneNotifyModeAll, mode)
}

func (suite *FollowServiceIntegrationTestSuite) TestSceneNotifyMode_NoFollowDefaultsToAll() {
	user := suite.createTestUser()
	sceneID := suite.createTestScene("Tucson", "AZ", "tucson-az")

	mode, err := suite.followService.SceneNotifyMode(user.ID, sceneID)
	suite.Require().NoError(err)
	suite.Equal(SceneNotifyModeAll, mode)
}

// PSY-1466 AC: "off" round-trips through Set/Get.
func (suite *FollowServiceIntegrationTestSuite) TestSceneNotifyMode_OffRoundTrips() {
	user := suite.createTestUser()
	sceneID := suite.createTestScene("Flagstaff", "AZ", "flagstaff-az")
	suite.Require().NoError(suite.followService.Follow(user.ID, "scene", sceneID))

	suite.Require().NoError(suite.followService.SetSceneNotifyMode(user.ID, sceneID, SceneNotifyModeOff))

	mode, err := suite.followService.SceneNotifyMode(user.ID, sceneID)
	suite.Require().NoError(err)
	suite.Equal(SceneNotifyModeOff, mode)
}

func (suite *FollowServiceIntegrationTestSuite) TestSceneNotifyMode_SwitchingBetweenAllModes() {
	user := suite.createTestUser()
	sceneID := suite.createTestScene("Tempe", "AZ", "tempe-az")
	suite.Require().NoError(suite.followService.Follow(user.ID, "scene", sceneID))

	for _, mode := range []string{SceneNotifyModeFollowedBands, SceneNotifyModeOff, SceneNotifyModeAll} {
		suite.Require().NoError(suite.followService.SetSceneNotifyMode(user.ID, sceneID, mode))
		got, err := suite.followService.SceneNotifyMode(user.ID, sceneID)
		suite.Require().NoError(err)
		suite.Equal(mode, got)
	}
}

func (suite *FollowServiceIntegrationTestSuite) TestSetSceneNotifyMode_InvalidModeRejected() {
	user := suite.createTestUser()
	sceneID := suite.createTestScene("Mesa", "AZ", "mesa-az")
	suite.Require().NoError(suite.followService.Follow(user.ID, "scene", sceneID))

	err := suite.followService.SetSceneNotifyMode(user.ID, sceneID, "muted")
	suite.Error(err)
}

func (suite *FollowServiceIntegrationTestSuite) TestSetSceneNotifyMode_NoFollowIsError() {
	user := suite.createTestUser()
	sceneID := suite.createTestScene("Yuma", "AZ", "yuma-az")

	err := suite.followService.SetSceneNotifyMode(user.ID, sceneID, SceneNotifyModeOff)
	suite.Error(err)
}

// =============================================================================
// Group 7: GetFollowers
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_ReturnsUserInfo() {
	user := suite.createTestUserWithUsername("follower-user")
	artistID := suite.createTestArtist("Followed Artist")

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

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
		suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
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

	suite.Require().NoError(suite.followService.Follow(user.ID, "venue", venueID))

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

	suite.Require().NoError(suite.followService.Follow(user.ID, "festival", festivalID))

	following, total, err := suite.followService.GetUserFollowing(user.ID, "festival", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)
	suite.Equal("festival", following[0].EntityType)
	suite.Equal("summer-fest", following[0].Name)
	suite.Equal("summer-fest", following[0].Slug)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_TagNameSlug() {
	user := suite.createTestUser()
	tagID := suite.createTestTag("shoegaze")

	suite.Require().NoError(suite.followService.Follow(user.ID, "tag", tagID))

	following, total, err := suite.followService.GetUserFollowing(user.ID, "tag", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)
	suite.Equal("tag", following[0].EntityType)
	suite.Contains(following[0].Name, "shoegaze")
	suite.NotEmpty(following[0].Slug)
}

func (suite *FollowServiceIntegrationTestSuite) TestFollowUnfollow_TagIdempotent() {
	user := suite.createTestUser()
	tagID := suite.createTestTag("noise-rock")

	suite.Require().NoError(suite.followService.Follow(user.ID, "tag", tagID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "tag", tagID)) // idempotent

	ok, err := suite.followService.IsFollowing(user.ID, "tag", tagID)
	suite.Require().NoError(err)
	suite.True(ok)

	suite.Require().NoError(suite.followService.Unfollow(user.ID, "tag", tagID))
	suite.Require().NoError(suite.followService.Unfollow(user.ID, "tag", tagID)) // idempotent

	ok, err = suite.followService.IsFollowing(user.ID, "tag", tagID)
	suite.Require().NoError(err)
	suite.False(ok)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_LabelNameSlug() {
	user := suite.createTestUser()
	labelID := suite.createTestLabel("Sub Pop")

	suite.Require().NoError(suite.followService.Follow(user.ID, "label", labelID))

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

	suite.Require().NoError(suite.followService.Follow(user1.ID, "venue", venueID))
	suite.Require().NoError(suite.followService.Follow(user2.ID, "venue", venueID))

	followers, total, err := suite.followService.GetFollowers("venue", venueID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(followers, 2)
}

func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_FestivalEntityType() {
	user := suite.createTestUserWithUsername("fest-follower")
	festivalID := suite.createTestFestival("m3f-fest")

	suite.Require().NoError(suite.followService.Follow(user.ID, "festival", festivalID))

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

	suite.Require().NoError(suite.followService.Follow(user.ID, "venue", venue1ID))

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

// =============================================================================
// Group 10: Radio-show follow target (PSY-1356)
// =============================================================================

func (suite *FollowServiceIntegrationTestSuite) TestFollow_RadioShow() {
	user := suite.createTestUser()
	showID := suite.createTestRadioShow("Show A", "show-a", "Station A", "station-a", "Host A", "2026-07-01")

	err := suite.followService.Follow(user.ID, "radio_show", showID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityRadioShow, showID, engagementm.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

// TestGetUserFollowing_RadioShowEnriched pins AC #2: the radio-show row carries
// station_name, station_slug, host_name, and the MOST RECENT episode's air_date
// (not the first-inserted), on top of the base name/slug (the show's).
func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_RadioShowEnriched() {
	user := suite.createTestUser()
	// Episodes inserted out of order — the latest (2026-07-05) must win.
	showID := suite.createTestRadioShow(
		"Techtonic", "techtonic", "WFMU", "wfmu", "Gary the DJ",
		"2026-06-28", "2026-07-05", "2026-07-01",
	)

	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", showID))

	following, total, err := suite.followService.GetUserFollowing(user.ID, "radio_show", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(following, 1)

	f := following[0]
	suite.Equal("radio_show", f.EntityType)
	suite.Equal(showID, f.EntityID)
	suite.Equal("Techtonic", f.Name) // base name = show name
	suite.Equal("techtonic", f.Slug) // base slug = show slug
	suite.Require().NotNil(f.StationName)
	suite.Equal("WFMU", *f.StationName)
	suite.Require().NotNil(f.StationSlug)
	suite.Equal("wfmu", *f.StationSlug)
	suite.Require().NotNil(f.HostName)
	suite.Equal("Gary the DJ", *f.HostName)
	suite.Require().NotNil(f.LastEpisodeDate)
	// Exact YYYY-MM-DD (to_char pins the format) and the latest of the three.
	suite.Equal("2026-07-05", *f.LastEpisodeDate)
}

// The aired-gate: a followed show with a pre-published FUTURE episode must
// surface its most recent PAST episode as last_episode_date, not the future one
// (WFMU pre-publishes upcoming rows — the PSY-1374 footgun). No station tz here,
// so the gate resolves via the UTC fallback; +14d is unambiguously future in
// every zone.
func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_RadioShowExcludesFutureEpisode() {
	user := suite.createTestUser()
	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 14).Format("2006-01-02")
	showID := suite.createTestRadioShow("Prepubbed", "prepubbed", "WFMU", "wfmu", "DJ", past, future)

	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", showID))

	following, _, err := suite.followService.GetUserFollowing(user.ID, "radio_show", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(following, 1)
	suite.Require().NotNil(following[0].LastEpisodeDate)
	suite.Equal(past, *following[0].LastEpisodeDate, "future pre-published episode must not win last_episode_date")
}

// The gate must be STATION-LOCAL, not the DB's UTC date. This is a DETERMINISTIC
// regression guard: it uses two stations at opposite UTC extremes whose
// date-divergence windows union to a full 24h, so a revert to a bare UTC gate
// (e.g. `<= CURRENT_DATE`) fails at least one assertion regardless of what time
// of day CI runs.
//
//   - EAST station (Pacific/Kiritimati, UTC+14): its local date runs AHEAD of
//     UTC (differs for UTC 10:00–24:00). An episode dated the station's LOCAL
//     today must be INCLUDED; a UTC gate would drop it whenever the station date
//     is UTC+1.
//   - WEST station (Etc/GMT+12, UTC-12): its local date runs BEHIND UTC (differs
//     for UTC 00:00–12:00). An episode dated the station's LOCAL tomorrow must be
//     EXCLUDED (local yesterday is the latest aired); a UTC gate would admit it
//     whenever the station date is UTC-1.
//
// Dates are computed in each station's own tz to match what the SQL resolves.
func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_RadioShowStationLocalGateDeterministic() {
	user := suite.createTestUser()

	const eastTZ, westTZ = "Pacific/Kiritimati", "Etc/GMT+12" // UTC+14, UTC-12
	eastLoc, err := time.LoadLocation(eastTZ)
	suite.Require().NoError(err)
	westLoc, err := time.LoadLocation(westTZ)
	suite.Require().NoError(err)

	eastToday := time.Now().In(eastLoc).Format("2006-01-02")
	westYesterday := time.Now().In(westLoc).AddDate(0, 0, -1).Format("2006-01-02")
	westTomorrow := time.Now().In(westLoc).AddDate(0, 0, 1).Format("2006-01-02")

	eastShow := suite.createTestRadioShowTZ("East", "east-show", "East Station", "east-station", eastTZ, "", eastToday)
	westShow := suite.createTestRadioShowTZ("West", "west-show", "West Station", "west-station", westTZ, "", westYesterday, westTomorrow)
	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", eastShow))
	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", westShow))

	following, _, err := suite.followService.GetUserFollowing(user.ID, "radio_show", 10, 0)
	suite.Require().NoError(err)
	byID := make(map[uint]*contracts.FollowingEntityResponse, len(following))
	for _, f := range following {
		byID[f.EntityID] = f
	}

	suite.Require().NotNil(byID[eastShow], "east show must be in the following list")
	suite.Require().NotNil(byID[eastShow].LastEpisodeDate, "east station-local-today must be INCLUDED (UTC gate would drop it during the UTC+1 window)")
	suite.Equal(eastToday, *byID[eastShow].LastEpisodeDate)

	suite.Require().NotNil(byID[westShow], "west show must be in the following list")
	suite.Require().NotNil(byID[westShow].LastEpisodeDate)
	suite.Equal(westYesterday, *byID[westShow].LastEpisodeDate,
		"west station-local-tomorrow must be EXCLUDED (UTC gate would admit it during the UTC-1 window)")
}

// AC (c): follower count + follow status resolve for radio_show through the
// entity-type-generic paths (real DB — not a handler mock). These methods have
// no per-type switch, so this guards against a future refactor special-casing
// radio_show breaking count/status resolution.
func (suite *FollowServiceIntegrationTestSuite) TestRadioShow_CountAndStatusResolve() {
	user := suite.createTestUser()
	showID := suite.createTestRadioShow("Count Show", "count-show", "Count Station", "count-station", "DJ", "2026-07-02")

	isFollowing, err := suite.followService.IsFollowing(user.ID, "radio_show", showID)
	suite.Require().NoError(err)
	suite.False(isFollowing)

	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", showID))

	isFollowing, err = suite.followService.IsFollowing(user.ID, "radio_show", showID)
	suite.Require().NoError(err)
	suite.True(isFollowing)

	count, err := suite.followService.GetFollowerCount("radio_show", showID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), count)

	batchCounts, err := suite.followService.GetBatchFollowerCounts("radio_show", []uint{showID})
	suite.Require().NoError(err)
	suite.Equal(int64(1), batchCounts[showID])

	batchFollowing, err := suite.followService.GetBatchUserFollowing(user.ID, "radio_show", []uint{showID})
	suite.Require().NoError(err)
	suite.True(batchFollowing[showID])

	suite.Require().NoError(suite.followService.Unfollow(user.ID, "radio_show", showID))
	count, err = suite.followService.GetFollowerCount("radio_show", showID)
	suite.Require().NoError(err)
	suite.Equal(int64(0), count)
}

// A followed show with no episodes (and no host) yields the station fields but
// nil host_name / last_episode_date — the outer join must not drop the row.
func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_RadioShowNoEpisodesNoHost() {
	user := suite.createTestUser()
	showID := suite.createTestRadioShow("Fresh Show", "fresh-show", "KEXP", "kexp", "")

	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", showID))

	following, _, err := suite.followService.GetUserFollowing(user.ID, "radio_show", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(following, 1)

	f := following[0]
	suite.Equal("Fresh Show", f.Name)
	suite.Require().NotNil(f.StationName)
	suite.Equal("KEXP", *f.StationName)
	suite.Nil(f.HostName)        // host_name NULL → nil
	suite.Nil(f.LastEpisodeDate) // no episodes → nil
}

// Non-radio follows must never carry the radio-only fields (additive-only).
func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_NonRadioHasNilEnrichedFields() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Enriched Nil Artist")

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	following, _, err := suite.followService.GetUserFollowing(user.ID, "artist", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(following, 1)

	f := following[0]
	suite.Nil(f.StationName)
	suite.Nil(f.StationSlug)
	suite.Nil(f.HostName)
	suite.Nil(f.LastEpisodeDate)
}

// The all-types (no filter) path must enrich radio rows AND leave others' radio
// fields nil in the same response.
func (suite *FollowServiceIntegrationTestSuite) TestGetUserFollowing_AllTypesEnrichesOnlyRadio() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("Mixed Artist")
	showID := suite.createTestRadioShow("Mixed Show", "mixed-show", "Mixed Station", "mixed-station", "DJ Mixed", "2026-07-04")

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "radio_show", showID))

	following, total, err := suite.followService.GetUserFollowing(user.ID, "", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(following, 2)

	for _, f := range following {
		switch f.EntityType {
		case "radio_show":
			suite.Require().NotNil(f.StationSlug)
			suite.Equal("mixed-station", *f.StationSlug)
			suite.Require().NotNil(f.LastEpisodeDate)
			suite.Equal("2026-07-04", *f.LastEpisodeDate)
		case "artist":
			suite.Nil(f.StationSlug)
			suite.Nil(f.LastEpisodeDate)
		}
	}
}
