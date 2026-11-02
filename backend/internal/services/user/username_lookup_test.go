package user

import (
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// GetUserByUsername answers a deactivated or soft-deleted holder exactly as a
// username nobody holds, GetUserByUsernameIncludingClosed still finds it, and
// reopening the account makes GetUserByUsername find it again.
func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_ClosedHolderIsNotFound() {
	for _, state := range []struct {
		name     string
		username string
		close    func(userID uint)
	}{
		{"deactivated", "closed-lookup-deactivated", func(id uint) { testutil.DeactivateAccount(suite.T(), suite.db, id) }},
		{"soft-deleted", "closed-lookup-soft-deleted", func(id uint) { testutil.SoftDeleteAccount(suite.T(), suite.db, id) }},
	} {
		user := &authm.User{
			Email:         stringPtr(state.username + "@example.com"),
			Username:      stringPtr(state.username),
			IsActive:      true,
			EmailVerified: true,
		}
		suite.Require().NoError(suite.db.Create(user).Error)
		state.close(user.ID)

		found, err := suite.userService.GetUserByUsername(state.username)
		suite.Require().NoError(err, state.name)
		suite.Nil(found, "%s: GetUserByUsername found a closed holder", state.name)

		including, err := suite.userService.GetUserByUsernameIncludingClosed(state.username)
		suite.Require().NoError(err, state.name)
		suite.Require().NotNil(including, "%s: GetUserByUsernameIncludingClosed lost the closed holder", state.name)
		suite.Equal(user.ID, including.ID, state.name)

		testutil.ReopenAccount(suite.T(), suite.db, user.ID)
		found, err = suite.userService.GetUserByUsername(state.username)
		suite.Require().NoError(err, state.name)
		suite.Require().NotNil(found, "%s: GetUserByUsername lost the reopened account", state.name)
		suite.Equal(user.ID, found.ID, state.name)
	}
}

// GetUserByUsernameIncludingClosed answers a username Postgres rejects as text
// as not found, as GetUserByUsername does.
func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsernameIncludingClosed_RejectedTextIsNotFound() {
	for _, username := range []string{"no\x00body", "no\xffbody"} {
		found, err := suite.userService.GetUserByUsernameIncludingClosed(username)
		suite.NoError(err, "%q", username)
		suite.Nil(found, "%q", username)
	}
}
