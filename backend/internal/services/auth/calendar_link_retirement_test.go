package auth

import (
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
			email := string(reason) + map[bool]string{true: "-read", false: "-unread"}[read] + "@retire.test"
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
