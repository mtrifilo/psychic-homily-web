package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewAttendanceService(t *testing.T) {
	svc := NewAttendanceService(nil)
	assert.NotNil(t, svc)
}

func TestAttendanceService_NilDatabase(t *testing.T) {
	svc := &AttendanceService{db: nil}

	t.Run("SetAttendance", func(t *testing.T) {
		err := svc.SetAttendance(1, 1, "going")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("RemoveAttendance", func(t *testing.T) {
		err := svc.RemoveAttendance(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetUserAttendance", func(t *testing.T) {
		status, err := svc.GetUserAttendance(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Empty(t, status)
	})

	t.Run("GetAttendanceCounts", func(t *testing.T) {
		counts, err := svc.GetAttendanceCounts(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, counts)
	})

	t.Run("GetBatchAttendanceCounts", func(t *testing.T) {
		result, err := svc.GetBatchAttendanceCounts([]uint{1, 2})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, result)
	})

	t.Run("GetBatchUserAttendance", func(t *testing.T) {
		result, err := svc.GetBatchUserAttendance(1, []uint{1, 2})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, result)
	})

	t.Run("GetUserAttendingShows", func(t *testing.T) {
		shows, total, err := svc.GetUserAttendingShows(1, "all", 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, shows)
		assert.Zero(t, total)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type AttendanceServiceIntegrationTestSuite struct {
	suite.Suite
	testDB            *testutil.TestDatabase
	db                *gorm.DB
	attendanceService *AttendanceService
}

func (suite *AttendanceServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.attendanceService = NewAttendanceService(suite.testDB.DB)
}

func (suite *AttendanceServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *AttendanceServiceIntegrationTestSuite) TearDownTest() {
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

func TestAttendanceServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AttendanceServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *AttendanceServiceIntegrationTestSuite) createApprovedShow(title string, userID uint) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7), // 1 week from now (upcoming)
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AttendanceServiceIntegrationTestSuite) createPastShow(title string, userID uint) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, -7), // 1 week ago
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AttendanceServiceIntegrationTestSuite) createShowWithVenue(title string, userID uint) (*models.Show, *models.Venue) {
	venue := &models.Venue{
		Name:  fmt.Sprintf("Venue for %s", title),
		City:  "Phoenix",
		State: "AZ",
	}
	suite.db.Create(venue)

	show := suite.createApprovedShow(title, userID)
	suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venue.ID})

	return show, venue
}

// =============================================================================
// Group 1: SetAttendance
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_Going() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Going Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	// Verify in DB
	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionGoing).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_Interested() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Interested Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "interested")
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionInterested).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ToggleGoingToInterested() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Toggle Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	err = suite.attendanceService.SetAttendance(user.ID, show.ID, "interested")
	suite.Require().NoError(err)

	var goingCount int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionGoing).
		Count(&goingCount)
	suite.Equal(int64(0), goingCount)

	var interestedCount int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionInterested).
		Count(&interestedCount)
	suite.Equal(int64(1), interestedCount)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ToggleInterestedToGoing() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Toggle Show 2", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "interested")
	suite.Require().NoError(err)

	err = suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	var interestedCount int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionInterested).
		Count(&interestedCount)
	suite.Equal(int64(0), interestedCount)

	var goingCount int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionGoing).
		Count(&goingCount)
	suite.Equal(int64(1), goingCount)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ClearWithEmptyString() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Clear Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	err = suite.attendanceService.SetAttendance(user.ID, show.ID, "")
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
			user.ID, models.BookmarkEntityShow, show.ID,
			[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested}).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_InvalidStatus() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Invalid Status Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "maybe")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid attendance status")
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ShowNotFound() {
	user := suite.createTestUser()

	err := suite.attendanceService.SetAttendance(user.ID, 99999, "going")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "show not found")
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_Idempotent() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Idempotent Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)
	err = suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionGoing).
		Count(&count)
	suite.Equal(int64(1), count)
}

