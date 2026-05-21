package engagement

import (
	"fmt"
	"sync"
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

	// Bound the connection pool. SetAttendance runs inside an explicit
	// transaction, so each concurrent call holds a connection for its whole
	// span. TestSetAttendance_ConcurrentIdempotent fires many calls at once;
	// without a cap the pool opens a connection per goroutine and exhausts the
	// container's max_connections (53300) before the unique-violation race can
	// even be exercised. A modest cap queues goroutines at the pool (the
	// realistic production shape) while still letting enough transactions
	// interleave to trip a SELECT-then-INSERT race.
	if sqlDB, err := suite.db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(25)
	}

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

func (suite *AttendanceServiceIntegrationTestSuite) createTestUser() *authm.User {
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

func (suite *AttendanceServiceIntegrationTestSuite) createApprovedShow(title string, userID uint) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7), // 1 week from now (upcoming)
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AttendanceServiceIntegrationTestSuite) createPastShow(title string, userID uint) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, -7), // 1 week ago
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AttendanceServiceIntegrationTestSuite) createShowWithVenue(title string, userID uint) (*catalogm.Show, *catalogm.Venue) {
	venue := &catalogm.Venue{
		Name:  fmt.Sprintf("Venue for %s", title),
		City:  "Phoenix",
		State: "AZ",
	}
	suite.db.Create(venue)

	show := suite.createApprovedShow(title, userID)
	suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID})

	return show, venue
}

// createShowWithVenues creates an approved upcoming show linked to venueCount
// distinct venues via show_venues. Used to reproduce pagination drift, where a
// multi-venue show fans out into multiple joined rows.
func (suite *AttendanceServiceIntegrationTestSuite) createShowWithVenues(title string, userID uint, venueCount int) *catalogm.Show {
	show := suite.createApprovedShow(title, userID)
	for v := 0; v < venueCount; v++ {
		venue := &catalogm.Venue{
			Name:  fmt.Sprintf("%s venue %d", title, v),
			City:  "Phoenix",
			State: "AZ",
		}
		suite.Require().NoError(suite.db.Create(venue).Error)
		suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)
	}
	return show
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_Interested() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Interested Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "interested")
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionInterested).
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing).
		Count(&goingCount)
	suite.Equal(int64(0), goingCount)

	var interestedCount int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionInterested).
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionInterested).
		Count(&interestedCount)
	suite.Equal(int64(0), interestedCount)

	var goingCount int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing).
		Count(&goingCount)
	suite.Equal(int64(1), goingCount)
}

// TestSetAttendance_ToggleLeavesExactlyOneRow asserts the going-XOR-interested
// invariant directly: after toggling going → interested, the user has exactly
// ONE attendance row for the show (not one of each). Guards GetAttendanceCounts
// against double-counting.
func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ToggleLeavesExactlyOneRow() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("XOR Toggle Show", user.ID)

	suite.Require().NoError(suite.attendanceService.SetAttendance(user.ID, show.ID, "going"))
	suite.Require().NoError(suite.attendanceService.SetAttendance(user.ID, show.ID, "interested"))

	var total int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID,
			[]engagementm.BookmarkAction{engagementm.BookmarkActionGoing, engagementm.BookmarkActionInterested}).
		Count(&total)
	suite.Equal(int64(1), total, "user must have exactly one attendance row after a toggle")
}

// TestSetAttendance_OppositeDeleteErrorRollsBack asserts the transaction rolls
// back when the opposite-status DELETE fails. Before the fix the DELETE error
// was discarded, so the upsert still ran and the user ended with BOTH going and
// interested rows. A failing Delete callback simulates a lock timeout / FK
// trigger / dropped connection on that statement.
func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_OppositeDeleteErrorRollsBack() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Delete Error Show", user.ID)

	// Seed the opposite status so the toggle has a row to delete.
	suite.Require().NoError(suite.attendanceService.SetAttendance(user.ID, show.ID, "going"))

	// Inject a DELETE failure scoped to user_bookmarks. Callbacks live on the
	// shared connection processor, so remove it after the test for isolation.
	const cbName = "test:fail_user_bookmarks_delete"
	err := suite.db.Callback().Delete().Before("gorm:delete").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_bookmarks" {
			_ = tx.AddError(fmt.Errorf("simulated delete failure"))
		}
	})
	suite.Require().NoError(err)
	defer func() {
		suite.Require().NoError(suite.db.Callback().Delete().Remove(cbName))
	}()

	// Toggling to interested must trigger the opposite (going) DELETE and fail.
	setErr := suite.attendanceService.SetAttendance(user.ID, show.ID, "interested")
	suite.Require().Error(setErr)
	suite.Contains(setErr.Error(), "failed to remove opposite attendance")

	// State unchanged: still exactly the original going row, no interested row.
	// (These reads are SELECTs, so the registered DELETE-fail callback is inert.)
	var goingCount, interestedCount int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing).
		Count(&goingCount)
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionInterested).
		Count(&interestedCount)
	suite.Equal(int64(1), goingCount, "original going row must survive the rolled-back toggle")
	suite.Equal(int64(0), interestedCount, "interested row must not be created when DELETE fails")
}

