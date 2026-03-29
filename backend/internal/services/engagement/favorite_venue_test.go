package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewFavoriteVenueService(t *testing.T) {
	svc := NewFavoriteVenueService(nil)
	assert.NotNil(t, svc)
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type FavoriteVenueServiceIntegrationTestSuite struct {
	suite.Suite
	testDB               *testutil.TestDatabase
	db                   *gorm.DB
	favoriteVenueService *FavoriteVenueService
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.favoriteVenueService = NewFavoriteVenueService(suite.testDB.DB)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestFavoriteVenueServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FavoriteVenueServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *FavoriteVenueServiceIntegrationTestSuite) createTestVenue(name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:  name,
		City:  city,
		State: state,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) createApprovedShowAtVenue(title string, venueID, userID uint, eventDate time.Time) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	err = suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error
	suite.Require().NoError(err)

	return show
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) createShowWithArtistAtVenue(title string, venueID, userID uint, eventDate time.Time, artistName string) (*models.Show, *models.Artist) {
	show := suite.createApprovedShowAtVenue(title, venueID, userID, eventDate)

	artist := &models.Artist{Name: artistName}
	suite.db.Create(artist)
	suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0})

	return show, artist
}

// =============================================================================
// Group 1: FavoriteVenue
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestFavoriteVenue_Success() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Favorite Me", "Phoenix", "AZ")

	err := suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, models.BookmarkEntityVenue, venue.ID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestFavoriteVenue_VenueNotFound() {
	user := suite.createTestUser()

	err := suite.favoriteVenueService.FavoriteVenue(user.ID, 99999)

	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestFavoriteVenue_Idempotent() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Idempotent Fav", "Phoenix", "AZ")

	err := suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)
	suite.Require().NoError(err)

	// Favorite again — should not error
	err = suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, models.BookmarkEntityVenue, venue.ID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(1), count)
}

// =============================================================================
// Group 2: UnfavoriteVenue
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestUnfavoriteVenue_Success() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Unfavorite Me", "Phoenix", "AZ")

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	err := suite.favoriteVenueService.UnfavoriteVenue(user.ID, venue.ID)

	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, models.BookmarkEntityVenue, venue.ID, models.BookmarkActionFollow).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestUnfavoriteVenue_NotFavorited() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Never Favorited", "Phoenix", "AZ")

	err := suite.favoriteVenueService.UnfavoriteVenue(user.ID, venue.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "venue was not favorited")
}

