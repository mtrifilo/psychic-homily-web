package engagement

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
)

// closeFollowerAccounts deactivates one account and soft-deletes the other, the
// two ways an account stops being live.
func (suite *FollowServiceIntegrationTestSuite) closeFollowerAccounts(deactivated, deleted *authm.User) {
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", false).Error)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": false, "deleted_at": time.Now()}).Error)
}

// reopenFollowerAccounts reverses closeFollowerAccounts.
func (suite *FollowServiceIntegrationTestSuite) reopenFollowerAccounts(deactivated, deleted *authm.User) {
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", true).Error)
	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deleted.ID).
		Updates(map[string]any{"is_active": true, "deleted_at": nil}).Error)
}

// Every follower count counts the live accounts the followers list shows: a
// deactivated or a soft-deleted follower is in none of GetFollowerCount,
// GetBatchFollowerCounts and the GetFollowers total, and reactivating or
// restoring the account counts it again.
func (suite *FollowServiceIntegrationTestSuite) TestFollowerCounts_CountOnlyLiveAccounts() {
	artistID := suite.createTestArtist("Live Count Artist")
	otherArtistID := suite.createTestArtist("Live Count Other Artist")
	live := suite.createTestUserWithUsername("live-counted")
	deactivated := suite.createTestUserWithUsername("deactivated-counted")
	deleted := suite.createTestUserWithUsername("deleted-counted")
	for _, u := range []*authm.User{live, deactivated, deleted} {
		suite.Require().NoError(suite.followService.Follow(u.ID, "artist", artistID))
	}
	suite.Require().NoError(suite.followService.Follow(deactivated.ID, "artist", otherArtistID))

	assertCounts := func(want, wantOther int64, msg string) {
		count, err := suite.followService.GetFollowerCount("artist", artistID)
		suite.Require().NoError(err)
		suite.Equal(want, count, "GetFollowerCount: %s", msg)

		batch, err := suite.followService.GetBatchFollowerCounts("artist", []uint{artistID, otherArtistID})
		suite.Require().NoError(err)
		suite.Equal(want, batch[artistID], "GetBatchFollowerCounts: %s", msg)
		suite.Contains(batch, otherArtistID, "a requested id answers even with no live follower: %s", msg)
		suite.Equal(wantOther, batch[otherArtistID], "GetBatchFollowerCounts other: %s", msg)

		followers, total, err := suite.followService.GetFollowers("artist", artistID, 10, 0)
		suite.Require().NoError(err)
		suite.Equal(want, total, "GetFollowers total: %s", msg)
		suite.Len(followers, int(want), "GetFollowers list: %s", msg)
	}

	assertCounts(3, 1, "all accounts live")

	suite.closeFollowerAccounts(deactivated, deleted)
	assertCounts(1, 0, "deactivated and soft-deleted followers")

	suite.reopenFollowerAccounts(deactivated, deleted)
	assertCounts(3, 1, "accounts reactivated and restored")
}

// When every follower is closed the list and its total agree on nothing: an
// empty page and a zero total.
func (suite *FollowServiceIntegrationTestSuite) TestGetFollowers_OnlyClosedFollowersAnswerEmptyPageAndZeroTotal() {
	artistID := suite.createTestArtist("Closed Only Artist")
	deactivated := suite.createTestUserWithUsername("closed-only-deactivated")
	deleted := suite.createTestUserWithUsername("closed-only-deleted")
	for _, u := range []*authm.User{deactivated, deleted} {
		suite.Require().NoError(suite.followService.Follow(u.ID, "artist", artistID))
	}
	suite.closeFollowerAccounts(deactivated, deleted)

	followers, total, err := suite.followService.GetFollowers("artist", artistID, 10, 0)
	suite.Require().NoError(err)
	suite.NotNil(followers)
	suite.Empty(followers)
	suite.Zero(total)
}
