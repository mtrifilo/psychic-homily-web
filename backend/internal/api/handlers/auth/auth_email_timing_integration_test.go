package auth

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/notification"
)

// blockedSenderResponseBound is how long a magic-link or recovery request may
// take while the email sender is blocked. The handler does a lookup and at
// most one insert, so this is generous; a handler that waits on the sender
// never answers inside it.
const blockedSenderResponseBound = 2 * time.Second

// blockingSender is an email service whose magic-link, recovery and
// verification sends block until release is called, counting the sends that
// complete.
type blockingSender struct {
	*testhelpers.MockEmailService
	released      chan struct{}
	releaseOnce   sync.Once
	magicLinks    atomic.Int32
	recoveries    atomic.Int32
	verifications atomic.Int32
	lastRecovery  atomic.Int32 // daysRemaining of the last recovery send
}

func newBlockingSender() *blockingSender {
	b := &blockingSender{released: make(chan struct{})}
	b.MockEmailService = &testhelpers.MockEmailService{
		IsConfiguredFn: func() bool { return true },
		SendMagicLinkEmailFn: func(string, string) error {
			<-b.released
			b.magicLinks.Add(1)
			return nil
		},
		SendAccountRecoveryEmailFn: func(_ string, _ string, daysRemaining int) error {
			<-b.released
			b.recoveries.Add(1)
			b.lastRecovery.Store(int32(daysRemaining))
			return nil
		},
		SendVerificationEmailFn: func(string, string) error {
			<-b.released
			b.verifications.Add(1)
			return nil
		},
	}
	return b
}

func (b *blockingSender) release() { b.releaseOnce.Do(func() { close(b.released) }) }

// newHandlerWithBlockingSender builds the handler around the real services,
// the real outbox, and sender.
func (s *AuthHandlerIntegrationSuite) newHandlerWithBlockingSender(sender *blockingSender) (*AuthHandler, *auth.AuthEmailOutbox) {
	jwtSvc := auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
	outbox := auth.NewAuthEmailOutbox(s.deps.DB, s.deps.UserService, jwtSvc, sender)
	h := NewAuthHandler(
		auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService),
		jwtSvc,
		s.deps.UserService,
		sender,
		outbox,
		notification.NewDiscordService(s.cfg),
		nil,
		s.cfg,
	)
	return h, outbox
}

// answersWithin runs call and fails the test if it does not return within
// blockedSenderResponseBound. It returns the call's result, or the zero value
// on a timeout.
func answersWithin[T any](s *AuthHandlerIntegrationSuite, label string, call func() T) T {
	var result T
	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		result = call()
	}()
	select {
	case <-done:
		s.T().Logf("%s answered in %v", label, time.Since(start))
		return result
	case <-time.After(blockedSenderResponseBound):
		s.Failf("request waited on the email sender", "%s did not answer within %v while the sender was blocked", label, blockedSenderResponseBound)
		var zero T
		return zero
	}
}

// outboxRows returns the user's auth_email_outbox rows.
func (s *AuthHandlerIntegrationSuite) outboxRows(userID uint) []authm.AuthEmailOutboxItem {
	var rows []authm.AuthEmailOutboxItem
	s.Require().NoError(s.deps.DB.Where("user_id = ?", userID).Order("id").Find(&rows).Error)
	return rows
}

func (s *AuthHandlerIntegrationSuite) TestSendMagicLink_AnswersWithoutWaitingOnSender() {
	sender := newBlockingSender()
	defer sender.release()
	h, outbox := s.newHandlerWithBlockingSender(sender)
	user := s.createUserWithPassword("magic-timing@test.com", "strong-password-123!")
	s.verifyEmail(user.ID)
	unverifiedUser := s.createUserWithPassword("magic-timing-unverified@test.com", "strong-password-123!")

	send := func(email string) func() *SendMagicLinkResponse {
		return func() *SendMagicLinkResponse {
			input := &SendMagicLinkRequest{}
			input.Body.Email = email
			resp, err := h.SendMagicLinkHandler(context.Background(), input)
			s.NoError(err)
			return resp
		}
	}
	unknown := answersWithin(s, "unknown email", send("nobody@test.com"))
	known := answersWithin(s, "known verified email", send("magic-timing@test.com"))
	unverified := answersWithin(s, "known unverified email", send("magic-timing-unverified@test.com"))
	s.Require().NotNil(unknown)
	s.Require().NotNil(known)
	s.Require().NotNil(unverified)
	s.Equal(unknown.Body, known.Body, "known and unknown emails get the same body")
	s.Equal(unknown.Body, unverified.Body, "unverified and unknown emails get the same body")
	s.Equal(int32(0), sender.magicLinks.Load(), "the handler sends no magic link itself")
	s.Equal(int32(0), sender.verifications.Load(), "the handler sends no verification email itself")

	rows := s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailKindMagicLink, rows[0].Kind)
	s.Equal(authm.AuthEmailStatusPending, rows[0].Status)
	unverifiedRows := s.outboxRows(unverifiedUser.ID)
	s.Require().Len(unverifiedRows, 1)
	s.Equal(authm.AuthEmailKindVerification, unverifiedRows[0].Kind)
	s.Equal(authm.AuthEmailStatusPending, unverifiedRows[0].Status)

	sender.release()
	outbox.RunNow(context.Background())

	s.Equal(int32(1), sender.magicLinks.Load(), "the poller sends the queued magic link once")
	s.Equal(int32(1), sender.verifications.Load(), "the poller sends the queued verification email once")
	rows = s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailStatusSent, rows[0].Status)
	s.NotNil(rows[0].SentAt)
	unverifiedRows = s.outboxRows(unverifiedUser.ID)
	s.Require().Len(unverifiedRows, 1)
	s.Equal(authm.AuthEmailStatusSent, unverifiedRows[0].Status)
}

func (s *AuthHandlerIntegrationSuite) TestRequestRecovery_AnswersWithoutWaitingOnSender() {
	sender := newBlockingSender()
	defer sender.release()
	h, outbox := s.newHandlerWithBlockingSender(sender)
	user := s.createUserWithPassword("recover-timing@test.com", "strong-password-123!")
	s.softDeleteUser(user.ID)

	request := func(email string) func() *RequestAccountRecoveryResponse {
		return func() *RequestAccountRecoveryResponse {
			input := &RequestAccountRecoveryRequest{}
			input.Body.Email = email
			resp, err := h.RequestAccountRecoveryHandler(context.Background(), input)
			s.NoError(err)
			return resp
		}
	}
	unknown := answersWithin(s, "unknown email", request("nobody@test.com"))
	known := answersWithin(s, "recoverable email", request("recover-timing@test.com"))
	s.Require().NotNil(unknown)
	s.Require().NotNil(known)
	s.Equal(unknown.Body, known.Body, "known and unknown emails get the same body")
	s.Equal(int32(0), sender.recoveries.Load(), "the handler sends nothing itself")

	rows := s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailKindAccountRecovery, rows[0].Kind)
	s.Equal(authm.AuthEmailStatusPending, rows[0].Status)

	sender.release()
	outbox.RunNow(context.Background())

	s.Equal(int32(1), sender.recoveries.Load(), "the poller sends the queued recovery email once")
	s.Positive(sender.lastRecovery.Load(), "days remaining is computed at send time")
	rows = s.outboxRows(user.ID)
	s.Require().Len(rows, 1)
	s.Equal(authm.AuthEmailStatusSent, rows[0].Status)
}
