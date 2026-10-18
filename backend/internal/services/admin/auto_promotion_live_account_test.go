package admin

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
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
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", false).Error)
	s.Require().NoError(s.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": false, "deleted_at": time.Now()}).Error)

	result, err := s.svc.EvaluateAllUsers()
	s.Require().NoError(err)
	s.Require().Len(result.Promoted, 1)
	s.Equal(live.ID, result.Promoted[0].UserID)
	s.Empty(result.Demoted)
	s.Zero(result.Unchanged, "a closed account is not evaluated at all")
	s.Zero(result.Errors)
}