func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ClearWithEmptyString() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Clear Show", user.ID)

	err := suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
	suite.Require().NoError(err)

	err = suite.attendanceService.SetAttendance(user.ID, show.ID, "")
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID,
			[]engagementm.BookmarkAction{engagementm.BookmarkActionGoing, engagementm.BookmarkActionInterested}).
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing).
		Count(&count)
	suite.Equal(int64(1), count)
}

// TestSetAttendance_ConcurrentIdempotent fires N parallel SetAttendance("going")
// calls for the same (user, show) and asserts none errors and exactly one row
// lands. PSY-755: the insert leg used FirstOrCreate (SELECT-then-INSERT) inside
// the transaction, so two racing taps could both miss the row and both INSERT,
// surfacing the 23505 unique violation. ON CONFLICT DO NOTHING makes the insert
// leg idempotent. The opposite-status DELETE + transaction rollback (PSY-753)
// stay intact and are unaffected here since no opposite row exists.
func (suite *AttendanceServiceIntegrationTestSuite) TestSetAttendance_ConcurrentIdempotent() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Concurrent Attendance Show", user.ID)

	const n = 150
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // release all goroutines together to maximize contention
			errs[idx] = suite.attendanceService.SetAttendance(user.ID, show.ID, "going")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		suite.NoError(err, "concurrent SetAttendance #%d must not surface a unique violation", i)
	}

	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing).
		Count(&count)
	suite.Equal(int64(1), count, "concurrent going taps must collapse to exactly one row")
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action IN ?",
			user.ID, engagementm.BookmarkEntityShow, show.ID,
			[]engagementm.BookmarkAction{engagementm.BookmarkActionGoing, engagementm.BookmarkActionInterested}).
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

	pendingShow := &catalogm.Show{
		Title:       "Pending Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusPending,
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

// TestGetUserAttendingShows_PaginationWithMultiVenueShows reproduces the
// pagination drift bug: 25 attending shows, 5 of which have 2 venues each.
// Before the fix, LIMIT/OFFSET applied to the venue-joined row stream pulled
// fewer than N distinct shows when multi-venue shows landed in a page. After
// the fix, page 1 returns 20 distinct shows and page 2 returns the remaining
// 5, with no show appearing on both pages.
func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_PaginationWithMultiVenueShows() {
	user := suite.createTestUser()

	const totalShows = 25
	const multiVenueShows = 5
	for i := 0; i < totalShows; i++ {
		var show *catalogm.Show
		if i < multiVenueShows {
			show = suite.createShowWithVenues(fmt.Sprintf("Multi Venue Show %02d", i), user.ID, 2)
		} else {
			show, _ = suite.createShowWithVenue(fmt.Sprintf("Single Venue Show %02d", i), user.ID)
		}
		suite.Require().NoError(suite.attendanceService.SetAttendance(user.ID, show.ID, "going"))
	}

	page1, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(totalShows), total)
	suite.Require().Len(page1, 20, "page 1 must return 20 distinct shows despite multi-venue fan-out")
	suite.assertDistinctShowIDs(page1)

	page2, _, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 20, 20)
	suite.Require().NoError(err)
	suite.Require().Len(page2, 5, "page 2 must return the remaining 5 distinct shows")
	suite.assertDistinctShowIDs(page2)

	// No show may appear on both pages.
	page1IDs := make(map[uint]bool, len(page1))
	for _, s := range page1 {
		page1IDs[s.ShowID] = true
	}
	for _, s := range page2 {
		suite.False(page1IDs[s.ShowID], "show %d appeared on both pages", s.ShowID)
	}

	// The two pages together cover all 25 distinct shows exactly once.
	suite.Equal(totalShows, len(page1)+len(page2))
}

// assertDistinctShowIDs fails if any show ID repeats within a page.
func (suite *AttendanceServiceIntegrationTestSuite) assertDistinctShowIDs(shows []*contracts.AttendingShowResponse) {
	seen := make(map[uint]bool, len(shows))
	for _, s := range shows {
		suite.False(seen[s.ShowID], "duplicate show %d within a single page", s.ShowID)
		seen[s.ShowID] = true
	}
}

func (suite *AttendanceServiceIntegrationTestSuite) TestGetUserAttendingShows_Empty() {
	user := suite.createTestUser()

	shows, total, err := suite.attendanceService.GetUserAttendingShows(user.ID, "all", 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(shows)
}
