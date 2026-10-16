package catalog

import (
	"context"
	"os"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
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

// releaseLinkSource reads a play's release_link_source, "" for NULL.
func (suite *RadioMatchingIntegrationTestSuite) releaseLinkSource(playID uint) string {
	if source := suite.reloadPlay(playID).ReleaseLinkSource; source != nil {
		return *source
	}
	return ""
}

// setReleaseLink points a play at a release with the given source ("" for NULL).
func (suite *RadioMatchingIntegrationTestSuite) setReleaseLink(playID, releaseID uint, source string) {
	var src interface{}
	if source != "" {
		src = source
	}
	suite.Require().NoError(suite.db.Exec(
		"UPDATE radio_plays SET release_id = ?, release_link_source = ? WHERE id = ?", releaseID, src, playID).Error)
}

func (suite *RadioMatchingIntegrationTestSuite) TestReleaseLinkSource_CheckRefusesAnUnknownSource() {
	release := suite.createRelease("Flood")
	play := suite.createPlay(suite.createStationShowEpisode(), 1, "They Might Be Giants", strPtr("Flood"), nil)
	err := suite.db.Exec("UPDATE radio_plays SET release_id = ?, release_link_source = 'guess' WHERE id = ?", release.ID, play.ID).Error
	suite.Require().ErrorIs(err, gorm.ErrCheckConstraintViolated)
	suite.Nil(suite.reloadPlay(play.ID).ReleaseID)
}

func (suite *RadioMatchingIntegrationTestSuite) TestMatchPlay_MarksTheReleaseItLinksAndClearsTheSourceWithTheRelease() {
	tmbg := suite.createArtist("They Might Be Giants")
	tmbgFlood := suite.createCreditedRelease("Flood", tmbg.ID)
	boris := suite.createArtist("Boris")
	borisPink := suite.createCreditedRelease("Pink", boris.ID)

	episodeID := suite.createStationShowEpisode()
	linked := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Flood"), nil)
	suite.True(suite.engine.matchPlay(linked))
	suite.Equal(&tmbgFlood.ID, suite.reloadPlay(linked.ID).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceMatcher, suite.releaseLinkSource(linked.ID))

	cleared := suite.createPlay(episodeID, 2, "They Might Be Giants", strPtr("Pink"), nil)
	suite.setReleaseLink(cleared.ID, borisPink.ID, catalogm.RadioPlayReleaseLinkSourceBackfill)
	reloaded := suite.reloadPlay(cleared.ID)
	suite.True(suite.engine.matchPlay(&reloaded))
	suite.Nil(suite.reloadPlay(cleared.ID).ReleaseID)
	suite.Empty(suite.releaseLinkSource(cleared.ID), "a cleared link carries no source")
}

func (suite *RadioMatchingIntegrationTestSuite) TestMatchPlay_KeepsTheSourceOfAReleaseItKeeps() {
	tmbg := suite.createArtist("They Might Be Giants")
	tmbgFlood := suite.createCreditedRelease("Flood", tmbg.ID)

	play := suite.createPlay(suite.createStationShowEpisode(), 1, "They Might Be Giants", strPtr("Flood"), nil)
	suite.setReleaseLink(play.ID, tmbgFlood.ID, catalogm.RadioPlayReleaseLinkSourceBackfill)
	reloaded := suite.reloadPlay(play.ID)
	suite.True(suite.engine.matchPlay(&reloaded))
	suite.Equal(&tmbgFlood.ID, suite.reloadPlay(play.ID).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceBackfill, suite.releaseLinkSource(play.ID))
}

// TestMatchPlay_NeverChangesAnAdminRelease: linking the play's artist neither
// replaces nor clears a release an admin chose, credited or not.
func (suite *RadioMatchingIntegrationTestSuite) TestMatchPlay_NeverChangesAnAdminRelease() {
	tmbg := suite.createArtist("They Might Be Giants")
	suite.createCreditedRelease("Flood", tmbg.ID)
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)

	play := suite.createPlay(suite.createStationShowEpisode(), 1, "They Might Be Giants", strPtr("Flood"), nil)
	svc := &RadioService{db: suite.db}
	suite.Require().NoError(svc.LinkPlay(play.ID, &contracts.LinkPlayRequest{ReleaseID: &borisFlood.ID}))
	reloaded := suite.reloadPlay(play.ID)

	suite.True(suite.engine.matchPlay(&reloaded))
	after := suite.reloadPlay(play.ID)
	suite.Equal(&tmbg.ID, after.ArtistID)
	suite.Equal(&borisFlood.ID, after.ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceAdmin, suite.releaseLinkSource(play.ID))
}

