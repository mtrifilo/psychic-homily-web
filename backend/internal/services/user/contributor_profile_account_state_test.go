package user

import (
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// A deactivated or soft-deleted account answers GetPublicProfile exactly as an
// unknown username does, for an anonymous viewer and for a viewer carrying the
// account's own ID, and reactivating or restoring the account serves the
// profile again.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_NonLiveAccountIsNotFoundUntilRestored() {
	users := NewUserService(suite.db)
	deactivated := suite.createTestUser("deactivated-profile")
	deleted := suite.createTestUser("deleted-profile")

	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", false).Error)
	suite.Require().NoError(users.SoftDeleteAccount(deleted.ID, nil))

	for _, u := range []*authm.User{deactivated, deleted} {
		for _, viewer := range []contracts.ShowViewer{{}, {UserID: u.ID}} {
			profile, err := suite.profileService.GetPublicProfile(*u.Username, viewer)
			suite.Require().NoError(err)
			suite.Nil(profile, "%s viewed by user %d", *u.Username, viewer.UserID)
		}
	}

	suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", deactivated.ID).
		Update("is_active", true).Error)
	suite.Require().NoError(users.RestoreAccount(deleted.ID))

	for _, u := range []*authm.User{deactivated, deleted} {
		profile, err := suite.profileService.GetPublicProfile(*u.Username, contracts.ShowViewer{})
		suite.Require().NoError(err)
		suite.Require().NotNil(profile, *u.Username)
		suite.Equal(*u.Username, profile.Username)
	}
}
