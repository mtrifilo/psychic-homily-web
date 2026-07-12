package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/testutil"
)

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

func (suite *SavedShowServiceIntegrationTestSuite) createTestUser() *authm.User {
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

func (suite *SavedShowServiceIntegrationTestSuite) createApprovedShow(title string, userID uint) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *SavedShowServiceIntegrationTestSuite) createShowWithVenueAndArtist(title string, userID uint) (*catalogm.Show, *catalogm.Venue, *catalogm.Artist) {
	venue := &catalogm.Venue{
		Name:  fmt.Sprintf("Venue for %s", title),
		City:  "Phoenix",
		State: "AZ",
	}
	suite.db.Create(venue)

	artist := &catalogm.Artist{Name: fmt.Sprintf("Artist for %s", title)}
	suite.db.Create(artist)

	show := suite.createApprovedShow(title, userID)

	suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID})
	suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0})

	return show, venue, artist
}

// =============================================================================
// Group 1: SaveShow
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSaveCount_CountsEveryUsersSave() {
	owner := suite.createTestUser()
	show := suite.createApprovedShow("Buzzy Show", owner.ID)
	u1 := suite.createTestUser()
	u2 := suite.createTestUser()

	suite.Require().NoError(suite.savedShowService.SaveShow(u1.ID, show.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(u2.ID, show.ID))

	count, err := suite.savedShowService.GetSaveCount(show.ID)
	suite.Require().NoError(err)
	suite.Equal(2, count, "the public count aggregates saves across all users")
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSaveCount_ZeroForUnsavedShow() {
	owner := suite.createTestUser()
	show := suite.createApprovedShow("Nobody Saved Me", owner.ID)

	count, err := suite.savedShowService.GetSaveCount(show.ID)
	suite.Require().NoError(err)
	suite.Equal(0, count)
}

// Guards the action/entity_type filter: `follow` and `bookmark` rows live in the
// same user_bookmarks table and must never inflate a show's save count.
func (suite *SavedShowServiceIntegrationTestSuite) TestGetSaveCount_IgnoresOtherActionsAndEntities() {
	owner := suite.createTestUser()
	show := suite.createApprovedShow("Filtered Show", owner.ID)
	user := suite.createTestUser()

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))
	// Same entity_id, different action.
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: user.ID, EntityType: engagementm.BookmarkEntityShow,
		EntityID: show.ID, Action: engagementm.BookmarkActionBookmark,
	}).Error)
	// Same entity_id + action, different entity_type.
	suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
		UserID: user.ID, EntityType: engagementm.BookmarkEntityArtist,
		EntityID: show.ID, Action: engagementm.BookmarkActionSave,
	}).Error)

	count, err := suite.savedShowService.GetSaveCount(show.ID)
	suite.Require().NoError(err)
	suite.Equal(1, count, "only entity_type=show AND action=save counts")
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetBatchSaveCounts_ZeroFillsRequestedShows() {
	owner := suite.createTestUser()
	saved := suite.createApprovedShow("Saved Show", owner.ID)
	unsaved := suite.createApprovedShow("Unsaved Show", owner.ID)
	user := suite.createTestUser()

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, saved.ID))

	counts, err := suite.savedShowService.GetBatchSaveCounts([]uint{saved.ID, unsaved.ID})
	suite.Require().NoError(err)
	suite.Equal(1, counts[saved.ID])

	// "Requested but nobody saved it" must be present as 0, not absent.
	zero, present := counts[unsaved.ID]
	suite.True(present, "requested show IDs are always present in the result map")
	suite.Equal(0, zero)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetBatchSaveCounts_EmptyInput() {
	counts, err := suite.savedShowService.GetBatchSaveCounts([]uint{})
	suite.Require().NoError(err)
	suite.Empty(counts)
}

// The public count must never become a side channel revealing that a hidden
// show exists and has engagement. A non-approved show reports 0 — the same as
// an approved show nobody saved — so there is no existence oracle.
func (suite *SavedShowServiceIntegrationTestSuite) TestGetSaveCount_HiddenShowsReportZero() {
	owner := suite.createTestUser()
	user := suite.createTestUser()

	approved := suite.createApprovedShow("Public Show", owner.ID)
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, approved.ID))

	for _, status := range []catalogm.ShowStatus{
		catalogm.ShowStatusPending,
		catalogm.ShowStatusRejected,
		catalogm.ShowStatusPrivate,
	} {
		hidden := suite.createApprovedShow("Hidden "+string(status), owner.ID)
		suite.Require().NoError(
			suite.db.Model(&catalogm.Show{}).Where("id = ?", hidden.ID).
				Update("status", status).Error,
		)
		// The show IS saved — only its visibility hides the count.
		suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, hidden.ID))

		count, err := suite.savedShowService.GetSaveCount(hidden.ID)
		suite.Require().NoError(err)
		suite.Equal(0, count, "a %s show must not expose its save count", status)

		batch, err := suite.savedShowService.GetBatchSaveCounts([]uint{approved.ID, hidden.ID})
		suite.Require().NoError(err)
		suite.Equal(1, batch[approved.ID], "the approved show still reports its count")
		zero, present := batch[hidden.ID]
		suite.True(present, "hidden show is zero-filled, not omitted (no existence oracle)")
		suite.Equal(0, zero)
	}
}

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_Success() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Save Me", user.ID)

	err := suite.savedShowService.SaveShow(user.ID, show.ID)

	suite.Require().NoError(err)

	// Verify in DB
	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionSave).
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
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionSave).
		Count(&count)
	suite.Equal(int64(1), count)
}

