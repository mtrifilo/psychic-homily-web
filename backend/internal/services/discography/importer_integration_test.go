package discography

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/testutil"
)

const (
	artistMBID = "11111111-1111-1111-1111-111111111111"
	rgAlbum    = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	rgEP       = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

type fakeBrowser struct {
	byMBID map[string][]pipeline.MBReleaseGroupResult
	err    error
}

func (f fakeBrowser) BrowseArtistReleaseGroups(_ context.Context, mbid string, _ map[string]bool) ([]pipeline.MBReleaseGroupResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byMBID[mbid], nil
}

type fakeCoverArt struct {
	byMBID map[string]*catalog.CoverArtResult
}

func (f fakeCoverArt) FrontCover(_ context.Context, rgMBID string) (*catalog.CoverArtResult, error) {
	return f.byMBID[rgMBID], nil
}

// DiscographyImporterTestSuite exercises the importer against real Postgres (so the
// PSY-1281 dedup keystone + artist_releases link are validated), with faked MB browse
// + Cover Art Archive calls (no network).
type DiscographyImporterTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *DiscographyImporterTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}
func (s *DiscographyImporterTestSuite) TearDownSuite() { s.testDB.Cleanup() }
func (s *DiscographyImporterTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{"artist_releases", "release_external_links", "image_enrich_queue", "releases", "artists"} {
		_, _ = sqlDB.Exec("DELETE FROM " + t)
	}
}
func TestDiscographyImporterTestSuite(t *testing.T) { suite.Run(t, new(DiscographyImporterTestSuite)) }

func (s *DiscographyImporterTestSuite) seedArtist(name, mbid string) uint {
	a := &catalogm.Artist{Name: name}
	if mbid != "" {
		a.MusicBrainzArtistID = &mbid
	}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *DiscographyImporterTestSuite) sleepDiscographyBrowser() fakeBrowser {
	return fakeBrowser{byMBID: map[string][]pipeline.MBReleaseGroupResult{
		artistMBID: {
			{ID: rgAlbum, Title: "Dopesmoker", PrimaryType: "Album", FirstReleaseDate: "2003-02-04"},
			{ID: rgEP, Title: "The Sciences", PrimaryType: "EP", FirstReleaseDate: "2018"},
		},
	}}
}

func (s *DiscographyImporterTestSuite) coverForAlbum() fakeCoverArt {
	return fakeCoverArt{byMBID: map[string]*catalog.CoverArtResult{
		rgAlbum: {ImageURL: "https://coverartarchive.org/release-group/" + rgAlbum + "/front",
			SourceURL: "https://musicbrainz.org/release-group/" + rgAlbum},
	}}
}

func (s *DiscographyImporterTestSuite) releaseByTitle(title string) catalogm.Release {
	var r catalogm.Release
	s.Require().NoError(s.db.Where("title = ?", title).First(&r).Error)
	return r
}

// Live run: creates one release per release-group, maps type + year + RG-MBID, links
// the artist, and sets cover art (only where the CAA had one).
func (s *DiscographyImporterTestSuite) TestCreatesMapsAndSetsCoverArt() {
	artistID := s.seedArtist("Sleep", artistMBID)

	rep, err := BackfillArtistDiscography(s.db, s.sleepDiscographyBrowser(), s.coverForAlbum(), Options{})
	s.Require().NoError(err)
	s.Equal(1, rep.ArtistsScanned)
	s.Equal(2, rep.ReleaseGroupsSeen)
	s.Equal(2, rep.Created)
	s.Equal(1, rep.CoverArtSet) // only the album had a CAA front cover
	s.Empty(rep.Errors)

	album := s.releaseByTitle("Dopesmoker")
	s.Equal(catalogm.ReleaseTypeLP, album.ReleaseType)
	s.Require().NotNil(album.ReleaseYear)
	s.Equal(2003, *album.ReleaseYear)
	s.Require().NotNil(album.MusicBrainzReleaseGroupID)
	s.Equal(rgAlbum, *album.MusicBrainzReleaseGroupID)
	s.Require().NotNil(album.CoverArtURL)
	s.Contains(*album.CoverArtURL, rgAlbum)
	s.Require().NotNil(album.CoverArtSource)
	s.Equal("cover_art_archive", *album.CoverArtSource)

	ep := s.releaseByTitle("The Sciences")
	s.Equal(catalogm.ReleaseTypeEP, ep.ReleaseType)
	s.Require().NotNil(ep.ReleaseYear)
	s.Equal(2018, *ep.ReleaseYear)
	s.Nil(ep.CoverArtURL) // no CAA cover → left empty (outbox is the backstop)

	var links int64
	s.Require().NoError(s.db.Table("artist_releases").Where("artist_id = ?", artistID).Count(&links).Error)
	s.Equal(int64(2), links)
}

