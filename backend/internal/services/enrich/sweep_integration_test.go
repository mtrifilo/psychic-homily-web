package enrich

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

// ArtistLocationSweepIntegrationTestSuite exercises the PSY-1250 sweep's real SQL
// against Postgres (the testcontainers DB runs all migrations, so this also validates
// the location_enrich_attempted_at column + partial index): the memo-filtered
// candidate selection, the bulk stamp, and a full cycle. Provider lookups are faked,
// so there is no external MusicBrainz/Bandcamp traffic.
type ArtistLocationSweepIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ArtistLocationSweepIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ArtistLocationSweepIntegrationTestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ArtistLocationSweepIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestArtistLocationSweepIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ArtistLocationSweepIntegrationTestSuite))
}

// Store-level: the memo-filtered candidate query selects city-less, not-recently-
// attempted artists and excludes city-set / recently-attempted ones.
func (s *ArtistLocationSweepIntegrationTestSuite) TestArtistsNeedingLocationMemoFilter() {
	recent := time.Now().Add(-1 * time.Hour)      // within a 30d window → skip
	stale := time.Now().Add(-60 * 24 * time.Hour) // beyond 30d → eligible

	seed := []*catalogm.Artist{
		{Name: "Never Tried"},                                        // city NULL, attempt NULL → in
		{Name: "Blank City", City: strptr("   ")},                    // TRIM-empty → in
		{Name: "Stale Tried", LocationEnrichAttemptedAt: &stale},     // stale attempt → in
		{Name: "Has City", City: strptr("Phoenix")},                  // has a city → out
		{Name: "Recently Tried", LocationEnrichAttemptedAt: &recent}, // recent attempt → out
	}
	for _, a := range seed {
		s.Require().NoError(s.db.Create(a).Error)
	}

	store := &gormArtistStore{db: s.db}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	got, err := store.ArtistsNeedingLocation(0, &cutoff)
	s.Require().NoError(err)

	names := map[string]bool{}
	for _, a := range got {
		names[a.Name] = true
	}
	s.True(names["Never Tried"], "never-attempted city-less artist should be selected")
	s.True(names["Blank City"], "blank (TRIM-empty) city should be selected")
	s.True(names["Stale Tried"], "stale-attempt artist should be selected")
	s.False(names["Has City"], "artist with a city must be excluded")
	s.False(names["Recently Tried"], "recently-attempted artist must be excluded")

	// nil cutoff (the manual cmd) ignores the memo: the recently-tried row reappears.
	all, err := store.ArtistsNeedingLocation(0, nil)
	s.Require().NoError(err)
	s.Len(all, 4, "memo-agnostic selection returns every city-less row regardless of attempt time")
}

// Store-level: the bulk stamp writes the memo column; empty slice is a no-op.
func (s *ArtistLocationSweepIntegrationTestSuite) TestStampLocationAttempted() {
	a := &catalogm.Artist{Name: "Stamp Me"}
	s.Require().NoError(s.db.Create(a).Error)
	s.Nil(a.LocationEnrichAttemptedAt)

	store := &gormArtistStore{db: s.db}
	at := time.Now().Truncate(time.Second)
	s.Require().NoError(store.StampLocationAttempted([]uint{a.ID}, at))

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, a.ID).Error)
	s.Require().NotNil(reloaded.LocationEnrichAttemptedAt)
	s.WithinDuration(at, *reloaded.LocationEnrichAttemptedAt, time.Second)

	s.NoError(store.StampLocationAttempted(nil, at), "empty slice is a no-op")
}

// Full cycle: the sweep fills a resolvable artist (city + PSY-1249 MBID), stamps the
// memo on every attempted row (incl. a miss), and on a second cycle re-processes
// nothing (all within the re-attempt window).
func (s *ArtistLocationSweepIntegrationTestSuite) TestSweepCycleFillsStampsAndConverges() {
	resolvable := &catalogm.Artist{Name: "Resolvable Band"}
	unresolvable := &catalogm.Artist{Name: "Unresolvable Band"}
	for _, a := range []*catalogm.Artist{resolvable, unresolvable} {
		s.Require().NoError(s.db.Create(a).Error)
	}

	mb := &fakeMB{byName: map[string][]pipeline.MBArtistResult{
		"Resolvable Band": {{ID: "11111111-1111-1111-1111-111111111111", Name: "Resolvable Band", BeginArea: &pipeline.MBArea{Name: "Austin", Type: "City"}, Country: "US"}},
	}}
	sweep := NewArtistLocationSweep(s.db, fakeBandcamp{}, mb)
	sweep.batch = 50
	sweep.reattempt = 30 * 24 * time.Hour

	sweep.RunSweepNow(context.Background())

	var r catalogm.Artist
	s.Require().NoError(s.db.First(&r, resolvable.ID).Error)
	s.Require().NotNil(r.City)
	s.Equal("Austin", *r.City)
	s.NotNil(r.LocationEnrichAttemptedAt, "a filled artist is stamped attempted")
	s.Require().NotNil(r.MusicBrainzArtistID)
	s.Equal("11111111-1111-1111-1111-111111111111", *r.MusicBrainzArtistID) // PSY-1249 MBID rides along

	var u catalogm.Artist
	s.Require().NoError(s.db.First(&u, unresolvable.ID).Error)
	s.Nil(u.City, "an unresolvable artist stays city-less")
	s.NotNil(u.LocationEnrichAttemptedAt, "even a miss is stamped (poison-row safety)")

	// Convergence: a second cycle finds nothing eligible (both within the window). Prove
	// it by blanking the unresolvable's stamp-clock is NOT needed — instead verify the
	// resolvable, if its city is cleared, is NOT re-filled because it's still stamped.
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", resolvable.ID).
		Update("city", nil).Error)
	sweep.RunSweepNow(context.Background())

	var r2 catalogm.Artist
	s.Require().NoError(s.db.First(&r2, resolvable.ID).Error)
	s.Nil(r2.City, "a within-window artist must NOT be re-processed (the memo holds)")
}
