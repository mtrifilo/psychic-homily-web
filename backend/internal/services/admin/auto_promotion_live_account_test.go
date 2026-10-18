package admin

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// EvaluateAllUsers evaluates live accounts only: of three otherwise identical
// promotable accounts, the deactivated and the soft-deleted ones are neither
// promoted nor counted, and the live one is promoted.
func (s *AutoPromotionIntegrationTestSuite) TestEvaluateAllUsers_EvaluatesOnlyLiveAccounts() {
	artist := s.createTestArtist("Live Account Artist")
	old := time.Now().Add(-15 * 24 * time.Hour)
	live := s.createUser(TierNewUser, true, old)
	deactivated := s.createUser(TierNewUser, true, old)
	deleted := s.createUser(TierNewUser, true, old)
	for _, u := range []*authm.User{live, deactivated, deleted} {
		for i := 0; i < 10; i++ {
			s.createApprovedPendingEdit(u.ID, "artist", artist.ID)
		}
	}
	testutil.DeactivateAccount(s.T(), s.db, deactivated.ID)
	testutil.SoftDeleteAccount(s.T(), s.db, deleted.ID)

	result, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)
	s.Equal(live.ID, result.Promoted[0].UserID)
	s.Empty(result.Demoted)
	s.Zero(result.Unchanged, "a closed account is not evaluated at all")
	s.Zero(result.Errors)
}