// =============================================================================
// Group 2: UnsaveShow
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestUnsaveShow_Success() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Unsave Me", user.ID)

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))

	err := suite.savedShowService.UnsaveShow(user.ID, show.ID)

	suite.Require().NoError(err)

	// Verify removed
	var count int64
	suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?", user.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionSave).
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

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show1.ID))
	time.Sleep(10 * time.Millisecond) // ensure different saved_at
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show2.ID))

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "")

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

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_IncludesVenueAndArtist() {
	user := suite.createTestUser()
	show, venue, artist := suite.createShowWithVenueAndArtist("Full Show", user.ID)

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))

	resp, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "")

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
		suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))
		time.Sleep(5 * time.Millisecond)
	}

	// Page 1
	resp1, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 0, "")
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 2, "")
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// Page 3
	resp3, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 4, "")
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

	suite.Require().NoError(suite.savedShowService.SaveShow(user1.ID, show1.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user2.ID, show2.ID))

	resp, total, err := suite.savedShowService.GetUserSavedShows(user1.ID, 10, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal(show1.ID, resp[0].ID)
}

// createShowAt creates an approved show at an explicit event date with a single
// venue carrying the given IANA timezone (nil = venue has no geocoded zone).
func (suite *SavedShowServiceIntegrationTestSuite) createShowAt(title string, userID uint, eventDate time.Time, timezone *string) *catalogm.Show {
	show := &catalogm.Show{
		Title:       title,
		EventDate:   eventDate.UTC(),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	venue := &catalogm.Venue{
		Name:     fmt.Sprintf("Venue for %s", title),
		City:     "Phoenix",
		State:    "AZ",
		Timezone: timezone,
	}
	suite.Require().NoError(suite.db.Create(venue).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)

	return show
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_UpcomingFilter() {
	user := suite.createTestUser()
	tz := stringPtr("America/Phoenix")
	pastShow := suite.createShowAt("Past Show", user.ID, time.Now().UTC().AddDate(0, 0, -7), tz)
	soonShow := suite.createShowAt("Soon Show", user.ID, time.Now().UTC().AddDate(0, 0, 3), tz)
	laterShow := suite.createShowAt("Later Show", user.ID, time.Now().UTC().AddDate(0, 0, 7), tz)

	// Save in reverse event order so a saved-at ordering would fail the assertions
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, laterShow.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, soonShow.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, pastShow.ID))

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "upcoming")

	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "total counts only the upcoming partition")
	suite.Require().Len(resp, 2)
	suite.Equal(soonShow.ID, resp[0].ID, "soonest upcoming show first")
	suite.Equal(laterShow.ID, resp[1].ID)
	suite.NotZero(resp[0].SavedAt, "saved_at survives the event-date path")
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_PastFilter() {
	user := suite.createTestUser()
	tz := stringPtr("America/Phoenix")
	oldShow := suite.createShowAt("Old Show", user.ID, time.Now().UTC().AddDate(0, 0, -7), tz)
	recentShow := suite.createShowAt("Recent Show", user.ID, time.Now().UTC().AddDate(0, 0, -3), tz)
	upcomingShow := suite.createShowAt("Upcoming Show", user.ID, time.Now().UTC().AddDate(0, 0, 7), tz)

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, oldShow.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, recentShow.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, upcomingShow.ID))

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "past")

	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "total counts only the past partition")
	suite.Require().Len(resp, 2)
	suite.Equal(recentShow.ID, resp[0].ID, "most recent past show first")
	suite.Equal(oldShow.ID, resp[1].ID)
}

