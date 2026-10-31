package auth

import (
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/testutil/softauthn"
)

// FinishRegistration stores a credential only for an account that may sign in,
// judged inside the transaction that writes it. Its caller checked the account
// when the request started, so these close the account after that check.

// registerAccountState is an account state a registration finish must tell
// apart.
type registerAccountState struct {
	name     string
	apply    func(suite *WebAuthnServiceIntegrationTestSuite, userID uint)
	mayStore bool
}

var registerAccountStates = []registerAccountState{
	{name: "active", apply: func(*WebAuthnServiceIntegrationTestSuite, uint) {}, mayStore: true},
	{name: "deactivated", apply: func(suite *WebAuthnServiceIntegrationTestSuite, userID uint) {
		suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", userID).Update("is_active", false).Error)
	}},
	{name: "soft-deleted", apply: func(suite *WebAuthnServiceIntegrationTestSuite, userID uint) {
		suite.Require().NoError(usersvc.NewUserService(suite.db).SoftDeleteAccount(userID, nil))
	}},
}

// serviceWithWriteBeforeItsFirst returns a WebAuthnService on the suite's
// connections that runs write once, before the first statement any of hooks
// fires on (testutil.WithWriteBeforeItsFirst). The returned flag reports
// whether write ran.
func (suite *WebAuthnServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirst(write func(), hooks ...testutil.Hook) (*WebAuthnService, *bool) {
	suite.T().Helper()
	hooked, ran := testutil.WithWriteBeforeItsFirst(suite.T(), suite.db, func(*gorm.DB) { write() }, hooks...)
	svc, err := NewWebAuthnService(hooked, suite.service.config)
	suite.Require().NoError(err)
	return svc, ran
}

// serviceWithWriteBeforeItsFirstStatement runs write before the service's
// first read or insert, whichever it issues first, so write lands after the
// caller's check and before the registration touches the database.
func (suite *WebAuthnServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstStatement(write func()) (*WebAuthnService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(write, testutil.BeforeQuery, testutil.BeforeCreate)
}

// serviceWithWriteBeforeItsFirstCreate runs write before the service's first
// INSERT, which a registration issues after it has read the account.
func (suite *WebAuthnServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstCreate(write func()) (*WebAuthnService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(write, testutil.BeforeCreate)
}

// startOnAnotherConnection is testutil.StartOnAnotherConnection on the
// suite's own handle.
func (suite *WebAuthnServiceIntegrationTestSuite) startOnAnotherConnection(write func() error) <-chan error {
	suite.T().Helper()
	return testutil.StartOnAnotherConnection(suite.T(), suite.db, write)
}

func (suite *WebAuthnServiceIntegrationTestSuite) awaitConcurrentWrite(done <-chan error) {
	suite.T().Helper()
	suite.Require().NoError(testutil.AwaitConcurrentWrite(suite.T(), done))
}

// pendingRegistration starts a registration for user and answers it with a
// fresh software authenticator, returning what FinishRegistration is handed.
func (suite *WebAuthnServiceIntegrationTestSuite) pendingRegistration(user *authm.User) (*webauthn.SessionData, []byte) {
	suite.T().Helper()
	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	body, err := softauthn.New(ceremonyOrigin).Register(options)
	suite.Require().NoError(err)
	return session, body
}

// An account closed after the caller checked it, and before the registration
// reads it, gets no credential: the write itself enforces the rule.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistration_AccountClosedAfterCallersCheck() {
	for _, state := range registerAccountStates {
		suite.Run(state.name, func() {
			user := suite.createUser("register-window-" + state.name + "@test.com")
			suite.Require().True(user.CanSignIn(), "the caller's check passes")
			session, body := suite.pendingRegistration(user)

			svc, ran := suite.serviceWithWriteBeforeItsFirstStatement(func() {
				state.apply(suite, user.ID)
			})

			cred, err := svc.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")

			suite.Require().True(*ran, "the account must change inside the registration's window")
			if state.mayStore {
				suite.Require().NoError(err)
				suite.Require().NotNil(cred)
				suite.Equal(user.ID, cred.UserID)
				suite.Equal(int64(1), suite.credentialRows(user.ID))
				return
			}
			suite.Nil(cred)
			suite.requireRefusal(err, apperrors.CodeAccountInactive, apperrors.ErrAccountInactive().UserMessage())
			suite.Zero(suite.credentialRows(user.ID), "a refused registration stores no credential")
		})
	}
}

// Once the registration has read the account as live, a deactivation waits
// for the registration to commit, so the outcome is a credential followed by
// a deactivation and never a credential written to an account already closed.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistration_DeactivationWaitsForTheRegistrationThatReadTheAccount() {
	user := suite.createUser("register-serialized@test.com")
	session, body := suite.pendingRegistration(user)

	var concurrent <-chan error
	waitedForRegistration := false
	svc, ran := suite.serviceWithWriteBeforeItsFirstCreate(func() {
		concurrent = suite.startOnAnotherConnection(func() error {
			return suite.db.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false).Error
		})
		waitedForRegistration = len(concurrent) == 0
	})

	cred, err := svc.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the deactivation must start before the registration's write")
	suite.awaitConcurrentWrite(concurrent)

	suite.True(waitedForRegistration, "the deactivation must wait for the registration holding the account row")
	suite.Equal(user.ID, cred.UserID)
	suite.Equal(int64(1), suite.credentialRows(user.ID))
	var account authm.User
	suite.Require().NoError(suite.db.First(&account, user.ID).Error)
	suite.False(account.IsActive)
}

