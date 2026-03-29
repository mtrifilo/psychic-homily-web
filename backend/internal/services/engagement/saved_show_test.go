package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestSavedShowService_NilDatabase(t *testing.T) {
	svc := &SavedShowService{db: nil}

	t.Run("SaveShow", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.SaveShow(1, 1)
		})
	})

	t.Run("UnsaveShow", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.UnsaveShow(1, 1)
		})
	})

	t.Run("GetUserSavedShows", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetUserSavedShows(1, 10, 0)
			return err
		})
	})

	t.Run("IsShowSaved", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.IsShowSaved(1, 1)
		})
	})

	t.Run("GetSavedShowIDs", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetSavedShowIDs(1, []uint{1, 2})
		})
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type SavedShowServiceIntegrationTestSuite struct {
	suite.Suite
	testDB           *testutil.TestDatabase
	db               *gorm.DB
	savedShowService *SavedShowService
}

func (suite *SavedShowServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.savedShowService = NewSavedShowService(suite.testDB.DB)
}

func (suite *SavedShowServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *SavedShowServiceIntegrationTestSuite) TearDownTest() {
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

func TestSavedShowServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SavedShowServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *SavedShowServiceIntegrationTestSuite) createApprovedShow(title string, userID uint) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *SavedShowServiceIntegrationTestSuite) createShowWithVenueAndArtist(title string, userID uint) (*models.Show, *models.Venue, *models.Artist) {
	venue := &models.Venue{
		Name:  fmt.Sprintf("Venue for %s", title),
		City:  "Phoenix",
		State: "AZ",
	}
	suite.db.Create(venue)

	artist := &models.Artist{Name: fmt.Sprintf("Artist for %s", title)}
	suite.db.Create(artist)

	show := suite.createApprovedShow(title, userID)

	suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venue.ID})
	suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0})

	return show, venue, artist
}

// =============================================================================
// Group 1: SaveShow
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_Success() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Save Me", user.ID)

	err := suite.savedShowService.SaveShow(user.ID, show.ID)

	suite.Require().NoError(err)

	// Verify in DB
	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_ShowNotFound() {
	user := suite.createTestUser()

	err := suite.savedShowService.SaveShow(user.ID, 99999)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_Idempotent() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Idempotent Save", user.ID)

	err := suite.savedShowService.SaveShow(user.ID, show.ID)
	suite.Require().NoError(err)

	// Save again — should not error (FirstOrCreate)
	err = suite.savedShowService.SaveShow(user.ID, show.ID)
	suite.Require().NoError(err)

	// Should still only be one record
	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(1), count)
}

// =============================================================================
// Group 2: UnsaveShow
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestUnsaveShow_Success() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Unsave Me", user.ID)

	suite.savedShowService.SaveShow(user.ID, show.ID)

	err := suite.savedShowService.UnsaveShow(user.ID, show.ID)

	suite.Require().NoError(err)

	// Verify removed
	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestUnsaveShow_NotSaved() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Never Saved", user.ID)

	err := suite.savedShowService.UnsaveShow(user.ID, show.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "show was not saved")
}

// =============================================================================
// Group 3: GetUserSavedShows
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_Success() {
	user := suite.createTestUser()
	show1, _, _ := suite.createShowWithVenueAndArtist("Saved Show 1", user.ID)
	show2, _, _ := suite.createShowWithVenueAndArtist("Saved Show 2", user.ID)

	suite.savedShowService.SaveShow(user.ID, show1.ID)
	time.Sleep(10 * time.Millisecond) // ensure different saved_at
	suite.savedShowService.SaveShow(user.ID, show2.ID)

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(resp, 2)
	// Most recently saved first
	suite.Equal(show2.ID, resp[0].ID)
	suite.Equal(show1.ID, resp[1].ID)
	suite.NotZero(resp[0].SavedAt)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_Empty() {
	user := suite.createTestUser()

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_IncludesVenueAndArtist() {
	user := suite.createTestUser()
	show, venue, artist := suite.createShowWithVenueAndArtist("Full Show", user.ID)

	suite.savedShowService.SaveShow(user.ID, show.ID)

	resp, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().Len(resp[0].Venues, 1)
	suite.Equal(venue.ID, resp[0].Venues[0].ID)
	suite.Require().Len(resp[0].Artists, 1)
	suite.Equal(artist.ID, resp[0].Artists[0].ID)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("Paginated Show %d", i), user.ID)
		suite.savedShowService.SaveShow(user.ID, show.ID)
		time.Sleep(5 * time.Millisecond)
	}

	// Page 1
	resp1, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// Page 3
	resp3, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 4)
	suite.Require().NoError(err)
	suite.Len(resp3, 1)

	// No overlap
	suite.NotEqual(resp1[0].ID, resp2[0].ID)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_OnlyOwnShows() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	show1 := suite.createApprovedShow("User1 Show", user1.ID)
	show2 := suite.createApprovedShow("User2 Show", user2.ID)

	suite.savedShowService.SaveShow(user1.ID, show1.ID)
	suite.savedShowService.SaveShow(user2.ID, show2.ID)

	resp, total, err := suite.savedShowService.GetUserSavedShows(user1.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal(show1.ID, resp[0].ID)
}

// =============================================================================
// Group 4: IsShowSaved
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestIsShowSaved_True() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Saved Check", user.ID)

	suite.savedShowService.SaveShow(user.ID, show.ID)

	saved, err := suite.savedShowService.IsShowSaved(user.ID, show.ID)

	suite.Require().NoError(err)
	suite.True(saved)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestIsShowSaved_False() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Not Saved Check", user.ID)

	saved, err := suite.savedShowService.IsShowSaved(user.ID, show.ID)

	suite.Require().NoError(err)
	suite.False(saved)
}

// =============================================================================
// Group 5: GetSavedShowIDs
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSavedShowIDs_Success() {
	user := suite.createTestUser()
	show1 := suite.createApprovedShow("Batch Show 1", user.ID)
	show2 := suite.createApprovedShow("Batch Show 2", user.ID)
	show3 := suite.createApprovedShow("Batch Show 3", user.ID)

	suite.savedShowService.SaveShow(user.ID, show1.ID)
	suite.savedShowService.SaveShow(user.ID, show3.ID)

	result, err := suite.savedShowService.GetSavedShowIDs(user.ID, []uint{show1.ID, show2.ID, show3.ID})

	suite.Require().NoError(err)
	suite.True(result[show1.ID])
	suite.False(result[show2.ID])
	suite.True(result[show3.ID])
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSavedShowIDs_EmptyInput() {
	user := suite.createTestUser()

	result, err := suite.savedShowService.GetSavedShowIDs(user.ID, []uint{})

	suite.Require().NoError(err)
	suite.Empty(result)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSavedShowIDs_NoneMatched() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Unmatched Show", user.ID)

	result, err := suite.savedShowService.GetSavedShowIDs(user.ID, []uint{show.ID})

	suite.Require().NoError(err)
	suite.False(result[show.ID])
}