// TestGetUserSavedShows_VenueTZBoundary pins the venue-local (not UTC) date
// comparison with shows whose venue-local calendar date and UTC instant
// disagree, deterministically for any run time:
//
//   - Honolulu (UTC-10): "yesterday 23:00 venue-local" is always past on the
//     venue calendar, but its UTC instant (today 09:00 UTC) can be hours in
//     the FUTURE when the test runs early in the UTC day.
//   - Kiritimati (UTC+14): "today 01:00 venue-local" is always upcoming on
//     the venue calendar, but its UTC instant can be up to ~13h in the PAST.
func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_VenueTZBoundary() {
	user := suite.createTestUser()

	honolulu, err := time.LoadLocation("Pacific/Honolulu")
	suite.Require().NoError(err)
	nowHonolulu := time.Now().In(honolulu)
	startOfTodayHonolulu := time.Date(nowHonolulu.Year(), nowHonolulu.Month(), nowHonolulu.Day(), 0, 0, 0, 0, honolulu)
	pastAtVenue := suite.createShowAt("Late Honolulu Show", user.ID,
		startOfTodayHonolulu.Add(-1*time.Hour), stringPtr("Pacific/Honolulu"))

	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	suite.Require().NoError(err)
	nowKiritimati := time.Now().In(kiritimati)
	startOfTodayKiritimati := time.Date(nowKiritimati.Year(), nowKiritimati.Month(), nowKiritimati.Day(), 0, 0, 0, 0, kiritimati)
	upcomingAtVenue := suite.createShowAt("Early Kiritimati Show", user.ID,
		startOfTodayKiritimati.Add(1*time.Hour), stringPtr("Pacific/Kiritimati"))

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, pastAtVenue.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, upcomingAtVenue.ID))

	upcoming, upcomingTotal, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), upcomingTotal)
	suite.Require().Len(upcoming, 1)
	suite.Equal(upcomingAtVenue.ID, upcoming[0].ID)

	past, pastTotal, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "past")
	suite.Require().NoError(err)
	suite.Equal(int64(1), pastTotal)
	suite.Require().Len(past, 1)
	suite.Equal(pastAtVenue.ID, past[0].ID)
}

// A venue without a geocoded timezone (and a show with no venue at all) must
// still classify — degrading to UTC — rather than error or vanish. The 48h
// margins keep the assertions true under any fallback zone.
func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_FilterTimezoneFallback() {
	user := suite.createTestUser()
	nullTZShow := suite.createShowAt("Null TZ Show", user.ID, time.Now().UTC().Add(48*time.Hour), nil)
	noVenueShow := suite.createApprovedShow("No Venue Show", user.ID) // no venue rows
	suite.Require().NoError(suite.db.Model(noVenueShow).Update("event_date", time.Now().UTC().Add(-48*time.Hour)).Error)

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, nullTZShow.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, noVenueShow.ID))

	upcoming, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "upcoming")
	suite.Require().NoError(err)
	suite.Require().Len(upcoming, 1)
	suite.Equal(nullTZShow.ID, upcoming[0].ID)

	past, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "past")
	suite.Require().NoError(err)
	suite.Require().Len(past, 1)
	suite.Equal(noVenueShow.ID, past[0].ID)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_FilterPagination() {
	user := suite.createTestUser()
	tz := stringPtr("America/Phoenix")
	var shows []*catalogm.Show
	for i := 1; i <= 3; i++ {
		show := suite.createShowAt(fmt.Sprintf("Upcoming %d", i), user.ID, time.Now().UTC().AddDate(0, 0, i), tz)
		shows = append(shows, show)
		suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))
	}

	page1, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 0, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Require().Len(page1, 2)
	suite.Equal(shows[0].ID, page1[0].ID)
	suite.Equal(shows[1].ID, page1[1].ID)

	page2, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 2, "upcoming")
	suite.Require().NoError(err)
	suite.Require().Len(page2, 1)
	suite.Equal(shows[2].ID, page2[0].ID)
}

// No-filter back-compat: mixed past + future shows all come back in a single
// list ordered by saved-at DESC, exactly as before the time_filter existed
// (the iOS app and the iCal calendar feed consume this path).
func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_NoFilterMixedDatesSavedOrder() {
	user := suite.createTestUser()
	tz := stringPtr("America/Phoenix")
	pastShow := suite.createShowAt("BackCompat Past", user.ID, time.Now().UTC().AddDate(0, 0, -7), tz)
	futureShow := suite.createShowAt("BackCompat Future", user.ID, time.Now().UTC().AddDate(0, 0, 7), tz)

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, pastShow.ID))
	time.Sleep(10 * time.Millisecond) // ensure different saved_at
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, futureShow.ID))

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "no filter returns past and upcoming alike")
	suite.Require().Len(resp, 2)
	suite.Equal(futureShow.ID, resp[0].ID, "most recently saved first")
	suite.Equal(pastShow.ID, resp[1].ID)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_InvalidFilterRejected() {
	user := suite.createTestUser()

	_, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0, "someday")

	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid time filter")
}

// =============================================================================
// Group 4: IsShowSaved
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestIsShowSaved_True() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Saved Check", user.ID)

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))

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

	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show1.ID))
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show3.ID))

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
