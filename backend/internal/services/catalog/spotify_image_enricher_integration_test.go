package catalog

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// fakeSpotifyAPI is an in-memory spotifyImageAPI for the backfill loop tests.
type fakeSpotifyAPI struct {
	albumsByTitle map[string][]SpotifyAlbum
	artistsByID   map[string]*SpotifyArtist
	searchErr     error // when set, SearchAlbums returns it (simulates a throttle abort)
	searchCalls   int
	artistCalls   int
}

func (f *fakeSpotifyAPI) SearchAlbums(_, title string, _ int) ([]SpotifyAlbum, error) {
	f.searchCalls++
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.albumsByTitle[title], nil
}

func (f *fakeSpotifyAPI) GetArtist(id string) (*SpotifyArtist, error) {
	f.artistCalls++
	a, ok := f.artistsByID[id]
	if !ok {
		return nil, fmt.Errorf("spotify artist %q not found", id)
	}
	return a, nil
}

type SpotifyEnrichIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *SpotifyEnrichIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *SpotifyEnrichIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *SpotifyEnrichIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestSpotifyEnrichIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SpotifyEnrichIntegrationTestSuite))
}

// seedFixture creates the standard catalog used across the cases:
//   - artist "Sleep" with a Spotify link, no image
//   - artist "BadLink" with an off-host Spotify link, no image (skipped)
//   - artist "NoLink" with no Spotify link (not even scanned)
//   - release "Dopesmoker" (2003) by Sleep, no cover
//   - release "Holy Mountain" (2018) by Sleep, WITH a cover (not scanned)
func (s *SpotifyEnrichIntegrationTestSuite) seedFixture() (sleepID, dopesmokerID uint) {
	sleep := &catalogm.Artist{
		Name:   "Sleep",
		Social: catalogm.Social{Spotify: spotifyStrPtr("https://open.spotify.com/artist/ABC123")},
	}
	s.Require().NoError(s.db.Create(sleep).Error)

	badLink := &catalogm.Artist{
		Name:   "BadLink",
		Social: catalogm.Social{Spotify: spotifyStrPtr("https://evil.test/artist/ABC123")},
	}
	s.Require().NoError(s.db.Create(badLink).Error)

	noLink := &catalogm.Artist{Name: "NoLink"}
	s.Require().NoError(s.db.Create(noLink).Error)

	dopesmoker := &catalogm.Release{Title: "Dopesmoker", ReleaseYear: spotifyIntPtr(2003)}
	s.Require().NoError(s.db.Create(dopesmoker).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
		ArtistID: sleep.ID, ReleaseID: dopesmoker.ID, Role: catalogm.ArtistReleaseRoleMain,
	}).Error)

	holy := &catalogm.Release{
		Title:       "Holy Mountain",
		ReleaseYear: spotifyIntPtr(2018),
		CoverArtURL: spotifyStrPtr("https://existing.test/cover.jpg"),
	}
	s.Require().NoError(s.db.Create(holy).Error)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{
		ArtistID: sleep.ID, ReleaseID: holy.ID, Role: catalogm.ArtistReleaseRoleMain,
	}).Error)

	return sleep.ID, dopesmoker.ID
}

func newFakeAPI() *fakeSpotifyAPI {
	return &fakeSpotifyAPI{
		albumsByTitle: map[string][]SpotifyAlbum{
			"Dopesmoker": {{
				ID:           "alb1",
				Name:         "Dopesmoker",
				Artists:      []SpotifyAlbumArtistRef{{ID: "ABC123", Name: "Sleep"}},
				ReleaseDate:  "2003",
				Images:       []SpotifyImage{{URL: "https://i.scdn.co/album/big", Width: 640, Height: 640}},
				ExternalURLs: SpotifyExternalURLs{Spotify: "https://open.spotify.com/album/alb1"},
			}},
		},
		artistsByID: map[string]*SpotifyArtist{
			"ABC123": {
				ID:           "ABC123",
				Name:         "Sleep",
				Images:       []SpotifyImage{{URL: "https://i.scdn.co/artist/big", Width: 640, Height: 640}},
				ExternalURLs: SpotifyExternalURLs{Spotify: "https://open.spotify.com/artist/ABC123"},
			},
		},
	}
}

