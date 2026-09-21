package user

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// Home layout persistence against a real database. Postgres, not a fake,
// because the behaviour under test is what the COLUMN holds: NULL
// (shipped default) has to stay distinguishable from a stored document, and
// that distinction is exactly what an in-memory stand-in would paper over.

type HomeLayoutIntegrationTestSuite struct {
	suite.Suite
	testDB      *testutil.TestDatabase
	db          *gorm.DB
	userService *UserService
}

func (suite *HomeLayoutIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.userService = NewUserService(suite.testDB.DB)
}

func (suite *HomeLayoutIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *HomeLayoutIntegrationTestSuite) createUser(email string) *authm.User {
	user := &authm.User{Email: stringPtr(email), IsActive: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

// rawHomeLayout reads the column verbatim so a test can tell NULL from a
// stored document rather than only seeing what a getter reports. An empty
// string means NULL or no row at all, which are the same state to a reader.
func (suite *HomeLayoutIntegrationTestSuite) rawHomeLayout(userID uint) string {
	type row struct{ HomeLayout string }
	var got row
	err := suite.db.Model(&authm.UserPreferences{}).
		Select("COALESCE(home_layout::text, '') AS home_layout").
		Where("user_id = ?", userID).
		Take(&got).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ""
	}
	suite.Require().NoError(err)
	return got.HomeLayout
}

// storedHomeLayout parses the stored document. Parsed, not bytes: Postgres
// normalises whitespace and key order in a jsonb column, so a byte comparison
// would test the driver rather than the write.
func (suite *HomeLayoutIntegrationTestSuite) storedHomeLayout(userID uint) authm.HomeLayout {
	stored := suite.rawHomeLayout(userID)
	suite.Require().NotEmpty(stored, "expected a stored document, column is NULL")

	var got authm.HomeLayout
	suite.Require().NoError(json.Unmarshal([]byte(stored), &got))
	return got
}

func (suite *HomeLayoutIntegrationTestSuite) assertStored(userID uint, want *authm.HomeLayout) {
	suite.Equal(*want, suite.storedHomeLayout(userID))
}

func shippedHomeLayout() *authm.HomeLayout {
	return &authm.HomeLayout{
		Version: authm.HomeLayoutVersion,
		Sections: []authm.HomeLayoutSection{
			{ID: authm.HomeSectionSavedShows, Visible: true},
			{ID: authm.HomeSectionNearbyShows, Visible: true},
			{ID: authm.HomeSectionCommunityStats, Visible: true},
			{ID: authm.HomeSectionCityGraph, Visible: false},
			{ID: authm.HomeSectionRadioShows, Visible: true},
		},
	}
}

// Rows are created lazily, so the first save has no row to update and must
// create one rather than silently affecting zero rows.
func (suite *HomeLayoutIntegrationTestSuite) TestSetHomeLayout_CreatesTheRowOnFirstSave() {
	user := suite.createUser("home-layout-first@example.com")
	suite.Require().Empty(suite.rawHomeLayout(user.ID), "precondition: no stored layout")

	layout := shippedHomeLayout()
	stored, err := suite.userService.SetHomeLayout(user.ID, layout)

	suite.Require().NoError(err)
	suite.Equal(layout, stored, "the service returns what it stored")
	suite.assertStored(user.ID, layout)
}

// Replace, not merge: the second write's list is the whole stored list, and a
// section the second write drops is gone rather than lingering in its old
// slot. The payload also pins the two properties that ride on replace, since a
// hidden section and a non-default order both survive it: saved_shows stays in
// slot 1 while hidden, and radio_shows leads although it ships last.
func (suite *HomeLayoutIntegrationTestSuite) TestSetHomeLayout_ReplacesTheWholeDocument() {
	user := suite.createUser("home-layout-replace@example.com")
	mustSet(suite, user.ID, shippedHomeLayout())

	reordered := &authm.HomeLayout{
		Version: authm.HomeLayoutVersion,
		Sections: []authm.HomeLayoutSection{
			{ID: authm.HomeSectionRadioShows, Visible: true},
			{ID: authm.HomeSectionSavedShows, Visible: false},
		},
	}
	mustSet(suite, user.ID, reordered)

	suite.assertStored(user.ID, reordered)
}

// Order is the payload, so a save that only reorders has to survive the round
// trip in the order it was sent.
func (suite *HomeLayoutIntegrationTestSuite) TestSetHomeLayout_PreservesSectionOrder() {
	user := suite.createUser("home-layout-order@example.com")

	reversed := &authm.HomeLayout{Version: authm.HomeLayoutVersion}
	shipped := shippedHomeLayout().Sections
	for i := len(shipped) - 1; i >= 0; i-- {
		reversed.Sections = append(reversed.Sections, shipped[i])
	}
	mustSet(suite, user.ID, reversed)

	suite.assertStored(user.ID, reversed)
}

// A rejected document must not reach the column: a partial write here would
// store a layout the frontend cannot render.
func (suite *HomeLayoutIntegrationTestSuite) TestSetHomeLayout_RejectionWritesNothing() {
	user := suite.createUser("home-layout-reject@example.com")
	mustSet(suite, user.ID, shippedHomeLayout())

	bad := &authm.HomeLayout{
		Version:  authm.HomeLayoutVersion,
		Sections: []authm.HomeLayoutSection{{ID: "mixtapes", Visible: true}},
	}
	stored, err := suite.userService.SetHomeLayout(user.ID, bad)

	suite.Require().Error(err)
	suite.ErrorIs(err, authm.ErrInvalidHomeLayout)
	suite.Nil(stored)
	suite.assertStored(user.ID, shippedHomeLayout())
}

// DELETE resets to NULL, the shipped default. NULL and not an empty document,
// because an empty document would say what NULL already says while also
// claiming the user made a choice.
func (suite *HomeLayoutIntegrationTestSuite) TestClearHomeLayout_ResetsToNull() {
	user := suite.createUser("home-layout-clear@example.com")
	mustSet(suite, user.ID, shippedHomeLayout())

	suite.Require().NoError(suite.userService.ClearHomeLayout(user.ID))

	suite.Empty(suite.rawHomeLayout(user.ID), "column should be NULL after a reset")
}

// A user who never customised anything is already at the default, so a reset
// must succeed without conjuring a preferences row.
func (suite *HomeLayoutIntegrationTestSuite) TestClearHomeLayout_NoRowIsANoOp() {
	user := suite.createUser("home-layout-clear-norow@example.com")

	suite.Require().NoError(suite.userService.ClearHomeLayout(user.ID))

	var rows int64
	suite.Require().NoError(suite.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", user.ID).Count(&rows).Error)
	suite.Zero(rows, "a reset must not create a preferences row")
}

// The layout is one column on a shared row: saving it must not disturb a
// sibling preference, and a sibling write must not clear it.
func (suite *HomeLayoutIntegrationTestSuite) TestHomeLayout_IsIndependentOfSiblingPreferences() {
	user := suite.createUser("home-layout-siblings@example.com")

	metro := "38060"
	suite.Require().NoError(suite.userService.SetHomeMetro(user.ID, &metro))
	mustSet(suite, user.ID, shippedHomeLayout())

	prefs, err := suite.userService.GetAlertPreferences(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(prefs.HomeMetro)
	suite.Equal(metro, *prefs.HomeMetro, "saving a layout must not clear the home metro")

	suite.Require().NoError(suite.userService.SetShowReminders(user.ID, true))
	suite.assertStored(user.ID, shippedHomeLayout())
}

// The document has to survive the path the frontend actually reads it by: the
// authenticated profile payload, where it rides as a raw JSONB column.
func (suite *HomeLayoutIntegrationTestSuite) TestHomeLayout_RidesInTheProfilePayload() {
	user := suite.createUser("home-layout-profile@example.com")
	layout := shippedHomeLayout()
	mustSet(suite, user.ID, layout)

	profile, err := suite.userService.GetUserByID(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(profile.Preferences)
	suite.Require().NotNil(profile.Preferences.HomeLayout)

	var got authm.HomeLayout
	suite.Require().NoError(json.Unmarshal(*profile.Preferences.HomeLayout, &got))
	suite.Equal(*layout, got)
}

func mustSet(suite *HomeLayoutIntegrationTestSuite, userID uint, layout *authm.HomeLayout) {
	_, err := suite.userService.SetHomeLayout(userID, layout)
	suite.Require().NoError(err)
}

func TestHomeLayoutIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(HomeLayoutIntegrationTestSuite))
}
