package user

import (
	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The two squats, end to end, each ending in a refusal.
//
// Both work the same way: an attacker gets an account onto an address they do
// not own, and the real owner then proves the mailbox, which is what the
// unauthenticated verification resend lets them do for a row they do not hold.
//
// These pin that proving the address changes nothing here: a matching address
// is never a reason to attach an identity, however verified it is.

// Squat one: the account is created by an OAuth sign-in whose provider vouches
// for nothing.
func (suite *UserServiceIntegrationTestSuite) TestSquat_OAuthCreated_VerifiedOwnerStillRefused() {
	const address = "squat.oauth@example.com"

	squatted, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "squat-oauth-attacker",
		Email:   address,
		RawData: map[string]any{"verified_email": false},
	}, "google")
	suite.Require().NoError(err)
	suite.Require().False(squatted.EmailVerified)

	// The owner proves the mailbox: exactly what the resend hands them.
	suite.Require().NoError(suite.userService.SetEmailVerified(squatted.ID, true))
	suite.assertStoredEmailVerified(squatted.ID, true)

	// The owner now signs in with a provider that DOES vouch for the address.
	owner, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "squat-oauth-real-owner",
		Email:   address,
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(owner)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)

	suite.assertOnlyIdentityIs(squatted.ID, "squat-oauth-attacker")
	suite.assertSingleUserForEmail(address)
}

// Squat two: the account is created by password registration, which never
// proves the address either.
func (suite *UserServiceIntegrationTestSuite) TestSquat_PasswordRegistered_VerifiedOwnerStillRefused() {
	const address = "squat.password@example.com"

	squatted, err := suite.userService.CreateUserWithPassword(address, "Attacker123!", "Squat", "Ter")
	suite.Require().NoError(err)
	suite.Require().False(squatted.EmailVerified)

	suite.Require().NoError(suite.userService.SetEmailVerified(squatted.ID, true))
	suite.assertStoredEmailVerified(squatted.ID, true)

	owner, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "squat-password-real-owner",
		Email:   address,
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(owner)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)
	// The refusal has to name the way in, or a refused owner has nowhere to go.
	suite.Contains(authErr.UserMessage(), "Settings")

	var rows int64
	suite.Require().NoError(
		suite.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", squatted.ID).Count(&rows).Error)
	suite.Equal(int64(0), rows, "the refused owner must not have been attached")
	suite.assertSingleUserForEmail(address)
}

// A verified address on a verified account is still not a link reason. This is
// the case most likely to be re-admitted by someone reasoning that both sides
// look trustworthy, so it is pinned on its own.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_VerifiedAccount_VerifiedProvider_StillRefused() {
	existing := &authm.User{
		Email:         stringPtr("fully.verified@example.com"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(existing).Error)

	linked, err := suite.userService.FindOrCreateUser(goth.User{
		UserID:  "verified-into-verified",
		Email:   "fully.verified@example.com",
		RawData: map[string]any{"verified_email": true},
	}, "google")

	suite.Require().Error(err)
	suite.Require().Nil(linked)

	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeOAuthLinkRefused, authErr.Code)
	suite.assertNoOAuthAccountFor(existing.ID)
}

func (suite *UserServiceIntegrationTestSuite) assertOnlyIdentityIs(userID uint, subject string) {
	suite.T().Helper()
	var accounts []authm.OAuthAccount
	suite.Require().NoError(
		suite.db.Where("user_id = ?", userID).Find(&accounts).Error)
	suite.Require().Len(accounts, 1, "the account must hold exactly the identity it started with")
	suite.Equal(subject, accounts[0].ProviderUserID)
}
