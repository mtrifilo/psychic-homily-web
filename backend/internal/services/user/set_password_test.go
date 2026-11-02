package user

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil/signupconsent"
)

// SetPassword gives an account with no password its first one, for a session
// that has not been revoked, on an account that may sign in, and never over a
// password that is already there.

func (suite *UserServiceIntegrationTestSuite) assertPasswordAlreadySet(err error) {
	suite.T().Helper()
	var refusal *apperrors.AuthError
	suite.Require().ErrorAs(err, &refusal, "got %v", err)
	suite.Equal(apperrors.CodePasswordAlreadySet, refusal.Code)
}

func (suite *UserServiceIntegrationTestSuite) accountHasPassword(userID uint) bool {
	suite.T().Helper()
	stored := suite.reloadAccount(userID)
	return stored.HasPassword()
}

func (suite *UserServiceIntegrationTestSuite) TestSetPassword_SetsTheFirstPassword() {
	cases := map[string]*string{"absent": nil, "empty": stringPtr("")}
	for name, hash := range cases {
		suite.Run(name, func() {
			user := &authm.User{Email: stringPtr("setpw-first-" + name + "@example.com"), PasswordHash: hash, IsActive: true}
			suite.Require().NoError(suite.db.Create(user).Error)

			suite.Require().NoError(suite.userService.SetPassword(user.ID, "FirstPassword1!", nil))

			stored := suite.reloadAccount(user.ID)
			suite.Require().True(stored.HasPassword())
			suite.NoError(suite.userService.VerifyPassword(*stored.PasswordHash, "FirstPassword1!"))
			suite.Nil(stored.SessionsValidAfter, "setting a password revokes no session")
		})
	}
}

func (suite *UserServiceIntegrationTestSuite) TestSetPassword_AccountWithPasswordRefused() {
	user, err := suite.userService.CreateUserWithPasswordWithLegal(
		"setpw-has@example.com", "ExistingPassword1!", "Has", "Password", signupconsent.CurrentLegalAcceptance(),
	)
	suite.Require().NoError(err)

	suite.assertPasswordAlreadySet(suite.userService.SetPassword(user.ID, "ReplacementPassword2!", nil))

	stored := suite.reloadAccount(user.ID)
	suite.NoError(suite.userService.VerifyPassword(*stored.PasswordHash, "ExistingPassword1!"), "the existing password stays")
}

// The second of two sets that overlap waits on the first one's row lock and
// then finds the password set. The overlap is forced: the second set starts
// while the first holds the row, between its account check and its write.
func (suite *UserServiceIntegrationTestSuite) TestSetPassword_OverlappingSetIsRefused() {
	owner := suite.createLiveAccount("setpw-overlap@example.com")

	var second <-chan error
	first, ran := suite.serviceWithWriteBeforeItsFirstUpdate(func(*gorm.DB) {
		second = suite.startOnAnotherConnection(func() error {
			return suite.userService.SetPassword(owner.ID, "SecondPassword2!", nil)
		})
	})

	suite.Require().NoError(first.SetPassword(owner.ID, "FirstPassword1!", nil))
	suite.Require().True(*ran, "the second set must start inside the first one's transaction")
	suite.assertPasswordAlreadySet(<-second)

	stored := suite.reloadAccount(owner.ID)
	suite.NoError(suite.userService.VerifyPassword(*stored.PasswordHash, "FirstPassword1!"), "the first set's password is the one stored")
}

// Many sets at once: exactly one wins and every other is the refusal, never a
// database error (two sets holding the row FOR SHARE and both writing would
// deadlock, and Postgres would end one with an error).
func (suite *UserServiceIntegrationTestSuite) TestSetPassword_ConcurrentSetsOneWins() {
	owner := suite.createLiveAccount("setpw-concurrent@example.com")

	const callers = 6
	results := make([]error, callers)
	var start, done sync.WaitGroup
	start.Add(1)
	for i := 0; i < callers; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			results[i] = suite.userService.SetPassword(owner.ID, fmt.Sprintf("ConcurrentPassword%d!", i), nil)
		}(i)
	}
	start.Done()
	done.Wait()

	winner := -1
	for i, err := range results {
		if err == nil {
			suite.Require().Equal(-1, winner, "two sets succeeded: %d and %d", winner, i)
			winner = i
			continue
		}
		var refusal *apperrors.AuthError
		suite.Require().True(errors.As(err, &refusal) && refusal.Code == apperrors.CodePasswordAlreadySet,
			"caller %d: want PASSWORD_ALREADY_SET, got %v", i, err)
	}
	suite.Require().NotEqual(-1, winner, "no set succeeded")

	stored := suite.reloadAccount(owner.ID)
	suite.NoError(suite.userService.VerifyPassword(*stored.PasswordHash, fmt.Sprintf("ConcurrentPassword%d!", winner)),
		"the stored password is the winner's")
}

