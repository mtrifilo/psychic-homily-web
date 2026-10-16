package user

import (
	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// An account that may not sign in is refused with ACCOUNT_INACTIVE on every
// sign-in path this service resolves, and an active one signs in.

// signInAccountStates are the account states a sign-in path must tell apart.
// users_live_account_check refuses an active row with deleted_at set, so that
// combination is covered by the CanSignIn unit test, not here.
var signInAccountStates = []struct {
	name      string
	apply     func(s *UserServiceIntegrationTestSuite, userID uint)
	maySignIn bool
}{
	{name: "active", apply: func(*UserServiceIntegrationTestSuite, uint) {}, maySignIn: true},
	{name: "deactivated", apply: func(s *UserServiceIntegrationTestSuite, userID uint) {
		s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
	}},
	{name: "soft-deleted", apply: func(s *UserServiceIntegrationTestSuite, userID uint) {
		s.Require().NoError(s.userService.SoftDeleteAccount(userID, nil))
	}},
}

// assertSignInOutcome checks a sign-in result against what the state allows.
func (suite *UserServiceIntegrationTestSuite) assertSignInOutcome(maySignIn bool, wantID uint, user *authm.User, err error) {
	suite.T().Helper()
	if maySignIn {
		suite.Require().NoError(err)
		suite.Require().NotNil(user)
		suite.Equal(wantID, user.ID)
		return
	}

	suite.Require().Error(err)
	suite.Nil(user)
	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeAccountInactive, authErr.Code)
	suite.Equal(apperrors.ErrAccountInactive().UserMessage(), authErr.UserMessage())
}

// A returning OAuth identity resolves to its account before anything else is
// consulted, so the account's state is the only thing between that identity
// and a session. Both entry points resolve a returning identity the same way.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_AccountState() {
	for _, state := range signInAccountStates {
		suite.Run(state.name, func() {
			identity := goth.User{
				UserID:  "google-state-" + state.name,
				Email:   "state." + state.name + "@example.com",
				RawData: map[string]any{"verified_email": true},
			}
			created, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", signupconsent.CurrentOAuthSignupConsent())
			suite.Require().NoError(err)
			state.apply(suite, created.ID)

			user, err := suite.userService.FindOrCreateUserWithConsent(identity, "google", signupconsent.CurrentOAuthSignupConsent())
			suite.assertSignInOutcome(state.maySignIn, created.ID, user, err)

			user, err = suite.userService.FindOrCreateUserWithConsent(identity, "google", nil)
			suite.assertSignInOutcome(state.maySignIn, created.ID, user, err)

			// A refusal leaves the identity attached to the one account.
			suite.assertOnlyIdentityIs(created.ID, identity.UserID)
		})
	}
}

func (suite *UserServiceIntegrationTestSuite) TestAuthenticateUserWithPassword_AccountState() {
	for _, state := range signInAccountStates {
		suite.Run(state.name, func() {
			address := "password.state." + state.name + "@example.com"
			created, err := suite.userService.CreateUserWithPasswordWithLegal(address, "CorrectPassword1!", "State", "User", signupconsent.CurrentLegalAcceptance())
			suite.Require().NoError(err)
			state.apply(suite, created.ID)

			user, err := suite.userService.AuthenticateUserWithPassword(address, "CorrectPassword1!")
			suite.assertSignInOutcome(state.maySignIn, created.ID, user, err)
		})
	}
}
