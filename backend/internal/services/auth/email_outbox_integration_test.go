package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/testutil"
)

// recordingSender records every magic-link, recovery and verification send and
// answers each with err. onSend, when set, runs before the answer, with the
// recipient.
type recordingSender struct {
	mu            sync.Mutex
	sends         []recordedSend
	err           error
	onSend        func(to string)
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
		onSend(send.to)
	}
	return err
}

func (r *recordingSender) SendMagicLinkEmail(to, token string) error {
	return r.record(recordedSend{kind: authm.AuthEmailKindMagicLink, to: to, token: token})
}

func (r *recordingSender) SendAccountRecoveryEmail(to, token string, daysRemaining int) error {
	return r.record(recordedSend{kind: authm.AuthEmailKindAccountRecovery, to: to, token: token, daysRemaining: daysRemaining})
}

func (r *recordingSender) SendVerificationEmail(to, token string) error {
	return r.record(recordedSend{kind: authm.AuthEmailKindVerification, to: to, token: token})
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
	// Ten days pass between the request and the send.
	s.Require().NoError(s.db.Exec(
		"UPDATE users SET deleted_at = deleted_at - INTERVAL '10 days' WHERE id = ?", user.ID).Error)

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal(authm.AuthEmailKindAccountRecovery, sends[0].kind)
	s.Equal("gone@example.com", sends[0].to)
	fullWindowDays := int(shared.AccountRecoveryGracePeriod / (24 * time.Hour))
	s.Equal(fullWindowDays-10, sends[0].daysRemaining, "days remaining counts from the send, not the request")
	claims, err := s.jwt.ValidateAccountRecoveryToken(sends[0].token)
	s.Require().NoError(err, "the token is a valid recovery token")
	s.Equal(user.ID, claims.UserID)
	s.Equal("gone@example.com", claims.Email)
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

func (s *AuthEmailOutboxSuite) TestVerification_MintedAtSendForTheCurrentAddress() {
	user := s.verifiedUser("unverified-old@example.com")
	s.Require().NoError(s.db.Model(user).Update("email_verified", false).Error)
	s.enqueue(authm.AuthEmailKindVerification, user.ID)
	s.Require().NoError(s.db.Model(user).Update("email", "unverified-new@example.com").Error)

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal(authm.AuthEmailKindVerification, sends[0].kind)
	s.Equal("unverified-new@example.com", sends[0].to, "the email goes to the account's address at send time")
	claims, err := s.jwt.ValidateVerificationToken(sends[0].token)
	s.Require().NoError(err, "the token is a valid verification token")
	s.Equal(user.ID, claims.UserID)
	s.Equal("unverified-new@example.com", claims.Email)
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

func (s *AuthEmailOutboxSuite) TestEnqueue_SurvivesCanceledRequestContext() {
	user := s.verifiedUser("hung-up@example.com")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.Require().NoError(s.outbox.Enqueue(ctx, authm.AuthEmailKindMagicLink, user.ID),
		"a client that disconnects after asking still gets its email")
	s.Len(s.rows(), 1)
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

	// Still soft-deleted, but its recovery window closes while it is queued.
	lapsed := s.softDeletedUser("lapsed@example.com")
	s.enqueue(authm.AuthEmailKindAccountRecovery, lapsed.ID)
	s.Require().NoError(s.db.Exec(
		"UPDATE users SET deleted_at = NOW() - INTERVAL '31 days' WHERE id = ?", lapsed.ID).Error)

	// Queued for verification, then verified some other way while queued.
	verifiedMeanwhile := s.verifiedUser("verified-meanwhile@example.com")
	s.Require().NoError(s.db.Model(verifiedMeanwhile).Update("email_verified", false).Error)
	s.enqueue(authm.AuthEmailKindVerification, verifiedMeanwhile.ID)
	s.Require().NoError(s.db.Model(verifiedMeanwhile).Update("email_verified", true).Error)

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	s.Require().Len(s.rows(), 4)
	for _, row := range s.rows() {
		s.Equal(authm.AuthEmailStatusSkipped, row.Status)
		s.Require().NotNil(row.LastError)
		s.Equal(authEmailSkipIneligible, *row.LastError)
	}
}

// expiredRow queues a magic link whose request is older than authEmailMaxAge.
func (s *AuthEmailOutboxSuite) expiredRow(email string) {
	user := s.verifiedUser(email)
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Require().NoError(s.db.Exec("UPDATE auth_email_outbox SET created_at = NOW() - INTERVAL '16 minutes'").Error)
}

func (s *AuthEmailOutboxSuite) assertSkippedExpired(wantAttempts int) {
	s.Empty(s.sender.recorded())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusSkipped, row.Status)
	s.Equal(wantAttempts, row.Attempts)
	s.Require().NotNil(row.LastError)
	s.Equal(authEmailSkipExpired, *row.LastError)
}

func (s *AuthEmailOutboxSuite) TestExpiredRequest_ReapedByHousekeeping() {
	s.expiredRow("late-reap@example.com")

	s.outbox.housekeep(context.Background())

	s.assertSkippedExpired(0)
}

func (s *AuthEmailOutboxSuite) TestExpiredRequest_SkippedWhenClaimed() {
	s.expiredRow("late-claim@example.com")
	s.outbox.lastHousekeeping = time.Now() // no reap this tick

	s.outbox.processTick(context.Background())

	s.assertSkippedExpired(1)
}

func (s *AuthEmailOutboxSuite) TestTick_DrainsPastOneBatch() {
	for i := 0; i < 2*defaultAuthEmailOutboxBatch+3; i++ {
		user := s.verifiedUser(fmt.Sprintf("burst-%d@example.com", i))
		s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	}

	s.outbox.RunNow(context.Background())

	s.Len(s.sender.recorded(), 2*defaultAuthEmailOutboxBatch+3, "one tick claims again while claims come back full")
	for _, row := range s.rows() {
		s.Equal(authm.AuthEmailStatusSent, row.Status)
	}
}

func (s *AuthEmailOutboxSuite) TestTick_RowsPastTheStartBudgetGoBackUnattempted() {
	user := s.verifiedUser("late-start@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	items, err := s.outbox.claim(context.Background())
	s.Require().NoError(err)
	s.Require().Len(items, 1)

	var c batchCounts
	s.outbox.sendBatch(context.Background(), items, time.Now().Add(-time.Second), &c)

	s.Empty(s.sender.recorded(), "a row reached after the start budget is not sent")
	s.Equal(int32(1), c.requeued.Load())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusPending, row.Status)
	s.Equal(0, row.Attempts, "with its claim's attempt undone")
}

func (s *AuthEmailOutboxSuite) TestPanickingRow_FailedOnceAndNeighboursSent() {
	for _, email := range []string{"poison@example.com", "fine-1@example.com", "fine-2@example.com", "fine-3@example.com"} {
		user := s.verifiedUser(email)
		s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	}
	s.sender.onSend = func(to string) {
		if to == "poison@example.com" {
			panic("template exploded")
		}
	}

	s.NotPanics(func() { s.outbox.RunNow(context.Background()) }, "a panicking row must not escape the tick")

	var poison, fine int
	for _, row := range s.rows() {
		user, err := s.users.GetUserByID(row.UserID)
		s.Require().NoError(err)
		if *user.Email == "poison@example.com" {
			poison++
			s.Equal(authm.AuthEmailStatusFailed, row.Status, "the panicking row is retired, not left processing")
			s.Require().NotNil(row.LastError)
			s.Contains(*row.LastError, "panic while sending")
			continue
		}
		fine++
		s.Equal(authm.AuthEmailStatusSent, row.Status, "the batch's other rows are still sent")
	}
	s.Equal(1, poison)
	s.Equal(3, fine)
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
	s.sender.onSend = func(string) { cancel() }
	s.outbox.workers = 1 // the second row is reached only after the cancel
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
