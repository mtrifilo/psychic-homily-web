package user

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1907: home area + account-level alert defaults, against a real database.
// These need Postgres because the behaviour under test is about what the
// COLUMN holds — NULL versus a stored false — which is exactly what an
// in-memory fake would paper over.

type AlertPreferencesIntegrationTestSuite struct {
	suite.Suite
	testDB      *testutil.TestDatabase
	db          *gorm.DB
	userService *UserService
}

func (suite *AlertPreferencesIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.userService = NewUserService(suite.testDB.DB)
}

func (suite *AlertPreferencesIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *AlertPreferencesIntegrationTestSuite) createUser(email string) *authm.User {
	user := &authm.User{Email: stringPtr(email), IsActive: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

// rawAlertColumns reads both columns verbatim, so a test can tell NULL from a
// stored value rather than only seeing what resolution reports.
func (suite *AlertPreferencesIntegrationTestSuite) rawAlertColumns(userID uint) (homeMetro, alertDefaults string) {
	type row struct {
		HomeMetro     string
		AlertDefaults string
	}
	var got row
	suite.Require().NoError(suite.db.Model(&authm.UserPreferences{}).
		Select("COALESCE(home_metro, '') AS home_metro", "COALESCE(alert_defaults::text, '') AS alert_defaults").
		Where("user_id = ?", userID).
		Take(&got).Error)
	return got.HomeMetro, got.AlertDefaults
}

// A user with no preferences row at all is the common case (rows are created
// lazily), and it must read as "no home area, everything inherited" rather
// than as an error.
func (suite *AlertPreferencesIntegrationTestSuite) TestGetAlertPreferences_NoRowInheritsEverything() {
	user := suite.createUser("alerts-norow@example.com")

	prefs, err := suite.userService.GetAlertPreferences(user.ID)

	suite.Require().NoError(err)
	suite.Nil(prefs.HomeMetro)
	suite.True(prefs.AlertDefaults.Shows.InApp, "in-app defaults ON")
	suite.False(prefs.AlertDefaults.Shows.Email, "email defaults OFF (opt-in)")
	suite.True(prefs.AlertDefaults.Releases.InApp)
	suite.False(prefs.AlertDefaults.Releases.Email)
}

// Setting a home area creates the preferences row when there is none, and the
// stored value is the CBSA code itself so it can be compared to venues.metro.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetHomeMetro_CreatesRowAndStoresCode() {
	user := suite.createUser("alerts-metro@example.com")
	phoenix := "38060"

	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &phoenix))

	stored, _ := suite.rawAlertColumns(user.ID)
	suite.Equal(phoenix, stored)

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(prefs.HomeMetro)
	suite.Equal(phoenix, *prefs.HomeMetro)
}

// Clearing is a real operation, not an absence: nil and blank both write NULL
// so near-me scoping falls back to everywhere instead of matching a stale area.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetHomeMetro_ClearsToNull() {
	user := suite.createUser("alerts-metro-clear@example.com")
	phoenix := "38060"
	blank := "   "

	for name, value := range map[string]*string{"nil": nil, "blank": &blank} {
		suite.Run(name, func() {
			suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &phoenix))
			suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, value))

			prefs, err := suite.userService.GetAlertPreferences(user.ID)
			suite.Require().NoError(err)
			suite.Nil(prefs.HomeMetro)
		})
	}
}

