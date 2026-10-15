package catalog

import (
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// releaseLinkFixture is one play per backfill outcome plus a play without a
// release link.
type releaseLinkFixture struct {
	niceNiceChrome, borisFlood, tmbgFlood, niceNiceUnique uint
	tySegall, tmbg                                        uint

	notCredited, relinkCredited, keptCredited uint
	keptUnique, removedUnlinked, relinkUnique uint
	adminSet, withoutRelease                  uint
}

func (suite *RadioMatchingIntegrationTestSuite) seedReleaseLinkFixture() releaseLinkFixture {
	var f releaseLinkFixture
	niceNice := suite.createArtist("Nice Nice")
	tySegall := suite.createArtist("Ty Segall")
	boris := suite.createArtist("Boris")
	tmbg := suite.createArtist("They Might Be Giants")
	f.tySegall, f.tmbg = tySegall.ID, tmbg.ID
	f.niceNiceChrome = suite.createCreditedRelease("Chrome", niceNice.ID).ID
	f.borisFlood = suite.createCreditedRelease("Flood", boris.ID).ID
	f.tmbgFlood = suite.createCreditedRelease("Flood", tmbg.ID).ID
	f.niceNiceUnique = suite.createCreditedRelease("Unique LP", niceNice.ID).ID

	episodeID := suite.createStationShowEpisode()
	position := 0
	play := func(artistName, album string, artistID *uint, releaseID *uint) uint {
		position++
		p := suite.createPlay(episodeID, position, artistName, strPtr(album), nil)
		suite.Require().NoError(suite.db.Model(p).Updates(map[string]interface{}{
			"artist_id": artistID, "release_id": releaseID,
		}).Error)
		return p.ID
	}
	f.notCredited = play("Ty Segall", "Chrome", &f.tySegall, &f.niceNiceChrome)
	f.relinkCredited = play("They Might Be Giants", "Flood", &f.tmbg, &f.borisFlood)
	f.keptCredited = play("They Might Be Giants", "Flood", &f.tmbg, &f.tmbgFlood)
	f.keptUnique = play("Nice Nice", "Unique LP", nil, &f.niceNiceUnique)
	f.removedUnlinked = play("Somebody Else", "Flood", nil, &f.borisFlood)
	f.relinkUnique = play("NICE NICE", "unique lp", nil, &f.niceNiceChrome)
	f.adminSet = play("Ty Segall", "Chrome", &f.tySegall, &f.niceNiceChrome)
	f.withoutRelease = play("Ty Segall", "Chrome", &f.tySegall, nil)

	suite.Require().NoError(suite.db.Exec(`
INSERT INTO audit_logs (action, entity_type, entity_id, metadata)
VALUES ('link_radio_play', 'radio_play', ?, jsonb_build_object('release_id', ?::int, 'artist_id', NULL))`,
		f.adminSet, f.niceNiceChrome).Error)
	return f
}

// releaseLinkState is every play's release_id, artist_id and match_state.
func (suite *RadioMatchingIntegrationTestSuite) releaseLinkState() map[uint]catalogm.RadioPlay {
	var plays []catalogm.RadioPlay
	suite.Require().NoError(suite.db.Order("id").Find(&plays).Error)
	state := make(map[uint]catalogm.RadioPlay, len(plays))
	for _, p := range plays {
		state[p.ID] = p
	}
	return state
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_DryRunClassifiesAndWritesNothing() {
	f := suite.seedReleaseLinkFixture()
	before := suite.releaseLinkState()

	recorder := testutil.NewSQLRecorder()
	report, err := BackfillRadioReleaseLinks(
		suite.db.Session(&gorm.Session{Logger: recorder}),
		RadioReleaseLinkBackfillOptions{DryRun: true},
	)
	suite.Require().NoError(err)

	suite.Equal(7, report.Scanned, "plays without a release link are not scanned")
	suite.Equal(map[RadioReleaseLinkOutcome]int{
		ReleaseLinkKeptCredited:          1,
		ReleaseLinkKeptUniqueTitle:       1,
		ReleaseLinkKeptAdminSet:          1,
		ReleaseLinkRemovedNotCredited:    1,
		ReleaseLinkRemovedUnlinkedArtist: 1,
		ReleaseLinkRelinkedCredited:      1,
		ReleaseLinkRelinkedUniqueTitle:   1,
	}, report.Counts)
	suite.Zero(report.Cleared)
	suite.Zero(report.Relinked)

	byPlay := make(map[uint]RadioReleaseLinkChange)
	for _, c := range report.Changes {
		byPlay[c.PlayID] = c
	}
	suite.Len(byPlay, 4)
	notCredited := byPlay[f.notCredited]
	suite.Equal(ReleaseLinkRemovedNotCredited, notCredited.Outcome)
	suite.Equal("Chrome", notCredited.CurrentReleaseTitle)
	suite.Equal([]string{"Nice Nice"}, notCredited.CurrentReleaseArtists)
	suite.Nil(notCredited.NewReleaseID)
	suite.Require().NotNil(byPlay[f.relinkCredited].NewReleaseID)
	suite.Equal(f.tmbgFlood, *byPlay[f.relinkCredited].NewReleaseID)
	suite.Equal(ReleaseLinkRemovedUnlinkedArtist, byPlay[f.removedUnlinked].Outcome)
	suite.Require().NotNil(byPlay[f.relinkUnique].NewReleaseID)
	suite.Equal(f.niceNiceUnique, *byPlay[f.relinkUnique].NewReleaseID)
	suite.Equal("Unique LP", byPlay[f.relinkUnique].NewReleaseTitle)

	suite.Equal(before, suite.releaseLinkState(), "a dry run changes no play")
	statements := recorder.Statements()
	suite.Require().NotEmpty(statements)
	for _, stmt := range statements {
		suite.True(strings.HasPrefix(strings.ToUpper(stmt), "SELECT"), "dry run issued a non-SELECT statement: %s", stmt)
	}
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_DryRunTransactionRejectsWrites() {
	f := suite.seedReleaseLinkFixture()

	err := withReleaseLinkBackfillTx(suite.db, true, func(tx *gorm.DB) error {
		return tx.Exec("UPDATE radio_plays SET release_id = NULL").Error
	})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "read-only transaction")
	suite.NotNil(suite.reloadPlay(f.notCredited).ReleaseID)
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_ConfirmClearsAndRelinks() {
	f := suite.seedReleaseLinkFixture()
	before := suite.releaseLinkState()

	recorder := testutil.NewSQLRecorder()
	report, err := BackfillRadioReleaseLinks(
		suite.db.Session(&gorm.Session{Logger: recorder}),
		RadioReleaseLinkBackfillOptions{DryRun: false},
	)
	suite.Require().NoError(err)
	suite.Equal(int64(2), report.Cleared)
	suite.Equal(int64(2), report.Relinked)

	updates := 0
	for _, stmt := range recorder.Statements() {
		if strings.HasPrefix(stmt, "UPDATE radio_plays") {
			updates++
		}
	}
	suite.Equal(2, updates, "the recorder sees the live run's writes")

	after := suite.releaseLinkState()
	wantRelease := map[uint]*uint{
		f.notCredited:     nil,
		f.relinkCredited:  &f.tmbgFlood,
		f.keptCredited:    &f.tmbgFlood,
		f.keptUnique:      &f.niceNiceUnique,
		f.removedUnlinked: nil,
		f.relinkUnique:    &f.niceNiceUnique,
		f.adminSet:        &f.niceNiceChrome,
		f.withoutRelease:  nil,
	}
	suite.Len(after, len(wantRelease), "no play is deleted")
	for playID, want := range wantRelease {
		got := after[playID]
		suite.Equal(want, got.ReleaseID, "release_id of play %d", playID)
		suite.Equal(before[playID].ArtistID, got.ArtistID, "artist_id of play %d", playID)
		suite.Equal(before[playID].MatchState, got.MatchState, "match_state of play %d", playID)
	}
	var releaseCount int64
	suite.Require().NoError(suite.db.Model(&catalogm.Release{}).Count(&releaseCount).Error)
	suite.Equal(int64(4), releaseCount, "no release is deleted")

	rerun, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Empty(rerun.Changes, "a second run finds nothing to change")
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_ApplySkipsRowsChangedSinceEvaluation() {
	f := suite.seedReleaseLinkFixture()
	report := &RadioReleaseLinkReport{Changes: []RadioReleaseLinkChange{
		{PlayID: f.notCredited, Outcome: ReleaseLinkRemovedNotCredited, CurrentReleaseID: f.borisFlood},
		{PlayID: f.relinkCredited, Outcome: ReleaseLinkRelinkedCredited, CurrentReleaseID: f.niceNiceChrome, NewReleaseID: &f.tmbgFlood},
	}}

	suite.Require().NoError(applyRadioReleaseLinkChanges(suite.db, report))
	suite.Zero(report.Cleared)
	suite.Zero(report.Relinked)
	suite.Equal(&f.niceNiceChrome, suite.reloadPlay(f.notCredited).ReleaseID)
	suite.Equal(&f.borisFlood, suite.reloadPlay(f.relinkCredited).ReleaseID)
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_LimitScansLowestIDsFirst() {
	f := suite.seedReleaseLinkFixture()

	report, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{DryRun: true, Limit: 1})
	suite.Require().NoError(err)
	suite.Equal(1, report.Scanned)
	suite.Require().Len(report.Changes, 1)
	suite.Equal(f.notCredited, report.Changes[0].PlayID)
}
