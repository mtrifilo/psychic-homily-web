package user

import (
	"time"

	"github.com/markbates/goth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// A repeat connect is LinkOAuthAccountToUser on an identity the account
// already holds. These tests place a second write on the same oauth_accounts
// row between the repeat connect's read of the row and its write, through a
// GORM Before("gorm:update") callback on the handle the service under test
// uses.

var (
	repeatConnectStoredExpiry     = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	repeatConnectConcurrentExpiry = time.Date(2031, 6, 1, 0, 0, 0, 0, time.UTC)
)

// linkedForRepeatConnect creates an account holding a google identity whose
// row carries known tokens and repeatConnectStoredExpiry.
func (suite *UserServiceIntegrationTestSuite) linkedForRepeatConnect(address, subject string) *authm.User {
	suite.T().Helper()
	owner := suite.createLiveAccount(address)
	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:       subject,
		Email:        address,
		AccessToken:  "stored-access",
		RefreshToken: "stored-refresh",
		ExpiresAt:    repeatConnectStoredExpiry,
	}, "google")
	suite.Require().NoError(err)
	return owner
}

func (suite *UserServiceIntegrationTestSuite) storedGoogleAccount(userID uint) authm.OAuthAccount {
	suite.T().Helper()
	var account authm.OAuthAccount
	suite.Require().NoError(suite.db.Where("user_id = ? AND provider = ?", userID, "google").First(&account).Error)
	return account
}

// serviceWithWriteBeforeItsFirstUpdate returns a UserService whose first
// UPDATE is preceded by write, which receives the in-flight statement's
// handle. The returned flag reports whether write ran.
func (suite *UserServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstUpdate(write func(inFlight *gorm.DB)) (*UserService, *bool) {
	suite.T().Helper()
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	hooked, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	suite.Require().NoError(err)

	ran := false
	suite.Require().NoError(hooked.Callback().Update().Before("gorm:update").
		Register("test:write_before_update", func(inFlight *gorm.DB) {
			if ran {
				return
			}
			ran = true
			write(inFlight)
		}))
	return &UserService{db: hooked}, &ran
}

// startOnAnotherConnection runs write in a goroutine on the suite's own handle
// and returns once write has either finished or is waiting on a lock. The
// returned channel delivers write's result.
func (suite *UserServiceIntegrationTestSuite) startOnAnotherConnection(write func() error) <-chan error {
	suite.T().Helper()
	done := make(chan error, 1)
	go func() { done <- write() }()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(done) > 0 {
			return done
		}
		var waiting int64
		suite.Require().NoError(suite.db.Raw(
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
		).Scan(&waiting).Error)
		if waiting > 0 {
			return done
		}
		time.Sleep(5 * time.Millisecond)
	}
	suite.FailNow("the concurrent write neither finished nor waited on a lock")
	return done
}

func (suite *UserServiceIntegrationTestSuite) awaitConcurrentWrite(done <-chan error) {
	suite.T().Helper()
	select {
	case err := <-done:
		suite.Require().NoError(err)
	case <-time.After(10 * time.Second):
		suite.FailNow("the concurrent write did not finish")
	}
}

// Two repeat connects on one identity: the row ends up holding one connect's
// tokens and expiry together, never one connect's tokens with an expiry that
// the other connect's write had already replaced.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectDoesNotUndoAConcurrentConnect() {
	owner := suite.linkedForRepeatConnect("repeat-connect-race@example.com", "repeat-connect-race-subject")

	var concurrent <-chan error
	svc, ran := suite.serviceWithWriteBeforeItsFirstUpdate(func(*gorm.DB) {
		concurrent = suite.startOnAnotherConnection(func() error {
			_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
				UserID:       "repeat-connect-race-subject",
				Email:        "repeat-connect-race@example.com",
				AccessToken:  "concurrent-access",
				RefreshToken: "concurrent-refresh",
				ExpiresAt:    repeatConnectConcurrentExpiry,
			}, "google")
			return err
		})
	})

	// This connect's provider sent no expiry.
	_, err := svc.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:       "repeat-connect-race-subject",
		Email:        "repeat-connect-race@example.com",
		AccessToken:  "hooked-access",
		RefreshToken: "hooked-refresh",
	}, "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the concurrent connect must start before the repeat connect's write")
	suite.awaitConcurrentWrite(concurrent)

	got := suite.storedGoogleAccount(owner.ID)
	suite.Require().NotNil(got.ExpiresAt)
	suite.WithinDuration(repeatConnectConcurrentExpiry, *got.ExpiresAt, 0,
		"the concurrent connect's expiry must survive the repeat connect")
	suite.Equal("concurrent-access", *got.AccessToken)
	suite.Equal("concurrent-refresh", *got.RefreshToken)
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

	_, err := svc.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:      "repeat-connect-unlink-subject",
		Email:       "repeat-connect-unlink@example.com",
		AccessToken: "hooked-access",
	}, "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the unlink must start before the repeat connect's write")
	suite.awaitConcurrentWrite(concurrent)

	var rows int64
	suite.Require().NoError(suite.db.Model(&authm.OAuthAccount{}).Where("user_id = ?", owner.ID).Count(&rows).Error)
	suite.Zero(rows, "the unlinked identity must not be written back")
}

