package user

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// First-save hint dismissal against a real database: what matters is what the
// column holds (NULL until dismissed, then the FIRST dismissal forever), and
// the first-wins rule lives in the SQL itself.

type FirstSaveHintIntegrationTestSuite struct {
	suite.Suite
	testDB      *testutil.TestDatabase
	db          *gorm.DB
	userService *UserService
}

func (suite *FirstSaveHintIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.userService = NewUserService(suite.testDB.DB)
}

func (suite *FirstSaveHintIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FirstSaveHintIntegrationTestSuite) createUser(email string) *authm.User {
	user := &authm.User{Email: stringPtr(email), IsActive: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

// storedDismissal reads the column verbatim. nil means NULL or no row, which
// are the same state to every reader: the hint has not been dismissed.
func (suite *FirstSaveHintIntegrationTestSuite) storedDismissal(userID uint) *time.Time {
	var prefs authm.UserPreferences
	err := suite.db.Where("user_id = ?", userID).Take(&prefs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	suite.Require().NoError(err)
	return prefs.FirstSaveHintDismissedAt
}

func (suite *FirstSaveHintIntegrationTestSuite) TestNewUserHasNotDismissed() {
	user := suite.createUser("first-save-hint-new@example.com")
	suite.Nil(suite.storedDismissal(user.ID))
}

// A user who has never written a preference has no row; dismissing creates
// it, and the sibling columns land on their DDL defaults rather than on Go
// zero values.
func (suite *FirstSaveHintIntegrationTestSuite) TestDismiss_CreatesTheRow() {
	user := suite.createUser("first-save-hint-norow@example.com")

	before := time.Now().Add(-time.Minute)
	got, err := suite.userService.DismissFirstSaveHint(user.ID)
	suite.Require().NoError(err)
	suite.True(got.After(before), "returned time should be now-ish, got %v", got)

	stored := suite.storedDismissal(user.ID)
	suite.Require().NotNil(stored)
	suite.True(stored.Equal(got), "response must report the stored value")

	var prefs authm.UserPreferences
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).Take(&prefs).Error)
	suite.True(prefs.NotifyOnCommentSubscription, "opt-out default must survive the insert")
	suite.False(prefs.NotifyOnCollectionDigest, "opt-in default must survive the insert")
	suite.Equal("anyone", prefs.DefaultReplyPermission)
}

// An existing row gets the column set without disturbing its siblings.
func (suite *FirstSaveHintIntegrationTestSuite) TestDismiss_UpdatesAnExistingRow() {
	user := suite.createUser("first-save-hint-existing@example.com")
	metro := "38060"
	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &metro))

	got, err := suite.userService.DismissFirstSaveHint(user.ID)
	suite.Require().NoError(err)
	stored := suite.storedDismissal(user.ID)
	suite.Require().NotNil(stored)
	suite.True(stored.Equal(got))

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(prefs.HomeMetro)
	suite.Equal(metro, *prefs.HomeMetro, "dismissing must not clear the home metro")
}

// Idempotent, first-write-wins: a repeat dismissal returns and keeps the
// original time.
func (suite *FirstSaveHintIntegrationTestSuite) TestDismiss_RepeatKeepsTheFirstTime() {
	user := suite.createUser("first-save-hint-repeat@example.com")

	first, err := suite.userService.DismissFirstSaveHint(user.ID)
	suite.Require().NoError(err)

	// Backdate the stored value so a second write that overwrote it would be
	// visible even inside one clock tick.
	backdated := first.Add(-24 * time.Hour)
	suite.Require().NoError(suite.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", user.ID).
		Update("first_save_hint_dismissed_at", backdated).Error)

	second, err := suite.userService.DismissFirstSaveHint(user.ID)
	suite.Require().NoError(err)
	suite.True(second.Equal(backdated), "repeat must return the stored time, got %v want %v", second, backdated)

	stored := suite.storedDismissal(user.ID)
	suite.Require().NotNil(stored)
	suite.True(stored.Equal(backdated), "repeat must not move the stored time")
}

// Two first-ever dismissals racing to create the row both succeed and agree
// on one stored time.
func (suite *FirstSaveHintIntegrationTestSuite) TestDismiss_ConcurrentFirstWritesAgree() {
	user := suite.createUser("first-save-hint-race@example.com")

	const writers = 4
	results := make([]time.Time, writers)
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = suite.userService.DismissFirstSaveHint(user.ID)
		}(i)
	}
	wg.Wait()

	for i := range writers {
		suite.Require().NoError(errs[i])
	}
	stored := suite.storedDismissal(user.ID)
	suite.Require().NotNil(stored)
	for i := range writers {
		suite.True(results[i].Equal(*stored), "writer %d reported %v, stored %v", i, results[i], *stored)
	}
}

// The value has to reach the frontend by the path it reads it: the profile
// payload's preferences.
func (suite *FirstSaveHintIntegrationTestSuite) TestDismissal_RidesInTheProfilePayload() {
	user := suite.createUser("first-save-hint-profile@example.com")

	profile, err := suite.userService.GetUserByID(user.ID)
	suite.Require().NoError(err)
	if profile.Preferences != nil {
		suite.Nil(profile.Preferences.FirstSaveHintDismissedAt)
	}

	got, err := suite.userService.DismissFirstSaveHint(user.ID)
	suite.Require().NoError(err)

	profile, err = suite.userService.GetUserByID(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(profile.Preferences)
	suite.Require().NotNil(profile.Preferences.FirstSaveHintDismissedAt)
	suite.True(profile.Preferences.FirstSaveHintDismissedAt.Equal(got))
}

func (suite *FirstSaveHintIntegrationTestSuite) TestDismiss_NilDB() {
	svc := &UserService{}
	_, err := svc.DismissFirstSaveHint(1)
	suite.Error(err)
}

func TestFirstSaveHintIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FirstSaveHintIntegrationTestSuite))
}
