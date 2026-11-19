package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

func (s *AuthEmailOutboxSuite) enqueueCalendarSteps(user *authm.User) error {
	return s.outbox.EnqueueCalendarSteps(context.Background(), user)
}

// markAllSent retires every row, as the poller would after sending, so the
// next request is not absorbed by the one-queued-per-account index.
func (s *AuthEmailOutboxSuite) markAllSent() {
	s.Require().NoError(s.db.Exec("UPDATE auth_email_outbox SET status = 'sent', sent_at = NOW()").Error)
}

func (s *AuthEmailOutboxSuite) calendarStepsRows(userID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&authm.AuthEmailOutboxItem{}).
		Where("user_id = ? AND kind = ?", userID, authm.AuthEmailKindCalendarSteps).Count(&n).Error)
	return n
}

// A verified account's request queues one row naming only the account, and
// the poller sends the email to the account's address with no token.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_QueuedAndSentToTheVerifiedAddress() {
	user := s.verifiedUser("steps@example.com")

	s.Require().NoError(s.enqueueCalendarSteps(user))

	row := s.onlyRow()
	s.Equal(authm.AuthEmailKindCalendarSteps, row.Kind)
	s.Equal(user.ID, row.UserID)
	s.Equal(authm.AuthEmailStatusPending, row.Status)
	s.Nil(row.OAuthLinkRef)

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal(authm.AuthEmailKindCalendarSteps, sends[0].kind)
	s.Equal("steps@example.com", sends[0].to)
	s.Empty(sends[0].token, "the calendar steps email carries no token")
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

// An address that is not verified, or no address, is refused with the
// verification code, and nothing is queued.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_RefusesAnAddressThatIsNotVerified() {
	unverified := s.verifiedUser("unverified@example.com")
	unverified.EmailVerified = false
	s.Require().NoError(s.db.Model(unverified).Update("email_verified", false).Error)

	noAddress := &authm.User{IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(noAddress).Error)

	for name, user := range map[string]*authm.User{"unverified": unverified, "no address": noAddress} {
		err := s.enqueueCalendarSteps(user)
		var authErr *apperrors.AuthError
		s.Require().True(errors.As(err, &authErr), "%s: got %v", name, err)
		s.Equal(apperrors.CodeEmailVerificationRequired, authErr.Code, name)
	}
	s.Empty(s.rows())
}

// Three requests in an hour are queued; the fourth is refused with the time
// until the oldest leaves the hour, and is queued once it has.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_FourthRequestInAnHourIsRefused() {
	user := s.verifiedUser("busy@example.com")
	for i := range calendarStepsPerWindow {
		s.Require().NoError(s.enqueueCalendarSteps(user), "request %d", i+1)
		s.markAllSent()
	}
	s.Require().EqualValues(calendarStepsPerWindow, s.calendarStepsRows(user.ID))

	// The oldest request is 40 minutes old, so a slot opens in 20.
	s.Require().NoError(s.db.Exec(
		"UPDATE auth_email_outbox SET created_at = created_at - INTERVAL '40 minutes' WHERE id = (SELECT MIN(id) FROM auth_email_outbox)").Error)

	err := s.enqueueCalendarSteps(user)
	var limited *apperrors.CalendarStepsRateLimitedError
	s.Require().True(errors.As(err, &limited), "got %v", err)
	s.Equal(calendarStepsPerWindow, limited.Limit)
	s.Greater(limited.RetryAfter, 19*time.Minute)
	s.LessOrEqual(limited.RetryAfter, 20*time.Minute)
	s.EqualValues(calendarStepsPerWindow, s.calendarStepsRows(user.ID), "a refused request queues nothing")

	// Every row counts, whatever became of it: a skipped one still does.
	s.Require().NoError(s.db.Exec("UPDATE auth_email_outbox SET status = 'skipped'").Error)
	s.Require().ErrorAs(s.enqueueCalendarSteps(user), &limited)

	// Once the oldest is past the hour, the next request is queued.
	s.Require().NoError(s.db.Exec(
		"UPDATE auth_email_outbox SET created_at = created_at - INTERVAL '21 minutes' WHERE id = (SELECT MIN(id) FROM auth_email_outbox)").Error)
	s.Require().NoError(s.enqueueCalendarSteps(user))
	s.EqualValues(calendarStepsPerWindow+1, s.calendarStepsRows(user.ID))
}

