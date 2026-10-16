package catalog

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// releaseLinkFixture is one play per backfill outcome, two plays carrying
// link_radio_play audit rows, and a play without a release link.
type releaseLinkFixture struct {
	niceNiceChrome, borisFlood, tmbgFlood, niceNiceUnique, tySegallRemixes, tySegallRemixesDeluxe uint
	tySegall, tmbg                                                                                uint

	notCredited, severalCredited, relinkCredited, keptCredited uint
	keptUnique, removedUnlinked, relinkUnique                  uint
	adminSet, adminSetOtherRelease, withoutRelease             uint
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
	niceNiceRemixes := suite.createCreditedRelease("Remixes", niceNice.ID).ID
	f.tySegallRemixes = suite.createCreditedRelease("Remixes", tySegall.ID).ID
	f.tySegallRemixesDeluxe = suite.createCreditedRelease("REMIXES", tySegall.ID).ID

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
	f.severalCredited = play("Ty Segall", "Remixes", &f.tySegall, &niceNiceRemixes)
	f.relinkCredited = play("They Might Be Giants", "Flood", &f.tmbg, &f.borisFlood)
	f.keptCredited = play("They Might Be Giants", "Flood", &f.tmbg, &f.tmbgFlood)
	f.keptUnique = play("Nice Nice", "Unique LP", nil, &f.niceNiceUnique)
	f.removedUnlinked = play("Somebody Else", "Flood", nil, &f.borisFlood)
	f.relinkUnique = play("NICE NICE", "unique lp", nil, &f.niceNiceChrome)
	f.adminSet = play("Ty Segall", "Chrome", &f.tySegall, &f.niceNiceChrome)
	f.adminSetOtherRelease = play("Ty Segall", "Chrome", &f.tySegall, &f.niceNiceChrome)
	f.withoutRelease = play("Ty Segall", "Chrome", &f.tySegall, nil)

	audit := func(playID, releaseID uint) {
		suite.Require().NoError(suite.db.Exec(`
INSERT INTO audit_logs (action, entity_type, entity_id, metadata)
VALUES (?, ?, ?, jsonb_build_object(?::text, ?::int, ?::text, NULL))`,
			contracts.AuditActionLinkRadioPlay, contracts.AuditEntityRadioPlay, playID,
			contracts.AuditMetaReleaseID, releaseID, contracts.AuditMetaArtistID).Error)
	}
	audit(f.adminSet, f.niceNiceChrome)
	audit(f.adminSetOtherRelease, f.borisFlood)
	return f
}

