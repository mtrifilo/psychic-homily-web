package engagement

import (
	"errors"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/shared"
)

// A feed token is minted only for an account that may sign in, by a session
// that has not been revoked, judged under the users-row lock in the
// transaction that writes the token.

// revokeSessions writes a new sessions_valid_after for userID, the column
// every session revocation writes, and returns it.
func (suite *CalendarIntegrationTestSuite) revokeSessions(userID uint) time.Time {
	suite.T().Helper()
	revokedAt := time.Now().UTC().Truncate(time.Second)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", userID).
		Update("sessions_valid_after", revokedAt).Error)
	return revokedAt
}

func (suite *CalendarIntegrationTestSuite) tokenRows(userID uint) int64 {
	suite.T().Helper()
	var rows int64
	suite.Require().NoError(suite.db.Model(&engagementm.CalendarToken{}).Where("user_id = ?", userID).Count(&rows).Error)
	return rows
}

func (suite *CalendarIntegrationTestSuite) requireRefusal(err error, code string) {
	suite.T().Helper()
	var refusal *apperrors.AuthError
	suite.Require().True(errors.As(err, &refusal), "expected an AuthError, got %v", err)
	suite.Equal(code, refusal.Code)
}

// serviceWithWriteBeforeItsFirst returns a CalendarService on the suite's
// connections that runs write once, before the first statement the hook that
// register installs fires on. The returned flag reports whether write ran.
func (suite *CalendarIntegrationTestSuite) serviceWithWriteBeforeItsFirst(
	register func(hooked *gorm.DB, hook func(*gorm.DB)) error,
	write func(),
) (*CalendarService, *bool) {
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
	return &CalendarService{db: hooked, savedShowSvc: suite.svc.savedShowSvc}, &ran
}

// startOnAnotherConnection runs write in a goroutine on the suite's own handle
// and returns once write has either finished or is waiting on a lock. The
// returned channel delivers write's result.
func (suite *CalendarIntegrationTestSuite) startOnAnotherConnection(write func() error) <-chan error {
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

func (suite *CalendarIntegrationTestSuite) awaitConcurrentWrite(done <-chan error) {
	suite.T().Helper()
	select {
	case err := <-done:
		suite.Require().NoError(err)
	case <-time.After(10 * time.Second):
		suite.FailNow("the concurrent write did not finish")
	}
}

// A session validated before a revocation committed mints nothing, and the
// refused mint leaves the token the account already held as it was.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_SessionRevokedBeforeTheMint_Refused() {
	user := suite.createTestUser(true)
	existing, err := suite.svc.CreateToken(user.ID, user.SessionsValidAfter, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.revokeSessions(user.ID)

	resp, err := suite.svc.CreateToken(user.ID, user.SessionsValidAfter, "http://localhost:8080")

	suite.Nil(resp)
	suite.requireRefusal(err, apperrors.CodeTokenInvalid)
	suite.ErrorIs(err, shared.ErrSessionRevoked)
	suite.Equal(int64(1), suite.tokenRows(user.ID))
	_, err = suite.svc.ValidateCalendarToken(existing.Token)
	suite.NoError(err, "a refused mint does not replace the token the account holds")
}

// A revocation that commits after the caller's session check and before the
// mint reads the account refuses the mint.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_SessionRevokedInsideTheWindow_Refused() {
	user := suite.createTestUser(true)
	svc, ran := suite.serviceWithWriteBeforeItsFirst(func(hooked *gorm.DB, hook func(*gorm.DB)) error {
		return hooked.Callback().Query().Before("gorm:query").Register("test:write_before_query", hook)
	}, func() { suite.revokeSessions(user.ID) })

	resp, err := svc.CreateToken(user.ID, user.SessionsValidAfter, "http://localhost:8080")

	suite.Require().True(*ran, "the revocation must land inside the mint's window")
	suite.Nil(resp)
	suite.requireRefusal(err, apperrors.CodeTokenInvalid)
	suite.Zero(suite.tokenRows(user.ID), "a refused mint writes no token")
}

// A session minted after the account's latest revocation carries its stamp
// and mints.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_SessionMintedAfterARevocationMints() {
	user := suite.createTestUser(true)
	revokedAt := suite.revokeSessions(user.ID)

	resp, err := suite.svc.CreateToken(user.ID, &revokedAt, "http://localhost:8080")

	suite.Require().NoError(err)
	suite.NotEmpty(resp.Token)
	suite.Equal(int64(1), suite.tokenRows(user.ID))
}

func (suite *CalendarIntegrationTestSuite) TestCreateToken_AccountThatMayNotSignIn_Refused() {
	for name, closeAccount := range map[string]map[string]any{
		"deactivated":  {"is_active": false},
		"soft-deleted": {"is_active": false, "deleted_at": time.Now()},
	} {
		suite.Run(name, func() {
			user := suite.createTestUser(true)
			suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", user.ID).Updates(closeAccount).Error)

			resp, err := suite.svc.CreateToken(user.ID, user.SessionsValidAfter, "http://localhost:8080")

			suite.Nil(resp)
			suite.requireRefusal(err, apperrors.CodeAccountInactive)
			suite.Zero(suite.tokenRows(user.ID))
		})
	}
}

// The other order: a revocation that starts after the mint has locked the
// account waits for the mint to commit, then deletes the token the mint
// wrote, so the token never outlives the revocation.
func (suite *CalendarIntegrationTestSuite) TestCreateToken_RevocationThatStartsDuringTheMintEvictsItsToken() {
	user := suite.createTestUser(true)

	var revocation <-chan error
	waitedForMint := false
	svc, ran := suite.serviceWithWriteBeforeItsFirst(func(hooked *gorm.DB, hook func(*gorm.DB)) error {
		return hooked.Callback().Create().Before("gorm:create").Register("test:write_before_create", hook)
	}, func() {
		revocation = suite.startOnAnotherConnection(func() error {
			return suite.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Model(&authm.User{}).Where("id = ?", user.ID).
					Update("sessions_valid_after", time.Now().UTC().Truncate(time.Second)).Error; err != nil {
					return err
				}
				_, err := DeleteCalendarTokensTx(tx, user.ID)
				return err
			})
		})
		waitedForMint = len(revocation) == 0
	})

	resp, err := svc.CreateToken(user.ID, user.SessionsValidAfter, "http://localhost:8080")
	suite.Require().NoError(err)
	suite.Require().True(*ran, "the revocation must start before the mint's insert")
	suite.awaitConcurrentWrite(revocation)

	suite.True(waitedForMint, "the revocation must wait for the mint holding the account row")
	suite.Zero(suite.tokenRows(user.ID), "the revocation deletes the token the mint wrote")
	_, err = suite.svc.ValidateCalendarToken(resp.Token)
	suite.Error(err)
}