// The repeat connect writes the profile, the tokens, the expiry when the
// provider sent one, and updated_at. A value written to any other column after
// its read is not replaced by what it read.
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectWritesOnlyTheColumnsItOwns() {
	owner := suite.linkedForRepeatConnect("repeat-connect-columns@example.com", "repeat-connect-columns-subject")
	writtenAfterRead := time.Date(2032, 3, 1, 0, 0, 0, 0, time.UTC)

	svc, ran := suite.serviceWithWriteBeforeItsFirstUpdate(func(inFlight *gorm.DB) {
		suite.Require().NoError(inFlight.Session(&gorm.Session{NewDB: true}).Exec(
			"UPDATE oauth_accounts SET expires_at = ?, created_at = ? WHERE user_id = ? AND provider = ?",
			writtenAfterRead, writtenAfterRead, owner.ID, "google").Error)
	})

	_, err := svc.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:       "repeat-connect-columns-subject",
		Email:        "repeat-connect-columns-new@example.com",
		AccessToken:  "hooked-access",
		RefreshToken: "hooked-refresh",
	}, "google")
	suite.Require().NoError(err)
	suite.Require().True(*ran)

	got := suite.storedGoogleAccount(owner.ID)
	suite.Require().NotNil(got.ExpiresAt)
	suite.WithinDuration(writtenAfterRead, *got.ExpiresAt, 0, "expires_at is not written when the provider sent none")
	suite.WithinDuration(writtenAfterRead, got.CreatedAt, 0, "created_at is never written by a repeat connect")
	suite.Equal("repeat-connect-columns-new@example.com", *got.ProviderEmail)
	suite.Equal("hooked-access", *got.AccessToken)
	suite.Equal("hooked-refresh", *got.RefreshToken)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectRefreshesProfileTokensAndExpiry() {
	owner := suite.linkedForRepeatConnect("repeat-connect-refresh@example.com", "repeat-connect-refresh-subject")
	before := suite.storedGoogleAccount(owner.ID)
	refreshedExpiry := time.Date(2033, 2, 1, 0, 0, 0, 0, time.UTC)

	linked, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:       "repeat-connect-refresh-subject",
		Email:        "repeat-connect-refresh-new@example.com",
		Name:         "Refreshed Name",
		AvatarURL:    "https://example.com/refreshed.png",
		AccessToken:  "refreshed-access",
		RefreshToken: "refreshed-refresh",
		ExpiresAt:    refreshedExpiry,
	}, "google")
	suite.Require().NoError(err)
	suite.Equal(owner.ID, linked.ID)
	suite.Require().Len(linked.OAuthAccounts, 1)

	got := suite.storedGoogleAccount(owner.ID)
	suite.Equal(before.ID, got.ID)
	suite.Equal("repeat-connect-refresh-subject", got.ProviderUserID)
	suite.Equal("repeat-connect-refresh-new@example.com", *got.ProviderEmail)
	suite.Equal("Refreshed Name", *got.ProviderName)
	suite.Equal("https://example.com/refreshed.png", *got.ProviderAvatarURL)
	suite.Equal("refreshed-access", *got.AccessToken)
	suite.Equal("refreshed-refresh", *got.RefreshToken)
	suite.Require().NotNil(got.ExpiresAt)
	suite.WithinDuration(refreshedExpiry, *got.ExpiresAt, 0)
	suite.True(got.UpdatedAt.After(before.UpdatedAt), "updated_at advances on a repeat connect")
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccountToUser_RepeatConnectWithoutExpiryKeepsTheStoredOne() {
	owner := suite.linkedForRepeatConnect("repeat-connect-no-expiry@example.com", "repeat-connect-no-expiry-subject")

	_, err := suite.userService.LinkOAuthAccountToUser(owner.ID, goth.User{
		UserID:      "repeat-connect-no-expiry-subject",
		Email:       "repeat-connect-no-expiry@example.com",
		AccessToken: "refreshed-access",
	}, "google")
	suite.Require().NoError(err)

	got := suite.storedGoogleAccount(owner.ID)
	suite.Equal("refreshed-access", *got.AccessToken)
	suite.Require().NotNil(got.ExpiresAt)
	suite.WithinDuration(repeatConnectStoredExpiry, *got.ExpiresAt, 0)
}
