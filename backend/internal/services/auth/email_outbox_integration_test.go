package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// recordingSender records every magic-link and recovery send and answers each
// with err. onSend, when set, runs before the answer.
type recordingSender struct {
	mu            sync.Mutex
	sends         []recordedSend
	err           error
	onSend        func()
	notConfigured bool
}

func (r *recordingSender) IsConfigured() bool { return !r.notConfigured }

type recordedSend struct {
	kind          authm.AuthEmailKind
	to            string
	token         string
	daysRemaining int
}

func (r *recordingSender) record(send recordedSend) error {
	r.mu.Lock()
	r.sends = append(r.sends, send)
	onSend, err := r.onSend, r.err
	r.mu.Unlock()
	if onSend != nil {
		onSend()
	}
	return err
}

func (r *recordingSender) SendMagicLinkEmail(to, token string) error {
	return r.record(recordedSend{kind: authm.AuthEmailKindMagicLink, to: to, token: token})
}

func (r *recordingSender) SendAccountRecoveryEmail(to, token string, daysRemaining int) error {
	return r.record(recordedSend{kind: authm.AuthEmailKindAccountRecovery, to: to, token: token, daysRemaining: daysRemaining})
}

func (r *recordingSender) recorded() []recordedSend {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedSend(nil), r.sends...)
}

type AuthEmailOutboxSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	users  *usersvc.UserService
	jwt    *JWTService
	sender *recordingSender
	outbox *AuthEmailOutbox
}

func TestAuthEmailOutboxSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(AuthEmailOutboxSuite))
}

func (s *AuthEmailOutboxSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.users = usersvc.NewUserService(s.db)
	s.jwt = NewJWTService(s.db, &config.Config{
		JWT: config.JWTConfig{SecretKey: "auth-email-outbox-test-secret-key-32!", Expiry: 24},
	}, s.users)
}

func (s *AuthEmailOutboxSuite) SetupTest() {
	s.sender = &recordingSender{}
	s.outbox = NewAuthEmailOutbox(s.db, s.users, s.jwt, s.sender)
}

func (s *AuthEmailOutboxSuite) TearDownTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM auth_email_outbox")
	_, _ = sqlDB.Exec("DELETE FROM user_preferences")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (s *AuthEmailOutboxSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *AuthEmailOutboxSuite) verifiedUser(email string) *authm.User {
	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *AuthEmailOutboxSuite) softDeletedUser(email string) *authm.User {
	user := s.verifiedUser(email)
	s.Require().NoError(s.users.SoftDeleteAccount(user.ID, nil))
	return user
}

func (s *AuthEmailOutboxSuite) enqueue(kind authm.AuthEmailKind, userID uint) {
	s.Require().NoError(s.outbox.Enqueue(context.Background(), kind, userID))
}

func (s *AuthEmailOutboxSuite) rows() []authm.AuthEmailOutboxItem {
	var rows []authm.AuthEmailOutboxItem
	s.Require().NoError(s.db.Order("id").Find(&rows).Error)
	return rows
}

func (s *AuthEmailOutboxSuite) onlyRow() authm.AuthEmailOutboxItem {
	rows := s.rows()
	s.Require().Len(rows, 1)
	return rows[0]
}

// makeDue pulls every row's next_attempt_at into the past.
func (s *AuthEmailOutboxSuite) makeDue() {
	s.Require().NoError(s.db.Exec("UPDATE auth_email_outbox SET next_attempt_at = NOW() - INTERVAL '1 second'").Error)
}

func (s *AuthEmailOutboxSuite) TestMagicLink_MintedAtSendForTheCurrentAddress() {
	user := s.verifiedUser("old@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Require().NoError(s.db.Model(user).Update("email", "new@example.com").Error)

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal("new@example.com", sends[0].to, "the email goes to the account's address at send time")
	claims, err := s.jwt.ValidateMagicLinkToken(sends[0].token)
	s.Require().NoError(err, "the token is a valid magic link")
	s.Equal(user.ID, claims.UserID)
	s.Equal("new@example.com", claims.Email)

	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusSent, row.Status)
	s.Equal(1, row.Attempts)
	s.NotNil(row.SentAt)
	s.Nil(row.LastError)
}