// Re-running is idempotent: every release-group dedups, no duplicate rows.
func (s *DiscographyImporterTestSuite) TestIdempotentRerun() {
	s.seedArtist("Sleep", artistMBID)
	browser, cover := s.sleepDiscographyBrowser(), s.coverForAlbum()

	_, err := BackfillArtistDiscography(s.db, browser, cover, Options{})
	s.Require().NoError(err)

	rep2, err := BackfillArtistDiscography(s.db, browser, cover, Options{})
	s.Require().NoError(err)
	s.Equal(0, rep2.Created)
	s.Equal(2, rep2.Deduped)

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Release{}).Count(&n).Error)
	s.Equal(int64(2), n, "re-run must not create duplicates")
}

// Dry-run writes nothing but reports the plan.
func (s *DiscographyImporterTestSuite) TestDryRunWritesNothing() {
	s.seedArtist("Sleep", artistMBID)

	rep, err := BackfillArtistDiscography(s.db, s.sleepDiscographyBrowser(), s.coverForAlbum(), Options{DryRun: true})
	s.Require().NoError(err)
	s.Equal(0, rep.Created)
	s.Len(rep.Plans, 2)
	s.Equal("create", rep.Plans[0].Action)

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Release{}).Count(&n).Error)
	s.Equal(int64(0), n, "dry-run must write no releases")
}

// H1 (PSY-1252 policy): a release-group carrying a SECONDARY type (Live / Compilation
// / …) is skipped even though its primary type is Album/EP — the studio-core gate.
func (s *DiscographyImporterTestSuite) TestSkipsSecondaryTypes() {
	s.seedArtist("Sleep", artistMBID)
	browser := fakeBrowser{byMBID: map[string][]pipeline.MBReleaseGroupResult{
		artistMBID: {
			{ID: rgAlbum, Title: "Studio LP", PrimaryType: "Album", FirstReleaseDate: "2001"},
			{ID: rgEP, Title: "Live At The Fillmore", PrimaryType: "Album", SecondaryTypes: []string{"Live"}, FirstReleaseDate: "2005"},
		},
	}}
	rep, err := BackfillArtistDiscography(s.db, browser, fakeCoverArt{}, Options{})
	s.Require().NoError(err)
	s.Equal(1, rep.ReleaseGroupsSeen, "the Live secondary-type release-group is skipped")
	s.Equal(1, rep.Created)

	var liveCount int64
	s.Require().NoError(s.db.Model(&catalogm.Release{}).Where("title = ?", "Live At The Fillmore").Count(&liveCount).Error)
	s.Equal(int64(0), liveCount, "a Live album must not be imported")
}

// A browse error is recorded and the run continues to the next artist (not fatal).
func (s *DiscographyImporterTestSuite) TestBrowseErrorRecordedAndContinues() {
	s.seedArtist("Sleep", artistMBID)
	s.seedArtist("Other", "22222222-2222-2222-2222-222222222222")

	rep, err := BackfillArtistDiscography(s.db, fakeBrowser{err: errors.New("mb down")}, fakeCoverArt{}, Options{})
	s.Require().NoError(err) // a per-artist browse error does not abort the whole run
	s.Equal(2, rep.ArtistsScanned)
	s.Len(rep.Errors, 2)
	s.Equal(0, rep.Created)
}

// A malformed release-group MBID is skipped (trust boundary) and not counted.
func (s *DiscographyImporterTestSuite) TestInvalidRGMBIDSkipped() {
	s.seedArtist("Sleep", artistMBID)
	browser := fakeBrowser{byMBID: map[string][]pipeline.MBReleaseGroupResult{
		artistMBID: {
			{ID: "not-a-uuid", Title: "Bad Id", PrimaryType: "Album", FirstReleaseDate: "2001"},
			{ID: rgAlbum, Title: "Good", PrimaryType: "Album", FirstReleaseDate: "2002"},
		},
	}}
	rep, err := BackfillArtistDiscography(s.db, browser, fakeCoverArt{}, Options{})
	s.Require().NoError(err)
	s.Equal(1, rep.ReleaseGroupsSeen, "the malformed-MBID release-group is skipped before counting")
	s.Equal(1, rep.Created)
}

// An artist without a stored MBID is not scanned.
func (s *DiscographyImporterTestSuite) TestArtistWithoutMBIDExcluded() {
	s.seedArtist("No MBID Band", "")

	rep, err := BackfillArtistDiscography(s.db, s.sleepDiscographyBrowser(), s.coverForAlbum(), Options{})
	s.Require().NoError(err)
	s.Equal(0, rep.ArtistsScanned)

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Release{}).Count(&n).Error)
	s.Equal(int64(0), n)
}
