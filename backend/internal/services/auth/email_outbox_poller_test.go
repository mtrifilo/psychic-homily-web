package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/notification"
)

// sentryRecorder is a Sentry client that keeps every event it is handed and
// sends none (no DSN).
type sentryRecorder struct {
	mu     sync.Mutex
	events []*sentry.Event
	client *sentry.Client
}

func newSentryRecorder(t *testing.T) *sentryRecorder {
	t.Helper()
	r := &sentryRecorder{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			r.mu.Lock()
			r.events = append(r.events, event)
			r.mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.client = client
	return r
}

func (r *sentryRecorder) recorded() []*sentry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*sentry.Event(nil), r.events...)
}

// recordSentry points the outbox's hubs at a recorder.
func (s *AuthEmailOutboxSuite) recordSentry() *sentryRecorder {
	r := newSentryRecorder(s.T())
	s.outbox.sentryClient = func() *sentry.Client { return r.client }
	return r
}

// withGlobalScopeTag pushes a scope carrying tag onto the global hub for the
// rest of the test, as a goroutine inside sentry.WithScope would.
func (s *AuthEmailOutboxSuite) withGlobalScopeTag(key, value string) {
	sentry.CurrentHub().PushScope().SetTag(key, value)
	s.T().Cleanup(sentry.CurrentHub().PopScope)
}

