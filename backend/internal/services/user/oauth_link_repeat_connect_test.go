package user

import (
	"time"

	"github.com/markbates/goth"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// A repeat connect is LinkOAuthAccountToUser on an identity the account
// already holds.

// linkedForRepeatConnect creates an account holding a google identity, linked
// by a connect whose provider sent tokens and an expiry.
func (suite *UserServiceIntegrationTestSuite) linkedForRepeatConnect(address, subject string) *authm.User {
	suite.T().Helper()
	owner := suite.createLinkableAccount(address)
	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, withProviderCredentials(goth.User{
		UserID: subject,
		Email:  address,
	}), "google")
	suite.Require().NoError(err)
	return owner
}

func (suite *UserServiceIntegrationTestSuite) storedGoogleAccount(userID uint) authm.OAuthAccount {
	suite.T().Helper()
	var account authm.OAuthAccount
	suite.Require().NoError(suite.db.Where("user_id = ? AND provider = ?", userID, "google").First(&account).Error)
	return account
}

// serviceWithWriteBeforeItsFirst returns a UserService on the suite's
// connections that runs write once, before the first statement hook fires on
// (testutil.WithWriteBeforeItsFirst). The returned flag reports whether write
// ran.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirst(hook testutil.Hook, write func(inFlight *gorm.DB)) (*UserService, *bool) {
	suite.T().Helper()
	hooked, ran := testutil.WithWriteBeforeItsFirst(suite.T(), suite.db, write, hook)
	return &UserService{db: hooked}, ran
}

// serviceWithWriteBeforeItsFirstUpdate runs write before the service's first
// UPDATE, which a repeat connect issues after it has read the row.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstUpdate(write func(inFlight *gorm.DB)) (*UserService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(testutil.BeforeUpdate, write)
}

// startOnAnotherConnection is testutil.StartOnAnotherConnection on the
// suite's own handle.
func (suite *UserServiceIntegrationTestSuite) startOnAnotherConnection(write func() error) <-chan error {
	suite.T().Helper()
	return testutil.StartOnAnotherConnection(suite.T(), suite.db, write)
}

func (suite *UserServiceIntegrationTestSuite) awaitConcurrentWrite(done <-chan error) {
	suite.T().Helper()
	suite.Require().NoError(testutil.AwaitConcurrentWrite(suite.T(), done))
}

// Two repeat connects on one identity serialize on the row lock
// checkOAuthIdentityAttach takes: a connect that reaches the row while another
// holds it waits, so its write lands after the other's and its profile is the
// one stored.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectsOnOneIdentitySerialize() {
	owner := suite.linkedForRepeatConnect("repeat-connect-race@example.com", "repeat-connect-race-subject")

	var concurrent <-chan error
	svc, ran := suite.serviceWithWriteBeforeItsFirstUpdate(func(*gorm.DB) {
		concurrent = suite.startOnAnotherConnection(func() error {
			_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, withProviderCredentials(goth.User{
				UserID:    "repeat-connect-race-subject",
				Email:     "repeat-connect-race-concurrent@example.com",
				Name:      "Concurrent Name",
				AvatarURL: "https://example.com/concurrent.png",
			}), "google")
			return err
		})
	})

	_, err := svc.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, withProviderCredentials(goth.User{
		UserID:    "repeat-connect-race-subject",
		Email:     "repeat-connect-race-hooked@example.com",
		Name:      "Hooked Name",
		AvatarURL: "https://example.com/hooked.png",
	}), "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the concurrent connect must start before the repeat connect's write")
	suite.awaitConcurrentWrite(concurrent)

	got := suite.storedGoogleAccount(owner.ID)
	suite.Equal("repeat-connect-race-concurrent@example.com", *got.ProviderEmail)
	suite.Equal("Concurrent Name", *got.ProviderName)
	suite.Equal("https://example.com/concurrent.png", *got.ProviderAvatarURL)
	suite.requireNoProviderCredential(got)
}

// A disconnect that lands while a repeat connect is in flight stays
// disconnected.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectDoesNotRestoreAConcurrentUnlink() {
	owner := suite.linkedForRepeatConnect("repeat-connect-unlink@example.com", "repeat-connect-unlink-subject")

	var concurrent <-chan error
	svc, ran := suite.serviceWithWriteBeforeItsFirstUpdate(func(*gorm.DB) {
		concurrent = suite.startOnAnotherConnection(func() error {
			return suite.userService.UnlinkOAuthAccount(owner.ID, "google")
		})
	})

	_, err := svc.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, withProviderCredentials(goth.User{
		UserID: "repeat-connect-unlink-subject",
		Email:  "repeat-connect-unlink@example.com",
	}), "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the unlink must start before the repeat connect's write")
	suite.awaitConcurrentWrite(concurrent)

	var rows int64
	suite.Require().NoError(suite.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", owner.ID).Count(&rows).Error)
	suite.Zero(rows, "the unlinked identity must not be written back")
}

// The repeat connect writes the profile, NULL to the three credential columns,
// and updated_at. A value written to any other column after its read is not replaced by what it read. The row lock
// keeps other transactions out of that window, so the write here runs inside
// the repeat connect's own transaction.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectWritesOnlyTheColumnsItOwns() {
	owner := suite.linkedForRepeatConnect("repeat-connect-columns@example.com", "repeat-connect-columns-subject")
	writtenAfterRead := time.Date(2032, 3, 1, 0, 0, 0, 0, time.UTC)

	svc, ran := suite.serviceWithWriteBeforeItsFirstUpdate(func(inFlight *gorm.DB) {
		suite.Require().NoError(inFlight.Session(&gorm.Session{NewDB: true}).Exec(
			"UPDATE oauth_accounts SET created_at = ? WHERE user_id = ? AND provider = ?",
			writtenAfterRead, owner.ID, "google").Error)
	})

	_, err := svc.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, withProviderCredentials(goth.User{
		UserID: "repeat-connect-columns-subject",
		Email:  "repeat-connect-columns-new@example.com",
	}), "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran)

	got := suite.storedGoogleAccount(owner.ID)
	suite.WithinDuration(writtenAfterRead, got.CreatedAt, 0, "created_at is never written by a repeat connect")
	suite.Equal("repeat-connect-columns-new@example.com", *got.ProviderEmail)
	suite.requireNoProviderCredential(got)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectRefreshesTheProfile() {
	owner := suite.linkedForRepeatConnect("repeat-connect-refresh@example.com", "repeat-connect-refresh-subject")
	before := suite.storedGoogleAccount(owner.ID)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, owner.SessionsValidAfter, withProviderCredentials(goth.User{
		UserID:    "repeat-connect-refresh-subject",
		Email:     "repeat-connect-refresh-new@example.com",
		Name:      "Refreshed Name",
		AvatarURL: "https://example.com/refreshed.png",
	}), "google")
	suite.Require().NoError(err)
	suite.Equal(owner.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)

	got := suite.storedGoogleAccount(owner.ID)
	suite.Equal(before.ID, got.ID)
	suite.Equal("repeat-connect-refresh-subject", got.ProviderUserID)
	suite.Equal("repeat-connect-refresh-new@example.com", *got.ProviderEmail)
	suite.Equal("Refreshed Name", *got.ProviderName)
	suite.Equal("https://example.com/refreshed.png", *got.ProviderAvatarURL)
	suite.requireNoProviderCredential(got)
	suite.True(got.UpdatedAt.After(before.UpdatedAt), "updated_at advances on a repeat connect")
}
