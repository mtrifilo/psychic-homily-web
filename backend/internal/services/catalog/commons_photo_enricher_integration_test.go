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
// Provider fakes
// =============================================================================

type fakeMBArtist struct {
	candidatesByName map[string][]MBArtistCandidate
	urlsByMBID       map[string][]string
	searchErr        error
	urlsErr          error
}

func (f *fakeMBArtist) SearchArtistCandidates(_ context.Context, name string) ([]MBArtistCandidate, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.candidatesByName[name], nil
}

func (f *fakeMBArtist) LookupArtistURLs(_ context.Context, mbid string) ([]string, error) {
	if f.urlsErr != nil {
		return nil, f.urlsErr
	}
	return f.urlsByMBID[mbid], nil
}

type fakeWikidata struct {
	filenameByQID map[string]string
	err           error
}

func (f *fakeWikidata) ImageFilename(_ context.Context, qid string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.filenameByQID[qid], nil
}

type fakeCommons struct {
	imageByFilename map[string]*CommonsImage
	err             error
}

func (f *fakeCommons) ImageInfo(_ context.Context, filename string) (*CommonsImage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.imageByFilename[filename], nil
}

// goodImage is a valid, freely-licensed Commons photo (hosts pass validation).
func goodImage() *CommonsImage {
	return &CommonsImage{
		ImageURL:       "https://upload.wikimedia.org/wikipedia/commons/thumb/a/ab/X.jpg/600px-X.jpg",
		DescriptionURL: "https://commons.wikimedia.org/wiki/File:X.jpg",
		License:        "CC BY 2.0",
		Author:         "A Photographer",
	}
}

// =============================================================================
// Suite
// =============================================================================

type CommonsPhotoEnrichIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestCommonsPhotoEnrichIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CommonsPhotoEnrichIntegrationTestSuite))
}

