package catalog

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// Provider fakes (in-memory; the strict gate + DB path are what we exercise)
// =============================================================================

type fakeMBSearcher struct {
	byTitle map[string][]MBReleaseGroupCandidate
	err     error
	calls   int
}

func (f *fakeMBSearcher) SearchReleaseGroups(_ context.Context, _, title string, _ int) ([]MBReleaseGroupCandidate, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byTitle[title], nil
}

type fakeCAA struct {
	byMBID map[string]*CoverArtResult
	err    error
	calls  int
}

func (f *fakeCAA) FrontCover(_ context.Context, mbid string) (*CoverArtResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byMBID[mbid], nil
}

type fakeDiscogs struct {
	byTitle map[string][]DiscogsRelease
	err     error
	calls   int
}

func (f *fakeDiscogs) SearchReleaseCovers(_ context.Context, _, title string, _ int) ([]DiscogsRelease, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byTitle[title], nil
}

// =============================================================================
// Suite
// =============================================================================

type CoverArtEnrichIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *CoverArtEnrichIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *CoverArtEnrichIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CoverArtEnrichIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestCoverArtEnrichIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CoverArtEnrichIntegrationTestSuite))
}

// seedDopesmoker creates artist "Sleep" + a cover-less "Dopesmoker" (2003) and a
// covered "Holy Mountain" (2018, not scanned). Returns the cover-less release id.
func (s *CoverArtEnrichIntegrationTestSuite) seedDopesmoker() uint {
	sleep := &catalogm.Artist{Name: "Sleep"}
	s.Require().NoError(s.db.Create(sleep).Error)

	dopesmoker := &catalogm.Release{Title: "Dopesmoker", ReleaseYear: intPtr(2003)}
	s.Require().NoError(s.db.Create(dopesmoker).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
		ArtistID: sleep.ID, ReleaseID: dopesmoker.ID, Role: catalogm.ArtistReleaseRoleMain,
	}).Error)

	holy := &catalogm.Release{
		Title:       "Holy Mountain",
		ReleaseYear: intPtr(2018),
		CoverArtURL: stringPtr("https://existing.test/cover.jpg"),
	}
	s.Require().NoError(s.db.Create(holy).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
		ArtistID: sleep.ID, ReleaseID: holy.ID, Role: catalogm.ArtistReleaseRoleMain,
	}).Error)

	return dopesmoker.ID
}

func mbWithDopesmoker(mbid string) *fakeMBSearcher {
	return &fakeMBSearcher{byTitle: map[string][]MBReleaseGroupCandidate{
		"Dopesmoker": {{MBID: mbid, Title: "Dopesmoker", ArtistNames: []string{"Sleep"}, FirstReleaseDate: "2003"}},
	}}
}

func caaWithCover(mbid, imageURL, sourceURL string) *fakeCAA {
	return &fakeCAA{byMBID: map[string]*CoverArtResult{mbid: {ImageURL: imageURL, SourceURL: sourceURL}}}
}

func (s *CoverArtEnrichIntegrationTestSuite) TestDryRun_SearchesButDoesNotWrite() {
	relID := s.seedDopesmoker()
	mb := mbWithDopesmoker("rg-1")
	caa := caaWithCover("rg-1", "https://coverartarchive.org/release-group/rg-1/front", "https://musicbrainz.org/release-group/rg-1")

	report, err := BackfillCoverArt(context.Background(), s.db, mb, caa, nil, CoverArtEnrichOptions{DryRun: true})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesScanned, "only the cover-less release is scanned")
	s.Equal(1, report.ReleasesMatchedCAA)
	s.Equal(0, report.ReleasesUpdated)

	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, relID).Error)
	s.Nil(rel.CoverArtURL)
	s.Nil(rel.CoverArtSource)
}

func (s *CoverArtEnrichIntegrationTestSuite) TestLiveRun_CAA_WritesReferenceAndIsIdempotent() {
	relID := s.seedDopesmoker()
	mb := mbWithDopesmoker("rg-1")
	caa := caaWithCover("rg-1", "https://coverartarchive.org/release-group/rg-1/front", "https://musicbrainz.org/release-group/rg-1")

	report, err := BackfillCoverArt(context.Background(), s.db, mb, caa, nil, CoverArtEnrichOptions{})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesMatchedCAA)
	s.Equal(1, report.ReleasesUpdated)

	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, relID).Error)
	s.Require().NotNil(rel.CoverArtURL)
	s.Equal("https://coverartarchive.org/release-group/rg-1/front", *rel.CoverArtURL)
	s.Require().NotNil(rel.CoverArtSource)
	s.Equal("cover_art_archive", *rel.CoverArtSource)
	s.Require().NotNil(rel.CoverArtSourceURL)
	s.Equal("https://musicbrainz.org/release-group/rg-1", *rel.CoverArtSourceURL)

	// Idempotent: the release now has a cover, so a re-run scans nothing.
	report2, err := BackfillCoverArt(context.Background(), s.db, mbWithDopesmoker("rg-1"), caa, nil, CoverArtEnrichOptions{})
	s.Require().NoError(err)
	s.Equal(0, report2.ReleasesScanned)
	s.Equal(0, report2.ReleasesUpdated)
}