func (suite *RadioMatchingIntegrationTestSuite) TestLinkPlay_MarksAReleaseAdminAndLeavesTheSourceOtherwise() {
	tmbg := suite.createArtist("They Might Be Giants")
	flood := suite.createCreditedRelease("Flood", tmbg.ID)
	episodeID := suite.createStationShowEpisode()
	svc := &RadioService{db: suite.db}

	withRelease := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Flood"), nil)
	suite.Require().NoError(svc.LinkPlay(withRelease.ID, &contracts.LinkPlayRequest{ReleaseID: &flood.ID}))
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceAdmin, suite.releaseLinkSource(withRelease.ID))

	artistOnly := suite.createPlay(episodeID, 2, "They Might Be Giants", strPtr("Flood"), nil)
	suite.setReleaseLink(artistOnly.ID, flood.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)
	suite.Require().NoError(svc.LinkPlay(artistOnly.ID, &contracts.LinkPlayRequest{ArtistID: &tmbg.ID}))
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceMatcher, suite.releaseLinkSource(artistOnly.ID))
}

// TestBulkLink_MarksMatcherAndKeepsAnAdminRelease covers the SQL passes'
// source writes: a link they make is 'matcher', a clear drops the source, and
// an admin-chosen release survives the artist link.
func (suite *RadioMatchingIntegrationTestSuite) TestBulkLink_MarksMatcherAndKeepsAnAdminRelease() {
	tmbg := suite.createArtist("They Might Be Giants")
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)
	borisPink := suite.createCreditedRelease("Pink", boris.ID)
	tmbgApollo := suite.createCreditedRelease("Apollo 18", tmbg.ID)

	episodeID := suite.createStationShowEpisode()
	linked := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Apollo 18"), nil)
	cleared := suite.createPlay(episodeID, 2, "They Might Be Giants", strPtr("Pink"), nil)
	suite.setReleaseLink(cleared.ID, borisPink.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)
	admin := suite.createPlay(episodeID, 3, "They Might Be Giants", strPtr("Flood"), nil)
	suite.setReleaseLink(admin.ID, borisFlood.ID, catalogm.RadioPlayReleaseLinkSourceAdmin)

	bulk, err := suite.engine.BulkLinkUnmatchedArtistPlays(context.Background())
	suite.Require().NoError(err)
	suite.Equal(int64(3), bulk.NameLinked)

	suite.Equal(&tmbgApollo.ID, suite.reloadPlay(linked.ID).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceMatcher, suite.releaseLinkSource(linked.ID))
	suite.Nil(suite.reloadPlay(cleared.ID).ReleaseID)
	suite.Empty(suite.releaseLinkSource(cleared.ID))
	adminAfter := suite.reloadPlay(admin.ID)
	suite.Equal(&tmbg.ID, adminAfter.ArtistID, "the artist is linked")
	suite.Equal(&borisFlood.ID, adminAfter.ReleaseID, "the admin-chosen release is kept")
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceAdmin, suite.releaseLinkSource(admin.ID))
}

// TestBulkLink_SeveralCreditedReleasesLeaveNone: an artist with two releases
// of the play's title gets neither; the uncredited release is cleared.
func (suite *RadioMatchingIntegrationTestSuite) TestBulkLink_SeveralCreditedReleasesLeaveNone() {
	tmbg := suite.createArtist("They Might Be Giants")
	suite.createCreditedRelease("Flood", tmbg.ID)
	suite.createCreditedRelease("Flood", tmbg.ID)
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)

	episodeID := suite.createStationShowEpisode()
	unlinked := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Flood"), nil)
	uncredited := suite.createPlay(episodeID, 2, "They Might Be Giants", strPtr("Flood"), nil)
	suite.setReleaseLink(uncredited.ID, borisFlood.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)

	_, err := suite.engine.BulkLinkUnmatchedArtistPlays(context.Background())
	suite.Require().NoError(err)
	suite.Nil(suite.reloadPlay(unlinked.ID).ReleaseID)
	suite.Nil(suite.reloadPlay(uncredited.ID).ReleaseID)
	suite.Empty(suite.releaseLinkSource(uncredited.ID))
}