// =============================================================================
// Group 3: GetUserFavoriteVenues
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUserFavoriteVenues_Success() {
	user := suite.createTestUser()
	venue1 := suite.createTestVenue("Fav Venue 1", "Phoenix", "AZ")
	venue2 := suite.createTestVenue("Fav Venue 2", "Tempe", "AZ")

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue1.ID)
	time.Sleep(10 * time.Millisecond)
	suite.favoriteVenueService.FavoriteVenue(user.ID, venue2.ID)

	resp, total, err := suite.favoriteVenueService.GetUserFavoriteVenues(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(resp, 2)
	// Most recently favorited first
	suite.Equal(venue2.ID, resp[0].ID)
	suite.Equal(venue1.ID, resp[1].ID)
	suite.NotZero(resp[0].FavoritedAt)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUserFavoriteVenues_Empty() {
	user := suite.createTestUser()

	resp, total, err := suite.favoriteVenueService.GetUserFavoriteVenues(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUserFavoriteVenues_WithUpcomingShowCount() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Shows Venue", "Phoenix", "AZ")

	// Create 2 upcoming approved shows at the venue
	suite.createApprovedShowAtVenue("Future Show 1", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowAtVenue("Future Show 2", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 14))

	// Create 1 past show (should not count)
	suite.createApprovedShowAtVenue("Past Show", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	resp, _, err := suite.favoriteVenueService.GetUserFavoriteVenues(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(2, resp[0].UpcomingShowCount)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUserFavoriteVenues_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		venue := suite.createTestVenue(fmt.Sprintf("Paginated Venue %d", i), "Phoenix", "AZ")
		suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)
		time.Sleep(5 * time.Millisecond)
	}

	resp1, total, err := suite.favoriteVenueService.GetUserFavoriteVenues(user.ID, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	resp2, _, err := suite.favoriteVenueService.GetUserFavoriteVenues(user.ID, 2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	resp3, _, err := suite.favoriteVenueService.GetUserFavoriteVenues(user.ID, 2, 4)
	suite.Require().NoError(err)
	suite.Len(resp3, 1)

	suite.NotEqual(resp1[0].ID, resp2[0].ID)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUserFavoriteVenues_OnlyOwnFavorites() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	venue1 := suite.createTestVenue("User1 Venue", "Phoenix", "AZ")
	venue2 := suite.createTestVenue("User2 Venue", "Tempe", "AZ")

	suite.favoriteVenueService.FavoriteVenue(user1.ID, venue1.ID)
	suite.favoriteVenueService.FavoriteVenue(user2.ID, venue2.ID)

	resp, total, err := suite.favoriteVenueService.GetUserFavoriteVenues(user1.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal(venue1.ID, resp[0].ID)
}

// =============================================================================
// Group 4: IsVenueFavorited
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestIsVenueFavorited_True() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Fav Check", "Phoenix", "AZ")

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	fav, err := suite.favoriteVenueService.IsVenueFavorited(user.ID, venue.ID)

	suite.Require().NoError(err)
	suite.True(fav)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestIsVenueFavorited_False() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Not Fav Check", "Phoenix", "AZ")

	fav, err := suite.favoriteVenueService.IsVenueFavorited(user.ID, venue.ID)

	suite.Require().NoError(err)
	suite.False(fav)
}

// =============================================================================
// Group 5: GetUpcomingShowsFromFavorites
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_Success() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Upcoming Fav Venue", "Phoenix", "AZ")

	suite.createApprovedShowAtVenue("Future Show", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowAtVenue("Past Show", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	resp, total, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Future Show", resp[0].Title)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_IncludesVenueInfo() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Venue Info Venue", "Phoenix", "AZ")

	suite.createApprovedShowAtVenue("Venue Show", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	resp, _, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(venue.ID, resp[0].VenueID)
	suite.Equal("Venue Info Venue", resp[0].VenueName)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_IncludesArtists() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Artist Venue", "Phoenix", "AZ")

	suite.createShowWithArtistAtVenue("Artist Show", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7), "Cool Band")
	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	resp, _, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().Len(resp[0].Artists, 1)
	suite.Equal("Cool Band", resp[0].Artists[0].Name)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_NoFavorites() {
	user := suite.createTestUser()

	resp, total, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_ExcludesNonApproved() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Approved Only Venue", "Phoenix", "AZ")

	// Create approved show
	suite.createApprovedShowAtVenue("Approved Show", venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	// Create pending show at same venue
	pendingShow := &models.Show{
		Title:       "Pending Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 14),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusPending,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(pendingShow)
	suite.db.Create(&models.ShowVenue{ShowID: pendingShow.ID, VenueID: venue.ID})

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	resp, total, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("Approved Show", resp[0].Title)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_MultipleVenues() {
	user := suite.createTestUser()
	venue1 := suite.createTestVenue("Multi Venue 1", "Phoenix", "AZ")
	venue2 := suite.createTestVenue("Multi Venue 2", "Tempe", "AZ")

	suite.createApprovedShowAtVenue("Show at V1", venue1.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowAtVenue("Show at V2", venue2.ID, user.ID, time.Now().UTC().AddDate(0, 0, 14))

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue1.ID)
	suite.favoriteVenueService.FavoriteVenue(user.ID, venue2.ID)

	resp, total, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
	// Sorted by event_date ASC (soonest first)
	suite.Equal("Show at V1", resp[0].Title)
	suite.Equal("Show at V2", resp[1].Title)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetUpcomingShowsFromFavorites_Pagination() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Paginated Fav Venue", "Phoenix", "AZ")

	for i := 1; i <= 5; i++ {
		suite.createApprovedShowAtVenue(
			fmt.Sprintf("Fav Show %d", i),
			venue.ID, user.ID,
			time.Now().UTC().AddDate(0, 0, i),
		)
	}

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue.ID)

	resp1, total, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	resp2, _, err := suite.favoriteVenueService.GetUpcomingShowsFromFavorites(user.ID, "UTC", 2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	suite.NotEqual(resp1[0].ID, resp2[0].ID)
}

// =============================================================================
// Group 6: GetFavoriteVenueIDs
// =============================================================================

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetFavoriteVenueIDs_Success() {
	user := suite.createTestUser()
	venue1 := suite.createTestVenue("Batch Venue 1", "Phoenix", "AZ")
	venue2 := suite.createTestVenue("Batch Venue 2", "Tempe", "AZ")
	venue3 := suite.createTestVenue("Batch Venue 3", "Mesa", "AZ")

	suite.favoriteVenueService.FavoriteVenue(user.ID, venue1.ID)
	suite.favoriteVenueService.FavoriteVenue(user.ID, venue3.ID)

	result, err := suite.favoriteVenueService.GetFavoriteVenueIDs(user.ID, []uint{venue1.ID, venue2.ID, venue3.ID})

	suite.Require().NoError(err)
	suite.True(result[venue1.ID])
	suite.False(result[venue2.ID])
	suite.True(result[venue3.ID])
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetFavoriteVenueIDs_EmptyInput() {
	user := suite.createTestUser()

	result, err := suite.favoriteVenueService.GetFavoriteVenueIDs(user.ID, []uint{})

	suite.Require().NoError(err)
	suite.Empty(result)
}

func (suite *FavoriteVenueServiceIntegrationTestSuite) TestGetFavoriteVenueIDs_NoneMatched() {
	user := suite.createTestUser()
	venue := suite.createTestVenue("Unmatched Venue", "Phoenix", "AZ")

	result, err := suite.favoriteVenueService.GetFavoriteVenueIDs(user.ID, []uint{venue.ID})

	suite.Require().NoError(err)
	suite.False(result[venue.ID])
}
