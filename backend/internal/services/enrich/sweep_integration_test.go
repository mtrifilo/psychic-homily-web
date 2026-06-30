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

// Store-level (PSY-1289): the MBID gate selects artists with no musicbrainz_artist_id
// (NULL or TRIM-empty) and excludes those that have one — independent of location, so
// it reaches the located-but-MBID-less rows the city gate can't.
func (s *ArtistLocationSweepIntegrationTestSuite) TestArtistsMissingMBIDGate() {
	mbid := "44444444-4444-4444-4444-444444444444"
	seed := []*catalogm.Artist{
		{Name: "No MBID No City"},                                // NULL MBID → in
		{Name: "No MBID Has City", City: strptr("Phoenix")},      // located but MBID-less → in (the population this gate exists to reach)
		{Name: "Blank MBID", MusicBrainzArtistID: strptr("   ")}, // TRIM-empty → in
		{Name: "Has MBID", MusicBrainzArtistID: &mbid},           // has MBID → out
	}
	for _, a := range seed {
		s.Require().NoError(s.db.Create(a).Error)
	}

	store := &gormArtistStore{db: s.db}
	got, err := store.ArtistsMissingMBID(0)
	s.Require().NoError(err)

	names := map[string]bool{}
	for _, a := range got {
		names[a.Name] = true
	}
	s.True(names["No MBID No City"], "NULL-MBID artist should be selected")
	s.True(names["No MBID Has City"], "located-but-MBID-less artist (the population this gate exists to reach) should be selected")
	s.True(names["Blank MBID"], "TRIM-empty MBID should be selected")
	s.False(names["Has MBID"], "artist with an MBID must be excluded")
	s.Len(got, 3)

	capped, err := store.ArtistsMissingMBID(2)
	s.Require().NoError(err)
	s.Len(capped, 2, "limit caps the batch")
}

// Store-level: the bulk stamp writes the memo column WITHOUT bumping updated_at; empty
// slice is a no-op. The no-bump is load-bearing — the memo marks "we tried", not a
// content edit, and the sweep stamps the whole batch (incl. misses) every cycle.
func (s *ArtistLocationSweepIntegrationTestSuite) TestStampLocationAttempted() {
	a := &catalogm.Artist{Name: "Stamp Me"}
	s.Require().NoError(s.db.Create(a).Error)
	s.Nil(a.LocationEnrichAttemptedAt)

	var before catalogm.Artist
	s.Require().NoError(s.db.First(&before, a.ID).Error)

	store := &gormArtistStore{db: s.db}
	at := time.Now().Truncate(time.Second)
	s.Require().NoError(store.StampLocationAttempted([]uint{a.ID}, at))

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, a.ID).Error)
	s.Require().NotNil(reloaded.LocationEnrichAttemptedAt)
	s.WithinDuration(at, *reloaded.LocationEnrichAttemptedAt, time.Second)
	// The bookkeeping write must NOT bump updated_at (uses Table, not Model).
	s.Equal(before.UpdatedAt.UnixMicro(), reloaded.UpdatedAt.UnixMicro(),
		"StampLocationAttempted must not bump updated_at")

	s.NoError(store.StampLocationAttempted(nil, at), "empty slice is a no-op")
}

// Store-level: NULLS-FIRST-then-stalest ordering under a LIMIT — the production
// convergence property (a bounded nightly batch walks never-tried + stalest rows
// before recently-checked ones).
func (s *ArtistLocationSweepIntegrationTestSuite) TestArtistsNeedingLocationOrdering() {
	older := time.Now().Add(-60 * 24 * time.Hour) // stale, beyond 30d
	newer := time.Now().Add(-45 * 24 * time.Hour) // stale, beyond 30d but newer than older

	never := &catalogm.Artist{Name: "A Never"} // NULL → first
	stalest := &catalogm.Artist{Name: "B Stalest", LocationEnrichAttemptedAt: &older}
	stale := &catalogm.Artist{Name: "C Stale", LocationEnrichAttemptedAt: &newer}
	// Create out of attempt-order to prove the ORDER BY (not insertion/id) decides.
	for _, a := range []*catalogm.Artist{stale, stalest, never} {
		s.Require().NoError(s.db.Create(a).Error)
	}

	store := &gormArtistStore{db: s.db}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	got, err := store.ArtistsNeedingLocation(2, &cutoff) // batch of 2
	s.Require().NoError(err)
	s.Require().Len(got, 2, "LIMIT must bound the batch")
	s.Equal("A Never", got[0].Name, "never-tried (NULLS FIRST) must come first")
	s.Equal("B Stalest", got[1].Name, "stalest attempt must come before the newer one")
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

// PSY-1251 idempotency, end-to-end against real Postgres: the on-create path
// (EnrichArtistLocationByID) stamps the memo, so the sweep's candidate query then
// EXCLUDES that row within the window — no double MB call / double write.
func (s *ArtistLocationSweepIntegrationTestSuite) TestOnCreateStampMakesSweepSkipTheRow() {
	a := &catalogm.Artist{Name: "Just Created"}
	s.Require().NoError(s.db.Create(a).Error)

	// A miss (no MB match, no Bandcamp) → stamps attempted but fills nothing.
	s.Require().NoError(EnrichArtistLocationByID(s.db, fakeBandcamp{}, &fakeMB{}, a.ID))

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, a.ID).Error)
	s.Require().NotNil(reloaded.LocationEnrichAttemptedAt, "on-create enrich must stamp the memo")
	s.Nil(reloaded.City, "a miss must not fill a location")

	// The sweep's candidate query (within the 30d window) now excludes the row.
	store := &gormArtistStore{db: s.db}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	got, err := store.ArtistsNeedingLocation(0, &cutoff)
	s.Require().NoError(err)
	for _, c := range got {
		s.NotEqualf(a.ID, c.ID, "the sweep must skip the just-on-create-stamped artist %d", a.ID)
	}
}
