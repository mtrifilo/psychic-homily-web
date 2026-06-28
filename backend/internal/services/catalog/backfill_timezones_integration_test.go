package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/testutil"
)

// stubGeocoder resolves a fixed set of cities, so the backfill write-path test
// doesn't depend on the embedded GeoNames dataset.
type stubGeocoder struct{ byCity map[string]geo.Result }

func (s stubGeocoder) Resolve(city, _, _ string) (geo.Result, bool) {
	r, ok := s.byCity[city]
	return r, ok
}

// ResolveUSState satisfies geo.Geocoder; this timezone-backfill test never calls
// it, so it always reports "not found".
func (s stubGeocoder) ResolveUSState(string) (string, geo.USStateStatus) {
	return "", geo.USStateNotFound
}

type BackfillIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (suite *BackfillIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
}

func (suite *BackfillIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *BackfillIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestBackfillIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(BackfillIntegrationTestSuite))
}

// seedShow creates a venue (with the given state), an artist, a show stamped at
// `stored`, and the show_venues + show_artists join rows (with the denormalized
// event_date pre-populated to `stored`, as the create path would).
func (suite *BackfillIntegrationTestSuite) seedShow(city, state string, stored time.Time) (venueID, showID, artistID uint) {
	venueSlug := "v-" + city + "-" + state
	venue := &catalogm.Venue{Name: "Venue " + city, Slug: &venueSlug, City: city, State: state}
	suite.Require().NoError(suite.db.Create(venue).Error)

	artistName := "Artist " + city + state
	artistSlug := "a-" + city + state
	artist := &catalogm.Artist{Name: artistName, Slug: &artistSlug}
	suite.Require().NoError(suite.db.Create(artist).Error)

	storedUTC := stored.UTC()
	show := &catalogm.Show{Title: "Show " + city, EventDate: storedUTC, Status: catalogm.ShowStatusApproved}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: artist.ID, EventDate: &storedUTC, VenueID: &venue.ID,
	}).Error)

	return venue.ID, show.ID, artist.ID
}

// A Berlin show stored under the old Phoenix default re-anchors to 18:00Z and
// cascades the corrected instant onto show_artists; a second run is a no-op.
func (suite *BackfillIntegrationTestSuite) TestBackfill_ReanchorsBerlinShowAndCascades() {
	phoenix := mustLoc(suite.T(), "America/Phoenix")
	// 20:00 Phoenix on 2026-07-17 = 03:00Z next day; non-US venue stored with
	// an empty state (so the assumed zone resolves to Phoenix).
	stored := time.Date(2026, 7, 17, 20, 0, 0, 0, phoenix)
	venueID, showID, artistID := suite.seedShow("Berlin", "", stored)

	stub := stubGeocoder{byCity: map[string]geo.Result{
		"Berlin": {Latitude: 52.52, Longitude: 13.405, Timezone: "Europe/Berlin"},
	}}

	// Dry-run: reports the change but writes nothing.
	report, err := BackfillVenueTimezones(suite.db, stub, BackfillOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(1, report.ShowsReanchored)
	suite.Empty(report.Errors)

	var showAfterDry catalogm.Show
	suite.Require().NoError(suite.db.First(&showAfterDry, showID).Error)
	suite.True(showAfterDry.EventDate.Equal(stored), "dry-run must not write event_date")

	// Confirm: writes venue tz + re-anchored instant + cascaded show_artists.
	report, err = BackfillVenueTimezones(suite.db, stub, BackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(1, report.ShowsReanchored)
	suite.Equal(1, report.VenuesSet)
	suite.Empty(report.Errors)

	want := "2026-07-17T18:00:00Z" // 20:00 Europe/Berlin (CEST, UTC+2)

	var venue catalogm.Venue
	suite.Require().NoError(suite.db.First(&venue, venueID).Error)
	suite.Require().NotNil(venue.Timezone)
	suite.Equal("Europe/Berlin", *venue.Timezone)

	var show catalogm.Show
	suite.Require().NoError(suite.db.First(&show, showID).Error)
	suite.Equal(want, show.EventDate.UTC().Format(time.RFC3339))

	var sa catalogm.ShowArtist
	suite.Require().NoError(suite.db.Where("show_id = ? AND artist_id = ?", showID, artistID).First(&sa).Error)
	suite.Require().NotNil(sa.EventDate)
	suite.Equal(want, sa.EventDate.UTC().Format(time.RFC3339), "show_artists.event_date must be cascaded")

	// Idempotent: a second confirm changes nothing.
	report, err = BackfillVenueTimezones(suite.db, stub, BackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(0, report.ShowsReanchored)
	suite.Equal(0, report.VenuesSet)
	suite.Equal(0, report.VenuesUpdated)
}

// A correctly-stored explicit-time US show must NOT be re-anchored — the
// regression guard for the adversarial-review CRITICAL finding, exercised
// through the real assumed-zone derivation (full StateTimezones map).
func (suite *BackfillIntegrationTestSuite) TestBackfill_DoesNotCorruptExplicitUSShow() {
	ny := mustLoc(suite.T(), "America/New_York")
	// A real 11pm Eastern show: 23:00 EDT = 03:00Z, which is also 20:00 Phoenix.
	stored := time.Date(2026, 7, 17, 23, 0, 0, 0, ny)
	_, showID, _ := suite.seedShow("Miami", "FL", stored)

	stub := stubGeocoder{byCity: map[string]geo.Result{
		"Miami": {Latitude: 25.7617, Longitude: -80.1918, Timezone: "America/New_York"},
	}}

	report, err := BackfillVenueTimezones(suite.db, stub, BackfillOptions{DryRun: false})
	suite.Require().NoError(err)
	suite.Equal(0, report.ShowsReanchored, "explicit-time FL show must not be re-anchored")
	suite.Equal(1, report.ShowsAmbiguous)

	var show catalogm.Show
	suite.Require().NoError(suite.db.First(&show, showID).Error)
	suite.Equal("2026-07-18T03:00:00Z", show.EventDate.UTC().Format(time.RFC3339),
		"event_date must be left exactly as stored")
}
