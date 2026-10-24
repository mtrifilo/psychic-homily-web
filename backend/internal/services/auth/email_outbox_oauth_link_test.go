package auth

import (
	"context"
	"time"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// startPendingLink holds a Google identity as a pending link to user and
// returns its reference, as the callback does.
func (s *AuthEmailOutboxSuite) startPendingLink(user *authm.User) *contracts.OAuthPendingLink {
	s.T().Helper()
	svc := NewAuthService(s.db, &config.Config{
		JWT: config.JWTConfig{SecretKey: "auth-email-outbox-test-secret-key-32!", Expiry: 24},
	}, s.users)
	token, err := svc.StartOAuthLink(contracts.OAuthIdentity{
		Provider:      "google",
		Subject:       "google-outbox-" + *user.Email,
		Email:         *user.Email,
		EmailVerified: true,
	}, user.ID, "/")
	s.Require().NoError(err)
	link, err := svc.PeekOAuthLink(token)
	s.Require().NoError(err)
	return link
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

// The mailbox proof is what the link is for, so an unverified address gets
// it, and the pending link lives as long as the link does.
func (s *AuthEmailOutboxSuite) TestOAuthLink_SentToAnUnverifiedAddressAndExtendsThePendingLink() {
	user := s.unverifiedUser("squatted@example.com")
	link := s.startPendingLink(user)
	s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), user.ID, link.Ref))
	s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), user.ID, link.Ref))
	s.Len(s.rows(), 1, "one queued link per account")

	before := time.Now()
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

	s.False(s.pendingLinkExpiry(link.Ref).Before(before.Add(contracts.MagicLinkTokenTTL).Add(-time.Second)),
		"the pending link outlives the emailed link")
	s.Equal(authm.AuthEmailStatusSent, s.onlyRow().Status)
}

func (s *AuthEmailOutboxSuite) TestOAuthLink_SkippedWhenThePendingLinkIsGone() {
	cases := map[string]func(user *authm.User, link *contracts.OAuthPendingLink){
		"used": func(_ *authm.User, link *contracts.OAuthPendingLink) {
			s.Require().NoError(s.db.Exec("DELETE FROM oauth_pending_signups WHERE token_hash = ?", link.Ref).Error)
		},
		"expired": func(_ *authm.User, link *contracts.OAuthPendingLink) {
			s.Require().NoError(s.db.Exec(
				"UPDATE oauth_pending_signups SET expires_at = NOW() - INTERVAL '1 second' WHERE token_hash = ?", link.Ref).Error)
		},
		"account closed": func(user *authm.User, _ *contracts.OAuthPendingLink) {
			s.Require().NoError(s.db.Model(user).Update("is_active", false).Error)
		},
	}
	for name, spoil := range cases {
		s.Run(name, func() {
			s.SetupTest()
			defer s.TearDownTest()
			user := s.unverifiedUser("gone-" + name + "@example.com")
			link := s.startPendingLink(user)
			s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), user.ID, link.Ref))
			spoil(user, link)

			s.outbox.RunNow(context.Background())

			s.Empty(s.sender.recorded(), "no link that could only be refused")
			row := s.onlyRow()
			s.Equal(authm.AuthEmailStatusSkipped, row.Status)
		})
	}
}

// The reference names one account's pending link; another account's
// queued email cannot borrow it.
func (s *AuthEmailOutboxSuite) TestOAuthLink_SkippedForAnotherAccountsPendingLink() {
	owner := s.unverifiedUser("owner@example.com")
	other := s.verifiedUser("other@example.com")
	link := s.startPendingLink(owner)
	s.Require().NoError(s.outbox.EnqueueOAuthLink(context.Background(), other.ID, link.Ref))

	s.outbox.RunNow(context.Background())

	s.Empty(s.sender.recorded())
	s.Equal(authm.AuthEmailStatusSkipped, s.onlyRow().Status)
}
