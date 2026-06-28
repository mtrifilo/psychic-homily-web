package catalog

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// FindOrCreateArtistTestSuite covers the single artist write funnel (PSY-1254):
// dedup-by-name, unique slug, apply-on-create-only, and missing-slug backfill.
type FindOrCreateArtistTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *FindOrCreateArtistTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}
func (s *FindOrCreateArtistTestSuite) TearDownSuite() { s.testDB.Cleanup() }
func (s *FindOrCreateArtistTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artists")
}
func TestFindOrCreateArtistTestSuite(t *testing.T) { suite.Run(t, new(FindOrCreateArtistTestSuite)) }

func (s *FindOrCreateArtistTestSuite) TestCreatesWhenAbsent() {
	a, created, err := FindOrCreateArtistTx(s.db, "Sleep", nil)
	s.Require().NoError(err)
	s.True(created)
	s.NotZero(a.ID)
	s.Require().NotNil(a.Slug)
	s.NotEmpty(*a.Slug)

	var count int64
	s.db.Model(&catalogm.Artist{}).Where("name = ?", "Sleep").Count(&count)
	s.Equal(int64(1), count)
}

func (s *FindOrCreateArtistTestSuite) TestFindsExistingCaseInsensitiveNoDup() {
	first, created, err := FindOrCreateArtistTx(s.db, "Boris", nil)
	s.Require().NoError(err)
	s.True(created)

	again, created2, err := FindOrCreateArtistTx(s.db, "boris", nil) // different case ⇒ same artist
	s.Require().NoError(err)
	s.False(created2)
	s.Equal(first.ID, again.ID)

	var count int64
	s.db.Model(&catalogm.Artist{}).Count(&count)
	s.Equal(int64(1), count, "case-insensitive match must not duplicate")
}

func (s *FindOrCreateArtistTestSuite) TestBackfillsMissingSlugOnFound() {
	// Seed an artist with NO slug (a legacy/bypass insert) directly.
	seed := &catalogm.Artist{Name: "Earth"}
	s.Require().NoError(s.db.Create(seed).Error)
	s.Require().Nil(seed.Slug)

	got, created, err := FindOrCreateArtistTx(s.db, "Earth", nil)
	s.Require().NoError(err)
	s.False(created)
	s.Equal(seed.ID, got.ID)
	s.Require().NotNil(got.Slug)
	s.NotEmpty(*got.Slug)

	var reloaded catalogm.Artist
	s.Require().NoError(s.db.First(&reloaded, seed.ID).Error)
	s.Require().NotNil(reloaded.Slug, "slug must be persisted, not just set in memory")
}

func (s *FindOrCreateArtistTestSuite) TestApplyRunsOnlyOnCreate() {
	city := "Olympia"
	a, created, err := FindOrCreateArtistTx(s.db, "Unwound", func(ar *catalogm.Artist) { ar.City = &city })
	s.Require().NoError(err)
	s.True(created)
	s.Require().NotNil(a.City)
	s.Equal("Olympia", *a.City)

	// Second call FINDS the artist ⇒ apply must NOT run (no overwrite).
	other := "Nowhere"
	again, created2, err := FindOrCreateArtistTx(s.db, "Unwound", func(ar *catalogm.Artist) { ar.City = &other })
	s.Require().NoError(err)
	s.False(created2)
	s.Require().NotNil(again.City)
	s.Equal("Olympia", *again.City, "apply must not run on an existing artist")
}

// TestConcurrentCreateConverges: N goroutines find-or-create the SAME name at once.
// The case-insensitive unique index (artists_lower_name_uniq) serializes the
// inserts; the funnel's conflict-safe recovery makes the losers re-select and
// return the winner. Invariant: exactly one row, exactly one created=true, and
// every caller converges to the same id with no error.
func (s *FindOrCreateArtistTestSuite) TestConcurrentCreateConverges() {
	const n = 8
	var wg sync.WaitGroup
	ids := make([]uint, n)
	errs := make([]error, n)
	var createdCount int32

	// Release all goroutines at once so their SELECTs run before any INSERT commits,
	// reliably forcing the losers down the unique-violation recovery path (not just
	// the SELECT-found path).
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			a, created, err := FindOrCreateArtistTx(s.db, "Concurrent Band", nil)
			errs[i] = err
			if err == nil {
				ids[i] = a.ID
			}
			if created {
				atomic.AddInt32(&createdCount, 1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < n; i++ {
		s.Require().NoError(errs[i], "goroutine %d errored", i)
	}
	var count int64
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("LOWER(name) = LOWER(?)", "Concurrent Band").Count(&count).Error)
	s.Equal(int64(1), count, "concurrent same-name creates must converge to one row")
	for i := 1; i < n; i++ {
		s.Equal(ids[0], ids[i], "all callers must return the same artist id")
	}
	s.Equal(int32(1), createdCount, "exactly one caller creates; the rest converge as found")
}

func (s *FindOrCreateArtistTestSuite) TestRejectsBlankName() {
	for _, name := range []string{"", "   ", "\t\n"} {
		_, _, err := FindOrCreateArtistTx(s.db, name, nil)
		s.Error(err, "blank/whitespace name %q must be rejected at the funnel boundary", name)
	}
	var count int64
	s.db.Model(&catalogm.Artist{}).Count(&count)
	s.Zero(count, "no artist row should be created for a blank name")
}
