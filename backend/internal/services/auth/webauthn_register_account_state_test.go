package auth

import (
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
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
// connections that runs write once, before the first statement the hooks that
// register installs fire on. The returned flag reports whether write ran.
func (suite *WebAuthnServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirst(
	register func(hooked *gorm.DB, hook func(*gorm.DB)) error,
	write func(),
) (*WebAuthnService, *bool) {
	suite.T().Helper()
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	hooked, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true})
	suite.Require().NoError(err)

	ran := false
	suite.Require().NoError(register(hooked, func(*gorm.DB) {
		if ran {
			return
		}
		ran = true
		write()
	}))
	return &WebAuthnService{webAuthn: suite.service.webAuthn, db: hooked, config: suite.service.config}, &ran
}

// serviceWithWriteBeforeItsFirstStatement runs write before the service's
// first read or insert, whichever it issues first, so write lands after the
// caller's check and before the registration touches the database.
func (suite *WebAuthnServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstStatement(write func()) (*WebAuthnService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(func(hooked *gorm.DB, hook func(*gorm.DB)) error {
		if err := hooked.Callback().Query().Before("gorm:query").Register("test:write_before_query", hook); err != nil {
			return err
		}
		return hooked.Callback().Create().Before("gorm:create").Register("test:write_before_create", hook)
	}, write)
}

// serviceWithWriteBeforeItsFirstCreate runs write before the service's first
// INSERT, which a registration issues after it has read the account.
func (suite *WebAuthnServiceIntegrationTestSuite) serviceWithWriteBeforeItsFirstCreate(write func()) (*WebAuthnService, *bool) {
	suite.T().Helper()
	return suite.serviceWithWriteBeforeItsFirst(func(hooked *gorm.DB, hook func(*gorm.DB)) error {
		return hooked.Callback().Create().Before("gorm:create").Register("test:write_before_create", hook)
	}, write)
}

// startOnAnotherConnection runs write in a goroutine on the suite's own handle
// and returns once write has either finished or is waiting on a lock. The
// returned channel delivers write's result.
func (suite *WebAuthnServiceIntegrationTestSuite) startOnAnotherConnection(write func() error) <-chan error {
	suite.T().Helper()
	done := make(chan error, 1)
	go func() { done <- write() }()

	suite.Require().Eventually(func() bool {
		if len(done) > 0 {
			return true
		}
		var waiting int64
		err := suite.db.Raw(
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'",
		).Scan(&waiting).Error
		return err == nil && waiting > 0
	}, 10*time.Second, 5*time.Millisecond, "the concurrent write neither finished nor waited on a lock")
	return done
}

func (suite *WebAuthnServiceIntegrationTestSuite) awaitConcurrentWrite(done <-chan error) {
	suite.T().Helper()
	select {
	case err := <-done:
		suite.Require().NoError(err)
	case <-time.After(10 * time.Second):
		suite.FailNow("the concurrent write did not finish")
	}
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
			suite.requireRefusal(err, apperrors.CodeAccountInactive, "Account unavailable. Please contact support.")
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