// A row that runs out of attempts is reported once, on the outbox's own hub,
// with the service and email_type tags and none of the global hub's scope.
func (s *AuthEmailOutboxSuite) TestSentry_TerminalFailureReportedWithItsTags() {
	rec := s.recordSentry()
	s.withGlobalScopeTag("pushed_elsewhere", "yes")
	user := s.verifiedUser("last-attempt@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.Require().NoError(s.db.Exec("UPDATE auth_email_outbox SET max_attempts = 1").Error)
	s.sender.err = errors.New("provider down")

	s.outbox.RunNow(context.Background())

	s.Equal(authm.AuthEmailStatusFailed, s.onlyRow().Status)
	events := rec.recorded()
	s.Require().Len(events, 1, "one report for the email the person never got")
	s.Equal(AuthEmailOutboxLoopName, events[0].Tags["service"])
	s.Equal(string(authm.AuthEmailKindMagicLink), events[0].Tags["email_type"])
	s.NotContains(events[0].Tags, "pushed_elsewhere", "no scope from the global hub")
	s.Require().NotEmpty(events[0].Exception)
	s.Equal("provider down", events[0].Exception[len(events[0].Exception)-1].Value)
}

// A failed attempt with attempts left is retried, not reported by the outbox
// (the email service reports each provider error itself).
func (s *AuthEmailOutboxSuite) TestSentry_RetriedAttemptNotReported() {
	rec := s.recordSentry()
	user := s.verifiedUser("retry-quiet@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	s.sender.err = errors.New("provider down")

	s.outbox.RunNow(context.Background())

	s.Equal(authm.AuthEmailStatusPending, s.onlyRow().Status)
	s.Empty(rec.recorded())
}

func (s *AuthEmailOutboxSuite) expiredRows(n int) {
	for i := 0; i < n; i++ {
		s.expiredRow("expired-" + string(rune('a'+i)) + "@example.com")
	}
}

func (s *AuthEmailOutboxSuite) assertOneExpiredWarning(events []*sentry.Event, count int64) {
	s.T().Helper()
	s.Require().Len(events, 1)
	s.Equal(sentry.LevelWarning, events[0].Level)
	s.Equal("auth email outbox: requests expired unsent", events[0].Message)
	s.Equal(AuthEmailOutboxLoopName, events[0].Tags["service"])
	s.Equal(count, events[0].Extra["count"])
	s.Equal(authEmailMaxAge.String(), events[0].Extra["max_age"])
}

// Requests that aged out unsent are one warning per reap pass, with the count.
func (s *AuthEmailOutboxSuite) TestSentry_ExpiredUnsentOneWarningPerReapPass() {
	rec := s.recordSentry()
	s.expiredRows(3)

	s.outbox.housekeep(context.Background())

	s.assertOneExpiredWarning(rec.recorded(), 3)
}

// Rows claimed after they aged out are one warning per tick, with the count.
func (s *AuthEmailOutboxSuite) TestSentry_ExpiredUnsentOneWarningPerTick() {
	rec := s.recordSentry()
	s.expiredRows(3)
	s.outbox.lastHousekeeping = time.Now() // no reap this tick

	s.outbox.processTick(context.Background())

	s.assertOneExpiredWarning(rec.recorded(), 3)
}

// Every send carries a hub of the outbox's own, never the global hub, so the
// email service's provider-error report lands on it.
func (s *AuthEmailOutboxSuite) TestSends_CarryAWorkerHub() {
	for _, email := range []string{"hub-1@example.com", "hub-2@example.com", "hub-3@example.com"} {
		user := s.verifiedUser(email)
		s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	}

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 3)
	hubs := map[*sentry.Hub]bool{}
	for _, send := range sends {
		s.Require().NotNil(send.hub)
		s.NotSame(sentry.CurrentHub(), send.hub)
		hubs[send.hub] = true
	}
	s.LessOrEqual(len(hubs), s.outbox.workers, "one hub per worker")
}

// The empty-address skip and the account-gone skip.
func (s *AuthEmailOutboxSuite) TestSkipReasons_NoAddressAndAccountGone() {
	noAddress := &authm.User{IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(noAddress).Error)
	s.enqueue(authm.AuthEmailKindMagicLink, noAddress.ID)

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	s.assertOnlyRowSkipped(authEmailSkipIneligible)

	// The users foreign key cascades a deleted account's rows away, so a row
	// naming a missing account reaches only deliver.
	reason, err := s.outbox.deliver(context.Background(),
		authm.AuthEmailOutboxItem{Kind: authm.AuthEmailKindMagicLink, UserID: noAddress.ID + 1000}, time.Now().Add(time.Minute))
	s.Require().NoError(err)
	s.Equal(authEmailSkipAccountGone, reason)
}

// An account read still waiting when the start budget runs out is cut short,
// and the row goes back unattempted instead of holding its claim.
func (s *AuthEmailOutboxSuite) TestAccountRead_CutShortByTheStartBudget() {
	user := s.verifiedUser("slow-read@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	items, err := s.outbox.claim(context.Background())
	s.Require().NoError(err)
	s.Require().Len(items, 1)

	lock := s.db.Begin()
	s.Require().NoError(lock.Error)
	s.Require().NoError(lock.Exec("LOCK TABLE users IN ACCESS EXCLUSIVE MODE").Error)

	var c batchCounts
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.outbox.sendBatch(context.Background(), items, time.Now().Add(300*time.Millisecond), &c)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
	lock.Rollback()
	<-done

	s.Empty(s.sender.recorded())
	s.Equal(int32(1), c.requeued.Load())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusPending, row.Status)
	s.Equal(0, row.Attempts, "with its claim's attempt undone")
}

// errPanicsContext panics when asked whether it is done, which is the first
// thing a worker asks of a row.
type errPanicsContext struct{ context.Context }

func (errPanicsContext) Err() error { panic("context exploded") }

// The check that hands a late row back runs inside the per-row recover: a
// panic there retires the row and the worker keeps going.
func (s *AuthEmailOutboxSuite) TestLateCheck_PanicContainedToItsRow() {
	user := s.verifiedUser("late-panic@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, user.ID)
	items, err := s.outbox.claim(context.Background())
	s.Require().NoError(err)

	var c batchCounts
	s.NotPanics(func() {
		s.outbox.sendBatch(errPanicsContext{context.Background()}, items, time.Now().Add(time.Minute), &c)
	})

	s.Equal(int32(1), c.other.Load())
	row := s.onlyRow()
	s.Equal(authm.AuthEmailStatusFailed, row.Status)
	s.Require().NotNil(row.LastError)
	s.Contains(*row.LastError, "panic while sending")
}

// Start runs ticks on the interval; Stop waits for the tick in flight and
// then no tick runs.
func (s *AuthEmailOutboxSuite) TestStartStop() {
	s.T().Setenv("AUTH_EMAIL_OUTBOX_INTERVAL_SECONDS", "1")
	s.outbox = NewAuthEmailOutbox(s.db, s.users, s.jwt, s.sender)
	first := s.verifiedUser("started@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, first.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.outbox.Start(ctx)
	s.Eventually(func() bool { return len(s.sender.recorded()) == 1 }, 10*time.Second, 50*time.Millisecond,
		"the poller sends without RunNow")

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.sender.mu.Lock()
	s.sender.onSend = func(string) {
		once.Do(func() { close(entered) })
		<-release
	}
	s.sender.mu.Unlock()
	second := s.verifiedUser("in-flight@example.com")
	s.enqueue(authm.AuthEmailKindMagicLink, second.ID)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		s.FailNow("the second send never started")
	}

	stopped := make(chan struct{})
	go func() {
		s.outbox.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		s.FailNow("Stop returned while a send was in flight")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		s.FailNow("Stop did not return after the send finished")
	}
	for _, row := range s.rows() {
		s.Equal(authm.AuthEmailStatusSent, row.Status, "the in-flight row is recorded before Stop returns")
	}
}

// The email service the container passes has the context forms, so its
// provider errors are reported on the worker's hub.
var _ authEmailContextSender = (*notification.EmailService)(nil)

func TestAsContextSender(t *testing.T) {
	recording := &recordingSender{}
	if got := asContextSender(recording); got != authEmailContextSender(recording) {
		t.Fatalf("a sender with the context forms is used as is, got %T", got)
	}

	// Embedding the interface hides the context forms, leaving only the
	// email contract's plain sends.
	plain := struct{ authEmailSender }{recording}
	cs := asContextSender(plain)
	ctx := sentry.SetHubOnContext(context.Background(), sentry.NewHub(nil, sentry.NewScope()))
	if err := cs.SendMagicLinkEmailContext(ctx, "a@example.com", "t"); err != nil {
		t.Fatal(err)
	}
	if err := cs.SendAccountRecoveryEmailContext(ctx, "b@example.com", "t", 3); err != nil {
		t.Fatal(err)
	}
	if err := cs.SendVerificationEmailContext(ctx, "c@example.com", "t"); err != nil {
		t.Fatal(err)
	}
	sends := recording.recorded()
	wantKinds := []authm.AuthEmailKind{authm.AuthEmailKindMagicLink, authm.AuthEmailKindAccountRecovery, authm.AuthEmailKindVerification}
	if len(sends) != len(wantKinds) {
		t.Fatalf("got %d sends, want %d", len(sends), len(wantKinds))
	}
	for i, send := range sends {
		if send.kind != wantKinds[i] {
			t.Fatalf("send %d kind = %s, want %s", i, send.kind, wantKinds[i])
		}
		if send.hub != nil {
			t.Fatalf("send %d reached the plain form with a context hub; the plain forms take no context", i)
		}
	}
}

func TestNewAuthEmailOutbox_EnvKnobs(t *testing.T) {
	for _, tc := range []struct {
		name, interval, batch string
		wantInterval          time.Duration
		wantBatch             int
	}{
		{"unset", "", "", defaultAuthEmailOutboxInterval, defaultAuthEmailOutboxBatch},
		{"within limits", "7", "4", 7 * time.Second, 4},
		{"above limits", "3600", "50", maxAuthEmailOutboxInterval, defaultAuthEmailOutboxBatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AUTH_EMAIL_OUTBOX_INTERVAL_SECONDS", tc.interval)
			t.Setenv("AUTH_EMAIL_OUTBOX_BATCH", tc.batch)
			o := NewAuthEmailOutbox(nil, nil, nil, nil)
			if o.interval != tc.wantInterval || o.batch != tc.wantBatch {
				t.Fatalf("interval, batch = %v, %d; want %v, %d", o.interval, o.batch, tc.wantInterval, tc.wantBatch)
			}
		})
	}
}
