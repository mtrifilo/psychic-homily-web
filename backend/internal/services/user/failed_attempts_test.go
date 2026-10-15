package user

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
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

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_DatabaseErrorIsReturnedWrapped() {
	user := suite.createLiveAccount("lockout-db-error@example.com")

	holder := suite.db.Begin()
	defer holder.Rollback()
	suite.Require().NoError(holder.Exec("SELECT id FROM users WHERE id = ? FOR UPDATE", user.ID).Error)

	blocked := suite.db.Begin()
	defer blocked.Rollback()
	suite.Require().NoError(blocked.Exec("SET LOCAL lock_timeout = '50ms'").Error)

	err := (&UserService{db: blocked}).IncrementFailedAttempts(user.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "failed to increment failed attempts: ")
	var pgErr *pgconn.PgError
	suite.Require().ErrorAs(err, &pgErr)
	suite.Equal("55P03", pgErr.Code, "lock_not_available")
	suite.NotErrorIs(err, gorm.ErrRecordNotFound)

	suite.Require().NoError(holder.Rollback().Error)
	got := suite.reloadAccount(user.ID)
	suite.Zero(got.FailedLoginAttempts)
	suite.Nil(got.LockedUntil)
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_AdvancesUpdatedAt() {
	user := suite.createLiveAccount("lockout-updated-at@example.com")
	stale := time.Now().Add(-time.Hour)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", user.ID).
		UpdateColumn("updated_at", stale).Error)

	before := time.Now()
	suite.Require().NoError(suite.userService.IncrementFailedAttempts(user.ID))
	after := time.Now()

	got := suite.reloadAccount(user.ID)
	// updated_at is stored at microsecond precision, below the nanosecond bound.
	suite.False(got.UpdatedAt.Before(before.Truncate(time.Microsecond)))
	suite.False(got.UpdatedAt.After(after))
}

// Every request that reads the row before the locking failure commits has its
// password checked and its failure counted; once the lock is set, even the
// correct password is refused.
func (suite *UserServiceIntegrationTestSuite) TestAuthenticateUserWithPassword_ParallelWrongPasswordsAreAllCountedThenLock() {
	const address, password = "lockout-parallel-burst@example.com", "CorrectPassword1!"
	created, err := suite.userService.CreateUserWithPassword(address, password, "Burst", "User")
	suite.Require().NoError(err)
	const requests = 2 * MaxFailedLoginAttempts

	start := make(chan struct{})
	codes := make(chan string, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := suite.userService.AuthenticateUserWithPassword(address, "WrongPassword1!")
			var authErr *apperrors.AuthError
			if errors.As(err, &authErr) {
				codes <- authErr.Code
				return
			}
			codes <- fmt.Sprintf("unexpected: %v", err)
		}()
	}
	close(start)
	wg.Wait()
	close(codes)

	checked, refused := 0, 0
	for code := range codes {
		switch code {
		case apperrors.CodeInvalidCredentials:
			checked++
		case apperrors.CodeAccountLocked:
			refused++
		default:
			suite.Failf("unexpected outcome", "%s", code)
		}
	}
	suite.Equal(requests, checked+refused)
	suite.GreaterOrEqual(checked, MaxFailedLoginAttempts)
	suite.Equal(checked, suite.reloadAccount(created.ID).FailedLoginAttempts)

	_, err = suite.userService.AuthenticateUserWithPassword(address, password)
	var authErr *apperrors.AuthError
	suite.Require().ErrorAs(err, &authErr)
	suite.Equal(apperrors.CodeAccountLocked, authErr.Code)
}