func (s *CoverArtEnrichIntegrationTestSuite) TestDiscogsFallback_WhenCAAHasNoCover() {
	relID := s.seedDopesmoker()
	mb := mbWithDopesmoker("rg-1")
	caa := &fakeCAA{byMBID: map[string]*CoverArtResult{}} // rg-1 → nil (matched, but no art)
	discogs := &fakeDiscogs{byTitle: map[string][]DiscogsRelease{
		"Dopesmoker": {{ID: 111, Title: "Sleep - Dopesmoker", Year: 2003,
			CoverImage: "https://i.discogs.com/a.jpg", SourceURL: "https://www.discogs.com/release/111"}},
	}}

	report, err := BackfillCoverArt(context.Background(), s.db, mb, caa, discogs, CoverArtEnrichOptions{})
	s.Require().NoError(err)
	s.Equal(0, report.ReleasesMatchedCAA)
	s.Equal(1, report.ReleasesMatchedDiscogs)
	s.Equal(1, report.ReleasesUpdated)
	s.Equal(1, caa.calls, "CAA is tried before Discogs")

	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, relID).Error)
	s.Require().NotNil(rel.CoverArtURL)
	s.Equal("https://i.discogs.com/a.jpg", *rel.CoverArtURL)
	s.Equal("discogs", *rel.CoverArtSource)
	s.Equal("https://www.discogs.com/release/111", *rel.CoverArtSourceURL)
}

func (s *CoverArtEnrichIntegrationTestSuite) TestAmbiguousMusicBrainz_SkipsWithoutCallingCAA() {
	s.seedDopesmoker()
	mb := &fakeMBSearcher{byTitle: map[string][]MBReleaseGroupCandidate{
		"Dopesmoker": {
			{MBID: "rg-1", Title: "Dopesmoker", ArtistNames: []string{"Sleep"}, FirstReleaseDate: "2003"},
			{MBID: "rg-2", Title: "Dopesmoker", ArtistNames: []string{"Sleep"}, FirstReleaseDate: "2003"},
		},
	}}
	caa := &fakeCAA{byMBID: map[string]*CoverArtResult{}}

	report, err := BackfillCoverArt(context.Background(), s.db, mb, caa, nil, CoverArtEnrichOptions{})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesScanned)
	s.Equal(0, report.ReleasesUpdated)
	s.Equal(1, report.ReleasesSkipped)
	s.Equal(0, caa.calls, "an ambiguous MB match never resolves to a CAA lookup")
}

func (s *CoverArtEnrichIntegrationTestSuite) TestValidationSkip_NonHTTPSImage() {
	relID := s.seedDopesmoker()
	mb := mbWithDopesmoker("rg-1")
	// A provider returning a non-https image is rejected by validate-on-write.
	caa := caaWithCover("rg-1", "http://insecure.test/front.jpg", "https://musicbrainz.org/release-group/rg-1")

	report, err := BackfillCoverArt(context.Background(), s.db, mb, caa, nil, CoverArtEnrichOptions{})
	s.Require().NoError(err)
	s.Equal(0, report.ReleasesMatchedCAA)
	s.Equal(1, report.ReleasesSkipped)
	s.Equal(0, report.ReleasesUpdated)

	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, relID).Error)
	s.Nil(rel.CoverArtURL)
}

func (s *CoverArtEnrichIntegrationTestSuite) TestProviderError_CountsAndContinues() {
	s.seedDopesmoker()
	mb := &fakeMBSearcher{err: fmt.Errorf("musicbrainz down")}

	report, err := BackfillCoverArt(context.Background(), s.db, mb, &fakeCAA{}, nil, CoverArtEnrichOptions{})
	s.Require().NoError(err, "a per-release provider error is not a run-level failure")
	s.Equal(1, report.ReleasesScanned)
	s.Equal(1, report.ReleaseErrors)
	s.Equal(0, report.ReleasesUpdated)
}

func (s *CoverArtEnrichIntegrationTestSuite) TestLimit_CapsReleases() {
	s.seedDopesmoker()
	extra := &catalogm.Release{Title: "Sleep's Holy Mountain", ReleaseYear: intPtr(1992)}
	s.Require().NoError(s.db.Create(extra).Error)

	report, err := BackfillCoverArt(context.Background(), s.db, &fakeMBSearcher{}, &fakeCAA{}, nil, CoverArtEnrichOptions{DryRun: true, Limit: 1})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesScanned)
}

func (s *CoverArtEnrichIntegrationTestSuite) TestCAAOnly_NilDiscogsIsSafe() {
	relID := s.seedDopesmoker()
	mb := mbWithDopesmoker("rg-1")
	caa := caaWithCover("rg-1", "https://coverartarchive.org/release-group/rg-1/front", "https://musicbrainz.org/release-group/rg-1")

	// Passing an untyped nil for discogs must not panic (the cmd does this when no
	// token is configured) and should still enrich via CAA.
	report, err := BackfillCoverArt(context.Background(), s.db, mb, caa, nil, CoverArtEnrichOptions{})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesUpdated)

	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, relID).Error)
	s.Require().NotNil(rel.CoverArtSource)
	s.Equal("cover_art_archive", *rel.CoverArtSource)
}
