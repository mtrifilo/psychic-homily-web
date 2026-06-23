package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS — release-derived Bandcamp embed backfill (PSY-1188)
// =============================================================================

type BandcampEmbedBackfillIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	artistService *ArtistService
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.artistService = &ArtistService{db: suite.testDB.DB}
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM release_external_links")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestBandcampEmbedBackfillIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(BandcampEmbedBackfillIntegrationTestSuite))
}

// ---- helpers ----------------------------------------------------------------

// makeArtist inserts an artist with the given name + optional embed/source and
// returns it. embed/source nil => column stays NULL.
func (suite *BandcampEmbedBackfillIntegrationTestSuite) makeArtist(name string, embed, source *string) *catalogm.Artist {
	slug := name
	a := &catalogm.Artist{
		Name:                name,
		Slug:                &slug,
		BandcampEmbedURL:    embed,
		BandcampEmbedSource: source,
	}
	suite.Require().NoError(suite.db.Create(a).Error)
	return a
}

// makeReleaseFor inserts a release credited to artistID with the given year and
// external links (platform/url pairs), returning the release ID.
func (suite *BandcampEmbedBackfillIntegrationTestSuite) makeReleaseFor(artistID uint, title string, year *int, links map[string]string) uint {
	slug := title + "-slug"
	r := &catalogm.Release{Title: title, Slug: &slug, ReleaseYear: year}
	suite.Require().NoError(suite.db.Create(r).Error)

	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelease{
		ArtistID:  artistID,
		ReleaseID: r.ID,
		Role:      catalogm.ArtistReleaseRoleMain,
	}).Error)

	for platform, url := range links {
		suite.Require().NoError(suite.db.Create(&catalogm.ReleaseExternalLink{
			ReleaseID: r.ID,
			Platform:  platform,
			URL:       url,
			CreatedAt: time.Now(),
		}).Error)
	}
	return r.ID
}

