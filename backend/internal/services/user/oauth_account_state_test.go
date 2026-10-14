package user

import (
	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// A returning OAuth identity resolves to its account before anything else is
// consulted, so the account's state is the only thing standing between that
// identity and a session.

// oauthSignIn signs subject in through the consent-free path, creating the
// account on first use.
func (suite *UserServiceIntegrationTestSuite) oauthSignIn(subject, address string) (*authm.User, error) {
	return suite.userService.FindOrCreateUser(goth.User{
		UserID:  subject,
		Email:   address,
		RawData: map[string]any{"verified_email": true},
	}, "google")
}

func (suite *UserServiceIntegrationTestSuite) assertAccountInactiveRefusal(user *authm.User, err error) {
	suite.T().Helper()
	suite.Require().Error(err)
	suite.Nil(user)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeAccountInactive, authErr.Code)
	suite.Equal(apperrors.ErrAccountInactive().UserMessage(), authErr.UserMessage())
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_DeactivatedAccount_Refused() {
	created, err := suite.oauthSignIn("google-state-deactivated", "state.deactivated@example.com")
	suite.Require().NoError(err)
	suite.Require().NoError(
		suite.db.Model(&authm.User{}).Where("id = ?", created.ID).Update("is_active", false).Error)

	user, err := suite.oauthSignIn("google-state-deactivated", "state.deactivated@example.com")
	suite.assertAccountInactiveRefusal(user, err)

	// The consent path resolves a returning identity the same way.
	user, err = suite.userService.FindOrCreateUserWithConsent(goth.User{
		UserID: "google-state-deactivated",
		Email:  "state.deactivated@example.com",
	}, "google", nil)
	suite.assertAccountInactiveRefusal(user, err)
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_SoftDeletedAccount_Refused() {
	created, err := suite.oauthSignIn("google-state-soft-deleted", "state.softdeleted@example.com")
	suite.Require().NoError(err)
	suite.Require().NoError(suite.userService.SoftDeleteAccount(created.ID, nil))

	user, err := suite.oauthSignIn("google-state-soft-deleted", "state.softdeleted@example.com")
	suite.assertAccountInactiveRefusal(user, err)

	// The refusal leaves the account and its identity as they were.
	var stored authm.User
	suite.Require().NoError(suite.db.First(&stored, created.ID).Error)
	suite.False(stored.IsActive)
	suite.NotNil(stored.DeletedAt)
	suite.assertOnlyIdentityIs(created.ID, "google-state-soft-deleted")
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_ActiveAccount_SignsIn() {
	created, err := suite.oauthSignIn("google-state-active", "state.active@example.com")
	suite.Require().NoError(err)

	user, err := suite.oauthSignIn("google-state-active", "state.active@example.com")
	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.Equal(created.ID, user.ID)
}

// The password path answers a soft-deleted account with the same refusal the
// OAuth paths use.
func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_SoftDeletedUser() {
	created, err := suite.userService.CreateUserWithPassword(
		"authsoftdeleted@example.com", "CorrectPassword1!", "Soft", "Deleted",
	)
	suite.Require().NoError(err)
	suite.Require().NoError(suite.userService.SoftDeleteAccount(created.ID, nil))

	user, err := suite.userService.AuthenticateUserWithPassword(
		"authsoftdeleted@example.com", "CorrectPassword1!",
	)
	suite.assertAccountInactiveRefusal(user, err)
}
