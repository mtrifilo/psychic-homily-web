package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

type VenueSlugBackfillIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (suite *VenueSlugBackfillIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
}

func (suite *VenueSlugBackfillIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *VenueSlugBackfillIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestVenueSlugBackfillIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(VenueSlugBackfillIntegrationTestSuite))
}

// seedVenue inserts a venue with an explicit (possibly corrupt/empty) slug,
// bypassing the service so we can reproduce the historical bad data.
func (suite *VenueSlugBackfillIntegrationTestSuite) seedVenue(name, city, state, slug string) uint {
	v := &catalogm.Venue{Name: name, City: city, State: state, Slug: &slug}
	suite.Require().NoError(suite.db.Create(v).Error)
	return v.ID
}

func (suite *VenueSlugBackfillIntegrationTestSuite) slugOf(id uint) string {
	var v catalogm.Venue
	suite.Require().NoError(suite.db.First(&v, id).Error)
	if v.Slug == nil {
		return ""
	}
	return *v.Slug
}

// The end-to-end cleanup: corrupted + empty slugs are rewritten, a canonical
// slug is untouched, dry-run writes nothing, and a second live run is a no-op.
func (suite *VenueSlugBackfillIntegrationTestSuite) TestBackfill_RewritesCorruptedSlugs() {
	badID := suite.seedVenue("Valley Bar", "Phoenix", "AZ", "alley-ar-hoenix")
	emptyID := suite.seedVenue("Palo Verde Lounge", "Tempe", "AZ", "")
	okID := suite.seedVenue("Empty Bottle", "Chicago", "IL", "empty-bottle-chicago-il")

	// Dry-run: reports the two changes, writes nothing.
	report, err := BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(3, report.Scanned)
	suite.Equal(2, report.Changed)
	suite.Equal(1, report.Unchanged)
	suite.Empty(report.Errors)
	suite.Equal("alley-ar-hoenix", suite.slugOf(badID), "dry-run must not write")
	suite.Equal("", suite.slugOf(emptyID), "dry-run must not write")

	// Live run: applies the changes.
	report, err = BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(2, report.Changed)
	suite.Empty(report.Errors)
	suite.Equal("valley-bar-phoenix-az", suite.slugOf(badID))
	suite.Equal("palo-verde-lounge-tempe-az", suite.slugOf(emptyID))
	suite.Equal("empty-bottle-chicago-il", suite.slugOf(okID), "canonical slug must be untouched")

	// Idempotent: a second live run changes nothing.
	report, err = BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(0, report.Changed)
	suite.Equal(3, report.Unchanged)
}

// Two venues whose canonical slug collides resolve deterministically in a live
// run: the first keeps the base, the second gets a "-2" suffix. The names differ
// (so the (name, city) composite unique index is satisfied) but normalize to the
// same slug — "The Venue" and "The Venue." both slugify to "the-venue-phoenix-az".
func (suite *VenueSlugBackfillIntegrationTestSuite) TestBackfill_ResolvesCollisionsSequentially() {
	firstID := suite.seedVenue("The Venue", "Phoenix", "AZ", "he-venue-hoenix")
	secondID := suite.seedVenue("The Venue.", "Phoenix", "AZ", "")

	report, err := BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(2, report.Changed)
	suite.Empty(report.Errors)

	first, second := suite.slugOf(firstID), suite.slugOf(secondID)
	suite.Equal("the-venue-phoenix-az", first)
	suite.Equal("the-venue-phoenix-az-2", second)
	suite.NotEqual(first, second, "collision must resolve to distinct slugs")
}
