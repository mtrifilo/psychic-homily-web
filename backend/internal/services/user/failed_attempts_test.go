package user

import (
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
)

// serviceThatSoftDeletesBeforeItsFirstUpdate returns a UserService whose first
// UPDATE is preceded by a committed SoftDeleteAccount of userID on another
// connection. The returned flag reports whether the delete ran.
func (suite *UserServiceIntegrationTestSuite) serviceThatSoftDeletesBeforeItsFirstUpdate(userID uint) (*UserService, *bool) {
	suite.T().Helper()
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	hooked, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	suite.Require().NoError(err)

	deleted := false
	suite.Require().NoError(hooked.Callback().Update().Before("gorm:update").
		Register("test:soft_delete_before_update", func(*gorm.DB) {
			if deleted {
				return
			}
			suite.Require().NoError(suite.userService.SoftDeleteAccount(userID, stringPtr("deleted mid-lockout")))
			deleted = true
		}))
	return &UserService{db: hooked}, &deleted
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_DoesNotReviveAccountDeletedBeforeTheWrite() {
	user := suite.createLiveAccount("lockout-delete-race@example.com")
	svc, deleted := suite.serviceThatSoftDeletesBeforeItsFirstUpdate(user.ID)

	suite.Require().NoError(svc.IncrementFailedAttempts(user.ID))

	suite.Require().True(*deleted, "the soft delete must commit before the lockout UPDATE")
	got := suite.reloadAccount(user.ID)
	suite.False(got.IsActive)
	suite.NotNil(got.DeletedAt)
	suite.Require().NotNil(got.DeletionReason)
	suite.Equal("deleted mid-lockout", *got.DeletionReason)
	suite.False(got.CanSignIn())
	suite.Equal(1, got.FailedLoginAttempts)
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_LocksAtExactlyTheThreshold() {
	user := suite.createLiveAccount("lockout-exact-threshold@example.com")

	for i := 1; i < MaxFailedLoginAttempts; i++ {
		suite.Require().NoError(suite.userService.IncrementFailedAttempts(user.ID))
		got := suite.reloadAccount(user.ID)
		suite.Equal(i, got.FailedLoginAttempts)
		suite.Nil(got.LockedUntil, "attempt %d is below the threshold", i)
	}

	before := time.Now()
	suite.Require().NoError(suite.userService.IncrementFailedAttempts(user.ID))
	after := time.Now()

	got := suite.reloadAccount(user.ID)
	suite.Equal(MaxFailedLoginAttempts, got.FailedLoginAttempts)
	suite.Require().NotNil(got.LockedUntil)
	// locked_until is stored at microsecond precision, below the nanosecond bound.
	suite.False(got.LockedUntil.Before(before.Add(AccountLockDuration).Truncate(time.Microsecond)))
	suite.False(got.LockedUntil.After(after.Add(AccountLockDuration)))
	suite.True(suite.userService.IsAccountLocked(&got))
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_PastTheThresholdRelocksFromNow() {
	user := suite.createLiveAccount("lockout-past-threshold@example.com")
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{"failed_login_attempts": MaxFailedLoginAttempts + 2, "locked_until": time.Now().Add(-time.Minute)}).Error)

	suite.Require().NoError(suite.userService.IncrementFailedAttempts(user.ID))

	got := suite.reloadAccount(user.ID)
	suite.Equal(MaxFailedLoginAttempts+3, got.FailedLoginAttempts)
	suite.Require().NotNil(got.LockedUntil)
	suite.True(got.LockedUntil.After(time.Now().Add(AccountLockDuration - time.Minute)))
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_BelowTheThresholdKeepsTheExistingLock() {
	expired := time.Now().Add(-time.Minute)
	user := suite.createLiveAccount("lockout-below-threshold@example.com")
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", user.ID).
		Update("locked_until", expired).Error)

	suite.Require().NoError(suite.userService.IncrementFailedAttempts(user.ID))

	got := suite.reloadAccount(user.ID)
	suite.Equal(1, got.FailedLoginAttempts)
	suite.Require().NotNil(got.LockedUntil)
	suite.WithinDuration(expired, *got.LockedUntil, time.Millisecond)
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_UnknownUserIsAnError() {
	user := suite.createLiveAccount("lockout-unknown-neighbour@example.com")

	err := suite.userService.IncrementFailedAttempts(user.ID + 1_000_000)

	suite.ErrorIs(err, gorm.ErrRecordNotFound)
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_ConcurrentFailuresAreAllCounted() {
	user := suite.createLiveAccount("lockout-concurrent@example.com")
	const workers, perWorker = 2, 25

	start := make(chan struct{})
	errs := make(chan error, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range perWorker {
				errs <- suite.userService.IncrementFailedAttempts(user.ID)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		suite.Require().NoError(err)
	}

	got := suite.reloadAccount(user.ID)
	suite.Equal(workers*perWorker, got.FailedLoginAttempts)
	suite.NotNil(got.LockedUntil)
}