// A session revoked before the call, or after the caller checked it and
// before the write reads the account, sets nothing: the write reads the row
// FOR UPDATE and judges it with shared.CheckSessionAccount, which compares the
// session's stamp with the account's.
func (suite *UserServiceIntegrationTestSuite) TestSetPassword_RevokedSessionRefused() {
	suite.Run("revoked before the call", func() {
		owner := suite.createLiveAccount("setpw-revoked@example.com")
		suite.revokeSessions(owner.ID)

		err := suite.userService.SetPassword(owner.ID, "FirstPassword1!", owner.SessionsValidAfter)
		suite.assertSessionRevoked(err)
		suite.False(suite.accountHasPassword(owner.ID))
	})

	suite.Run("revoked inside the window", func() {
		owner := suite.createLiveAccount("setpw-revoked-window@example.com")
		svc, ran := suite.serviceWithWriteBeforeItsFirstQuery(func() { suite.revokeSessions(owner.ID) })

		err := svc.SetPassword(owner.ID, "FirstPassword1!", owner.SessionsValidAfter)
		suite.Require().True(*ran, "the revocation must land inside the set's window")
		suite.assertSessionRevoked(err)
		suite.False(suite.accountHasPassword(owner.ID))
	})

	suite.Run("minted after the revocation", func() {
		owner := suite.createLiveAccount("setpw-after-revocation@example.com")
		revokedAt := suite.revokeSessions(owner.ID)

		suite.Require().NoError(suite.userService.SetPassword(owner.ID, "FirstPassword1!", &revokedAt))
		suite.True(suite.accountHasPassword(owner.ID))
	})
}

func (suite *UserServiceIntegrationTestSuite) assertSessionRevoked(err error) {
	suite.T().Helper()
	var refusal *apperrors.AuthError
	suite.Require().ErrorAs(err, &refusal, "got %v", err)
	suite.Equal(apperrors.CodeTokenInvalid, refusal.Code)
	suite.ErrorIs(err, shared.ErrSessionRevoked)
}

func (suite *UserServiceIntegrationTestSuite) TestSetPassword_ClosedAccountRefused() {
	for _, state := range signInAccountStates {
		if state.maySignIn() {
			continue
		}
		suite.Run(state.name, func() {
			owner := suite.createLiveAccount("setpw-state-" + state.name + "@example.com")
			state.apply(suite, owner.ID)

			err := suite.userService.SetPassword(owner.ID, "FirstPassword1!", nil)
			var refusal *apperrors.AuthError
			suite.Require().ErrorAs(err, &refusal, "got %v", err)
			suite.Equal(apperrors.CodeAccountInactive, refusal.Code)
			suite.ErrorIs(err, shared.ErrSessionAccountClosed)
			suite.False(suite.accountHasPassword(owner.ID))
		})
	}
}

// bcrypt cannot hash more than 72 bytes, which the validator's 128-character
// bound admits, so a longer password is a refusal the caller can act on.
func (suite *UserServiceIntegrationTestSuite) TestSetPassword_TooLongToHashRefused() {
	owner := suite.createLiveAccount("setpw-too-long@example.com")

	err := suite.userService.SetPassword(owner.ID, strings.Repeat("a", 73), nil)
	var refusal *apperrors.AuthError
	suite.Require().ErrorAs(err, &refusal, "got %v", err)
	suite.Equal(apperrors.CodeValidationFailed, refusal.Code)
	suite.False(suite.accountHasPassword(owner.ID))

	suite.Require().NoError(suite.userService.SetPassword(owner.ID, strings.Repeat("a", 72), nil),
		"72 bytes is within bcrypt's limit")
}
