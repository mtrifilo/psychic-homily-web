package user

import (
	"encoding/json"
	"fmt"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
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

// The percentile cohort is live accounts only, in the population count and in
// every dimension: closed accounts with no contributions neither dilute the
// denominator nor count as ranked below the subject.
func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_CohortIsLiveAccountsOnly() {
	live := suite.createManyUsers("livecohort", 10)
	subject := live[0]
	closed := suite.createManyUsers("closedcohort", 10)
	for i, u := range closed {
		updates := map[string]any{"is_active": false}
		if i%2 == 1 {
			updates["deleted_at"] = time.Now()
		}
		suite.Require().NoError(suite.db.Model(&authm.User{}).Where("id = ?", u.ID).Updates(updates).Error)
	}

	show := suite.createShow(subject.ID, "Cohort Show")
	suite.createVenue(subject.ID, "Cohort Venue")
	tag := &catalogm.Tag{Name: "cohort tag", Slug: "cohort-tag", Category: catalogm.TagCategoryGenre}
	suite.Require().NoError(suite.db.Create(tag).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.EntityTag{
		TagID: tag.ID, EntityType: "show", EntityID: show.ID, AddedByUserID: subject.ID,
	}).Error)
	changes := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&adminm.PendingEntityEdit{
		EntityType: "artist", EntityID: 1, SubmittedBy: subject.ID, FieldChanges: &changes,
		Summary: "Cohort edit", Status: adminm.PendingEditStatusApproved,
	}).Error)
	suite.Require().NoError(suite.db.Create(&communitym.Request{
		Title: "Cohort request", EntityType: "artist", RequesterID: live[1].ID,
		FulfillerID: &subject.ID, Status: communitym.RequestStatusFulfilled,
	}).Error)

	result, err := suite.profileService.GetPercentileRankings(subject.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Require().Len(result.Rankings, len(percentileDimensions))
	for _, r := range result.Rankings {
		suite.Equal(int64(1), r.Value, r.Dimension)
		// 9 of the 10 live accounts have none: 90. Counting the 10 closed
		// accounts in the population gives 45; counting them as ranked below
		// gives 100.
		suite.Equal(90, r.Percentile, fmt.Sprintf("%s percentile", r.Dimension))
	}
}