// A registration that reaches the account while a deactivation holds the row
// waits for it, then reads the committed state and is refused.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistration_WaitsForAnInFlightDeactivationThenRefuses() {
	user := suite.createUser("register-behind-deactivation@test.com")
	session, body := suite.pendingRegistration(user)

	var committed <-chan error
	svc, ran := suite.serviceWithWriteBeforeItsFirstStatement(func() {
		deactivation := suite.db.Begin()
		suite.Require().NoError(deactivation.Error)
		suite.Require().NoError(deactivation.Model(&authm.User{}).Where("id = ?", user.ID).Update("is_active", false).Error)
		done := make(chan error, 1)
		committed = done
		go func() {
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				var waiting int64
				err := suite.db.Raw(
					"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
				).Scan(&waiting).Error
				if err == nil && waiting > 0 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			done <- deactivation.Commit().Error
		}()
	})

	cred, err := svc.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")

	suite.Require().True(*ran, "the deactivation must hold the row before the registration reads it")
	suite.awaitConcurrentWrite(committed)
	suite.Nil(cred)
	suite.requireRefusal(err, apperrors.CodeAccountInactive, apperrors.ErrAccountInactive().UserMessage())
	suite.Zero(suite.credentialRows(user.ID), "a refused registration stores no credential")
}

// An account removed after the caller checked it is not found by the
// registration's read, which is a failure rather than a refusal, and stores
// no credential.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistration_AccountRemovedAfterCallersCheckFails() {
	user := suite.createUser("register-removed@test.com")
	session, body := suite.pendingRegistration(user)

	svc, ran := suite.serviceWithWriteBeforeItsFirstStatement(func() {
		suite.Require().NoError(suite.db.Exec("DELETE FROM users WHERE id = ?", user.ID).Error)
	})

	cred, err := svc.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")

	suite.Require().True(*ran, "the account must be removed inside the registration's window")
	suite.Nil(cred)
	suite.requireFailure(err)
	suite.Contains(err.Error(), "failed to load user")
	suite.Zero(suite.credentialRows(user.ID))
}

// A session whose account's sessions were revoked after the caller loaded it
// stores no credential: the write compares the session's revocation stamp
// with the account's, under the row lock.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistration_SessionRevokedAfterCallersCheck() {
	user := suite.createUser("register-revoked@test.com")
	session, body := suite.pendingRegistration(user)

	svc, ran := suite.serviceWithWriteBeforeItsFirstStatement(func() {
		suite.Require().NoError(RevokeAllSessions(suite.db, &authm.User{ID: user.ID}, time.Now()))
	})

	cred, err := svc.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")

	suite.Require().True(*ran, "the revocation must land inside the registration's window")
	suite.Nil(cred)
	suite.requireRefusal(err, apperrors.CodeTokenInvalid, apperrors.ErrTokenInvalid(nil).UserMessage())
	suite.ErrorIs(err, shared.ErrSessionRevoked)
	suite.Zero(suite.credentialRows(user.ID), "a refused registration stores no credential")
}

// A session minted after the account's latest revocation carries its stamp
// and registers.
func (suite *WebAuthnServiceIntegrationTestSuite) TestFinishRegistration_SessionMintedAfterARevocationRegisters() {
	user := suite.createUser("register-after-revocation@test.com")
	suite.Require().NoError(RevokeAllSessions(suite.db, user, time.Now()))
	suite.Require().NotNil(user.SessionsValidAfter)
	session, body := suite.pendingRegistration(user)

	cred, err := suite.service.FinishRegistration(user, session, suite.parseCreation(body), "Laptop")

	suite.Require().NoError(err)
	suite.Equal(user.ID, cred.UserID)
	suite.Equal(int64(1), suite.credentialRows(user.ID))
}
