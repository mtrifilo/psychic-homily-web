package user

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// The profile's followers count counts live accounts only: a deactivated or a
// soft-deleted follower leaves it, and reactivating or restoring the account
// counts it again.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_FollowersCountIsLiveAccountsOnly() {
	target := suite.createTestUser("followed-live")
	live := suite.createTestUser("fan-live")
	deactivated := suite.createTestUser("fan-deactivated")
	deleted := suite.createTestUser("fan-deleted")
	for _, u := range []*authm.User{live, deactivated, deleted} {
		suite.Require().NoError(suite.db.Create(&engagementm.UserBookmark{
			UserID: u.ID, EntityType: engagementm.BookmarkEntityType("user"),
			EntityID: target.ID, Action: engagementm.BookmarkActionFollow,
		}).Error)
	}

	followers := func() int64 {
		stats, err := suite.profileService.GetContributionStats(target.ID, contracts.ShowViewer{UserID: target.ID})
		suite.Require().NoError(err)
		return stats.FollowersCount
	}

	suite.Equal(int64(3), followers())

	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", false).Error)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": false, "deleted_at": time.Now()}).Error)

	suite.Equal(int64(1), followers())

	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", true).Error)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": true, "deleted_at": nil}).Error)

	suite.Equal(int64(3), followers())
}
