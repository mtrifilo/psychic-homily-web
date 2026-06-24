package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// fakeDiscoverer returns a fixed candidate set per artist id (and an optional
// error), so RunSweep is exercised end-to-end against a real DB WITHOUT any
// MusicBrainz network I/O. The real DiscoverMusicService is the production
// discoverer; this fake stands in for it so the sweep's loop/filter/upsert logic
// is what's under test.
type fakeDiscoverer struct {
	byArtist map[uint][]contracts.MusicLinkCandidate
	errFor   map[uint]error
	calls    []uint // ordered record of artist ids passed in
}

func (f *fakeDiscoverer) DiscoverMusic(_ context.Context, artistID uint, _ string) (*contracts.DiscoverMusicResult, error) {
	f.calls = append(f.calls, artistID)
	if err := f.errFor[artistID]; err != nil {
		return nil, err
	}
	return &contracts.DiscoverMusicResult{
		ArtistID:   artistID,
		Candidates: f.byArtist[artistID],
	}, nil
}

func cand(platform, url, confidence string) contracts.MusicLinkCandidate {
	return contracts.MusicLinkCandidate{
		Platform: platform,
		URL:      url,
		// "musicbrainz" is the ONLY value the source CHECK constraint allows, and
		// the production DiscoverMusic always sets exactly this — mirror it so the
		// insert exercises the real ON CONFLICT path, not a constraint rejection.
		Source:       catalogm.LinkSuggestionSourceMusicBrainz,
		MBArtistID:   "mbid-" + url,
		MBArtistName: "MB " + platform,
		Confidence:   confidence,
		RegionMatch:  confidence == contracts.MusicConfidenceHigh,
		Live:         true,
		Notes:        "note",
	}
}

type SweepIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *SweepIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *SweepIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *SweepIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artist_link_suggestions")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestSweepIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SweepIntegrationTestSuite))
}

// seedArtist inserts an artist with the given name and optional bandcamp embed /
// spotify link, returning its id. A nil pointer leaves the column NULL.
func (s *SweepIntegrationTestSuite) seedArtist(name string, bandcampEmbed, spotify *string) uint {
	a := catalogm.Artist{Name: name, BandcampEmbedURL: bandcampEmbed}
	a.Social.Spotify = spotify
	require.NoError(s.T(), s.db.Create(&a).Error)
	return a.ID
}

func (s *SweepIntegrationTestSuite) countSuggestions(artistID uint) int64 {
	var n int64
	require.NoError(s.T(), s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("artist_id = ?", artistID).Count(&n).Error)
	return n
}