func (s *SpotifyEnrichIntegrationTestSuite) TestDryRun_SearchesButDoesNotWrite() {
	sleepID, dopesmokerID := s.seedFixture()
	api := newFakeAPI()

	report, err := BackfillSpotifyImages(s.db, api, SpotifyEnrichOptions{DryRun: true})
	s.Require().NoError(err)

	// Only the cover-less release + the two image-less artists with a link column
	// are scanned; the cover'd release and the link-less artist are excluded.
	s.Equal(1, report.ReleasesScanned)
	s.Equal(1, report.ReleasesMatched)
	s.Equal(0, report.ReleasesUpdated)

	s.Equal(2, report.ArtistsScanned) // Sleep + BadLink (NoLink excluded by query)
	s.Equal(1, report.ArtistsMatched) // Sleep matches; BadLink's off-host link is skipped
	s.Equal(1, report.ArtistsSkipped)
	s.Equal(0, report.ArtistsUpdated)

	// Nothing written.
	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, dopesmokerID).Error)
	s.Nil(rel.CoverArtURL)
	s.Nil(rel.CoverArtSource)

	var ar catalogm.Artist
	s.Require().NoError(s.db.First(&ar, sleepID).Error)
	s.Nil(ar.ImageURL)
	s.Nil(ar.ImageSource)

	// BadLink's off-host link never reaches GetArtist.
	s.Equal(1, api.artistCalls)
}

func (s *SpotifyEnrichIntegrationTestSuite) TestLiveRun_WritesReferencesAndIsIdempotent() {
	sleepID, dopesmokerID := s.seedFixture()
	api := newFakeAPI()

	report, err := BackfillSpotifyImages(s.db, api, SpotifyEnrichOptions{DryRun: false})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesUpdated)
	s.Equal(1, report.ArtistsUpdated)

	// Release cover reference stored (URL + provider id + linkback), no bytes.
	var rel catalogm.Release
	s.Require().NoError(s.db.First(&rel, dopesmokerID).Error)
	s.Require().NotNil(rel.CoverArtURL)
	s.Equal("https://i.scdn.co/album/big", *rel.CoverArtURL)
	s.Require().NotNil(rel.CoverArtSource)
	s.Equal("spotify", *rel.CoverArtSource)
	s.Require().NotNil(rel.CoverArtSourceURL)
	s.Equal("https://open.spotify.com/album/alb1", *rel.CoverArtSourceURL)

	// Artist photo reference stored.
	var ar catalogm.Artist
	s.Require().NoError(s.db.First(&ar, sleepID).Error)
	s.Require().NotNil(ar.ImageURL)
	s.Equal("https://i.scdn.co/artist/big", *ar.ImageURL)
	s.Require().NotNil(ar.ImageSource)
	s.Equal("spotify", *ar.ImageSource)
	s.Require().NotNil(ar.ImageSourceURL)
	s.Equal("https://open.spotify.com/artist/ABC123", *ar.ImageSourceURL)

	// Idempotent: the enriched entities now have images, so a re-run touches them
	// not. Only BadLink (still image-less, has a link column) is re-scanned.
	api2 := newFakeAPI()
	report2, err := BackfillSpotifyImages(s.db, api2, SpotifyEnrichOptions{DryRun: false})
	s.Require().NoError(err)
	s.Equal(0, report2.ReleasesScanned)
	s.Equal(0, report2.ReleasesUpdated)
	s.Equal(1, report2.ArtistsScanned) // only BadLink remains image-less
	s.Equal(0, report2.ArtistsUpdated)
}

func (s *SpotifyEnrichIntegrationTestSuite) TestLimit_CapsPerType() {
	// Two cover-less releases by Sleep; limit=1 should scan only one.
	s.seedFixture()
	extra := &catalogm.Release{Title: "Sleep's Holy Mountain", ReleaseYear: spotifyIntPtr(1992)}
	s.Require().NoError(s.db.Create(extra).Error)

	api := newFakeAPI()
	report, err := BackfillSpotifyImages(s.db, api, SpotifyEnrichOptions{DryRun: true, Limit: 1})
	s.Require().NoError(err)
	s.Equal(1, report.ReleasesScanned)
	s.Equal(1, report.ArtistsScanned)
}

func (s *SpotifyEnrichIntegrationTestSuite) TestRateLimitedAbortsRun() {
	// A persistent throttle (ErrSpotifyRateLimited from the client) must abort the
	// whole run, not fail each entity — so we don't grind the catalog through the
	// backoff. BackfillSpotifyImages returns an error wrapping the sentinel.
	s.seedFixture()
	api := &fakeSpotifyAPI{searchErr: fmt.Errorf("search: %w", ErrSpotifyRateLimited)}

	_, err := BackfillSpotifyImages(s.db, api, SpotifyEnrichOptions{DryRun: true})
	s.Require().Error(err)
	s.True(errors.Is(err, ErrSpotifyRateLimited), "a persistent throttle must abort the run with the sentinel")
	s.Equal(1, api.searchCalls, "abort on the first throttled search, not after grinding every release")
}
