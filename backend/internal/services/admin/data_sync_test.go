package admin

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type DataSyncServiceIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *DataSyncService
}

func (suite *DataSyncServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.service = NewDataSyncService(suite.testDB.DB)
}

func (suite *DataSyncServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *DataSyncServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestDataSyncServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(DataSyncServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) createVenue(name, city, state string, verified bool) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(name, " ", "-")), strings.ToLower(city))
	venue := &catalogm.Venue{
		Name:     name,
		Slug:     &slug,
		City:     city,
		State:    state,
		Verified: verified,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *DataSyncServiceIntegrationTestSuite) createArtist(name string) *catalogm.Artist {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	artist := &catalogm.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataSyncServiceIntegrationTestSuite) createArtistWithSocial(name string, instagram *string) *catalogm.Artist {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	artist := &catalogm.Artist{
		Name: name,
		Slug: &slug,
		Social: catalogm.Social{
			Instagram: instagram,
		},
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataSyncServiceIntegrationTestSuite) createVenueWithSocial(name, city, state string, instagram *string) *catalogm.Venue {
	slug := fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(name, " ", "-")), strings.ToLower(city))
	venue := &catalogm.Venue{
		Name:     name,
		Slug:     &slug,
		City:     city,
		State:    state,
		Verified: true,
		Social: catalogm.Social{
			Instagram: instagram,
		},
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *DataSyncServiceIntegrationTestSuite) createShow(title string, eventDate time.Time, status catalogm.ShowStatus, venue *catalogm.Venue, artists ...*catalogm.Artist) *catalogm.Show {
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	city := "NYC"
	state := "NY"
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: eventDate.UTC(),
		City:      &city,
		State:     &state,
		Status:    status,
		Source:    catalogm.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	if venue != nil {
		sv := catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}
		suite.db.Create(&sv)
	}

	for i, artist := range artists {
		sa := catalogm.ShowArtist{
			ShowID:   show.ID,
			ArtistID: artist.ID,
			Position: i,
			SetType:  "performer",
		}
		suite.db.Create(&sa)
	}

	return show
}

func dssBoolPtr(b bool) *bool {
	return &b
}