// TestTargetFilter_OnlyLinkless asserts only artists with BOTH columns NULL are
// swept — an artist with a bandcamp embed OR a spotify link is excluded.
func (s *SweepIntegrationTestSuite) TestTargetFilter_OnlyLinkless() {
	embed := "https://x.bandcamp.com/album/y"
	spot := "https://open.spotify.com/artist/z"
	linkless := s.seedArtist("Linkless Band", nil, nil)
	hasEmbed := s.seedArtist("Has Embed", &embed, nil)
	hasSpotify := s.seedArtist("Has Spotify", nil, &spot)

	disc := &fakeDiscoverer{byArtist: map[uint][]contracts.MusicLinkCandidate{
		linkless:   {cand(contracts.MusicPlatformBandcamp, "https://a.bandcamp.com", contracts.MusicConfidenceHigh)},
		hasEmbed:   {cand(contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/q", contracts.MusicConfidenceReview)},
		hasSpotify: {cand(contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/r", contracts.MusicConfidenceReview)},
	}}

	report, err := RunSweep(context.Background(), s.db, disc, false /* dry-run */)
	require.NoError(s.T(), err)

	// Only the link-less artist is scanned at all.
	require.Equal(s.T(), 1, report.ArtistsScanned)
	require.Equal(s.T(), []uint{linkless}, disc.calls)
}

// TestDryRun_NoWrites asserts dry-run reports the planned count but writes nothing.
func (s *SweepIntegrationTestSuite) TestDryRun_NoWrites() {
	id := s.seedArtist("Band", nil, nil)
	disc := &fakeDiscoverer{byArtist: map[uint][]contracts.MusicLinkCandidate{
		id: {
			cand(contracts.MusicPlatformBandcamp, "https://a.bandcamp.com", contracts.MusicConfidenceHigh),
			cand(contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/q", contracts.MusicConfidenceReview),
		},
	}}

	report, err := RunSweep(context.Background(), s.db, disc, true /* dry-run */)
	require.NoError(s.T(), err)

	require.Equal(s.T(), 1, report.ArtistsScanned)
	require.Equal(s.T(), 1, report.ArtistsWithCandidates)
	require.Equal(s.T(), 2, report.SuggestionsFound)
	require.Equal(s.T(), 0, report.SuggestionsWritten)
	require.Zero(s.T(), s.countSuggestions(id), "dry-run must not write")
}

// TestConfirm_UpsertsPending asserts --confirm writes pending rows with the
// candidate fields mapped through.
func (s *SweepIntegrationTestSuite) TestConfirm_UpsertsPending() {
	id := s.seedArtist("Band", nil, nil)
	disc := &fakeDiscoverer{byArtist: map[uint][]contracts.MusicLinkCandidate{
		id: {
			cand(contracts.MusicPlatformBandcamp, "https://a.bandcamp.com", contracts.MusicConfidenceHigh),
			cand(contracts.MusicPlatformSpotify, "https://open.spotify.com/artist/q", contracts.MusicConfidenceReview),
		},
	}}

	report, err := RunSweep(context.Background(), s.db, disc, false /* confirm */)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, report.SuggestionsFound)
	require.Equal(s.T(), 2, report.SuggestionsWritten)

	var rows []catalogm.ArtistLinkSuggestion
	require.NoError(s.T(), s.db.Where("artist_id = ?", id).Order("platform").Find(&rows).Error)
	require.Len(s.T(), rows, 2)
	require.Equal(s.T(), catalogm.LinkSuggestionStatusPending, rows[0].Status)
	require.Equal(s.T(), contracts.MusicPlatformBandcamp, rows[0].Platform)
	require.Equal(s.T(), contracts.MusicConfidenceHigh, rows[0].Confidence)
	require.True(s.T(), rows[0].RegionMatch)
	require.True(s.T(), rows[0].Live)
	require.NotNil(s.T(), rows[0].MBArtistID)
}

// TestIdempotent_RerunWritesZero asserts a second confirm run inserts nothing new
// (ON CONFLICT DO NOTHING) — the resumability guarantee.
func (s *SweepIntegrationTestSuite) TestIdempotent_RerunWritesZero() {
	id := s.seedArtist("Band", nil, nil)
	disc := &fakeDiscoverer{byArtist: map[uint][]contracts.MusicLinkCandidate{
		id: {cand(contracts.MusicPlatformBandcamp, "https://a.bandcamp.com", contracts.MusicConfidenceHigh)},
	}}

	r1, err := RunSweep(context.Background(), s.db, disc, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, r1.SuggestionsWritten)

	r2, err := RunSweep(context.Background(), s.db, disc, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, r2.SuggestionsFound, "re-discovers the same candidate")
	require.Equal(s.T(), 0, r2.SuggestionsWritten, "but writes nothing — idempotent")
	require.EqualValues(s.T(), 1, s.countSuggestions(id), "still exactly one row")
}

// TestNoResurrect_ReviewedRowUntouched is the critical safety property: a row a
// human already ACCEPTED or REJECTED must NEVER be flipped back to pending by a
// re-sweep that re-discovers the same (artist, platform, url).
func (s *SweepIntegrationTestSuite) TestNoResurrect_ReviewedRowUntouched() {
	id := s.seedArtist("Band", nil, nil)
	url := "https://a.bandcamp.com"

	// Simulate a prior sweep + human review: insert a rejected row for this exact key.
	rejected := catalogm.ArtistLinkSuggestion{
		ArtistID:   id,
		Platform:   contracts.MusicPlatformBandcamp,
		URL:        url,
		Source:     catalogm.LinkSuggestionSourceMusicBrainz,
		Confidence: contracts.MusicConfidenceHigh,
		Status:     catalogm.LinkSuggestionStatusRejected,
	}
	require.NoError(s.T(), s.db.Create(&rejected).Error)

	disc := &fakeDiscoverer{byArtist: map[uint][]contracts.MusicLinkCandidate{
		id: {cand(contracts.MusicPlatformBandcamp, url, contracts.MusicConfidenceHigh)},
	}}

	report, err := RunSweep(context.Background(), s.db, disc, false /* confirm */)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 0, report.SuggestionsWritten, "the rejected row's key conflicts → DO NOTHING")

	var row catalogm.ArtistLinkSuggestion
	require.NoError(s.T(), s.db.Where("artist_id = ? AND platform = ? AND url = ?",
		id, contracts.MusicPlatformBandcamp, url).First(&row).Error)
	require.Equal(s.T(), catalogm.LinkSuggestionStatusRejected, row.Status,
		"reviewed status must survive the sweep — never resurrected to pending")
	require.EqualValues(s.T(), 1, s.countSuggestions(id), "no duplicate row inserted")
}

// TestPerArtistErrorNonFatal asserts one artist's discovery failure is recorded
// but does not abort the sweep — the next artist is still processed.
func (s *SweepIntegrationTestSuite) TestPerArtistErrorNonFatal() {
	bad := s.seedArtist("AAA Bad", nil, nil)
	good := s.seedArtist("ZZZ Good", nil, nil)

	disc := &fakeDiscoverer{
		byArtist: map[uint][]contracts.MusicLinkCandidate{
			good: {cand(contracts.MusicPlatformBandcamp, "https://g.bandcamp.com", contracts.MusicConfidenceHigh)},
		},
		errFor: map[uint]error{bad: errors.New("mb boom")},
	}

	report, err := RunSweep(context.Background(), s.db, disc, false)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 2, report.ArtistsScanned)
	require.Len(s.T(), report.Errors, 1)
	require.Equal(s.T(), 1, report.SuggestionsWritten, "the good artist still got written")
	require.EqualValues(s.T(), 1, s.countSuggestions(good))
	require.EqualValues(s.T(), 0, s.countSuggestions(bad))
}

// TestSequentialOrder asserts artists are processed in id order, one at a time —
// a structural check that there is no parallel fan-out reordering calls.
func (s *SweepIntegrationTestSuite) TestSequentialOrder() {
	a := s.seedArtist("Artist A", nil, nil)
	b := s.seedArtist("Artist B", nil, nil)
	c := s.seedArtist("Artist C", nil, nil)

	disc := &fakeDiscoverer{byArtist: map[uint][]contracts.MusicLinkCandidate{}}
	_, err := RunSweep(context.Background(), s.db, disc, true)
	require.NoError(s.T(), err)
	require.Equal(s.T(), []uint{a, b, c}, disc.calls, "sequential, id-ordered")
}