// seedArtist creates a photo-less artist, optionally with a Bandcamp link.
func (s *CommonsPhotoEnrichIntegrationTestSuite) seedArtist(name, bandcamp string) uint {
	a := &catalogm.Artist{Name: name}
	if bandcamp != "" {
		a.Social.Bandcamp = stringPtr(bandcamp)
	}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) run(mb *fakeMBArtist, wd *fakeWikidata, cm *fakeCommons, opts CommonsEnrichOptions) *CommonsEnrichReport {
	report, err := BackfillCommonsPhotos(context.Background(), s.db, mb, wd, cm, opts)
	s.Require().NoError(err)
	return report
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestUniqueMatch_WritesAndIsIdempotent() {
	id := s.seedArtist("Phoebe Bridgers", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Phoebe Bridgers": {{MBID: "m1", Name: "Phoebe Bridgers"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q1"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "X.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"X.jpg": goodImage()}}

	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsMatched)
	s.Equal(1, report.ArtistsUpdated)

	var ar catalogm.Artist
	s.Require().NoError(s.db.First(&ar, id).Error)
	s.Require().NotNil(ar.ImageURL)
	s.Equal(goodImage().ImageURL, *ar.ImageURL)
	s.Equal("commons", *ar.ImageSource)
	s.Equal("https://commons.wikimedia.org/wiki/File:X.jpg", *ar.ImageSourceURL)
	s.Equal("CC BY 2.0", *ar.ImageLicense)
	s.Require().NotNil(ar.ImageAuthor)
	s.Equal("A Photographer", *ar.ImageAuthor)

	// Idempotent: the artist now has a photo, so a re-run scans nothing.
	report2 := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(0, report2.ArtistsScanned)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestAmbiguous_NoAnchor_Skips() {
	s.seedArtist("Crush", "") // no links → can't disambiguate
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Crush": {{MBID: "m1", Name: "Crush"}, {MBID: "m2", Name: "Crush"}}},
		urlsByMBID: map[string][]string{
			"m1": {"https://www.wikidata.org/wiki/Q1"},
			"m2": {"https://www.wikidata.org/wiki/Q2"},
		},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "A.jpg", "Q2": "B.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"A.jpg": goodImage(), "B.jpg": goodImage()}}

	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsScanned)
	s.Equal(0, report.ArtistsUpdated, "two same-name artists with no shared-link anchor must be skipped")
	s.Equal(1, report.ArtistsSkipped)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestAmbiguous_Anchored_Resolves() {
	id := s.seedArtist("Crush", "https://crush.bandcamp.com")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Crush": {{MBID: "m1", Name: "Crush"}, {MBID: "m2", Name: "Crush"}}},
		urlsByMBID: map[string][]string{
			"m1": {"https://other.bandcamp.com", "https://www.wikidata.org/wiki/Q1"},
			"m2": {"https://crush.bandcamp.com", "https://www.wikidata.org/wiki/Q2"}, // shares our link
		},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "A.jpg", "Q2": "B.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"B.jpg": goodImage()}}

	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsUpdated, "the shared Bandcamp link anchors to the correct same-name artist")

	var ar catalogm.Artist
	s.Require().NoError(s.db.First(&ar, id).Error)
	s.Require().NotNil(ar.ImageURL) // resolved via Q2 → B.jpg
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestNoWikidataLink_Skips() {
	s.seedArtist("Lonesome Act", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Lonesome Act": {{MBID: "m1", Name: "Lonesome Act"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://lonesome.bandcamp.com"}}, // no wikidata rel
	}
	report := s.run(mb, &fakeWikidata{}, &fakeCommons{}, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsSkipped)
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestNoP18Image_Skips() {
	s.seedArtist("No Photo Act", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"No Photo Act": {{MBID: "m1", Name: "No Photo Act"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q9"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{}} // Q9 → no P18
	report := s.run(mb, wd, &fakeCommons{}, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsSkipped)
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestNonFreeImage_Skips() {
	s.seedArtist("Restricted Act", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Restricted Act": {{MBID: "m1", Name: "Restricted Act"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q5"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q5": "Locked.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{}} // Locked.jpg → nil (non-free / missing)
	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsSkipped)
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestValidationSkip_BadImageHost() {
	s.seedArtist("Bad Host Act", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Bad Host Act": {{MBID: "m1", Name: "Bad Host Act"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q7"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q7": "Evil.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"Evil.jpg": {
		ImageURL:       "https://evil.test/x.jpg", // off the Commons CDN host
		DescriptionURL: "https://commons.wikimedia.org/wiki/File:Evil.jpg",
		License:        "CC BY 2.0",
	}}}
	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(0, report.ArtistsMatched)
	s.Equal(1, report.ArtistsSkipped)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestDryRun_DoesNotWrite() {
	id := s.seedArtist("Phoebe Bridgers", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Phoebe Bridgers": {{MBID: "m1", Name: "Phoebe Bridgers"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q1"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "X.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"X.jpg": goodImage()}}

	report := s.run(mb, wd, cm, CommonsEnrichOptions{DryRun: true})
	s.Equal(1, report.ArtistsMatched)
	s.Equal(0, report.ArtistsUpdated)

	var ar catalogm.Artist
	s.Require().NoError(s.db.First(&ar, id).Error)
	s.Nil(ar.ImageURL)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestLimit_CapsArtists() {
	s.seedArtist("Artist One", "")
	s.seedArtist("Artist Two", "")
	report := s.run(&fakeMBArtist{}, &fakeWikidata{}, &fakeCommons{}, CommonsEnrichOptions{DryRun: true, Limit: 1})
	s.Equal(1, report.ArtistsScanned)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestSearchError_CountsAndContinues() {
	s.seedArtist("Erroring Act", "")
	mb := &fakeMBArtist{searchErr: fmt.Errorf("musicbrainz down")}
	report := s.run(mb, &fakeWikidata{}, &fakeCommons{}, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsScanned)
	s.Equal(1, report.ArtistErrors)
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestUniqueMatch_ContradictingAnchor_Skips() {
	// One MB artist named "Muscle", but its Bandcamp link contradicts ours → a
	// different same-named artist → skip even though the name is unique.
	s.seedArtist("Muscle", "https://realmuscle.bandcamp.com")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Muscle": {{MBID: "m1", Name: "Muscle"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://fakemuscle.bandcamp.com", "https://www.wikidata.org/wiki/Q1"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "X.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"X.jpg": goodImage()}}
	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsSkipped, "a same-platform link contradiction skips even a unique name")
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestUniqueMatch_NonContradictingLink_Stores() {
	// Our artist has a Bandcamp link; the unique MB artist lacks it but has only
	// unrelated links (no same-platform mismatch) → NOT a contradiction → stored.
	// Recall is preserved when MB profiles are merely incomplete.
	id := s.seedArtist("Muscle", "https://realmuscle.bandcamp.com")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Muscle": {{MBID: "m1", Name: "Muscle"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://en.wikipedia.org/wiki/Muscle", "https://www.wikidata.org/wiki/Q1"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "X.jpg"}}
	cm := &fakeCommons{imageByFilename: map[string]*CommonsImage{"X.jpg": goodImage()}}
	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistsUpdated)
	var ar catalogm.Artist
	s.Require().NoError(s.db.First(&ar, id).Error)
	s.Require().NotNil(ar.ImageURL)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestURLRelsError_CountsError() {
	s.seedArtist("Phoebe Bridgers", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Phoebe Bridgers": {{MBID: "m1", Name: "Phoebe Bridgers"}}},
		urlsErr:          fmt.Errorf("musicbrainz url-rels 503"),
	}
	report := s.run(mb, &fakeWikidata{}, &fakeCommons{}, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistErrors, "a url-rels failure is counted + retryable, not a clean skip")
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestWikidataError_CountsError() {
	s.seedArtist("Phoebe Bridgers", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Phoebe Bridgers": {{MBID: "m1", Name: "Phoebe Bridgers"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q1"}},
	}
	wd := &fakeWikidata{err: fmt.Errorf("wikidata 500")}
	report := s.run(mb, wd, &fakeCommons{}, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistErrors)
	s.Equal(0, report.ArtistsUpdated)
}

func (s *CommonsPhotoEnrichIntegrationTestSuite) TestCommonsError_CountsError() {
	s.seedArtist("Phoebe Bridgers", "")
	mb := &fakeMBArtist{
		candidatesByName: map[string][]MBArtistCandidate{"Phoebe Bridgers": {{MBID: "m1", Name: "Phoebe Bridgers"}}},
		urlsByMBID:       map[string][]string{"m1": {"https://www.wikidata.org/wiki/Q1"}},
	}
	wd := &fakeWikidata{filenameByQID: map[string]string{"Q1": "X.jpg"}}
	cm := &fakeCommons{err: fmt.Errorf("commons 500")}
	report := s.run(mb, wd, cm, CommonsEnrichOptions{})
	s.Equal(1, report.ArtistErrors)
	s.Equal(0, report.ArtistsUpdated)
}