// A code no venue can ever carry would make near-me scoping deliver nothing
// while looking configured, so it is rejected rather than stored. The
// validation source is the same embedded dataset that assigns venues.metro.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetHomeMetro_RejectsUnknownCode() {
	user := suite.createUser("alerts-metro-bad@example.com")

	for name, bad := range map[string]string{
		"absent from the dataset": "99999",
		"not a code at all":       "not-a-code",
		"zero":                    "0",
		// Longer than the column, so it must be refused before the lookup and
		// before the value reaches an error message.
		"far longer than the column": strings.Repeat("3", 4096),
	} {
		suite.Run(name, func() {
			value := bad
			err := suite.userService.SetHomeMetro(user.ID, &value)
			suite.Require().Error(err)

			// Typed, so the handler can tell a rejected VALUE from a failed
			// WRITE and answer 422 rather than 500 for exactly one of them.
			var authErr *autherrors.AuthError
			suite.Require().ErrorAs(err, &authErr)
			suite.Equal(autherrors.CodeUnknownHomeMetro, authErr.Code)
			suite.NotContains(authErr.Message, bad, "the rejected value stays out of the user-facing message")
			suite.Less(len(err.Error()), 200, "the error must not carry an unbounded value")

			prefs, prefsErr := suite.userService.GetAlertPreferences(user.ID)
			suite.Require().NoError(prefsErr)
			suite.Nil(prefs.HomeMetro, "a rejected code must not be stored")
		})
	}
}

// The first write creates the row and stores ONLY the cell the user touched;
// everything else stays absent so it keeps inheriting.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetAccountAlertDefaults_CreatesRowWithOnlyTheSetCell() {
	user := suite.createUser("alerts-defaults-create@example.com")

	suite.Require().NoError(suite.userService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Shows: &authm.AlertChannelDefaultsUpdate{Email: boolPtrLocal(true)},
		}))

	_, stored := suite.rawAlertColumns(user.ID)
	suite.JSONEq(`{"shows":{"email":true}}`, stored)

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.True(prefs.AlertDefaults.Shows.Email)
	suite.True(prefs.AlertDefaults.Shows.InApp, "untouched channel still inherits ON")
	suite.False(prefs.AlertDefaults.Releases.Email, "untouched alert type still inherits OFF")
}

// This is the cell the GORM bool trap would eat: an explicit false must round
// trip through the column as false, not be dropped in favour of a default.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetAccountAlertDefaults_ExplicitFalseRoundTrips() {
	user := suite.createUser("alerts-defaults-false@example.com")

	suite.Require().NoError(suite.userService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Shows:    &authm.AlertChannelDefaultsUpdate{InApp: boolPtrLocal(false)},
			Releases: &authm.AlertChannelDefaultsUpdate{InApp: boolPtrLocal(false)},
		}))

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.False(prefs.AlertDefaults.Shows.InApp)
	suite.False(prefs.AlertDefaults.Releases.InApp)
}

// Successive partial writes accumulate rather than replace: a settings page
// that toggles one switch must not reset the other three.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetAccountAlertDefaults_PartialWritesAccumulate() {
	user := suite.createUser("alerts-defaults-merge@example.com")

	suite.Require().NoError(suite.userService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Shows: &authm.AlertChannelDefaultsUpdate{Email: boolPtrLocal(true)},
		}))
	suite.Require().NoError(suite.userService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Releases: &authm.AlertChannelDefaultsUpdate{InApp: boolPtrLocal(false)},
		}))

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.True(prefs.AlertDefaults.Shows.Email, "the earlier write survived")
	suite.False(prefs.AlertDefaults.Releases.InApp, "the later write applied")
	suite.True(prefs.AlertDefaults.Shows.InApp, "a cell nobody set still inherits")
}

// An update that pins nothing must leave the column exactly as it was, rather
// than writing an empty document that would say what NULL already says.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetAccountAlertDefaults_EmptyUpdateWritesNothing() {
	user := suite.createUser("alerts-defaults-empty@example.com")
	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, nil))

	suite.Require().NoError(suite.userService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{Shows: &authm.AlertChannelDefaultsUpdate{}}))

	_, stored := suite.rawAlertColumns(user.ID)
	suite.Empty(stored, "the column stays NULL")
}