func (s *AuthEmailOutboxSuite) TestRecovery_DaysRemainingComputedAtSend() {
	user := s.softDeletedUser("gone@example.com")
	s.enqueue(authm.AuthEmailKindAccountRecovery, user.ID)

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal(authm.AuthEmailKindAccountRecovery, sends[0].kind)
	s.Equal("gone@example.com", sends[0].to)
	stored, err := s.users.GetUserByID(user.ID)
	s.Require().NoError(err)
	s.Equal(s.users.GetDaysUntilPermanentDeletion(stored), sends[0].daysRemaining)
	_, err = s.jwt.ValidateAccountRecoveryToken(sends[0].token)
	s.NoError(err, "the token is a valid recovery token")
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

func (s *AuthEmailOutboxSuite) TestEnqueue_OneQueuedEmailPerAccountAndKind() {
	user := s.verifiedUser("repeat@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Len(s.rows(), 1, "a repeat while one is queued is a no-op")

	s.outbox.RunNow(context.Background())
	s.Len(s.sender.recorded(), 1)

	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Len(s.rows(), 2, "a request after the send queues a new email")
}

func (s *AuthEmailOutboxSuite) TestKillSwitch_StopsBothEnds() {
	user := s.verifiedUser("switch@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)

	s.T().Setenv(DisableAuthEmailOutboxEnvVar, "1")
	err := s.outbox.Enqueue(context.Background(), authm.AuthEmailKindAccountRecovery, user.ID)
	s.ErrorIs(err, ErrAuthEmailOutboxDisabled)
	s.outbox.RunNow(context.Background())
	s.Empty(s.sender.recorded(), "nothing is sent while the switch is set")
	s.Len(s.rows(), 1, "nothing is queued while the switch is set")

	s.T().Setenv(DisableAuthEmailOutboxEnvVar, "")
	s.outbox.RunNow(context.Background())
	s.Len(s.sender.recorded(), 1, "the queued email is sent once the switch is cleared")
}

func (s *AuthEmailOutboxSuite) TestUnconfiguredSender_LeavesRowsQueued() {
	user := s.verifiedUser("no-provider@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.sender.notConfigured = true

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusPending, row.Status)
	s.Equal(0, row.Attempts, "no attempt is spent without a provider")
}

func (s *AuthEmailOutboxSuite) TestFailedSend_BacksOffThenFails() {
	user := s.verifiedUser("flaky@example.com")
	s.sender.err = errors.New("provider down")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)

	s.outbox.RunNow(context.Background())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusPending, row.Status)
	s.Equal(1, row.Attempts)
	s.Require().NotNil(row.LastError)
	s.Equal("provider down", *row.LastError)
	s.WithinDuration(time.Now().Add(authEmailRetryBase), row.NextAttemptAt, 10*time.Second)

	s.outbox.RunNow(context.Background())
	s.Len(s.sender.recorded(), 1, "a row is not retried before its backoff")

	s.makeDue()
	s.outbox.RunNow(context.Background())
	row = s.onlyRow()
	s.Equal(2, row.Attempts)
	s.WithinDuration(time.Now().Add(2*authEmailRetryBase), row.NextAttemptAt, 10*time.Second, "the backoff doubles")

	s.makeDue()
	s.outbox.RunNow(context.Background())
	row = s.onlyRow()
	s.Equal(authm.AuthEmailStatusFailed, row.Status, "the last attempt fails the row")
	s.Equal(3, row.Attempts)
	s.Len(s.sender.recorded(), 3)

	s.makeDue()
	s.outbox.RunNow(context.Background())
	s.Len(s.sender.recorded(), 3, "a failed row is never claimed again")
}

func (s *AuthEmailOutboxSuite) TestIneligibleAtSend_Skipped() {
	unverified := s.verifiedUser("unverified@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, unverified.ID)
	s.Require().NoError(s.db.Model(unverified).Update("email_verified", false).Error)

	restored := s.softDeletedUser("restored@example.com")
	s.enqueue(authm.AuthEmailKindAccountRecovery, restored.ID)
	s.Require().NoError(s.users.RestoreAccount(restored.ID))

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	for _, row := range s.rows() {
		s.Equal(authm.AuthEmailStatusSkipped, row.Status)
		s.Require().NotNil(row.LastError)
		s.Equal(authEmailSkipIneligible, *row.LastError)
	}
}

func (s *AuthEmailOutboxSuite) TestExpiredRequest_SkippedUnsent() {
	user := s.verifiedUser("late@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Require().NoError(s.db.Exec("UPDATE auth_email_outbox SET created_at = NOW() - INTERVAL '16 minutes'").Error)

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusSkipped, row.Status)
	s.Require().NotNil(row.LastError)
	s.Equal(authEmailSkipExpired, *row.LastError)
}

func (s *AuthEmailOutboxSuite) TestStrandedProcessingRow_ReclaimedAndSent() {
	user := s.verifiedUser("stranded@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Require().NoError(s.db.Exec(
		"UPDATE auth_email_outbox SET status = 'processing', attempts = 1, updated_at = NOW() - INTERVAL '11 minutes'").Error)

	s.outbox.RunNow(context.Background())

	s.Len(s.sender.recorded(), 1)
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

func (s *AuthEmailOutboxSuite) TestTerminalRows_PrunedAfterRetention() {
	user := s.verifiedUser("old-audit@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.outbox.RunNow(context.Background())
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.outbox.RunNow(context.Background())
	s.Require().Len(s.rows(), 2)
	first := s.rows()[0]
	s.Require().NoError(s.db.Exec(
		"UPDATE auth_email_outbox SET updated_at = NOW() - INTERVAL '8 days' WHERE id = ?", first.ID).Error)

	s.outbox.RunNow(context.Background())

	rows := s.rows()
	s.Require().Len(rows, 1, "only the row past the retention window is pruned")
	s.NotEqual(first.ID, rows[0].ID)
}

func (s *AuthEmailOutboxSuite) TestCanceledTick_RequeuesUnreachedRowsWithoutAttempt() {
	first := s.verifiedUser("first@example.com")
	second := s.verifiedUser("second@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, first.ID)
	s.enqueue(authm.AuthEmailKindMagicLink, second.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.sender.onSend = cancel
	s.outbox.RunNow(ctx)

	s.Len(s.sender.recorded(), 1, "the tick stops after the send that saw the cancellation")
	rows := s.rows()
	s.Require().Len(rows, 2)
	s.Equal(authm.AuthEmailStatusSent, rows[0].Status, "the row already sent is recorded")
	s.Equal(authm.AuthEmailStatusPending, rows[1].Status, "the unreached row is handed back")
	s.Equal(0, rows[1].Attempts, "with its claim's attempt undone")

	s.sender.onSend = nil
	s.outbox.RunNow(context.Background())
	s.Len(s.sender.recorded(), 2)
}

func (s *AuthEmailOutboxSuite) TestAccountDeleted_RowsGoWithIt() {
	user := s.verifiedUser("purged@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Require().NoError(s.users.PermanentlyDeleteUser(user.ID))
	s.Empty(s.rows(), "ON DELETE CASCADE removes the account's queued email")
}

// TestDeliver_UnknownKindFails covers a kind the CHECK constraint keeps out of
// the table, so it runs without one.
func TestDeliver_UnknownKindFails(t *testing.T) {
	email := "someone@example.com"
	user := &authm.User{ID: 3, Email: &email, EmailVerified: true, IsActive: true}
	sender := &recordingSender{}
	outbox := NewAuthEmailOutbox(nil, newStubUserService(user), nil, sender)

	reason, err := outbox.deliver(authm.AuthEmailOutboxItem{Kind: "password_reset", UserID: 3})
	if !errors.Is(err, errUnknownAuthEmailKind) || reason != "" {
		t.Fatalf("deliver = (%q, %v), want errUnknownAuthEmailKind", reason, err)
	}
	if len(sender.recorded()) != 0 {
		t.Fatal("an unknown kind sends nothing")
	}
}
