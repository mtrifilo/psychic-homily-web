package catalog

import (
	"context"

	"psychic-homily-backend/internal/services/contracts"
)

// TestBulkLink_AppliesTheCreditRuleToReleases: when a bulk-link pass links a
// play's artist, the play keeps no release credited to someone else. It gets
// the one release with its album title credited to the artist, and no release
// when there is none.
func (suite *RadioMatchingIntegrationTestSuite) TestBulkLink_AppliesTheCreditRuleToReleases() {
	tmbg := suite.createArtist("They Might Be Giants")
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)
	borisLincoln := suite.createCreditedRelease("Lincoln", boris.ID)
	tmbgLincoln := suite.createCreditedRelease("Lincoln", tmbg.ID)
	tmbgApollo := suite.createCreditedRelease("Apollo 18", tmbg.ID)

	episodeID := suite.createStationShowEpisode()
	cleared := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Flood"), nil)
	relinked := suite.createPlay(episodeID, 2, "They Might Be Giants", strPtr("Lincoln"), nil)
	linked := suite.createPlay(episodeID, 3, "They Might Be Giants", strPtr("Apollo 18"), nil)
	suite.Require().NoError(suite.db.Model(cleared).Update("release_id", borisFlood.ID).Error)
	suite.Require().NoError(suite.db.Model(relinked).Update("release_id", borisLincoln.ID).Error)

	bulk, err := suite.engine.BulkLinkUnmatchedArtistPlays(context.Background())
	suite.Require().NoError(err)
	suite.Equal(int64(3), bulk.NameLinked)

	for _, id := range []uint{cleared.ID, relinked.ID, linked.ID} {
		reloaded := suite.reloadPlay(id)
		suite.Require().NotNil(reloaded.ArtistID)
		suite.Equal(tmbg.ID, *reloaded.ArtistID)
	}
	suite.Nil(suite.reloadPlay(cleared.ID).ReleaseID, "no release with the title credits the artist")
	relinkedRelease := suite.reloadPlay(relinked.ID).ReleaseID
	suite.Require().NotNil(relinkedRelease)
	suite.Equal(tmbgLincoln.ID, *relinkedRelease)
	linkedRelease := suite.reloadPlay(linked.ID).ReleaseID
	suite.Require().NotNil(linkedRelease)
	suite.Equal(tmbgApollo.ID, *linkedRelease)
}

// TestLabelRematch_KeepsAnAdminSetRelease: an admin linked a play to one of
// two releases with its title credited to its artist; a label rematch keeps
// that release rather than taking the lower-id one.
func (suite *RadioMatchingIntegrationTestSuite) TestLabelRematch_KeepsAnAdminSetRelease() {
	tmbg := suite.createArtist("They Might Be Giants")
	suite.createCreditedRelease("Flood", tmbg.ID)
	deluxe := suite.createCreditedRelease("Flood", tmbg.ID)

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Flood"), strPtr("Elektra"))
	svc := &RadioService{db: suite.db}
	suite.Require().NoError(svc.LinkPlay(play.ID, &contracts.LinkPlayRequest{ArtistID: &tmbg.ID, ReleaseID: &deluxe.ID}))
	label := suite.createLabel("Elektra")

	result, err := suite.engine.MatchUnmatchedPlaysForLabelName("Elektra")
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	reloaded := suite.reloadPlay(play.ID)
	suite.Require().NotNil(reloaded.LabelID)
	suite.Equal(label.ID, *reloaded.LabelID)
	suite.Require().NotNil(reloaded.ReleaseID)
	suite.Equal(deluxe.ID, *reloaded.ReleaseID, "the admin-set release is kept")
}

// TestBackfillReleaseLinks_KeepsAnAdminSetReleaseWithoutAnAuditRow: the
// backfill never changes a release an admin set, even one not credited to the
// play's artist and with no audit row to name it.
func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_KeepsAnAdminSetReleaseWithoutAnAuditRow() {
	tySegall := suite.createArtist("Ty Segall")
	niceNice := suite.createArtist("Nice Nice")
	niceNiceChrome := suite.createCreditedRelease("Chrome", niceNice.ID)

	episodeID := suite.createStationShowEpisode()
	play := suite.createPlay(episodeID, 1, "Ty Segall", strPtr("Chrome"), nil)
	svc := &RadioService{db: suite.db}
	suite.Require().NoError(svc.LinkPlay(play.ID, &contracts.LinkPlayRequest{ArtistID: &tySegall.ID, ReleaseID: &niceNiceChrome.ID}))

	report, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(1, report.Scanned)
	suite.Empty(report.Changes, "an admin-set release is not removed or relinked")
}
