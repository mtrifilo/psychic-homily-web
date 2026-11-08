package user

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// Calendar sync choice persistence against a real database, for the reason
// HomeLayoutIntegrationTestSuite gives: NULL ("not decided") has to stay
// distinguishable from a stored document.

type CalendarSyncIntegrationTestSuite struct {
	suite.Suite
	testDB      *testutil.TestDatabase
	db          *gorm.DB
	userService *UserService
}

func (suite *CalendarSyncIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.userService = NewUserService(suite.testDB.DB)
}

func (suite *CalendarSyncIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *CalendarSyncIntegrationTestSuite) createUser(email string) *authm.User {
	user := &authm.User{Email: stringPtr(email), IsActive: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

var calendarSyncTestDecidedAt = time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC)

func connectedChoice(providers ...string) *authm.CalendarSync {
	return &authm.CalendarSync{
		Version:   authm.CalendarSyncVersion,
		State:     authm.CalendarSyncConnected,
		Providers: providers,
		DecidedAt: calendarSyncTestDecidedAt,
	}
}

func notNowChoice() *authm.CalendarSync {
	until := calendarSyncTestDecidedAt.Add(30 * 24 * time.Hour)
	declines := 1
	return &authm.CalendarSync{
		Version:       authm.CalendarSyncVersion,
		State:         authm.CalendarSyncNotNow,
		DecidedAt:     calendarSyncTestDecidedAt,
		DeclinedUntil: &until,
		Declines:      &declines,
	}
}

func neverChoice() *authm.CalendarSync {
	return &authm.CalendarSync{
		Version:   authm.CalendarSyncVersion,
		State:     authm.CalendarSyncNever,
		DecidedAt: calendarSyncTestDecidedAt,
	}
}

func (suite *CalendarSyncIntegrationTestSuite) mustSetChoice(userID uint, choice *authm.CalendarSync) {
	_, err := suite.userService.SetCalendarSync(userID, choice)
	suite.Require().NoError(err)
}

func (suite *CalendarSyncIntegrationTestSuite) readChoice(userID uint) *authm.CalendarSync {
	got, err := suite.userService.GetCalendarSync(userID)
	suite.Require().NoError(err)
	return got
}

func (suite *CalendarSyncIntegrationTestSuite) preferenceRows(userID uint) int64 {
	var rows int64
	suite.Require().NoError(suite.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).Count(&rows).Error)
	return rows
}

// A user with no preferences row has not decided, and reading that must not
// conjure a row.
func (suite *CalendarSyncIntegrationTestSuite) TestGetCalendarSync_NoRowIsUndecided() {
	user := suite.createUser("calendar-sync-norow@example.com")

	suite.Nil(suite.readChoice(user.ID))
	suite.Zero(suite.preferenceRows(user.ID))
}

// A preferences row that exists for some other preference still reads as
// undecided while the column is NULL.
func (suite *CalendarSyncIntegrationTestSuite) TestGetCalendarSync_NullColumnIsUndecided() {
	user := suite.createUser("calendar-sync-null@example.com")
	suite.Require().NoError(suite.userService.SetShowReminders(user.ID, true))

	suite.Nil(suite.readChoice(user.ID))
}

// Each state survives the write and the read unchanged, including a stored
// first write on a user with no preferences row yet.
func (suite *CalendarSyncIntegrationTestSuite) TestSetCalendarSync_RoundTripsEachState() {
	cases := map[string]*authm.CalendarSync{
		"connected": connectedChoice(authm.CalendarProviderGoogleSubscribe, authm.CalendarProviderAppleSubscribe),
		"not_now":   notNowChoice(),
		"never":     neverChoice(),
	}
	for name, choice := range cases {
		suite.Run(name, func() {
			user := suite.createUser("calendar-sync-roundtrip-" + name + "@example.com")

			stored, err := suite.userService.SetCalendarSync(user.ID, choice)
			suite.Require().NoError(err)
			suite.Equal(choice, stored, "the service returns what it stored")
			suite.Equal(choice, suite.readChoice(user.ID))
		})
	}
}

