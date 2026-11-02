package user

import (
	"time"

	"github.com/markbates/goth"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// LinkOAuthAccountToUser attaches an identity only to an account that may sign
// in, for a session that has not been revoked, judged inside the transaction
// that writes the link.

// createLinkableAccount is createLiveAccount with a verified address, the
// state LinkOAuthAccountToUser requires before it attaches anything.
func (suite *UserServiceIntegrationTestSuite) createLinkableAccount(address string) *authm.User {
	suite.T().Helper()
	user := &authm.User{Email: stringPtr(address), IsActive: true, EmailVerified: true}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

// serviceWithWriteBeforeItsFirstQuery runs write before the service's first
// query. A link's first query is the first statement of its transaction, so
// write lands after any check a caller made beforehand and before the link
// reads anything.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstQuery(write func()) (*UserService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(testutil.BeforeQuery, func(*gorm.DB) { write() })
}

// serviceWithWriteBeforeItsFirstCreate runs write before the service's first
// INSERT, which a first connect issues after its reads.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstCreate(write func()) (*UserService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(testutil.BeforeCreate, func(*gorm.DB) { write() })
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AccountState() {
	for _, state := range signInAccountStates {
		suite.Run(state.name, func() {
			address := "link-state-" + state.name + "@example.com"
			owner := suite.createLinkableAccount(address)
			state.apply(suite, owner.ID)

			linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
				UserID: "link-state-" + state.name + "-subject",
				Email:  address,
			}, "google")

			suite.assertSignInOutcome(state.linkRefusal(), owner.ID, linked, err)
			if state.maySignIn() {
				suite.Require().Len(linked.OAuthAccounts, 1)
			} else {
				suite.assertNoOAuthAccountFor(owner.ID)
			}
		})
	}
}

// A repeat connect on an account that may not sign in is refused too, and the
// stored identity keeps the tokens it held.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectOnClosedAccount_Refused() {
	for _, state := range signInAccountStates {
		if state.maySignIn() {
			continue
		}
		suite.Run(state.name, func() {
			address := "link-state-repeat-" + state.name + "@example.com"
			subject := "link-state-repeat-" + state.name + "-subject"
			owner := suite.linkedForRepeatConnect(address, subject)
			state.apply(suite, owner.ID)

			linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
				UserID:      subject,
				Email:       address,
				AccessToken: "refused-access",
			}, "google")

			suite.assertSignInOutcome(apperrors.ErrAccountInactive(), owner.ID, linked, err)
			suite.Equal("stored-access", *suite.storedGoogleAccount(owner.ID).AccessToken)
		})
	}
}

// An account closed after the caller checked it, and before the link reads it,
// is still refused: the write itself enforces the rule.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AccountClosedAfterCallersCheck_Refused() {
	for _, state := range signInAccountStates {
		if state.maySignIn() {
			continue
		}
		suite.Run(state.name, func() {
			address := "link-state-window-" + state.name + "@example.com"
			owner := suite.createLinkableAccount(address)
			callerView, err := suite.userService.GetUserByID(owner.ID)
			suite.Require().NoError(err)
			suite.Require().True(callerView.CanSignIn())

			svc, ran := suite.serviceWithWriteBeforeItsFirstQuery(func() {
				state.apply(suite, owner.ID)
			})

			linked, err := svc.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
				UserID: "link-state-window-" + state.name + "-subject",
				Email:  address,
			}, "google")

			suite.Require().True(*ran, "the account must close inside the link's window")
			suite.assertSignInOutcome(apperrors.ErrAccountInactive(), owner.ID, linked, err)
			suite.assertNoOAuthAccountFor(owner.ID)
		})
	}
}

// Once the link has read the account as live, a deactivation waits for the
// link to commit, so the outcome is a link followed by a deactivation and
// never a link written to an account that was already closed.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_DeactivationWaitsForTheLinkThatReadTheAccount() {
	owner := suite.createLinkableAccount("link-state-serialized@example.com")

	var concurrent <-chan error
	waitedForLink := false
	svc, ran := suite.serviceWithWriteBeforeItsFirstCreate(func() {
		concurrent = suite.startOnAnotherConnection(func() error {
			return suite.db.Model(&authm.User{}).Where("id = ?", owner.ID).Update("is_active", false).Error
		})
		waitedForLink = len(concurrent) == 0
	})

	linked, err := svc.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
		UserID: "link-state-serialized-subject",
		Email:  "link-state-serialized@example.com",
	}, "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the deactivation must start before the link's write")
	suite.awaitConcurrentWrite(concurrent)

	suite.True(waitedForLink, "the deactivation must wait for the link holding the account row")
	suite.Equal(owner.ID, linked.ID)
	suite.Equal("link-state-serialized-subject", suite.storedGoogleAccount(owner.ID).ProviderUserID)
	suite.False(suite.reloadAccount(owner.ID).IsActive)
}

// revokeSessions writes a new sessions_valid_after for userID, the column
// every session revocation writes, and returns it.
func (suite *UserServiceIntegrationTestSuite) revokeSessions(userID uint) time.Time {
	suite.T().Helper()
	revokedAt := time.Now().UTC().Truncate(time.Second)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", userID).
		Update("sessions_valid_after", revokedAt).Error)
	return revokedAt
}

// A session revoked after the caller checked it, and before the link reads
// the account, links nothing: the write compares the session's revocation
// stamp with the account's under the row lock.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_SessionRevokedAfterCallersCheck_Refused() {
	owner := suite.createLinkableAccount("link-revoked-window@example.com")
	svc, ran := suite.serviceWithWriteBeforeItsFirstQuery(func() { suite.revokeSessions(owner.ID) })

	linked, err := svc.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, goth.User{
		UserID: "link-revoked-window-subject",
		Email:  "link-revoked-window@example.com",
	}, "github")

	suite.Require().True(*ran, "the revocation must land inside the link's window")
	suite.Nil(linked)
	var refusal *apperrors.AuthError
	suite.Require().ErrorAs(err, &refusal)
	suite.Equal(apperrors.CodeTokenInvalid, refusal.Code)
	suite.ErrorIs(err, shared.ErrSessionRevoked)
	suite.assertNoOAuthAccountFor(owner.ID)
}

// A session minted after the account's latest revocation carries its stamp
// and links.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_SessionMintedAfterARevocationLinks() {
	owner := suite.createLinkableAccount("link-after-revocation@example.com")
	revokedAt := suite.revokeSessions(owner.ID)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, &revokedAt, goth.User{
		UserID: "link-after-revocation-subject",
		Email:  "link-after-revocation@example.com",
	}, "github")

	suite.Require().NoError(err)
	suite.Require().Len(linked.OAuthAccounts, 1)
	suite.Equal("github", linked.OAuthAccounts[0].Provider)
}
