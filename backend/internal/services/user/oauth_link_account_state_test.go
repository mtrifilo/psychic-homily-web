package user

import (
	"github.com/markbates/goth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// LinkOAuthAccountToUser attaches an identity only to an account that may sign
// in, judged inside the transaction that writes the link.

// closedAccountStates are the account states a link must refuse.
var closedAccountStates = []struct {
	name  string
	close func(s *UserServiceIntegrationTestSuite, userID uint)
}{
	{name: "deactivated", close: func(s *UserServiceIntegrationTestSuite, userID uint) {
		s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
	}},
	{name: "soft-deleted", close: func(s *UserServiceIntegrationTestSuite, userID uint) {
		s.Require().NoError(s.userService.SoftDeleteAccount(userID, nil))
	}},
}

func (suite *UserServiceIntegrationTestSuite) assertLinkRefusedAsInactive(linked *authm.User, err error) {
	suite.T().Helper()
	suite.Require().Error(err)
	suite.Nil(linked)
	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeAccountInactive, authErr.Code)
}

// serviceWithWriteBeforeItsFirstQuery returns a UserService whose first query
// is preceded by write. A link's first query is the first statement of its
// transaction, so write lands after any check a caller made beforehand and
// before the link reads anything. The returned flag reports whether write ran.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstQuery(write func()) (*UserService, *bool) {
	suite.T().Helper()
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	hooked, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	suite.Require().NoError(err)

	ran := false
	suite.Require().NoError(hooked.Callback().Query().Before("gorm:query").
		Register("test:write_before_query", func(*gorm.DB) {
			if ran {
				return
			}
			ran = true
			write()
		}))
	return &UserService{db: hooked}, &ran
}

// serviceWithWriteBeforeItsFirstCreate is serviceWithWriteBeforeItsFirstQuery
// for the first insert, which a first connect issues after its reads.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstCreate(write func()) (*UserService, *bool) {
	suite.T().Helper()
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	hooked, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	suite.Require().NoError(err)

	ran := false
	suite.Require().NoError(hooked.Callback().Create().Before("gorm:create").
		Register("test:write_before_create", func(*gorm.DB) {
			if ran {
				return
			}
			ran = true
			write()
		}))
	return &UserService{db: hooked}, &ran
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_LiveAccount_Links() {
	owner := suite.createLiveAccount("link-state-live@example.com")

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID: "link-state-live-subject",
		Email:  "link-state-live@example.com",
	}, "google")

	suite.Require().NoError(err)
	suite.Equal(owner.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_ClosedAccount_Refused() {
	for _, state := range closedAccountStates {
		suite.Run(state.name, func() {
			owner := suite.createLiveAccount("link-state-" + state.name + "@example.com")
			state.close(suite, owner.ID)

			linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
				UserID: "link-state-" + state.name + "-subject",
				Email:  "link-state-" + state.name + "@example.com",
			}, "google")

			suite.assertLinkRefusedAsInactive(linked, err)
			suite.assertNoOAuthAccountFor(owner.ID)
		})
	}
}

// A repeat connect on a closed account is refused too, and the stored identity
// keeps the tokens it held.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectOnClosedAccount_Refused() {
	for _, state := range closedAccountStates {
		suite.Run(state.name, func() {
			subject := "link-state-repeat-" + state.name + "-subject"
			owner := suite.linkedForRepeatConnect("link-state-repeat-"+state.name+"@example.com", subject)
			state.close(suite, owner.ID)

			linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
				UserID:      subject,
				Email:       "link-state-repeat-" + state.name + "@example.com",
				AccessToken: "refused-access",
			}, "google")

			suite.assertLinkRefusedAsInactive(linked, err)
			suite.Equal("stored-access", *suite.storedGoogleAccount(owner.ID).AccessToken)
		})
	}
}

// An account closed after the caller checked it, and before the link reads it,
// is still refused: the write itself enforces the rule.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_AccountClosedAfterCallersCheck_Refused() {
	for _, state := range closedAccountStates {
		suite.Run(state.name, func() {
			owner := suite.createLiveAccount("link-state-window-" + state.name + "@example.com")
			callerView, err := suite.userService.GetUserByID(owner.ID)
			suite.Require().NoError(err)
			suite.Require().True(callerView.CanSignIn())

			svc, ran := suite.serviceWithWriteBeforeItsFirstQuery(func() {
				state.close(suite, owner.ID)
			})

			linked, err := svc.LinkOAuthAccountToUser(owner.ID, goth.User{
				UserID: "link-state-window-" + state.name + "-subject",
				Email:  "link-state-window-" + state.name + "@example.com",
			}, "google")

			suite.Require().True(*ran, "the account must close inside the link's window")
			suite.assertLinkRefusedAsInactive(linked, err)
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