// Replace, not merge: the second write's provider list is the whole stored
// list, and moving to a state without providers drops them.
func (suite *CalendarSyncIntegrationTestSuite) TestSetCalendarSync_ReplacesTheWholeDocument() {
	user := suite.createUser("calendar-sync-replace@example.com")
	suite.mustSetChoice(user.ID, connectedChoice(authm.CalendarProviderGoogleSubscribe, authm.CalendarProviderCopied))

	suite.mustSetChoice(user.ID, connectedChoice(authm.CalendarProviderAppleSubscribe))
	suite.Equal([]string{authm.CalendarProviderAppleSubscribe}, suite.readChoice(user.ID).Providers)

	suite.mustSetChoice(user.ID, neverChoice())
	suite.Equal(neverChoice(), suite.readChoice(user.ID))
}

// A rejected document must not reach the column.
func (suite *CalendarSyncIntegrationTestSuite) TestSetCalendarSync_RejectionWritesNothing() {
	user := suite.createUser("calendar-sync-reject@example.com")
	suite.mustSetChoice(user.ID, neverChoice())

	stored, err := suite.userService.SetCalendarSync(user.ID, connectedChoice())

	suite.Require().Error(err)
	suite.ErrorIs(err, authm.ErrInvalidCalendarSync)
	suite.Nil(stored)
	suite.Equal(neverChoice(), suite.readChoice(user.ID))
}

// The read returns the document as stored: an expired not_now is not rewritten
// to "undecided", because the client decides what a lapsed decline means.
func (suite *CalendarSyncIntegrationTestSuite) TestGetCalendarSync_LeavesAnExpiredNotNowAlone() {
	user := suite.createUser("calendar-sync-expired@example.com")
	longAgo := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	until := longAgo.Add(30 * 24 * time.Hour)
	declines := 2
	expired := &authm.CalendarSync{
		Version:       authm.CalendarSyncVersion,
		State:         authm.CalendarSyncNotNow,
		DecidedAt:     longAgo,
		DeclinedUntil: &until,
		Declines:      &declines,
	}
	suite.mustSetChoice(user.ID, expired)

	suite.Equal(expired, suite.readChoice(user.ID))
	suite.Equal(expired, suite.readChoice(user.ID), "a second read sees the same document")
}

// DELETE resets to NULL without removing the row, so sibling preferences on
// the row survive.
func (suite *CalendarSyncIntegrationTestSuite) TestClearCalendarSync_ResetsToNullAndKeepsTheRow() {
	user := suite.createUser("calendar-sync-clear@example.com")
	metro := "38060"
	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &metro))
	suite.mustSetChoice(user.ID, connectedChoice(authm.CalendarProviderDownloaded))

	suite.Require().NoError(suite.userService.ClearCalendarSync(user.ID))

	suite.Nil(suite.readChoice(user.ID))
	suite.EqualValues(1, suite.preferenceRows(user.ID), "the preferences row must survive a reset")
	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(prefs.HomeMetro, "a reset must not clear sibling preferences")
	suite.Equal(metro, *prefs.HomeMetro)
}

func (suite *CalendarSyncIntegrationTestSuite) TestClearCalendarSync_NoRowIsANoOp() {
	user := suite.createUser("calendar-sync-clear-norow@example.com")

	suite.Require().NoError(suite.userService.ClearCalendarSync(user.ID))

	suite.Zero(suite.preferenceRows(user.ID), "a reset must not create a preferences row")
}

// The choice is one column on a shared row: a sibling write must not clear it.
func (suite *CalendarSyncIntegrationTestSuite) TestCalendarSync_IsIndependentOfSiblingPreferences() {
	user := suite.createUser("calendar-sync-siblings@example.com")
	suite.mustSetChoice(user.ID, notNowChoice())

	suite.Require().NoError(suite.userService.SetShowReminders(user.ID, true))
	suite.Require().NoError(suite.userService.ClearHomeLayout(user.ID))

	suite.Equal(notNowChoice(), suite.readChoice(user.ID))
}

// The profile payload is one of the two places the client reads the choice,
// where it rides beside home_layout as a raw JSONB column.
func (suite *CalendarSyncIntegrationTestSuite) TestCalendarSync_RidesInTheProfilePayload() {
	user := suite.createUser("calendar-sync-profile@example.com")
	choice := connectedChoice(authm.CalendarProviderGoogleSubscribe)
	suite.mustSetChoice(user.ID, choice)

	profile, err := suite.userService.GetUserByID(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(profile.Preferences)
	suite.Require().NotNil(profile.Preferences.CalendarSync)

	var got authm.CalendarSync
	suite.Require().NoError(json.Unmarshal(*profile.Preferences.CalendarSync, &got))
	suite.Equal(*choice, got)
}

func TestCalendarSyncIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CalendarSyncIntegrationTestSuite))
}