// =============================================================================
// Group 2: RemoveAttendance
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestRemoveAttendance_RemovesBoth() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Remove Both Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	err = suite.attendanceService.RemoveAttendance(user.ID, show.ID)
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&models.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
			user.ID, models.BookmarkEntityShow, show.ID,
			[]models.BookmarkAction{models.BookmarkActionGoing, models.BookmarkActionInterested}).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestRemoveAttendance_NoExistingStatus() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Never Attended Show", user.ID)

	err := suite.attendanceService.RemoveAttendance(user.ID, show.ID)
	suite.Require().NoError(err)
}

// =============================================================================
// Group 3: GetUserAttendance
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendance_Going() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Get Going Show", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show.ID, "going")

	status, err := suite.attendanceService.GetUserAttendance(user.ID, show.ID)
	suite.Require().NoError(err)
	suite.Equal("going", status)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendance_Interested() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Get Interested Show", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show.ID, "interested")

	status, err := suite.attendanceService.GetUserAttendance(user.ID, show.ID)
	suite.Require().NoError(err)
	suite.Equal("interested", status)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendance_None() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Not Attending Show", user.ID)

	status, err := suite.attendanceService.GetUserAttendance(user.ID, show.ID)
	suite.Require().NoError(err)
	suite.Empty(status)
}

// =============================================================================
// Group 4: GetAttendanceCounts
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestGetAttendanceCounts_MultipleUsers() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	user3 := suite.createTestUser()
	show := suite.createApprovedShow("Count Show", user1.ID)

	suite.attendanceService.SetAttendance(user1.ID, show.ID, "going")
	suite.attendanceService.SetAttendance(user2.ID, show.ID, "going")
	suite.attendanceService.SetAttendance(user3.ID, show.ID, "interested")

	counts, err := suite.attendanceService.GetAttendanceCounts(show.ID)
	suite.Require().NoError(err)
	suite.Equal(show.ID, counts.ShowID)
	suite.Equal(2, counts.GoingCount)
	suite.Equal(1, counts.InterestedCount)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetAttendanceCounts_NoAttendance() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Empty Count Show", user.ID)

	counts, err := suite.attendanceService.GetAttendanceCounts(show.ID)
	suite.Require().NoError(err)
	suite.Equal(0, counts.GoingCount)
	suite.Equal(0, counts.InterestedCount)
}

// =============================================================================
// Group 5: GetBatchAttendanceCounts
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestGetBatchAttendanceCounts_MultipleShows() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	show1 := suite.createApprovedShow("Batch Show 1", user1.ID)
	show2 := suite.createApprovedShow("Batch Show 2", user1.ID)
	show3 := suite.createApprovedShow("Batch Show 3", user1.ID)

	suite.attendanceService.SetAttendance(user1.ID, show1.ID, "going")
	suite.attendanceService.SetAttendance(user2.ID, show1.ID, "interested")
	suite.attendanceService.SetAttendance(user1.ID, show2.ID, "interested")

	result, err := suite.attendanceService.GetBatchAttendanceCounts([]uint{show1.ID, show2.ID, show3.ID})
	suite.Require().NoError(err)
	suite.Len(result, 3)

	suite.Equal(1, result[show1.ID].GoingCount)
	suite.Equal(1, result[show1.ID].InterestedCount)
	suite.Equal(0, result[show2.ID].GoingCount)
	suite.Equal(1, result[show2.ID].InterestedCount)
	suite.Equal(0, result[show3.ID].GoingCount)
	suite.Equal(0, result[show3.ID].InterestedCount)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetBatchAttendanceCounts_Empty() {
	result, err := suite.attendanceService.GetBatchAttendanceCounts([]uint{})
	suite.Require().NoError(err)
	suite.Empty(result)
}