func (suite *RadioMatchingIntegrationTestSuite) TestAdminBulkLinkPlays_AppliesTheCreditRule() {
	tmbg := suite.createArtist("They Might Be Giants")
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)
	borisPink := suite.createCreditedRelease("Pink", boris.ID)
	tmbgApollo := suite.createCreditedRelease("Apollo 18", tmbg.ID)

	episodeID := suite.createStationShowEpisode()
	linked := suite.createPlay(episodeID, 1, "TMBG", strPtr("Apollo 18"), nil)
	cleared := suite.createPlay(episodeID, 2, "TMBG", strPtr("Pink"), nil)
	suite.setReleaseLink(cleared.ID, borisPink.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)
	admin := suite.createPlay(episodeID, 3, "TMBG", strPtr("Flood"), nil)
	suite.setReleaseLink(admin.ID, borisFlood.ID, catalogm.RadioPlayReleaseLinkSourceAdmin)
	other := suite.createPlay(episodeID, 4, "Someone Else", strPtr("Pink"), nil)
	suite.setReleaseLink(other.ID, borisPink.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)

	svc := &RadioService{db: suite.db}
	result, err := svc.BulkLinkPlays(&contracts.BulkLinkRequest{ArtistName: "TMBG", ArtistID: tmbg.ID})
	suite.Require().NoError(err)
	suite.Equal(3, result.Updated)

	suite.Equal(&tmbgApollo.ID, suite.reloadPlay(linked.ID).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceMatcher, suite.releaseLinkSource(linked.ID))
	suite.Nil(suite.reloadPlay(cleared.ID).ReleaseID)
	suite.Empty(suite.releaseLinkSource(cleared.ID))
	suite.Equal(&borisFlood.ID, suite.reloadPlay(admin.ID).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceAdmin, suite.releaseLinkSource(admin.ID))
	otherAfter := suite.reloadPlay(other.ID)
	suite.Nil(otherAfter.ArtistID, "a play with another name is not linked")
	suite.Equal(&borisPink.ID, otherAfter.ReleaseID, "nor is its release judged")
}

// TestBackfillReleaseLinks_RespectsAndWritesTheSource: the backfill counts
// links by source, never changes an admin link, marks a relink 'backfill' and
// clears the source with a removal.
func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_RespectsAndWritesTheSource() {
	tmbg := suite.createArtist("They Might Be Giants")
	tmbgFlood := suite.createCreditedRelease("Flood", tmbg.ID)
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)
	borisPink := suite.createCreditedRelease("Pink", boris.ID)

	episodeID := suite.createStationShowEpisode()
	play := func(position int, album string, releaseID uint, source string) uint {
		p := suite.createPlay(episodeID, position, "They Might Be Giants", strPtr(album), nil)
		suite.Require().NoError(suite.db.Model(p).Update("artist_id", tmbg.ID).Error)
		suite.setReleaseLink(p.ID, releaseID, source)
		return p.ID
	}
	admin := play(1, "Pink", borisPink.ID, catalogm.RadioPlayReleaseLinkSourceAdmin)
	relinked := play(2, "Flood", borisFlood.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)
	cleared := play(3, "Pink", borisPink.ID, catalogm.RadioPlayReleaseLinkSourceMatcher)
	unmarked := play(4, "Flood", tmbgFlood.ID, "")

	report, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{})
	suite.Require().NoError(err)
	suite.Equal(4, report.Scanned)
	suite.Equal(map[string]int{
		catalogm.RadioPlayReleaseLinkSourceAdmin:   1,
		catalogm.RadioPlayReleaseLinkSourceMatcher: 2,
		"": 1,
	}, report.SourceCounts)
	suite.Equal(1, report.Counts[RadioReleaseLinkKeptAdmin])
	suite.Equal(int64(1), report.Relinked)
	suite.Equal(int64(1), report.Cleared)

	suite.Equal(&borisPink.ID, suite.reloadPlay(admin).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceAdmin, suite.releaseLinkSource(admin))
	suite.Equal(&tmbgFlood.ID, suite.reloadPlay(relinked).ReleaseID)
	suite.Equal(catalogm.RadioPlayReleaseLinkSourceBackfill, suite.releaseLinkSource(relinked))
	suite.Nil(suite.reloadPlay(cleared).ReleaseID)
	suite.Empty(suite.releaseLinkSource(cleared))
	suite.Equal(&tmbgFlood.ID, suite.reloadPlay(unmarked).ReleaseID, "a kept link is not rewritten")
	suite.Empty(suite.releaseLinkSource(unmarked))
}

