package catalog

import (
	"fmt"
	"strings"
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

// caseVariants returns n case-variants of base that all LOWER() to the same key,
// so a concurrent-create collision among them can ONLY be caught by the
// case-insensitive index (artists_lower_name_uniq) — not by any case-sensitive one.
func caseVariants(base string, n int) []string {
	out := make([]string, n)
	for i := range out {
		switch i % 3 {
		case 0:
			out[i] = base
		case 1:
			out[i] = strings.ToLower(base)
		default:
			out[i] = strings.ToUpper(base)
		}
	}
	return out
}

// assertConverges releases len(names) goroutines simultaneously, each find-or-
// creating its (case-variant) name via call, and asserts they converge: exactly one
// row for the lowered key, exactly one created=true, every caller returns the same
// id, none error. Looped by the callers so the unique-violation recovery path is hit
// with overwhelming probability across the run (a single trial only hits it ~once).
func (s *FindOrCreateArtistTestSuite) assertConverges(names []string, call func(name string) (*catalogm.Artist, bool, error)) {
	n := len(names)
	// n is small (6), well under the default connection pool, so no SetMaxOpenConns
	// is needed here; raise the pool if this grows to dozens of goroutines.
	var wg sync.WaitGroup
	ids := make([]uint, n)
	errs := make([]error, n)
	var createdCount int32

	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			a, created, err := call(names[i])
			errs[i] = err
			if err == nil && a != nil {
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
		s.Require().NoError(errs[i], "goroutine %d (name %q) errored", i, names[i])
	}
	var count int64
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("LOWER(name) = LOWER(?)", names[0]).Count(&count).Error)
	s.Equal(int64(1), count, "concurrent case-variant creates must converge to one row")
	for i := 1; i < n; i++ {
		s.Equal(ids[0], ids[i], "all callers must return the same artist id")
	}
	s.Equal(int32(1), createdCount, "exactly one caller creates; the rest converge")
}

// TestConcurrentCreateConverges: mixed-case concurrent creates converge to one row
// via the funnel's recovery path, on the BASE *gorm.DB (standalone-tx insert; the
// path admin/data-sync/seed use). Mixed case ensures the collision is caught by the
// new case-insensitive index, not a case-sensitive one. Looped to reliably exercise
// the unique-violation recovery branch.
func (s *FindOrCreateArtistTestSuite) TestConcurrentCreateConverges() {
	for trial := 0; trial < 8; trial++ {
		names := caseVariants(fmt.Sprintf("Concurrent Band %d", trial), 6)
		s.assertConverges(names, func(name string) (*catalogm.Artist, bool, error) {
			return FindOrCreateArtistTx(s.db, name, nil)
		})
	}
}

// TestConcurrentCreateConvergesInCallerTx: same, but each call runs INSIDE a caller
// transaction, so the recovery uses a SAVEPOINT (RollbackTo) and must leave the
// caller tx healthy enough to commit — the show-import / discovery path. This is the
// branch the savepoint design exists for.
func (s *FindOrCreateArtistTestSuite) TestConcurrentCreateConvergesInCallerTx() {
	for trial := 0; trial < 8; trial++ {
		names := caseVariants(fmt.Sprintf("Concurrent TX Band %d", trial), 6)
		s.assertConverges(names, func(name string) (a *catalogm.Artist, created bool, err error) {
			err = s.db.Transaction(func(tx *gorm.DB) error {
				var e error
				a, created, e = FindOrCreateArtistTx(tx, name, nil)
				return e
			})
			return a, created, err
		})
	}
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