// =============================================================================
// Group 6: GetBatchUserAttendance
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestGetBatchUserAttendance_MixedStatuses() {
	user := suite.createTestUser()
	show1 := suite.createApprovedShow("Batch User Show 1", user.ID)
	show2 := suite.createApprovedShow("Batch User Show 2", user.ID)
	show3 := suite.createApprovedShow("Batch User Show 3", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show1.ID, "going")
	suite.attendanceService.SetAttendance(user.ID, show2.ID, "interested")

	result, err := suite.attendanceService.GetBatchUserAttendance(user.ID, []uint{show1.ID, show2.ID, show3.ID})
	suite.Require().NoError(err)

	suite.Equal("going", result[show1.ID])
	suite.Equal("interested", result[show2.ID])
	_, hasShow3 := result[show3.ID]
	suite.False(hasShow3)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetBatchUserAttendance_Empty() {
	user := suite.createTestUser()

	result, err := suite.attendanceService.GetBatchUserAttendance(user.ID, []uint{})
	suite.Require().NoError(err)
	suite.Empty(result)
}

// =============================================================================
// Group 7: GetUserAttendingShows
// =============================================================================

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_All() {
	user := suite.createTestUser()
	show1, _ := suite.createShowWithVenue("Attending Show 1", user.ID)
	show2, _ := suite.createShowWithVenue("Attending Show 2", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show1.ID, "going")
	suite.attendanceService.SetAttendance(user.ID, show2.ID, "interested")

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(shows, 2)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_FilterGoing() {
	user := suite.createTestUser()
	show1, _ := suite.createShowWithVenue("Going Only Show", user.ID)
	show2, _ := suite.createShowWithVenue("Interested Only Show", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show1.ID, "going")
	suite.attendanceService.SetAttendance(user.ID, show2.ID, "interested")

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "going", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(shows, 1)
	suite.Equal("going", shows[0].Status)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_FilterInterested() {
	user := suite.createTestUser()
	show1, _ := suite.createShowWithVenue("Going Show for Filter", user.ID)
	show2, _ := suite.createShowWithVenue("Interested Show for Filter", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show1.ID, "going")
	suite.attendanceService.SetAttendance(user.ID, show2.ID, "interested")

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "interested", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(shows, 1)
	suite.Equal("interested", shows[0].Status)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_OnlyUpcoming() {
	user := suite.createTestUser()
	upcomingShow, _ := suite.createShowWithVenue("Upcoming Show", user.ID)
	pastShow := suite.createPastShow("Past Show", user.ID)

	suite.attendanceService.SetAttendance(user.ID, upcomingShow.ID, "going")
	suite.attendanceService.SetAttendance(user.ID, pastShow.ID, "going")

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(shows, 1)
	suite.Equal(upcomingShow.ID, shows[0].ShowID)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_OnlyApproved() {
	user := suite.createTestUser()
	approvedShow, _ := suite.createShowWithVenue("Approved Show", user.ID)

	pendingShow := &models.Show{
		Title:       "Pending Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusPending,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(pendingShow)

	suite.attendanceService.SetAttendance(user.ID, approvedShow.ID, "going")
	suite.attendanceService.SetAttendance(user.ID, pendingShow.ID, "going")

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(shows, 1)
	suite.Equal(approvedShow.ID, shows[0].ShowID)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		show, _ := suite.createShowWithVenue(fmt.Sprintf("Paginated Show %d", i), user.ID)
		suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	}

	shows1, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(shows1, 2)

	shows2, _, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 2, 2)
	suite.Require().NoError(err)
	suite.Len(shows2, 2)

	shows3, _, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 2, 4)
	suite.Require().NoError(err)
	suite.Len(shows3, 1)

	suite.NotEqual(shows1[0].ShowID, shows2[0].ShowID)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_IncludesVenueInfo() {
	user := suite.createTestUser()
	show, venue := suite.createShowWithVenue("Venue Info Show", user.ID)

	suite.attendanceService.SetAttendance(user.ID, show.ID, "going")

	shows, _, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(shows, 1)
	suite.Require().NotNil(shows[0].VenueName)
	suite.Equal(venue.Name, *shows[0].VenueName)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_Empty() {
	user := suite.createTestUser()

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(shows)
}