// releaseLinkState is every play row, keyed by id.
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
	var readOnly, isolation string
	report, err := BackfillRadioReleaseLinks(
		suite.db.Session(&gorm.Session{Logger: recorder}),
		RadioReleaseLinkBackfillOptions{DryRun: true, txProbe: func(tx *gorm.DB) error {
			return tx.Raw("SELECT current_setting('transaction_read_only'), current_setting('transaction_isolation')").Row().Scan(&readOnly, &isolation)
		}},
	)
	suite.Require().NoError(err)
	suite.Equal("on", readOnly, "the dry run's transaction is READ ONLY")
	suite.Equal("repeatable read", isolation)

	suite.Equal(9, report.Scanned, "plays without a release link are not scanned")
	suite.Equal(map[RadioReleaseLinkOutcome]int{
		RadioReleaseLinkKeptCredited:           1,
		RadioReleaseLinkKeptUniqueTitle:        1,
		RadioReleaseLinkRemovedNotCredited:     3,
		RadioReleaseLinkRemovedSeveralCredited: 1,
		RadioReleaseLinkRemovedUnlinkedArtist:  1,
		RadioReleaseLinkRelinkedCredited:       1,
		RadioReleaseLinkRelinkedUniqueTitle:    1,
	}, report.Counts)
	suite.Zero(report.Cleared)
	suite.Zero(report.Relinked)

	byPlay := make(map[uint]RadioReleaseLinkChange)
	for _, c := range report.Changes {
		byPlay[c.PlayID] = c
	}
	suite.Len(byPlay, 7)
	notCredited := byPlay[f.notCredited]
	suite.Equal(RadioReleaseLinkRemovedNotCredited, notCredited.Outcome)
	suite.Equal("Ty Segall", notCredited.LinkedArtistName)
	suite.Equal("Chrome", notCredited.CurrentReleaseTitle)
	suite.Equal([]string{"Nice Nice"}, notCredited.CurrentReleaseArtists)
	suite.Nil(notCredited.NewReleaseID)
	suite.False(notCredited.AdminSet)
	suite.Equal(RadioReleaseLinkRemovedSeveralCredited, byPlay[f.severalCredited].Outcome)
	suite.Require().NotNil(byPlay[f.relinkCredited].NewReleaseID)
	suite.Equal(f.tmbgFlood, *byPlay[f.relinkCredited].NewReleaseID)
	suite.Equal(RadioReleaseLinkRemovedUnlinkedArtist, byPlay[f.removedUnlinked].Outcome)
	suite.Empty(byPlay[f.removedUnlinked].LinkedArtistName)
	suite.Require().NotNil(byPlay[f.relinkUnique].NewReleaseID)
	suite.Equal(f.niceNiceUnique, *byPlay[f.relinkUnique].NewReleaseID)
	suite.Equal("Unique LP", byPlay[f.relinkUnique].NewReleaseTitle)
	suite.True(byPlay[f.adminSet].AdminSet, "an audit row naming the current release flags the change")
	suite.Equal(RadioReleaseLinkRemovedNotCredited, byPlay[f.adminSet].Outcome, "a link an audit row names but not marked 'admin' is judged like any other")
	suite.False(byPlay[f.adminSetOtherRelease].AdminSet, "an audit row naming another release does not flag it")

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

	var seenBeforeApply *RadioReleaseLinkReport
	recorder := testutil.NewSQLRecorder()
	report, err := BackfillRadioReleaseLinks(
		suite.db.Session(&gorm.Session{Logger: recorder}),
		RadioReleaseLinkBackfillOptions{BeforeApply: func(r *RadioReleaseLinkReport) error {
			seenBeforeApply = r
			return nil
		}},
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(seenBeforeApply)
	suite.Len(seenBeforeApply.Changes, 7)
	suite.Equal(int64(5), report.Cleared)
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
		f.notCredited:          nil,
		f.severalCredited:      nil,
		f.relinkCredited:       &f.tmbgFlood,
		f.keptCredited:         &f.tmbgFlood,
		f.keptUnique:           &f.niceNiceUnique,
		f.removedUnlinked:      nil,
		f.relinkUnique:         &f.niceNiceUnique,
		f.adminSet:             nil,
		f.adminSetOtherRelease: nil,
		f.withoutRelease:       nil,
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
	suite.Equal(int64(7), releaseCount, "no release is deleted")

	rerun, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Empty(rerun.Changes, "a second run finds nothing to change")
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_LiveRunIsRepeatableReadWrite() {
	suite.seedReleaseLinkFixture()
	var readOnly, isolation string
	_, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{txProbe: func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('transaction_read_only'), current_setting('transaction_isolation')").Row().Scan(&readOnly, &isolation)
	}})
	suite.Require().NoError(err)
	suite.Equal("off", readOnly)
	suite.Equal("repeatable read", isolation)
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_ConcurrentUpdateAbortsTheRun() {
	f := suite.seedReleaseLinkFixture()

	_, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{
		BeforeApply: func(*RadioReleaseLinkReport) error {
			// Another session changes a play this run is about to clear.
			return suite.db.Exec("UPDATE radio_plays SET release_id = ? WHERE id = ?", f.borisFlood, f.notCredited).Error
		},
	})
	suite.Require().Error(err)
	suite.True(shared.IsSerializationFailure(err), "got %v", err)
	suite.Equal(&f.borisFlood, suite.reloadPlay(f.notCredited).ReleaseID, "the other session's change stands")
	suite.Equal(&f.borisFlood, suite.reloadPlay(f.relinkCredited).ReleaseID, "nothing from the aborted run is applied")
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_BeforeApplyErrorWritesNothing() {
	suite.seedReleaseLinkFixture()
	before := suite.releaseLinkState()

	refused := errors.New("change list not written")
	_, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{
		BeforeApply: func(*RadioReleaseLinkReport) error { return refused },
	})
	suite.Require().ErrorIs(err, refused)
	suite.Equal(before, suite.releaseLinkState())
}

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_ApplyMatchesOnlyTheEvaluatedRelease() {
	f := suite.seedReleaseLinkFixture()
	report := &RadioReleaseLinkReport{Changes: []RadioReleaseLinkChange{
		{PlayID: f.notCredited, Outcome: RadioReleaseLinkRemovedNotCredited, CurrentReleaseID: f.borisFlood},
		{PlayID: f.relinkCredited, Outcome: RadioReleaseLinkRelinkedCredited, CurrentReleaseID: f.niceNiceChrome, NewReleaseID: &f.tmbgFlood},
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

func (suite *RadioMatchingIntegrationTestSuite) TestBackfillReleaseLinks_EdgeRows() {
	tmbg := suite.createArtist("They Might Be Giants")
	lincoln := suite.createCreditedRelease("Lincoln", tmbg.ID)
	boris := suite.createArtist("Boris")
	borisFlood := suite.createCreditedRelease("Flood", boris.ID)

	episodeID := suite.createStationShowEpisode()
	position := 0
	play := func(artistName string, album *string, artistID *uint, releaseID uint) uint {
		position++
		p := suite.createPlay(episodeID, position, artistName, album, nil)
		suite.Require().NoError(suite.db.Model(p).Updates(map[string]interface{}{
			"artist_id": artistID, "release_id": releaseID,
		}).Error)
		return p.ID
	}
	otherTitle := play("They Might Be Giants", strPtr("Flood"), &tmbg.ID, lincoln.ID)
	emptyTitle := play("They Might Be Giants", strPtr("  "), &tmbg.ID, borisFlood.ID)
	noTitle := play("Somebody", nil, nil, borisFlood.ID)
	suite.Require().NoError(suite.db.Exec(`
INSERT INTO audit_logs (action, entity_type, entity_id, metadata)
VALUES (?, ?, ?, jsonb_build_object(?::text, NULL))`,
		contracts.AuditActionLinkRadioPlay, contracts.AuditEntityRadioPlay, emptyTitle, contracts.AuditMetaReleaseID).Error)

	report, err := BackfillRadioReleaseLinks(suite.db, RadioReleaseLinkBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	byPlay := make(map[uint]RadioReleaseLinkChange)
	for _, c := range report.Changes {
		byPlay[c.PlayID] = c
	}
	suite.NotContains(byPlay, otherTitle, "a release credited to the play's artist is kept whatever its title")
	suite.Equal(1, report.Counts[RadioReleaseLinkKeptCredited])
	suite.Equal(RadioReleaseLinkRemovedNotCredited, byPlay[emptyTitle].Outcome, "a title that normalizes to empty finds no credited release")
	suite.False(byPlay[emptyTitle].AdminSet, "an audit row with a null release_id flags nothing")
	suite.Equal(RadioReleaseLinkRemovedUnlinkedArtist, byPlay[noTitle].Outcome)
	suite.Empty(byPlay[noTitle].AlbumTitle)
}
