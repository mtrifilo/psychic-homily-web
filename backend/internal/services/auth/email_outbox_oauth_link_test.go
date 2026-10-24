package auth

import (
	"context"
	"time"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func (s *AuthEmailOutboxSuite) linkService() *AuthService {
	return NewAuthService(s.db, &config.Config{
		JWT: config.JWTConfig{SecretKey: "auth-email-outbox-test-secret-key-32!", Expiry: 24},
	}, s.users)
}

// startPendingLink holds a Google identity as a pending link to user, as the
// callback does, and returns its token.
func (s *AuthEmailOutboxSuite) startPendingLink(user *authm.User) string {
	s.T().Helper()
	token, err := s.linkService().StartOAuthLink(contracts.OAuthIdentity{
		Provider:      "google",
		Subject:       "google-outbox-" + *user.Email,
		Email:         *user.Email,
		EmailVerified: true,
	}, user.ID, "/")
	s.Require().NoError(err)
	return token
}

func (s *AuthEmailOutboxSuite) unverifiedUser(email string) *authm.User {
	user := &authm.User{Email: &email, IsActive: true}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *AuthEmailOutboxSuite) pendingLinkExpiry(ref string) time.Time {
	var row authm.OAuthPendingSignup
	s.Require().NoError(s.db.Where("token_hash = ?", ref).First(&row).Error)
	return row.ExpiresAt
}

// Asking for the emailed link keeps the pending link alive for as long as
// the outbox may hold the email plus the emailed link's own lifetime, so a
// send at any moment the outbox may make it delivers a link that works.
func (s *AuthEmailOutboxSuite) TestHoldOAuthLinkForEmail_KeepsThePendingLinkForTheWholeDelivery() {
	user := s.unverifiedUser("held@example.com")
	token := s.startPendingLink(user)

	before := time.Now()
	link, err := s.linkService().HoldOAuthLinkForEmail(token)
	s.Require().NoError(err)
	s.False(s.pendingLinkExpiry(link.Ref).Before(before.Add(authEmailMaxAge + contracts.MagicLinkTokenTTL).Add(-time.Second)))

	// A gone pending link is refused and kept gone.
	s.Require().NoError(s.db.Exec(
		"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE token_hash = ?", link.Ref).Error)
	_, err = s.linkService().HoldOAuthLinkForEmail(token)
	s.Error(err)
	s.True(s.pendingLinkExpiry(link.Ref).Before(time.Now()))
}

// The mailbox proof is what the link is for, so an unverified address gets
// it, and the token names the pending link.
func (s *AuthEmailOutboxSuite) TestOAuthLink_SentToAnUnverifiedAddress() {
	user := s.unverifiedUser("squatted@example.com")
	link, err := s.linkService().HoldOAuthLinkForEmail(s.startPendingLink(user))
	s.Require().NoError(err)
	s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), user.ID, link.Ref))
	s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), user.ID, link.Ref))
	s.Len(s.rows(), 1, "one queued link per account")

	s.outbox.RunNow(context.Background())

	sends := s.sender.recorded()
	s.Require().Len(sends, 1)
	s.Equal("squatted@example.com", sends[0].to)
	claims, err := s.jwt.ValidateOAuthLinkToken(sends[0].token)
	s.Require().NoError(err, "the email carries a link token")
	s.Equal(user.ID, claims.UserID)
	s.Equal(link.Ref, claims.PendingLink)
	_, err = s.jwt.ValidateMagicLinkToken(sends[0].token)
	s.Error(err, "the link token is not a plain sign-in")
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

// An account that may no longer sign in gets no link.
func (s *AuthEmailOutboxSuite) TestOAuthLink_SkippedForAClosedAccount() {
	user := s.unverifiedUser("closed@example.com")
	link, err := s.linkService().HoldOAuthLinkForEmail(s.startPendingLink(user))
	s.Require().NoError(err)
	s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), user.ID, link.Ref))
	s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	s.Equal(authm.AuthEmailStatusSkipped, s.onlyRow().Status)
}
