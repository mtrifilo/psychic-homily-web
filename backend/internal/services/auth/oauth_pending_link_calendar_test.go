package auth

import (
	"gorm.io/gorm"
)

// The mailbox-proof reclaim revokes the feed link and leaves no link-reset
// marker, whether or not a calendar app had read the link: the link belonged
// to whoever held the account before the proof.
func (s *JWTIntegrationSuite) TestEvictOtherCredentials_RevokesTheFeedAndLeavesNoLinkReset() {
	cases := []struct {
		name  string
		email string
		read  bool
	}{
		{"link a calendar app had read", "evict-read-feed@test.com", true},
		{"link no calendar app had read", "evict-unread-feed@test.com", false},
	}
	for _, c := range cases {
		s.Run(c.name, func() {
			user := s.createActiveUser(c.email)
			feed := s.feedToken(user.ID)
			if c.read {
				s.recordCalendarRead(feed)
			}

			s.Require().NoError(s.db.Transaction(func(tx *gorm.DB) error {
				return evictOtherCredentials(tx, user.ID, "google")
			}))

			s.requireFeedTokenRevoked(user.ID, feed)
			s.requireNoLinkReset(user.ID)
		})
	}
}
