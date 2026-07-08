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
	// Surface a cleanup failure rather than silently polluting the next test's
	// exact-count assertions.
	suite.Require().NoError(suite.db.Exec("DELETE FROM venues").Error)
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

// seedVenueNilSlug inserts a venue with a SQL NULL slug (distinct from "").
func (suite *VenueSlugBackfillIntegrationTestSuite) seedVenueNilSlug(name, city, state string) uint {
	v := &catalogm.Venue{Name: name, City: city, State: state, Slug: nil}
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

// The end-to-end cleanup: corrupted (dropped-tail), empty, and NULL slugs are
// rewritten; a canonical slug AND a renamed venue's deliberately-stable slug are
// left untouched; dry-run writes nothing; and a second live run is a no-op.
func (suite *VenueSlugBackfillIntegrationTestSuite) TestBackfill_RewritesCorruptedSlugs() {
	badID := suite.seedVenue("Valley Bar", "Phoenix", "AZ", "alley-ar-hoenix")
	emptyID := suite.seedVenue("Palo Verde Lounge", "Tempe", "AZ", "")
	nilID := suite.seedVenueNilSlug("Nile Theater", "Mesa", "AZ")
	// Renamed venue: slug reflects the OLD name but is well-formed (has the
	// location tail). UpdateVenue never regenerates it, so it must be left alone.
	renamedID := suite.seedVenue("The Rebel Lounge", "Phoenix", "AZ", "the-rogue-bar-phoenix-az")
	okID := suite.seedVenue("Empty Bottle", "Chicago", "IL", "empty-bottle-chicago-il")

	// Dry-run: reports the three corrupt rows, writes nothing.
	report, err := BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(5, report.Scanned)
	suite.Equal(3, report.Changed)
	suite.Equal(2, report.Unchanged)
	suite.Empty(report.Errors)
	suite.Equal("alley-ar-hoenix", suite.slugOf(badID), "dry-run must not write")
	suite.Equal("", suite.slugOf(emptyID), "dry-run must not write")

	// Live run: applies the changes.
	report, err = BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(3, report.Changed)
	suite.Empty(report.Errors)
	suite.Equal("valley-bar-phoenix-az", suite.slugOf(badID))
	suite.Equal("palo-verde-lounge-tempe-az", suite.slugOf(emptyID))
	suite.Equal("nile-theater-mesa-az", suite.slugOf(nilID))
	suite.Equal("the-rogue-bar-phoenix-az", suite.slugOf(renamedID), "renamed venue's stable slug must be untouched")
	suite.Equal("empty-bottle-chicago-il", suite.slugOf(okID), "canonical slug must be untouched")

	// Idempotent: a second live run changes nothing.
	report, err = BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(0, report.Changed)
	suite.Equal(5, report.Unchanged)
}

// Two venues whose canonical slug collides resolve deterministically: the first
// keeps the base, the second gets a "-2" suffix — and the dry-run PREVIEW must
// match that live outcome (not show two identical slugs). The names differ (so
// the (name, city) composite unique index is satisfied) but normalize to the
// same slug — "The Venue" and "The Venue." both slugify to "the-venue-phoenix-az".
func (suite *VenueSlugBackfillIntegrationTestSuite) TestBackfill_ResolvesCollisionsSequentially() {
	suite.seedVenue("The Venue", "Phoenix", "AZ", "he-venue-hoenix")
	suite.seedVenue("The Venue.", "Phoenix", "AZ", "")

	// Dry-run preview already shows the distinct "-2" resolution a live run applies.
	dry, err := BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Len(dry.Changes, 2)
	previewed := []string{dry.Changes[0].NewSlug, dry.Changes[1].NewSlug}
	suite.ElementsMatch([]string{"the-venue-phoenix-az", "the-venue-phoenix-az-2"}, previewed,
		"dry-run must preview the same distinct slugs the live run assigns")

	report, err := BackfillVenueSlugs(suite.db, VenueSlugBackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(2, report.Changed)
	suite.Empty(report.Errors)

	var slugs []string
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Order("id").Pluck("slug", &slugs).Error)
	suite.ElementsMatch([]string{"the-venue-phoenix-az", "the-venue-phoenix-az-2"}, slugs,
		"live run must assign the two distinct slugs previewed by the dry-run")
}