// ---- DeriveBandcampEmbedForArtist ------------------------------------------

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestDerive_NoReleases_ReturnsNil() {
	a := suite.makeArtist("No Releases", nil, nil)
	got, err := suite.artistService.DeriveBandcampEmbedForArtist(a.ID)
	suite.Require().NoError(err)
	suite.Nil(got)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestDerive_PicksMostRecentAlbum() {
	a := suite.makeArtist("Has Releases", nil, nil)
	suite.makeReleaseFor(a.ID, "Old", intPtr(2015), map[string]string{
		"bandcamp": "https://hasreleases.bandcamp.com/album/old",
	})
	suite.makeReleaseFor(a.ID, "New", intPtr(2023), map[string]string{
		"bandcamp": "https://hasreleases.bandcamp.com/album/new",
	})

	got, err := suite.artistService.DeriveBandcampEmbedForArtist(a.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got)
	suite.Equal("https://hasreleases.bandcamp.com/album/new", *got)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestDerive_NullYearSortsLast() {
	a := suite.makeArtist("Mixed Years", nil, nil)
	suite.makeReleaseFor(a.ID, "Undated", nil, map[string]string{
		"bandcamp": "https://mixedyears.bandcamp.com/album/undated",
	})
	suite.makeReleaseFor(a.ID, "Dated", intPtr(2009), map[string]string{
		"bandcamp": "https://mixedyears.bandcamp.com/album/dated",
	})

	got, err := suite.artistService.DeriveBandcampEmbedForArtist(a.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got)
	// Dated release wins even though its year (2009) is old, because nil sorts last.
	suite.Equal("https://mixedyears.bandcamp.com/album/dated", *got)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestDerive_NoBandcampLink_ReturnsNil() {
	a := suite.makeArtist("Spotify Only", nil, nil)
	suite.makeReleaseFor(a.ID, "Rec", intPtr(2020), map[string]string{
		"spotify": "https://open.spotify.com/album/abc",
	})
	got, err := suite.artistService.DeriveBandcampEmbedForArtist(a.ID)
	suite.Require().NoError(err)
	suite.Nil(got)
}

// ---- Backfill ---------------------------------------------------------------

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestBackfill_DryRun_WritesNothing() {
	a := suite.makeArtist("DryRun Artist", nil, nil)
	suite.makeReleaseFor(a.ID, "Rec", intPtr(2021), map[string]string{
		"bandcamp": "https://dryrunartist.bandcamp.com/album/rec",
	})

	report, err := BackfillArtistBandcampEmbeds(suite.db, BandcampEmbedBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(1, report.Filled)
	suite.Len(report.Fills, 1)
	suite.Equal("https://dryrunartist.bandcamp.com/album/rec", report.Fills[0].EmbedURL)

	// Dry-run must NOT write.
	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Nil(reloaded.BandcampEmbedURL)
	suite.Nil(reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestBackfill_Apply_FillsAndStampsReleaseDerived() {
	a := suite.makeArtist("Apply Artist", nil, nil)
	suite.makeReleaseFor(a.ID, "Rec", intPtr(2021), map[string]string{
		"bandcamp": "https://applyartist.bandcamp.com/album/rec",
	})

	report, err := BackfillArtistBandcampEmbeds(suite.db, BandcampEmbedBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(1, report.Filled)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Require().NotNil(reloaded.BandcampEmbedURL)
	suite.Equal("https://applyartist.bandcamp.com/album/rec", *reloaded.BandcampEmbedURL)
	suite.Require().NotNil(reloaded.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceReleaseDerived, *reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestBackfill_NeverOverwritesNonNullEmbed() {
	existing := "https://applyartist.bandcamp.com/album/curated"
	manual := catalogm.BandcampEmbedSourceManual
	a := suite.makeArtist("Already Set", &existing, &manual)
	// Give it a DIFFERENT release link the backfill would otherwise pick.
	suite.makeReleaseFor(a.ID, "Rec", intPtr(2023), map[string]string{
		"bandcamp": "https://applyartist.bandcamp.com/album/different",
	})

	report, err := BackfillArtistBandcampEmbeds(suite.db, BandcampEmbedBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(0, report.Filled)
	suite.Equal(0, report.ArtistsScanned) // excluded by the IS NULL gate
	suite.Equal(1, report.Left)

	// The manual value + source are untouched.
	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Require().NotNil(reloaded.BandcampEmbedURL)
	suite.Equal(existing, *reloaded.BandcampEmbedURL)
	suite.Require().NotNil(reloaded.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceManual, *reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestBackfill_SkipsNoLinkArtist() {
	a := suite.makeArtist("No Link", nil, nil)
	suite.makeReleaseFor(a.ID, "Rec", intPtr(2020), map[string]string{
		"spotify": "https://open.spotify.com/album/x",
	})

	report, err := BackfillArtistBandcampEmbeds(suite.db, BandcampEmbedBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(1, report.ArtistsScanned)
	suite.Equal(0, report.Filled)
	suite.Equal(1, report.SkippedNoLink)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Nil(reloaded.BandcampEmbedURL)
	suite.Nil(reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestBackfill_Idempotent() {
	a := suite.makeArtist("Idem Artist", nil, nil)
	suite.makeReleaseFor(a.ID, "Rec", intPtr(2021), map[string]string{
		"bandcamp": "https://idemartist.bandcamp.com/album/rec",
	})

	r1, err := BackfillArtistBandcampEmbeds(suite.db, BandcampEmbedBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(1, r1.Filled)

	// Second run: the now-non-null row is excluded, so nothing is filled.
	r2, err := BackfillArtistBandcampEmbeds(suite.db, BandcampEmbedBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(0, r2.ArtistsScanned)
	suite.Equal(0, r2.Filled)
	suite.Equal(1, r2.Left)
}

// ---- Write-path provenance stamping ----------------------------------------

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestCreateArtist_StampsManualWhenEmbedSet() {
	resp, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{
		Name:             "Created With Embed",
		BandcampEmbedURL: stringPtr("https://createdwithembed.bandcamp.com/album/x"),
	})
	suite.Require().NoError(err)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, resp.ID).Error)
	suite.Require().NotNil(reloaded.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceManual, *reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestCreateArtist_NoEmbed_LeavesSourceNull() {
	resp, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{
		Name: "Created No Embed",
	})
	suite.Require().NoError(err)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, resp.ID).Error)
	suite.Nil(reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestUpdateArtist_StampsManualWhenEmbedSet() {
	a := suite.makeArtist("Update Target", nil, nil)
	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{
		BandcampEmbedURL: stringPtr("https://updatetarget.bandcamp.com/album/x"),
	})
	suite.Require().NoError(err)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Require().NotNil(reloaded.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceManual, *reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestUpdateArtist_ClearingEmbedClearsSource() {
	// Start from a release-derived embed, then clear it via an admin/AI update.
	embed := "https://updatetarget.bandcamp.com/album/x"
	derived := catalogm.BandcampEmbedSourceReleaseDerived
	a := suite.makeArtist("Clear Target", &embed, &derived)

	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{
		BandcampEmbedURL: stringPtr(""), // empty clears the embed
	})
	suite.Require().NoError(err)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Nil(reloaded.BandcampEmbedURL)
	suite.Nil(reloaded.BandcampEmbedSource)
}

func (suite *BandcampEmbedBackfillIntegrationTestSuite) TestUpdateArtist_UnrelatedFieldLeavesSourceUntouched() {
	// A release-derived embed must survive an update that doesn't touch the embed.
	embed := "https://updatetarget.bandcamp.com/album/x"
	derived := catalogm.BandcampEmbedSourceReleaseDerived
	a := suite.makeArtist("Keep Source", &embed, &derived)

	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{
		City: stringPtr("Phoenix"),
	})
	suite.Require().NoError(err)

	var reloaded catalogm.Artist
	suite.Require().NoError(suite.db.First(&reloaded, a.ID).Error)
	suite.Require().NotNil(reloaded.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceReleaseDerived, *reloaded.BandcampEmbedSource)
	suite.Require().NotNil(reloaded.BandcampEmbedURL)
	suite.Equal(embed, *reloaded.BandcampEmbedURL)
}
