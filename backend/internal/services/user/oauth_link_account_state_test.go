package user

import (
	"github.com/markbates/goth"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// LinkOAuthAccountToUser attaches an identity only to an account that may sign
// in, judged inside the transaction that writes the link.

// serviceWithWriteBeforeItsFirstQuery runs write before the service's first
// query. A link's first query is the first statement of its transaction, so
// write lands after any check a caller made beforehand and before the link
// reads anything.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstQuery(write func()) (*UserService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(func(hooked *gorm.DB, hook func(*gorm.DB)) error {
		return hooked.Callback().Query().Before("gorm:query").Register("test:write_before_query", hook)
	}, func(*gorm.DB) { write() })
}

// serviceWithWriteBeforeItsFirstCreate runs write before the service's first
// INSERT, which a first connect issues after its reads.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstCreate(write func()) (*UserService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(func(hooked *gorm.DB, hook func(*gorm.DB)) error {
		return hooked.Callback().Create().Before("gorm:create").Register("test:write_before_create", hook)
	}, func(*gorm.DB) { write() })
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AccountState() {
	for _, state := range signInAccountStates {
		suite.Run(state.name, func() {
			address := "link-state-" + state.name + "@example.com"
			owner := suite.createLiveAccount(address)
			state.apply(suite, owner.ID)

			linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
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

			linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
				UserID:      subject,
				Email:       address,
				AccessToken: "refused-access",
			}, "google")

			suite.assertSignInOutcome(apperrors.ErrAccountInactive, owner.ID, linked, err)
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
			owner := suite.createLiveAccount(address)
			callerView, err := suite.userService.GetUserByID(owner.ID)
			suite.Require().NoError(err)
			suite.Require().True(callerView.CanSignIn())

			svc, ran := suite.serviceWithWriteBeforeItsFirstQuery(func() {
				state.apply(suite, owner.ID)
			})

			linked, err := svc.LinkOAuthAccountToUser(owner.ID, goth.User{
				UserID: "link-state-window-" + state.name + "-subject",
				Email:  address,
			}, "google")

			suite.Require().True(*ran, "the account must close inside the link's window")
			suite.assertSignInOutcome(apperrors.ErrAccountInactive, owner.ID, linked, err)
			suite.assertNoOAuthAccountFor(owner.ID)
		})
	}
}

// Once the link has read the account as live, a deactivation waits for the
// link to commit, so the outcome is a link followed by a deactivation and
// never a link written to an account that was already closed.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_DeactivationWaitsForTheLinkThatReadTheAccount() {
	owner := suite.createLiveAccount("link-state-serialized@example.com")

	var concurrent <-chan error
	waitedForLink := false
	svc, ran := suite.serviceWithWriteBeforeItsFirstCreate(func() {
		concurrent = suite.startOnAnotherConnection(func() error {
			return suite.db.Model(&authm.User{}).Where("id = ?", owner.ID).Update("is_active", false).Error
		})
		waitedForLink = len(concurrent) == 0
	})

	linked, err := svc.LinkOAuthAccountToUser(owner.ID, goth.User{
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
