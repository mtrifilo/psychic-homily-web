package user

import (
	"time"

	"github.com/markbates/goth"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// An account that may not sign in is refused on every sign-in path this
// service resolves, and an active one signs in. A deleted account inside its
// recovery window is refused with ACCOUNT_RECOVERABLE; every other closed
// account with ACCOUNT_INACTIVE.

// signInAccountState is an account state a sign-in path must tell apart.
type signInAccountState struct {
	name  string
	apply func(s *UserServiceIntegrationTestSuite, userID uint)
	// refusal is the refusal a proved credential gets in this state, or nil
	// when the state may sign in.
	refusal *apperrors.AuthError
}

func (state signInAccountState) maySignIn() bool { return state.refusal == nil }

// recoverable is a deleted account inside its recovery window.
func (state signInAccountState) recoverable() bool {
	return state.refusal != nil && state.refusal.Code == apperrors.CodeAccountRecoverable
}

// linkRefusal is the refusal LinkOAuthAccountToUser gives in this state, or nil
// when the state may sign in. A link names no recovery: its caller holds a
// session, which no closed account may.
func (state signInAccountState) linkRefusal() *apperrors.AuthError {
	if state.maySignIn() {
		return nil
	}
	return apperrors.ErrAccountInactive()
}

// signInAccountStates are the states every sign-in path is tested in.
// users_live_account_check refuses an active row with deleted_at set, so that
// combination is covered by the CanSignIn unit test, not here.
var signInAccountStates = []signInAccountState{
	{name: "active", apply: func(*UserServiceIntegrationTestSuite, uint) {}},
	{name: "deactivated", apply: func(s *UserServiceIntegrationTestSuite, userID uint) {
		s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
	}, refusal: apperrors.ErrAccountInactive()},
	{name: "soft-deleted", apply: func(s *UserServiceIntegrationTestSuite, userID uint) {
		s.Require().NoError(s.userService.SoftDeleteAccount(userID, nil))
	}, refusal: apperrors.ErrAccountRecoverable()},
	{name: "soft-deleted-past-window", apply: (*UserServiceIntegrationTestSuite).softDeletePastRecoveryWindow,
		refusal: apperrors.ErrAccountInactive()},
}

// softDeletePastRecoveryWindow soft-deletes the account with a deleted_at a day
// older than the recovery window.
func (suite *UserServiceIntegrationTestSuite) softDeletePastRecoveryWindow(userID uint) {
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", userID).Updates(map[string]any{
		"is_active":  false,
		"deleted_at": time.Now().Add(-AccountRecoveryGracePeriod - 24*time.Hour),
	}).Error)
}

// assertSignInOutcome checks a sign-in result against what the state allows:
// the account itself when refusal is nil, otherwise exactly that refusal.
func (suite *UserServiceIntegrationTestSuite) assertSignInOutcome(refusal *apperrors.AuthError, wantID uint, user *authm.User, err error) {
	suite.T().Helper()
	if refusal == nil {
		suite.Require().NoError(err)
		suite.Require().NotNil(user)
		suite.Equal(wantID, user.ID)
		return
	}

	suite.Require().Error(err)
	suite.Nil(user)
	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(refusal.Code, authErr.Code)
	suite.Equal(refusal.UserMessage(), authErr.UserMessage())
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
			suite.assertSignInOutcome(state.refusal, created.ID, user, err)

			user, err = suite.userService.FindOrCreateUserWithConsent(identity, "google", nil)
			suite.assertSignInOutcome(state.refusal, created.ID, user, err)

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
			suite.assertSignInOutcome(state.refusal, created.ID, user, err)

			// A wrong password learns nothing of the account's state.
			user, err = suite.userService.AuthenticateUserWithPassword(address, "WrongPassword1!")
			suite.assertSignInOutcome(apperrors.ErrInvalidCredentials(nil), created.ID, user, err)
		})
	}
}