// TestBackfillReleaseLinks_ApplySkipsAnAdminLink: a change list naming a play
// an admin linked after evaluation writes nothing to it.
func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_ApplySkipsAnAdminLink() {
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)
	tmbg := suite.createArtist("They Might Be Giants")
	tmbgFlood := suite.createCreditedRelease("Flood", tmbg.ID)

	episodeID := suite.createStationShowEpisode()
	toClear := suite.createPlay(episodeID, 1, "They Might Be Giants", strPtr("Flood"), nil)
	suite.setReleaseLink(toClear.ID, borisFlood.ID, catalogm.RadioPlayReleaseLinkSourceAdmin)
	toRelink := suite.createPlay(episodeID, 2, "They Might Be Giants", strPtr("Flood"), nil)
	suite.setReleaseLink(toRelink.ID, borisFlood.ID, catalogm.RadioPlayReleaseLinkSourceAdmin)
	report := &RadioReleaseLinkReport{Changes: []RadioReleaseLinkChange{
		{PlayID: toClear.ID, Outcome: RadioReleaseLinkRemovedNotCredited, CurrentReleaseID: borisFlood.ID},
		{PlayID: toRelink.ID, Outcome: RadioReleaseLinkRelinkedCredited, CurrentReleaseID: borisFlood.ID, NewReleaseID: &tmbgFlood.ID},
	}}

	suite.Require().NoError(applyRadioReleaseLinkChanges(suite.db, report))
	suite.Zero(report.Cleared)
	suite.Zero(report.Relinked)
	suite.Equal(&borisFlood.ID, suite.reloadPlay(toClear.ID).ReleaseID)
	suite.Equal(&borisFlood.ID, suite.reloadPlay(toRelink.ID).ReleaseID)
}

// TestReleaseLinkSourceMigration_MarksExistingLinks replays the migration's
// down and up over seeded rows: a link an audit row names with its current
// release is 'admin', every other link 'matcher', and a play without a release
// stays NULL.
func (suite *RadioMatchingIntegrationTestSuite) TestReleaseLinkSourceMigration_MarksExistingLinks() {
	flood := suite.createRelease("Flood")
	pink := suite.createRelease("Pink")
	episodeID := suite.createStationShowEpisode()
	play := func(position int, releaseID *uint) uint {
		p := suite.createPlay(episodeID, position, "They Might Be Giants", strPtr("Flood"), nil)
		if releaseID != nil {
			suite.setReleaseLink(p.ID, *releaseID, "")
		}
		return p.ID
	}
	admin := play(1, &flood.ID)
	otherRelease := play(2, &flood.ID)
	otherAction := play(3, &flood.ID)
	noAudit := play(4, &pink.ID)
	noRelease := play(5, nil)
	audit := func(action string, playID uint, releaseID *uint) {
		suite.Require().NoError(suite.db.Exec(`
INSERT INTO audit_logs (action, entity_type, entity_id, metadata)
VALUES (?, ?, ?, jsonb_build_object(?::text, ?::int, ?::text, NULL))`,
			action, contracts.AuditEntityRadioPlay, playID,
			contracts.AuditMetaReleaseID, releaseID, contracts.AuditMetaArtistID).Error)
	}
	audit(contracts.AuditActionLinkRadioPlay, admin, &flood.ID)
	audit(contracts.AuditActionLinkRadioPlay, otherRelease, &pink.ID)
	audit("bulk_link_radio_plays", otherAction, &flood.ID)
	audit(contracts.AuditActionLinkRadioPlay, noRelease, nil)

	for _, direction := range []string{"down", "up"} {
		body, err := os.ReadFile("../../../db/migrations/20261015210000_radio_plays_release_link_source." + direction + ".sql")
		suite.Require().NoError(err)
		suite.Require().NoError(suite.db.Transaction(func(tx *gorm.DB) error {
			return tx.Exec(string(body)).Error
		}), "migration %s", direction)
	}

	want := map[uint]string{
		admin:        catalogm.RadioPlayReleaseLinkSourceAdmin,
		otherRelease: catalogm.RadioPlayReleaseLinkSourceMatcher,
		otherAction:  catalogm.RadioPlayReleaseLinkSourceMatcher,
		noAudit:      catalogm.RadioPlayReleaseLinkSourceMatcher,
		noRelease:    "",
	}
	for playID, source := range want {
		suite.Equal(source, suite.releaseLinkSource(playID), "release_link_source of play %d", playID)
	}
}