// =============================================================================
// ExportShows Tests
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_Empty() {
	result, err := suite.service.ExportShows(contracts.ExportShowsParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Empty(result.Shows)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_DefaultLimit() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	// Create 60 shows
	for i := 0; i < 60; i++ {
		suite.createShow(fmt.Sprintf("Show %d", i), time.Now().Add(time.Duration(i)*time.Hour), catalogm.ShowStatusApproved, venue)
	}

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "all"})
	suite.Require().NoError(err)
	suite.Equal(int64(60), result.Total)
	suite.Len(result.Shows, 50) // Default limit
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_MaxLimit() {
	result, err := suite.service.ExportShows(contracts.ExportShowsParams{Limit: 500, Status: "all"})
	suite.Require().NoError(err)
	// Limit capped to 200 (no data, but verifies no error)
	suite.Equal(int64(0), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_StatusFilter_Approved() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Approved 1", time.Now(), catalogm.ShowStatusApproved, venue)
	suite.createShow("Approved 2", time.Now(), catalogm.ShowStatusApproved, venue)
	suite.createShow("Pending 1", time.Now(), catalogm.ShowStatusPending, venue)

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "approved"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
	suite.Len(result.Shows, 2)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_StatusFilter_Pending() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Approved 1", time.Now(), catalogm.ShowStatusApproved, venue)
	suite.createShow("Pending 1", time.Now(), catalogm.ShowStatusPending, venue)

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "pending"})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_StatusFilter_All() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Approved", time.Now(), catalogm.ShowStatusApproved, venue)
	suite.createShow("Pending", time.Now().Add(time.Hour), catalogm.ShowStatusPending, venue)
	suite.createShow("Rejected", time.Now().Add(2*time.Hour), catalogm.ShowStatusRejected, venue)

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "all"})
	suite.Require().NoError(err)
	suite.Equal(int64(3), result.Total)
	suite.Len(result.Shows, 3)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_DateFilter() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Old Show", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), catalogm.ShowStatusApproved, venue)
	suite.createShow("New Show", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), catalogm.ShowStatusApproved, venue)

	fromDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result, err := suite.service.ExportShows(contracts.ExportShowsParams{
		Status:   "approved",
		FromDate: &fromDate,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
	suite.Equal("New Show", result.Shows[0].Title)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_LocationFilter() {
	venueNYC := suite.createVenue("NYC Venue", "New York", "NY", true)
	venueLA := suite.createVenue("LA Venue", "Los Angeles", "CA", true)

	showNYC := suite.createShow("NYC Show", time.Now(), catalogm.ShowStatusApproved, venueNYC)
	suite.createShow("LA Show", time.Now(), catalogm.ShowStatusApproved, venueLA)
	// Shows have city set from the createShow helper
	_ = showNYC

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{
		Status: "approved",
		City:   "NYC",
	})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_WithArtistsAndVenues() {
	venue := suite.createVenue("The Hall", "NYC", "NY", true)
	artist1 := suite.createArtist("Band One")
	artist2 := suite.createArtist("Band Two")
	suite.createShow("Big Show", time.Now(), catalogm.ShowStatusApproved, venue, artist1, artist2)

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "all"})
	suite.Require().NoError(err)
	suite.Require().Len(result.Shows, 1)

	show := result.Shows[0]
	suite.Equal("Big Show", show.Title)
	suite.Require().Len(show.Venues, 1)
	suite.Equal("The Hall", show.Venues[0].Name)
	suite.Require().Len(show.Artists, 2)
	// Check artist names exist (order from DB join may vary)
	artistNames := []string{show.Artists[0].Name, show.Artists[1].Name}
	suite.Contains(artistNames, "Band One")
	suite.Contains(artistNames, "Band Two")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_Pagination() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	for i := 0; i < 5; i++ {
		suite.createShow(fmt.Sprintf("Show %d", i), time.Now().Add(time.Duration(i)*time.Hour), catalogm.ShowStatusApproved, venue)
	}

	result, err := suite.service.ExportShows(contracts.ExportShowsParams{
		Status: "all",
		Limit:  2,
		Offset: 2,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(5), result.Total)
	suite.Len(result.Shows, 2)
}

// =============================================================================
// ExportArtists Tests
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_Empty() {
	result, err := suite.service.ExportArtists(contracts.ExportArtistsParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Empty(result.Artists)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_DefaultLimit() {
	for i := 0; i < 60; i++ {
		suite.createArtist(fmt.Sprintf("Artist %03d", i))
	}

	result, err := suite.service.ExportArtists(contracts.ExportArtistsParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(60), result.Total)
	suite.Len(result.Artists, 50) // Default limit
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_Search() {
	suite.createArtist("The Band")
	suite.createArtist("Another Band")
	suite.createArtist("Solo Singer")

	result, err := suite.service.ExportArtists(contracts.ExportArtistsParams{Search: "band"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_Pagination() {
	for i := 0; i < 5; i++ {
		suite.createArtist(fmt.Sprintf("Artist %d", i))
	}

	result, err := suite.service.ExportArtists(contracts.ExportArtistsParams{Limit: 2, Offset: 2})
	suite.Require().NoError(err)
	suite.Equal(int64(5), result.Total)
	suite.Len(result.Artists, 2)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_WithSocial() {
	insta := "@theband"
	suite.createArtistWithSocial("The Band", &insta)

	result, err := suite.service.ExportArtists(contracts.ExportArtistsParams{})
	suite.Require().NoError(err)
	suite.Require().Len(result.Artists, 1)
	suite.Equal("The Band", result.Artists[0].Name)
	suite.NotNil(result.Artists[0].Instagram)
	suite.Equal("@theband", *result.Artists[0].Instagram)
}

// =============================================================================
// ExportVenues Tests
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_Empty() {
	result, err := suite.service.ExportVenues(contracts.ExportVenuesParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Empty(result.Venues)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_DefaultLimit() {
	for i := 0; i < 60; i++ {
		suite.createVenue(fmt.Sprintf("Venue %03d", i), "NYC", "NY", true)
	}

	result, err := suite.service.ExportVenues(contracts.ExportVenuesParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(60), result.Total)
	suite.Len(result.Venues, 50) // Default limit
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_Search() {
	suite.createVenue("Music Hall", "NYC", "NY", true)
	suite.createVenue("Concert Hall", "LA", "CA", true)
	suite.createVenue("The Dive Bar", "CHI", "IL", true)

	result, err := suite.service.ExportVenues(contracts.ExportVenuesParams{Search: "hall"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_FilterVerified() {
	suite.createVenue("Verified 1", "NYC", "NY", true)
	suite.createVenue("Verified 2", "LA", "CA", true)
	suite.createVenue("Unverified", "CHI", "IL", false)

	result, err := suite.service.ExportVenues(contracts.ExportVenuesParams{Verified: dssBoolPtr(true)})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_FilterCity() {
	suite.createVenue("Venue NYC", "New York", "NY", true)
	suite.createVenue("Venue LA", "Los Angeles", "CA", true)

	result, err := suite.service.ExportVenues(contracts.ExportVenuesParams{City: "New York"})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
	suite.Equal("Venue NYC", result.Venues[0].Name)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_WithSocial() {
	insta := "@thevenue"
	suite.createVenueWithSocial("The Venue", "NYC", "NY", &insta)

	result, err := suite.service.ExportVenues(contracts.ExportVenuesParams{})
	suite.Require().NoError(err)
	suite.Require().Len(result.Venues, 1)
	suite.NotNil(result.Venues[0].Instagram)
	suite.Equal("@thevenue", *result.Venues[0].Instagram)
}

// =============================================================================
// ImportData Tests — Artists
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_EmptyRequest() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{})
	suite.Require().NoError(err)
	suite.Equal(0, result.Shows.Total)
	suite.Equal(0, result.Artists.Total)
	suite.Equal(0, result.Venues.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_Success() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: "New Band"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Imported)
	suite.Contains(result.Artists.Messages[0], "IMPORTED")

	// Verify created with slug
	var artist catalogm.Artist
	err = suite.db.Where("name = ?", "New Band").First(&artist).Error
	suite.Require().NoError(err)
	suite.NotNil(artist.Slug)
}

// The import body is JSON an admin hands us, not a value this system wrote, so
// bandcamp_embed_url meets the same release-page rule the artist endpoints
// enforce (PSY-1966). The row is refused rather than the field silently blanked:
// a blanked field would report the artist as imported while losing what the
// operator meant to move.
// One representative value: the accepted and rejected shapes are settled by the
// predicate's own table (utils.TestIsValidBandcampEmbedURL). What this proves is
// that the import path consults it at all, and what happens to the row when it
// refuses.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_RejectsNonBandcampEmbedURL() {
	bad := "https://evil.test/album/checkout"
	name := fmt.Sprintf("Import Guard %d", time.Now().UnixNano())
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{{Name: name, BandcampEmbedURL: &bad}},
	})
	suite.Require().NoError(err)
	suite.Equal(0, result.Artists.Imported)
	suite.Equal(1, result.Artists.Errors)
	suite.Contains(result.Artists.Messages[0], "Bandcamp embed URL")

	var count int64
	suite.Require().NoError(suite.db.Model(&catalogm.Artist{}).
		Where("name = ?", name).Count(&count).Error)
	suite.Zero(count, "a refused row must not create the artist")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_AcceptsBandcampReleaseURL() {
	release := "https://kingbuffalo.bandcamp.com/album/regenerator"
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{{Name: "Import Guard OK", BandcampEmbedURL: &release}},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Imported)

	var artist catalogm.Artist
	suite.Require().NoError(suite.db.Where("name = ?", "Import Guard OK").First(&artist).Error)
	suite.Require().NotNil(artist.BandcampEmbedURL)
	suite.Equal(release, *artist.BandcampEmbedURL)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_Duplicate() {
	suite.createArtist("Existing Band")

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: "Existing Band"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Duplicates)
	suite.Contains(result.Artists.Messages[0], "DUPLICATE")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_DuplicateBackfillSlug() {
	// Create artist WITHOUT a slug
	artist := &catalogm.Artist{Name: "No Slug Band"}
	suite.db.Create(artist)
	suite.Nil(artist.Slug)

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: "No Slug Band"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Duplicates)

	// Verify slug was backfilled
	var updated catalogm.Artist
	suite.db.First(&updated, artist.ID)
	suite.NotNil(updated.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_EmptyName() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: ""},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Errors)
	suite.Contains(result.Artists.Messages[0], "SKIP")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_DryRun() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: "Dry Run Band"},
		},
		DryRun: true,
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Imported)
	suite.Contains(result.Artists.Messages[0], "WOULD IMPORT")

	// Verify NOT actually created
	var count int64
	suite.db.Model(&catalogm.Artist{}).Where("name = ?", "Dry Run Band").Count(&count)
	suite.Equal(int64(0), count)
}

// =============================================================================
// ImportData Tests — Venues
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_Success() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{
			{Name: "New Venue", City: "NYC", State: "NY"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Imported)
	suite.Contains(result.Venues.Messages[0], "IMPORTED")

	// Verify created with slug
	var venue catalogm.Venue
	err = suite.db.Where("name = ?", "New Venue").First(&venue).Error
	suite.Require().NoError(err)
	suite.NotNil(venue.Slug)
}

// TestImportVenue_Geocodes verifies PSY-985: imported venues are geocoded so
// timezone/coordinates are populated like the VenueService create path.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_Geocodes() {
	_, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{
			{Name: "Geocoded Venue", City: "Phoenix", State: "AZ"},
		},
	})
	suite.Require().NoError(err)

	var venue catalogm.Venue
	suite.Require().NoError(suite.db.Where("name = ?", "Geocoded Venue").First(&venue).Error)
	suite.Require().NotNil(venue.Timezone, "imported venue should be geocoded")
	suite.Equal("America/Phoenix", *venue.Timezone)
	suite.Require().NotNil(venue.Latitude)
	suite.Require().NotNil(venue.Longitude)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_Duplicate() {
	suite.createVenue("Existing Venue", "NYC", "NY", true)

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{
			{Name: "Existing Venue", City: "NYC", State: "NY"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Duplicates)
	suite.Contains(result.Venues.Messages[0], "DUPLICATE")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_MissingFields() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{
			{Name: "Venue Only"}, // Missing city and state
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Errors)
	suite.Contains(result.Venues.Messages[0], "SKIP")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_DryRun() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{
			{Name: "Dry Run Venue", City: "NYC", State: "NY"},
		},
		DryRun: true,
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Imported)
	suite.Contains(result.Venues.Messages[0], "WOULD IMPORT")

	var count int64
	suite.db.Model(&catalogm.Venue{}).Where("name = ?", "Dry Run Venue").Count(&count)
	suite.Equal(int64(0), count)
}

// =============================================================================
// ImportData Tests — Shows
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_Success() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "New Show",
				EventDate: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Test Venue", City: "NYC", State: "NY"}},
				Artists:   []contracts.ExportedShowArtist{{Name: "Test Band", Position: 0, SetType: "performer"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)
	suite.Contains(result.Shows.Messages[0], "IMPORTED")

	// Verify show, venue, and artist all created
	var show catalogm.Show
	err = suite.db.Where("title = ?", "New Show").First(&show).Error
	suite.Require().NoError(err)
	suite.NotNil(show.Slug)

	var venue catalogm.Venue
	err = suite.db.Where("name = ?", "Test Venue").First(&venue).Error
	suite.Require().NoError(err)

	var artist catalogm.Artist
	err = suite.db.Where("name = ?", "Test Band").First(&artist).Error
	suite.Require().NoError(err)
}

// PSY-1959: the persisted slug must name the act the export CURATED as the
// headliner, not whichever act happens to hold position 0. A slug does not
// regenerate, so a position-only read writes the wrong act down permanently.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_SlugNamesTheCuratedHeadliner() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Curated Slug Show",
				EventDate: time.Date(2027, 5, 12, 20, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Curated Slug Venue", City: "NYC", State: "NY"}},
				Artists: []contracts.ExportedShowArtist{
					{Name: "Slug Opener", Position: 0, SetType: "opener"},
					{Name: "Slug Headliner", Position: 1, SetType: "headliner"},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().Equal(1, result.Shows.Imported, result.Shows.Messages)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("title = ?", "Curated Slug Show").First(&show).Error)
	suite.Require().NotNil(show.Slug)
	suite.Contains(*show.Slug, "slug-headliner", "the curated headliner names the slug")
	suite.NotContains(*show.Slug, "slug-opener")
}

// backfillShowSlugs is the second persisted-slug writer in this file and runs on
// the DUPLICATE branch, filling a slug an existing show never got. It ranks the
// same way importShow does, so the act named there is the curated headliner.
func (suite *DataSyncServiceIntegrationTestSuite) TestBackfillShowSlugs_NamesTheCuratedHeadliner() {
	venue := suite.createVenue("Backfill Slug Venue", "NYC", "NY", true)
	eventDate := time.Date(2027, 4, 22, 20, 0, 0, 0, time.UTC)
	existing := suite.createShow("Backfill Slug Show", eventDate, catalogm.ShowStatusApproved, venue)

	// The show exists with no slug, so re-importing it takes the duplicate
	// branch and backfills instead of creating.
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).
		Where("id = ?", existing.ID).Update("slug", nil).Error)

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Backfill Slug Show",
				EventDate: eventDate.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Backfill Slug Venue", City: "NYC", State: "NY"}},
				Artists: []contracts.ExportedShowArtist{
					{Name: "Backfill Opener", Position: 0, SetType: "opener"},
					{Name: "Backfill Headliner", Position: 1, SetType: "headliner"},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().Equal(1, result.Shows.Duplicates, result.Shows.Messages)

	var backfilled catalogm.Show
	suite.Require().NoError(suite.db.First(&backfilled, existing.ID).Error)
	suite.Require().NotNil(backfilled.Slug)
	suite.Contains(*backfilled.Slug, "backfill-headliner", "the curated headliner names the backfilled slug")
	suite.NotContains(*backfilled.Slug, "backfill-opener")
}

// The other half of the rule: an export that curates nobody still dates its slug
// from the lowest position, so an uncurated import is unchanged.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_UncuratedSlugStillNamesLowestPosition() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Uncurated Slug Show",
				EventDate: time.Date(2027, 5, 13, 20, 0, 0, 0, time.UTC).Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Uncurated Slug Venue", City: "NYC", State: "NY"}},
				Artists: []contracts.ExportedShowArtist{
					{Name: "Uncurated Second", Position: 1, SetType: "performer"},
					{Name: "Uncurated First", Position: 0, SetType: "performer"},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().Equal(1, result.Shows.Imported, result.Shows.Messages)

	var show catalogm.Show
	suite.Require().NoError(suite.db.Where("title = ?", "Uncurated Slug Show").First(&show).Error)
	suite.Require().NotNil(show.Slug)
	suite.Contains(*show.Slug, "uncurated-first", "position, not list order, decides an uncurated bill")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_Duplicate() {
	venue := suite.createVenue("Dupe Venue", "NYC", "NY", true)
	eventDate := time.Date(2025, 6, 15, 20, 0, 0, 0, time.UTC)
	suite.createShow("Dupe Show", eventDate, catalogm.ShowStatusApproved, venue)

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Dupe Show",
				EventDate: eventDate.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Dupe Venue", City: "NYC", State: "NY"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Duplicates)
	suite.Contains(result.Shows.Messages[0], "DUPLICATE")
}

// The import's title+venue duplicate gate is scoped to (name, city), the pair
// venues are unique on and the pair FindOrCreateVenue resolves below it. A tour
// playing the same-named room in two cities on one night is two shows.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_SameNamedVenueInAnotherCityIsNotADuplicate() {
	venue := suite.createVenue("Twin Name Room", "NYC", "NY", true)
	eventDate := time.Date(2027, 4, 9, 20, 0, 0, 0, time.UTC)
	suite.createShow("Twin Name Show", eventDate, catalogm.ShowStatusApproved, venue)

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Twin Name Show",
				EventDate: eventDate.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Twin Name Room", City: "Boston", State: "MA"}},
				Artists:   []contracts.ExportedShowArtist{{Name: "Twin Name Act", Position: 0, SetType: "headliner"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(0, result.Shows.Duplicates, result.Shows.Messages)
	suite.Equal(1, result.Shows.Imported, result.Shows.Messages)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_SameDayDifferentTime_NotDuplicate() {
	// A matinee and an evening show with the same title+venue on the same calendar
	// day differ only in time-of-day. They are distinct shows, not duplicates: the
	// dedup gate keys on the full event_date timestamp, so the second import must be
	// created rather than skipped.
	venue := suite.createVenue("Matinee Venue", "NYC", "NY", true)
	matinee := time.Date(2025, 6, 15, 15, 0, 0, 0, time.UTC)
	evening := time.Date(2025, 6, 15, 21, 0, 0, 0, time.UTC)
	existing := suite.createShow("Same Day Show", matinee, catalogm.ShowStatusApproved, venue)

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Same Day Show",
				EventDate: evening.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Matinee Venue", City: "NYC", State: "NY"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)
	suite.Equal(0, result.Shows.Duplicates)

	// Both the matinee and the newly imported evening show persist as distinct rows.
	var count int64
	suite.db.Model(&catalogm.Show{}).Where("title = ?", "Same Day Show").Count(&count)
	suite.Equal(int64(2), count)

	var eveningShow catalogm.Show
	err = suite.db.Where("title = ? AND event_date = ?", "Same Day Show", evening).First(&eveningShow).Error
	suite.Require().NoError(err)
	suite.NotEqual(existing.ID, eveningShow.ID)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_DuplicateBackfillSlugs() {
	// Create entities without slugs
	venue := &catalogm.Venue{Name: "No Slug Venue", City: "NYC", State: "NY", Verified: true}
	suite.db.Create(venue)
	artist := &catalogm.Artist{Name: "No Slug Artist"}
	suite.db.Create(artist)

	eventDate := time.Date(2025, 7, 1, 20, 0, 0, 0, time.UTC)
	show := &catalogm.Show{
		Title:     "No Slug Show",
		EventDate: eventDate,
		Status:    catalogm.ShowStatusApproved,
		Source:    catalogm.ShowSourceUser,
	}
	suite.db.Create(show)
	suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID})
	suite.db.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0, SetType: "performer"})

	// Import duplicate — should backfill slugs
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "No Slug Show",
				EventDate: eventDate.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "No Slug Venue", City: "NYC", State: "NY"}},
				Artists:   []contracts.ExportedShowArtist{{Name: "No Slug Artist", Position: 0, SetType: "performer"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Duplicates)

	// Verify slugs backfilled
	var updatedShow catalogm.Show
	suite.db.First(&updatedShow, show.ID)
	suite.NotNil(updatedShow.Slug)

	var updatedVenue catalogm.Venue
	suite.db.First(&updatedVenue, venue.ID)
	suite.NotNil(updatedVenue.Slug)

	var updatedArtist catalogm.Artist
	suite.db.First(&updatedArtist, artist.ID)
	suite.NotNil(updatedArtist.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_MissingFields() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{Title: "", EventDate: ""},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Errors)
	suite.Contains(result.Shows.Messages[0], "SKIP")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_InvalidDate() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{Title: "Bad Date Show", EventDate: "not-a-date"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Errors)
	suite.Contains(result.Shows.Messages[0], "ERROR")
	suite.Contains(result.Shows.Messages[0], "Invalid event date")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_CreatesNewVenueAndArtist() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Show With New Everything",
				EventDate: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Brand New Venue", City: "Portland", State: "OR"}},
				Artists:   []contracts.ExportedShowArtist{{Name: "Brand New Band", Position: 0, SetType: "headliner"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)

	// Verify venue created with slug
	var venue catalogm.Venue
	err = suite.db.Where("name = ?", "Brand New Venue").First(&venue).Error
	suite.Require().NoError(err)
	suite.NotNil(venue.Slug)
	suite.Equal("Portland", venue.City)

	// Verify artist created with slug
	var artist catalogm.Artist
	err = suite.db.Where("name = ?", "Brand New Band").First(&artist).Error
	suite.Require().NoError(err)
	suite.NotNil(artist.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_DryRun() {
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Dry Run Show",
				EventDate: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
				Status:    "approved",
				Venues:    []contracts.ExportedVenue{{Name: "Dry Venue", City: "NYC", State: "NY"}},
			},
		},
		DryRun: true,
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)
	suite.Contains(result.Shows.Messages[0], "WOULD IMPORT")

	var count int64
	suite.db.Model(&catalogm.Show{}).Where("title = ?", "Dry Run Show").Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_StatusParsing() {
	eventDates := []time.Time{
		time.Now().Add(24 * time.Hour),
		time.Now().Add(48 * time.Hour),
		time.Now().Add(72 * time.Hour),
	}

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{
			{
				Title:     "Pending Import",
				EventDate: eventDates[0].UTC().Format(time.RFC3339),
				Status:    "pending",
				Venues:    []contracts.ExportedVenue{{Name: "Venue A", City: "NYC", State: "NY"}},
			},
			{
				Title:     "Rejected Import",
				EventDate: eventDates[1].UTC().Format(time.RFC3339),
				Status:    "rejected",
				Venues:    []contracts.ExportedVenue{{Name: "Venue B", City: "LA", State: "CA"}},
			},
			{
				Title:     "Private Import",
				EventDate: eventDates[2].UTC().Format(time.RFC3339),
				Status:    "private",
				Venues:    []contracts.ExportedVenue{{Name: "Venue C", City: "CHI", State: "IL"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(3, result.Shows.Imported)

	var pendingShow catalogm.Show
	suite.db.Where("title = ?", "Pending Import").First(&pendingShow)
	suite.Equal(catalogm.ShowStatusPending, pendingShow.Status)

	var rejectedShow catalogm.Show
	suite.db.Where("title = ?", "Rejected Import").First(&rejectedShow)
	suite.Equal(catalogm.ShowStatusRejected, rejectedShow.Status)

	var privateShow catalogm.Show
	suite.db.Where("title = ?", "Private Import").First(&privateShow)
	suite.Equal(catalogm.ShowStatusPrivate, privateShow.Status)
}

// =============================================================================
// Full Round-Trip Test
// =============================================================================

// TestImportData_RoundTripsShowTimes pins doors_at/music_at through the
// export/import pair, whose entire purpose is fidelity. Without it either
// mapping line can be deleted and the suite stays green while a stage-to-prod
// sync silently nulls admin-entered show times.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_RoundTripsShowTimes() {
	venue := suite.createVenue("Times Venue", "NYC", "NY", true)
	artist := suite.createArtist("Times Band")
	eventDate := time.Date(2025, 8, 1, 20, 0, 0, 0, time.UTC)
	show := suite.createShow("Times Show", eventDate, catalogm.ShowStatusApproved, venue, artist)

	doors := eventDate.Add(-time.Hour)
	music := eventDate
	suite.Require().NoError(suite.db.Model(show).
		Updates(map[string]interface{}{"doors_at": doors, "music_at": music}).Error)

	exportResult, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "approved"})
	suite.Require().NoError(err)
	suite.Require().Len(exportResult.Shows, 1)
	suite.Require().NotNil(exportResult.Shows[0].DoorsAt, "export must carry doorsAt")
	suite.Require().NotNil(exportResult.Shows[0].MusicAt, "export must carry musicAt")

	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")

	importResult, err := suite.service.ImportData(contracts.DataImportRequest{Shows: exportResult.Shows})
	suite.Require().NoError(err)
	suite.Equal(1, importResult.Shows.Imported)

	var reimported catalogm.Show
	suite.Require().NoError(suite.db.Where("title = ?", "Times Show").First(&reimported).Error)
	suite.Require().NotNil(reimported.DoorsAt, "import must land doors_at")
	suite.Require().NotNil(reimported.MusicAt, "import must land music_at")
	suite.Equal(doors.Unix(), reimported.DoorsAt.Unix())
	suite.Equal(music.Unix(), reimported.MusicAt.Unix())
}

// TestImportData_RejectsMalformedShowTime covers the failure mode of the new
// parse step: a bad value aborts the import rather than silently landing a
// show with no doors time.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_RejectsMalformedShowTime() {
	bad := "not-a-timestamp"
	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: []contracts.ExportedShow{{
			Title:     "Malformed Times",
			EventDate: time.Date(2025, 8, 1, 20, 0, 0, 0, time.UTC).Format(time.RFC3339),
			DoorsAt:   &bad,
			Status:    string(catalogm.ShowStatusApproved),
			Venues:    []contracts.ExportedVenue{{Name: "V", City: "NYC", State: "NY"}},
			Artists:   []contracts.ExportedShowArtist{{Name: "A", Position: 0, SetType: "performer"}},
		}},
	})

	// ImportData reports per-show failures in the result rather than aborting
	// the batch, matching how a malformed event_date is handled.
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Errors, "a malformed doorsAt must be counted as an error")
	suite.Equal(0, result.Shows.Imported, "the show must not land with its doors time dropped")

	var count int64
	suite.db.Model(&catalogm.Show{}).Where("title = ?", "Malformed Times").Count(&count)
	suite.Zero(count)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_FullRoundTrip() {
	// Create data to export
	insta := "@thevenue"
	venue := suite.createVenueWithSocial("RT Venue", "NYC", "NY", &insta)
	artist := suite.createArtist("RT Band")
	eventDate := time.Date(2025, 8, 1, 20, 0, 0, 0, time.UTC)
	suite.createShow("RT Show", eventDate, catalogm.ShowStatusApproved, venue, artist)

	// Export
	exportResult, err := suite.service.ExportShows(contracts.ExportShowsParams{Status: "approved"})
	suite.Require().NoError(err)
	suite.Require().Len(exportResult.Shows, 1)

	exportedShow := exportResult.Shows[0]
	suite.Equal("RT Show", exportedShow.Title)
	suite.Require().Len(exportedShow.Venues, 1)
	suite.Equal("RT Venue", exportedShow.Venues[0].Name)

	// Clean up data
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")

	// Import the exported data
	importResult, err := suite.service.ImportData(contracts.DataImportRequest{
		Shows: exportResult.Shows,
	})
	suite.Require().NoError(err)
	suite.Equal(1, importResult.Shows.Imported)

	// Verify re-created
	var show catalogm.Show
	err = suite.db.Where("title = ?", "RT Show").Preload("Venues").Preload("Artists").First(&show).Error
	suite.Require().NoError(err)
	suite.NotNil(show.Slug)
	suite.Equal(catalogm.ShowStatusApproved, show.Status)
	suite.Require().Len(show.Venues, 1)
	suite.Equal("RT Venue", show.Venues[0].Name)
	suite.Require().Len(show.Artists, 1)
	suite.Equal("RT Band", show.Artists[0].Name)
}