// providerVouches are the shapes of a provider's email-verification claim: a
// vouch, an explicit refusal to vouch, and no claim at all.
var providerVouches = []struct {
	kind    string
	rawData map[string]any
	vouched bool
}{
	{kind: "vouched", rawData: map[string]any{"verified_email": true}, vouched: true},
	{kind: "unvouched", rawData: map[string]any{"verified_email": false}},
	{kind: "no-claim", rawData: nil},
}

// An OAuth identity attached to no account, whose address belongs to a
// verified one, is refused with OAUTH_LINK_REFUSED in every state of that
// account except one: a recoverable deleted account, when the provider vouches
// for the address, is refused with ACCOUNT_RECOVERABLE. Nothing is attached and
// nothing is created either way.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_AddressOfClosedAccount() {
	for _, state := range signInAccountStates {
		for _, vouch := range providerVouches {
			suite.Run(state.name+"/"+vouch.kind, func() {
				suffix := state.name + "." + vouch.kind
				address := "address.state." + suffix + "@example.com"
				existing, err := suite.userService.CreateUserWithPasswordWithLegal(address, "CorrectPassword1!", "State", "User", signupconsent.CurrentLegalAcceptance())
				suite.Require().NoError(err)
				suite.Require().NoError(suite.userService.SetEmailVerified(existing.ID, true))
				state.apply(suite, existing.ID)

				user, err := suite.userService.FindOrCreateUserWithConsent(goth.User{
					UserID:  "address-state-" + suffix,
					Email:   address,
					RawData: vouch.rawData,
				}, "google", signupconsent.CurrentOAuthSignupConsent())

				want := apperrors.ErrOAuthLinkRefused(address)
				if vouch.vouched && state.recoverable() {
					want = apperrors.ErrAccountRecoverable()
				}
				suite.assertSignInOutcome(want, existing.ID, user, err)

				suite.assertNoOAuthAccountFor(existing.ID)
				suite.assertSingleUserForEmail(address)
			})
		}
	}
}

// A recoverable deleted account whose address was never verified may be a
// squat on the caller's address, and recovery would restore it with the
// squatter's password on it. A vouched OAuth sign-in on that address is
// refused with OAUTH_LINK_REFUSED, never sent to recovery.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_AddressOfRecoverableUnverifiedAccount() {
	const address = "address.recoverable.unverified@example.com"
	squatted, err := suite.userService.CreateUserWithPasswordWithLegal(address, "Squatter123!", "Squat", "Ter", signupconsent.CurrentLegalAcceptance())
	suite.Require().NoError(err)
	suite.Require().False(squatted.EmailVerified)
	suite.Require().NoError(suite.userService.SoftDeleteAccount(squatted.ID, nil))

	user, err := suite.userService.FindOrCreateUserWithConsent(goth.User{
		UserID:  "address-recoverable-unverified-owner",
		Email:   address,
		RawData: map[string]any{"verified_email": true},
	}, "google", signupconsent.CurrentOAuthSignupConsent())

	suite.assertSignInOutcome(apperrors.ErrOAuthLinkRefused(address), squatted.ID, user, err)
	suite.assertNoOAuthAccountFor(squatted.ID)
	suite.assertSingleUserForEmail(address)
}

// Account recovery finds and mails an account by its address, so a recoverable
// deleted account with none is not sent there: its identity's sign-in is
// refused with ACCOUNT_INACTIVE.
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUserWithConsent_RecoverableAccountWithNoAddress() {
	identity := goth.User{UserID: "github-no-address"}
	created, err := suite.userService.FindOrCreateUserWithConsent(identity, "github", signupconsent.CurrentOAuthSignupConsent())
	suite.Require().NoError(err)
	suite.Require().Nil(created.Email)
	suite.Require().NoError(suite.userService.SoftDeleteAccount(created.ID, nil))

	user, err := suite.userService.FindOrCreateUserWithConsent(identity, "github", nil)
	suite.assertSignInOutcome(apperrors.ErrAccountInactive(), created.ID, user, err)
}
