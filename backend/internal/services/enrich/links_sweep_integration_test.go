package enrich

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/testutil"
)

// ArtistLinksSweepIntegrationTestSuite exercises PSY-1279 against Postgres.
type ArtistLinksSweepIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ArtistLinksSweepIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ArtistLinksSweepIntegrationTestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ArtistLinksSweepIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestArtistLinksSweepIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ArtistLinksSweepIntegrationTestSuite))
}

func (s *ArtistLinksSweepIntegrationTestSuite) TestArtistsNeedingLinksMemoFilter() {
	mbid := "11111111-1111-1111-1111-111111111111"
	spotify := "https://open.spotify.com/artist/existing"
	recent := time.Now().Add(-1 * time.Hour)
	stale := time.Now().Add(-100 * 24 * time.Hour)

	seed := []*catalogm.Artist{
		{Name: "Needs Links", MusicBrainzArtistID: &mbid},
		{Name: "Partial Links", MusicBrainzArtistID: strptr("22222222-2222-2222-2222-222222222222"), Social: catalogm.Social{Spotify: &spotify}},
		{Name: "Recently Tried", MusicBrainzArtistID: strptr("33333333-3333-3333-3333-333333333333"), LinksEnrichAttemptedAt: &recent},
		{Name: "Stale Tried", MusicBrainzArtistID: strptr("44444444-4444-4444-4444-444444444444"), LinksEnrichAttemptedAt: &stale},
		{Name: "No MBID"},
	}
	for _, a := range seed {
		s.Require().NoError(s.db.Create(a).Error)
	}

	store := &gormArtistStore{db: s.db}
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	got, err := store.ArtistsNeedingLinks(0, &cutoff)
	s.Require().NoError(err)

	names := map[string]bool{}
	for _, a := range got {
		names[a.Name] = true
	}
	s.True(names["Needs Links"])
	s.True(names["Partial Links"], "missing bandcamp/website still qualifies")
	s.True(names["Stale Tried"])
	s.False(names["Recently Tried"])
	s.False(names["No MBID"])
}

func (s *ArtistLinksSweepIntegrationTestSuite) TestStampLinksAttempted() {
	mbid := "11111111-1111-1111-1111-111111111111"
	a := &catalogm.Artist{Name: "Stamp Me", MusicBrainzArtistID: &mbid}
	s.Require().NoError(s.db.Create(a).Error)

	var before catalogm.Artist
	s.Require().NoError(s.db.First(&before, a.ID).Error)

	at := time.Now().Truncate(time.Second)
	s.Require().NoError((&gormArtistStore{db: s.db}).StampLinksAttempted([]uint{a.ID}, at))

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, a.ID).Error)
	s.Require().NotNil(reloaded.LinksEnrichAttemptedAt)
	s.WithinDuration(at, *reloaded.LinksEnrichAttemptedAt, time.Second)
	s.Equal(before.UpdatedAt.UnixMicro(), reloaded.UpdatedAt.UnixMicro())
}

func (s *ArtistLinksSweepIntegrationTestSuite) TestSweepCycleFillsAndConverges() {
	mbid := "11111111-1111-1111-1111-111111111111"
	a := &catalogm.Artist{Name: "Resolvable", MusicBrainzArtistID: &mbid}
	s.Require().NoError(s.db.Create(a).Error)

	mb := &fakeLinksMB{rels: map[string][]pipeline.MBURLRelation{
		mbid: {
			mbURLRel("free streaming", "https://open.spotify.com/artist/abc123"),
			mbURLRel("bandcamp", "https://resolvable.bandcamp.com/"),
			mbURLRel("official homepage", "https://resolvable.example.com"),
		},
	}}
	writer := &recordingLinksWriter{db: s.db}
	sweep := NewArtistLinksSweep(s.db, mb, writer)
	sweep.batch = 50
	sweep.reattempt = 90 * 24 * time.Hour

	sweep.RunSweepNow(context.Background())

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, a.ID).Error)
	s.Require().NotNil(reloaded.Social.Spotify)
	s.Contains(*reloaded.Social.Spotify, "open.spotify.com/artist/abc123")
	s.Require().NotNil(reloaded.Social.Bandcamp)
	s.Contains(*reloaded.Social.Bandcamp, "resolvable.bandcamp.com")
	s.Require().NotNil(reloaded.Social.Website)
	s.Equal("https://resolvable.example.com", *reloaded.Social.Website)
	s.NotNil(reloaded.LinksEnrichAttemptedAt)

	sweep.RunSweepNow(context.Background())
	got, err := (&gormArtistStore{db: s.db}).ArtistsNeedingLinks(0, ptrTime(time.Now().Add(-90*24*time.Hour)))
	s.Require().NoError(err)
	s.Empty(got, "within-window artist must not be re-processed")
}

type fakeLinksMB struct {
	rels map[string][]pipeline.MBURLRelation
	err  error
}

func (f *fakeLinksMB) LookupArtistURLRelations(_ context.Context, mbid string) ([]pipeline.MBURLRelation, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rels[mbid], nil
}

// recordingLinksWriter applies UpdateArtist-style writes for integration tests.
type recordingLinksWriter struct {
	db *gorm.DB
}

func (w *recordingLinksWriter) UpdateArtist(artistID uint, req *contracts.UpdateArtistRequest) (*contracts.ArtistDetailResponse, error) {
	updates := map[string]interface{}{}
	if req.Spotify != nil {
		updates["spotify"] = *req.Spotify
	}
	if req.Bandcamp != nil {
		updates["bandcamp"] = *req.Bandcamp
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if len(updates) == 0 {
		return &contracts.ArtistDetailResponse{}, nil
	}
	if err := w.db.Model(&catalogm.Artist{}).Where("id = ?", artistID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &contracts.ArtistDetailResponse{}, nil
}

func ptrTime(t time.Time) *time.Time { return &t }
