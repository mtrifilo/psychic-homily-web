package discography

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/testutil"
)

// ArtistDiscographySweepIntegrationTestSuite exercises the PSY-1291 sweep's real SQL
// against Postgres (the testcontainers DB runs all migrations, so this also validates
// the discography_synced_at column + partial index): the memo-filtered candidate
// selection, the bulk stamp, and a full cycle. Provider lookups are faked, so there is
// no external MusicBrainz/CAA traffic.
type ArtistDiscographySweepIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ArtistDiscographySweepIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ArtistDiscographySweepIntegrationTestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ArtistDiscographySweepIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{"artist_releases", "release_external_links", "image_enrich_queue", "releases", "artists"} {
		_, _ = sqlDB.Exec("DELETE FROM " + t)
	}
}

func TestArtistDiscographySweepIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ArtistDiscographySweepIntegrationTestSuite))
}

func strptr(s string) *string { return &s }

// Memo-filtered candidate query: MBID-bearing, not-recently-synced artists are selected;
// MBID-less / recently-synced ones are excluded.
func (s *ArtistDiscographySweepIntegrationTestSuite) TestArtistsNeedingDiscographyMemoFilter() {
	mbid := "11111111-1111-1111-1111-111111111111"
	otherMBID := "22222222-2222-2222-2222-222222222222"
	recent := time.Now().Add(-1 * time.Hour)
	stale := time.Now().Add(-100 * 24 * time.Hour)

	seed := []*catalogm.Artist{
		{Name: "Never Synced", MusicBrainzArtistID: &mbid},
		{Name: "Stale Synced", MusicBrainzArtistID: &otherMBID, DiscographySyncedAt: &stale},
		{Name: "Recently Synced", MusicBrainzArtistID: strptr("33333333-3333-3333-3333-333333333333"), DiscographySyncedAt: &recent},
		{Name: "No MBID"},
	}
	for _, a := range seed {
		s.Require().NoError(s.db.Create(a).Error)
	}

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	got, err := loadArtistsWithMBID(s.db, 0, &cutoff)
	s.Require().NoError(err)

	names := map[string]bool{}
	for _, a := range got {
		names[a.Name] = true
	}
	s.True(names["Never Synced"], "never-synced MBID artist should be selected")
	s.True(names["Stale Synced"], "stale-sync artist should be selected")
	s.False(names["Recently Synced"], "recently-synced artist must be excluded")
	s.False(names["No MBID"], "artist without MBID must be excluded")

	// nil cutoff (the manual cmd) ignores the memo: the recently-synced row reappears.
	all, err := loadArtistsWithMBID(s.db, 0, nil)
	s.Require().NoError(err)
	s.Len(all, 3, "memo-agnostic selection returns every MBID-bearing row regardless of sync time")
}

// Bulk stamp writes the memo column WITHOUT bumping updated_at; empty slice is a no-op.
func (s *ArtistDiscographySweepIntegrationTestSuite) TestStampDiscographySynced() {
	mbid := "11111111-1111-1111-1111-111111111111"
	a := &catalogm.Artist{Name: "Stamp Me", MusicBrainzArtistID: &mbid}
	s.Require().NoError(s.db.Create(a).Error)
	s.Nil(a.DiscographySyncedAt)

	var before catalogm.Artist
	s.Require().NoError(s.db.First(&before, a.ID).Error)

	at := time.Now().Truncate(time.Second)
	s.Require().NoError(stampDiscographySynced(s.db, []uint{a.ID}, at))

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, a.ID).Error)
	s.Require().NotNil(reloaded.DiscographySyncedAt)
	s.WithinDuration(at, *reloaded.DiscographySyncedAt, time.Second)
	s.Equal(before.UpdatedAt.UnixMicro(), reloaded.UpdatedAt.UnixMicro(),
		"stampDiscographySynced must not bump updated_at")

	s.NoError(stampDiscographySynced(s.db, nil, at), "empty slice is a no-op")
}

// NULLS-FIRST-then-stalest ordering under a LIMIT — the production convergence property.
func (s *ArtistDiscographySweepIntegrationTestSuite) TestArtistsNeedingDiscographyOrdering() {
	mbidA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	mbidB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	mbidC := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	older := time.Now().Add(-100 * 24 * time.Hour)
	newer := time.Now().Add(-95 * 24 * time.Hour)

	never := &catalogm.Artist{Name: "A Never", MusicBrainzArtistID: &mbidA}
	stalest := &catalogm.Artist{Name: "B Stalest", MusicBrainzArtistID: &mbidB, DiscographySyncedAt: &older}
	stale := &catalogm.Artist{Name: "C Stale", MusicBrainzArtistID: &mbidC, DiscographySyncedAt: &newer}
	for _, a := range []*catalogm.Artist{stale, stalest, never} {
		s.Require().NoError(s.db.Create(a).Error)
	}

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	got, err := loadArtistsWithMBID(s.db, 2, &cutoff)
	s.Require().NoError(err)
	s.Require().Len(got, 2, "LIMIT must bound the batch")
	s.Equal("A Never", got[0].Name, "never-synced (NULLS FIRST) must come first")
	s.Equal("B Stalest", got[1].Name, "stalest sync must come before the newer one")
}

// Full cycle: the sweep imports for a resolvable artist, stamps the memo on every attempted
// row (incl. a browse miss), and on a second cycle re-processes nothing within the window.
func (s *ArtistDiscographySweepIntegrationTestSuite) TestSweepCycleImportsStampsAndConverges() {
	mbidOK := "11111111-1111-1111-1111-111111111111"
	mbidMiss := "22222222-2222-2222-2222-222222222222"
	rgAlbum := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	resolvable := &catalogm.Artist{Name: "Resolvable Band", MusicBrainzArtistID: &mbidOK}
	unresolvable := &catalogm.Artist{Name: "Empty Discography", MusicBrainzArtistID: &mbidMiss}
	for _, a := range []*catalogm.Artist{resolvable, unresolvable} {
		s.Require().NoError(s.db.Create(a).Error)
	}

	browser := fakeBrowser{byMBID: map[string][]pipeline.MBReleaseGroupResult{
		mbidOK: {{ID: rgAlbum, Title: "Studio LP", PrimaryType: "Album", FirstReleaseDate: "2001"}},
		mbidMiss: {},
	}}
	sweep := NewArtistDiscographySweep(s.db, browser, fakeCoverArt{})
	sweep.batch = 50
	sweep.reattempt = 90 * 24 * time.Hour

	sweep.RunSweepNow(context.Background())

	var r catalogm.Artist
	s.Require().NoError(s.db.First(&r, resolvable.ID).Error)
	s.NotNil(r.DiscographySyncedAt, "a synced artist is stamped")

	var releaseCount int64
	s.Require().NoError(s.db.Model(&catalogm.Release{}).Where("title = ?", "Studio LP").Count(&releaseCount).Error)
	s.Equal(int64(1), releaseCount)

	var u catalogm.Artist
	s.Require().NoError(s.db.First(&u, unresolvable.ID).Error)
	s.NotNil(u.DiscographySyncedAt, "even a no-release browse is stamped (poison-row safety)")

	// Convergence: second cycle finds nothing eligible (both within the window).
	sweep.RunSweepNow(context.Background())

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	got, err := loadArtistsWithMBID(s.db, 0, &cutoff)
	s.Require().NoError(err)
	s.Empty(got, "within-window artists must not be re-processed")
}
