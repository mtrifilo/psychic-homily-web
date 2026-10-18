package user

import (
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
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

	testutil.DeactivateAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, deleted.ID)
	suite.Equal(int64(1), followers())

	testutil.ReopenAccount(suite.T(), suite.db, deactivated.ID)
	testutil.ReopenAccount(suite.T(), suite.db, deleted.ID)
	suite.Equal(int64(3), followers())
}

// A clone's timeline row drops its source attribution while the source's
// creator is closed, as the fork's detail does, and keeps it again once the
// creator is reopened. The contribution itself stays.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_ScrubsCloneSourceOfClosedCreator() {
	cloner := suite.createTestUser("closed-source-cloner")
	stranger := suite.createTestUser("closed-source-stranger")
	deactivated := suite.createTestUser("closed-source-deactivated")
	deleted := suite.createTestUser("closed-source-deleted")

	for _, creator := range []*authm.User{deactivated, deleted} {
		source := suite.createCollectionForHistory(creator.ID, "Source "+*creator.Username, "source-"+*creator.Username, true)
		clone := suite.createCollectionForHistory(cloner.ID, "Clone "+*creator.Username, "clone-"+*creator.Username, true)
		suite.auditLog.LogAction(cloner.ID, "clone_collection", "collection", clone.ID,
			map[string]interface{}{"source_slug": source.Slug, "source_id": source.ID})
	}

	attributed := func() int {
		entries, _, err := suite.profileService.GetContributionHistory(
			cloner.ID, 50, 0, "", contracts.ShowViewer{UserID: stranger.ID})
		suite.Require().NoError(err)
		suite.Require().Len(entries, 2, "the clones are public, so the contributions stay")
		n := 0
		for _, e := range entries {
			if _, ok := e.Metadata["source_slug"]; ok {
				n++
			}
		}
		return n
	}

	suite.Equal(2, attributed())

	testutil.DeactivateAccount(suite.T(), suite.db, deactivated.ID)
	testutil.SoftDeleteAccount(suite.T(), suite.db, deleted.ID)
	suite.Equal(0, attributed())

	testutil.ReopenAccount(suite.T(), suite.db, deactivated.ID)
	testutil.ReopenAccount(suite.T(), suite.db, deleted.ID)
	suite.Equal(2, attributed())
}
