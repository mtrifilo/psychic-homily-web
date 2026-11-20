package auth

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	engagementm "psychic-homily-backend/internal/models/engagement"
)

// requireRetiredAsReset checks that the old feed address token is remembered
// as reset, and that its notice carries nothing about the account.
func (s *JWTIntegrationSuite) requireRetiredAsReset(token, email string) {
	s.T().Helper()
	calendar, err := s.calendar().RetiredSavedShowsNotice(token)
	s.Require().NoError(err)
	s.Require().NotNil(calendar, "the old address is remembered")
	unfolded := strings.ReplaceAll(string(calendar.Body), "\r\n ", "")
	s.Contains(unfolded, "\r\nSUMMARY:Psychic Homily: this calendar link was reset. Add it again from your Library.\r\n")

	feed, err := s.calendar().RetiredFollowsNotice(token)
	s.Require().NoError(err)
	s.Require().NotNil(feed)
	s.Contains(string(feed.Body), "<title>This feed link was reset</title>")

	for _, body := range [][]byte{calendar.Body, feed.Body} {
		s.NotContains(string(body), email)
		s.NotContains(string(body), token)
	}
}

// Signing out everywhere and a password reset retire the old address as
// reset, whether or not a calendar app had read it.
func (s *JWTIntegrationSuite) TestRevokeSessionsAndFeedToken_RetiresTheAddressAsReset() {
	for _, reason := range engagementm.CalendarLinkResetReasons {
		for _, read := range []bool{true, false} {
			email := fmt.Sprintf("%s-read-%t@retire.test", reason, read)
			user := s.createActiveUser(email)
			feed := s.feedToken(user.ID)
			if read {
				s.recordCalendarRead(feed)
			}

			s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
				_, err := revokeSessionsAndFeedToken(tx, user, time.Now(), reason)
				return err
			}))

			s.requireFeedTokenRevoked(user.ID, feed)
			s.requireRetiredAsReset(feed, email)
		}
	}
}

// Through the entry points the site calls: a completed password reset and
// signing out everywhere else each retire the old address as reset.
func (s *JWTIntegrationSuite) TestPasswordResetAndSignOutEverywhere_RetireTheAddressAsReset() {
	reset := s.createActiveUser("retire-through-reset@test.com")
	s.setPasswordHash(reset.ID, "planted-password-1!")
	resetFeed := s.feedToken(reset.ID)
	_, err := s.authSvc.CompletePasswordReset(s.resetClaimsFor(reset.ID), "owners-new-password-1!")
	s.Require().NoError(err)
	s.requireFeedTokenRevoked(reset.ID, resetFeed)
	s.requireRetiredAsReset(resetFeed, "retire-through-reset@test.com")

	signOut := s.createActiveUser("retire-through-sign-out@test.com")
	signOutFeed := s.feedToken(signOut.ID)
	_, err = s.authSvc.SignOutOtherSessions(s.reloadUser(signOut.ID), time.Now())
	s.Require().NoError(err)
	s.requireFeedTokenRevoked(signOut.ID, signOutFeed)
	s.requireRetiredAsReset(signOutFeed, "retire-through-sign-out@test.com")
}

// The mailbox-proof reclaim leaves the reset note on the old address and
// nothing about the account's new owner.
func (s *JWTIntegrationSuite) TestEvictOtherCredentials_RetiresTheAddressAsReset() {
	email := "evict-retire@test.com"
	user := s.createActiveUser(email)
	feed := s.feedToken(user.ID)

	s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
		return evictOtherCredentials(tx, user.ID, "google")
	}))

	s.requireFeedTokenRevoked(user.ID, feed)
	s.requireNoLinkReset(user.ID)
	s.requireRetiredAsReset(feed, email)
}
