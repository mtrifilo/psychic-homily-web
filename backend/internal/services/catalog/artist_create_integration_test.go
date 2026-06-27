package catalog

import (
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

func (s *FindOrCreateArtistTestSuite) TestRejectsBlankName() {
	for _, name := range []string{"", "   ", "\t\n"} {
		_, _, err := FindOrCreateArtistTx(s.db, name, nil)
		s.Error(err, "blank/whitespace name %q must be rejected at the funnel boundary", name)
	}
	var count int64
	s.db.Model(&catalogm.Artist{}).Count(&count)
	s.Zero(count, "no artist row should be created for a blank name")
}