// The two preferences share a row but not a value: writing one must not
// disturb the other, in either order.
func (suite *AlertPreferencesIntegrationTestSuite) TestAlertPreferences_HomeMetroAndDefaultsAreIndependent() {
	user := suite.createUser("alerts-independent@example.com")
	phoenix := "38060"

	suite.Require().NoError(suite.userService.SetAccountAlertDefaults(user.ID,
		authm.AccountAlertDefaultsUpdate{
			Shows: &authm.AlertChannelDefaultsUpdate{Email: boolPtrLocal(true)},
		}))
	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &phoenix))

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(prefs.HomeMetro)
	suite.Equal(phoenix, *prefs.HomeMetro)
	suite.True(prefs.AlertDefaults.Shows.Email, "the alert matrix survived the home-metro write")

	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, nil))
	prefs, err = suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.Nil(prefs.HomeMetro)
	suite.True(prefs.AlertDefaults.Shows.Email, "clearing the home area is not clearing the matrix")
}

// Two first-ever writes for the same user must both survive. Before the row
// exists there is nothing to lock, so without creating it up front each write
// would merge from NULL and the loser would overwrite the winner's whole
// document: an alerts card saving both alert types at once would report
// success and silently drop one of them.
func (suite *AlertPreferencesIntegrationTestSuite) TestSetAccountAlertDefaults_ConcurrentFirstWritesBothSurvive() {
	user := suite.createUser("alerts-defaults-race@example.com")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	updates := []authm.AccountAlertDefaultsUpdate{
		{Shows: &authm.AlertChannelDefaultsUpdate{Email: boolPtrLocal(true)}},
		{Releases: &authm.AlertChannelDefaultsUpdate{InApp: boolPtrLocal(false)}},
	}
	for i, update := range updates {
		wg.Add(1)
		go func(i int, update authm.AccountAlertDefaultsUpdate) {
			defer wg.Done()
			errs[i] = suite.userService.SetAccountAlertDefaults(user.ID, update)
		}(i, update)
	}
	wg.Wait()
	for _, err := range errs {
		suite.Require().NoError(err)
	}

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.True(prefs.AlertDefaults.Shows.Email, "the shows write survived")
	suite.False(prefs.AlertDefaults.Releases.InApp, "the releases write survived")

	var rows int64
	suite.Require().NoError(suite.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	suite.EqualValues(1, rows, "exactly one preferences row")
}

// A preferences row created by an alert write must still get the DDL defaults
// for every OTHER preference. GORM drops zero values for fields carrying a
// `default` tag, which is what makes the opt-out flags insert as TRUE; if that
// ever stopped holding, a user would silently lose notifications they never
// turned off.
func (suite *AlertPreferencesIntegrationTestSuite) TestAlertWriteCreatesRowWithDDLDefaults() {
	user := suite.createUser("alerts-ddl-defaults@example.com")
	phoenix := "38060"

	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &phoenix))

	var prefs authm.UserPreferences
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).Take(&prefs).Error)
	suite.True(prefs.NotifyOnCommentSubscription, "opt-OUT preference must insert as TRUE")
	suite.True(prefs.NotifyOnMention, "opt-OUT preference must insert as TRUE")
	suite.True(prefs.NotifyOnTierNotifications)
	suite.True(prefs.NotifyOnEditNotifications)
	suite.False(prefs.NotifyOnCollectionDigest, "opt-IN preference must insert as FALSE")
	suite.False(prefs.NotifyOnSceneDigest, "opt-IN preference must insert as FALSE")
	suite.Equal("anyone", prefs.DefaultReplyPermission)
}

func boolPtrLocal(b bool) *bool { return &b }

// A nil database is a programming error, not a user-visible one, but it must
// report rather than panic.
func TestAlertPreferences_NilDatabase(t *testing.T) {
	svc := &UserService{}
	metro := "38060"

	_, err := svc.GetAlertPreferences(1)
	assert.Error(t, err)
	assert.Error(t, svc.SetHomeMetro(1, &metro))
	assert.Error(t, svc.SetAccountAlertDefaults(1, authm.AccountAlertDefaultsUpdate{
		Shows: &authm.AlertChannelDefaultsUpdate{Email: boolPtrLocal(true)},
	}))
}

func TestAlertPreferencesIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AlertPreferencesIntegrationTestSuite))
}