// The cap is per account: another account's requests do not count.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_CapIsPerAccount() {
	busy := s.verifiedUser("busy-a@example.com")
	other := s.verifiedUser("other-b@example.com")
	for range calendarStepsPerWindow {
		s.Require().NoError(s.enqueueCalendarSteps(busy))
		s.markAllSent()
	}
	s.Require().NoError(s.enqueueCalendarSteps(other))
	s.EqualValues(1, s.calendarStepsRows(other.ID))
}

// A request while one is still queued is a no-op: one email, one row, and
// the repeat does not spend the cap.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_RepeatWhileQueuedAddsNoRow() {
	user := s.verifiedUser("repeat@example.com")
	s.Require().NoError(s.enqueueCalendarSteps(user))
	s.Require().NoError(s.enqueueCalendarSteps(user))
	s.EqualValues(1, s.calendarStepsRows(user.ID))
}

// An address that stops being verified between the request and the send gets
// nothing, and the row is retired as ineligible.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_SkippedWhenUnverifiedBeforeSend() {
	user := s.verifiedUser("changed@example.com")
	s.Require().NoError(s.enqueueCalendarSteps(user))
	s.Require().NoError(s.db.Model(user).Update("email_verified", false).Error)

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	s.assertOnlyRowSkipped(authEmailSkipIneligible)
}

// No row is queued that the poller could not send: not while the kill switch
// is set, and not while the process has no email provider.
func (s *AuthEmailOutboxSuite) TestCalendarSteps_RefusedWhenTheOutboxCannotSend() {
	user := s.verifiedUser("off@example.com")

	s.T().Setenv(DisableAuthEmailOutboxEnvVar, "1")
	s.Require().ErrorIs(s.enqueueCalendarSteps(user), ErrAuthEmailOutboxDisabled)
	s.T().Setenv(DisableAuthEmailOutboxEnvVar, "")

	s.sender.notConfigured = true
	s.Require().ErrorIs(s.enqueueCalendarSteps(user), ErrAuthEmailSenderNotConfigured)

	s.Empty(s.rows())
}

// runMigrationFile applies one migration file in a transaction, as the
// migrator does (its SET LOCAL needs one).
func (s *AuthEmailOutboxSuite) runMigrationFile(name string) {
	_, here, _, _ := runtime.Caller(0)
	sqlText, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "db", "migrations", name))
	s.Require().NoError(err)
	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(string(sqlText)).Error
	}))
}

// The kind's migration goes down cleanly, taking the kind's rows with it and
// refusing the kind again, and comes back up.
func (s *AuthEmailOutboxSuite) TestCalendarStepsKindMigration_DownThenUp() {
	const version = "20261119203824_add_calendar_steps_outbox_kind"
	user := s.verifiedUser("migrate@example.com")
	s.Require().NoError(s.enqueueCalendarSteps(user))
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)

	s.runMigrationFile(version + ".down.sql")
	defer s.runMigrationFile(version + ".up.sql")

	s.Zero(s.calendarStepsRows(user.ID), "down removes the kind's rows")
	s.Len(s.rows(), 1, "and leaves every other kind's")
	err := s.db.Create(&authm.AuthEmailOutboxItem{Kind: authm.AuthEmailKindCalendarSteps, UserID: user.ID, Status: authm.AuthEmailStatusSent}).Error
	s.Require().ErrorIs(err, gorm.ErrCheckConstraintViolated, "down refuses the kind again")

	s.runMigrationFile(version + ".up.sql")
	s.Require().NoError(s.enqueueCalendarSteps(user), "up accepts the kind")
}
