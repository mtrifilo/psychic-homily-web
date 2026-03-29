package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewDataQualityService(t *testing.T) {
	t.Run("NilDB", func(t *testing.T) {
		svc := NewDataQualityService(nil)
		assert.NotNil(t, svc)
	})

	t.Run("ExplicitDB", func(t *testing.T) {
		db := &gorm.DB{}
		svc := NewDataQualityService(db)
		assert.NotNil(t, svc)
	})
}

func TestDataQualityService_InvalidCategory(t *testing.T) {
	svc := &DataQualityService{db: &gorm.DB{}}
	_, _, err := svc.GetCategoryItems("nonexistent_category", 10, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type DataQualityServiceIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *DataQualityService
}

func (suite *DataQualityServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.service = &DataQualityService{db: suite.testDB.DB}
}

func (suite *DataQualityServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *DataQualityServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artist_aliases")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestDataQualityServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(DataQualityServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) createArtist(name string, social *models.Social) *models.Artist {
	artist := &models.Artist{Name: name}
	if social != nil {
		artist.Social = *social
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataQualityServiceIntegrationTestSuite) createArtistWithLocation(name string, city, state *string) *models.Artist {
	artist := &models.Artist{
		Name:  name,
		City:  city,
		State: state,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataQualityServiceIntegrationTestSuite) createVenue(name, city, state string, verified bool, social *models.Social) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: verified,
	}
	if social != nil {
		venue.Social = *social
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	// If not verified, update explicitly due to GORM bool zero-value gotcha
	if !verified {
		suite.db.Exec("UPDATE venues SET verified = false WHERE id = ?", venue.ID)
	}
	return venue
}

func (suite *DataQualityServiceIntegrationTestSuite) createShow(title string, status models.ShowStatus, price *float64) *models.Show {
	show := &models.Show{
		Title:     title,
		EventDate: time.Now().Add(7 * 24 * time.Hour), // future
		Status:    status,
		Source:    models.ShowSourceUser,
		Price:     price,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *DataQualityServiceIntegrationTestSuite) createShowWithDate(title string, status models.ShowStatus, eventDate time.Time) *models.Show {
	show := &models.Show{
		Title:     title,
		EventDate: eventDate,
		Status:    status,
		Source:    models.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *DataQualityServiceIntegrationTestSuite) linkShowArtist(showID, artistID uint, position int) {
	sa := &models.ShowArtist{
		ShowID:   showID,
		ArtistID: artistID,
		Position: position,
	}
	err := suite.db.Create(sa).Error
	suite.Require().NoError(err)
}

func (suite *DataQualityServiceIntegrationTestSuite) linkShowVenue(showID, venueID uint) {
	sv := &models.ShowVenue{
		ShowID:  showID,
		VenueID: venueID,
	}
	err := suite.db.Create(sv).Error
	suite.Require().NoError(err)
}

func (suite *DataQualityServiceIntegrationTestSuite) createAlias(artistID uint, alias string) {
	a := &models.ArtistAlias{
		ArtistID: artistID,
		Alias:    alias,
	}
	err := suite.db.Create(a).Error
	suite.Require().NoError(err)
}

// =============================================================================
// TESTS: GetSummary
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestGetSummary_Empty() {
	summary, err := suite.service.GetSummary()
	suite.Require().NoError(err)
	suite.Equal(0, summary.TotalItems)
	suite.Len(summary.Categories, 7)

	// All counts should be 0 in an empty DB
	for _, cat := range summary.Categories {
		suite.Equal(0, cat.Count, "category %s should have count 0", cat.Key)
	}
}

func (suite *DataQualityServiceIntegrationTestSuite) TestGetSummary_WithData() {
	// Artist with no links
	suite.createArtist("No Links Band", nil)

	// Artist with links (should NOT appear)
	ig := "insta"
	suite.createArtist("Has Links Band", &models.Social{Instagram: &ig})

	// Artist with no location
	suite.createArtistWithLocation("No Location Band", nil, nil)

	summary, err := suite.service.GetSummary()
	suite.Require().NoError(err)
	suite.Greater(summary.TotalItems, 0)

	// Find the artists_missing_links category
	for _, cat := range summary.Categories {
		if cat.Key == "artists_missing_links" {
			// "No Links Band" and "No Location Band" both have no links
			suite.Equal(2, cat.Count, "artists_missing_links should count artists with no social links")
			break
		}
	}
}

// =============================================================================
// TESTS: GetCategoryItems - Artists Missing Links
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestArtistsMissingLinks() {
	// Create two artists with no links
	a1 := suite.createArtist("Popular Band", nil)
	a2 := suite.createArtist("Unknown Band", nil)

	// Give Popular Band more shows
	show1 := suite.createShow("Show 1", models.ShowStatusApproved, nil)
	show2 := suite.createShow("Show 2", models.ShowStatusApproved, nil)
	suite.linkShowArtist(show1.ID, a1.ID, 0)
	suite.linkShowArtist(show2.ID, a1.ID, 0)
	suite.linkShowArtist(show1.ID, a2.ID, 1)

	// Artist with links (should NOT appear)
	ig := "insta"
	suite.createArtist("Has Links", &models.Social{Instagram: &ig})

	items, total, err := suite.service.GetCategoryItems("artists_missing_links", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(items, 2)

	// Popular Band should appear first (more shows)
	suite.Equal("Popular Band", items[0].Name)
	suite.Equal(2, items[0].ShowCount)
	suite.Equal("Unknown Band", items[1].Name)
	suite.Equal(1, items[1].ShowCount)
}

// =============================================================================
// TESTS: GetCategoryItems - Artists Missing Location
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestArtistsMissingLocation() {
	// Artist with no location
	suite.createArtistWithLocation("No Location Band", nil, nil)

	// Artist with location (should NOT appear)
	city := "Phoenix"
	state := "AZ"
	suite.createArtistWithLocation("Located Band", &city, &state)

	items, total, err := suite.service.GetCategoryItems("artists_missing_location", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("No Location Band", items[0].Name)
}

// =============================================================================
// TESTS: GetCategoryItems - Artists No Aliases
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestArtistsNoAliases() {
	// Artist with 5+ shows but no aliases
	a1 := suite.createArtist("Prolific Band", nil)
	for i := 0; i < 5; i++ {
		show := suite.createShow(fmt.Sprintf("Show %d", i), models.ShowStatusApproved, nil)
		suite.linkShowArtist(show.ID, a1.ID, 0)
	}

	// Artist with 5+ shows AND aliases (should NOT appear)
	a2 := suite.createArtist("Aliased Band", nil)
	for i := 0; i < 5; i++ {
		show := suite.createShow(fmt.Sprintf("Aliased Show %d", i), models.ShowStatusApproved, nil)
		suite.linkShowArtist(show.ID, a2.ID, 0)
	}
	suite.createAlias(a2.ID, "Alt Name")

	// Artist with <5 shows (should NOT appear)
	a3 := suite.createArtist("New Band", nil)
	show := suite.createShow("Single Show", models.ShowStatusApproved, nil)
	suite.linkShowArtist(show.ID, a3.ID, 0)

	items, total, err := suite.service.GetCategoryItems("artists_no_aliases", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("Prolific Band", items[0].Name)
	suite.GreaterOrEqual(items[0].ShowCount, 5)
}

// =============================================================================
// TESTS: GetCategoryItems - Venues Missing Social
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestVenuesMissingSocial() {
	// Venue with no social links
	suite.createVenue("Bare Venue", "Phoenix", "AZ", true, nil)

	// Venue with social links (should NOT appear)
	ig := "insta"
	suite.createVenue("Social Venue", "Phoenix", "AZ", true, &models.Social{Instagram: &ig})

	items, total, err := suite.service.GetCategoryItems("venues_missing_social", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("Bare Venue", items[0].Name)
}

// =============================================================================
// TESTS: GetCategoryItems - Venues Unverified With Shows
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestVenuesUnverifiedWithShows() {
	// Unverified venue with 3 approved shows
	v1 := suite.createVenue("Busy Unverified", "Phoenix", "AZ", false, nil)
	for i := 0; i < 3; i++ {
		show := suite.createShow(fmt.Sprintf("Busy Show %d", i), models.ShowStatusApproved, nil)
		suite.linkShowVenue(show.ID, v1.ID)
	}

	// Unverified venue with only 2 shows (should NOT appear)
	v2 := suite.createVenue("Quiet Unverified", "Phoenix", "AZ", false, nil)
	for i := 0; i < 2; i++ {
		show := suite.createShow(fmt.Sprintf("Quiet Show %d", i), models.ShowStatusApproved, nil)
		suite.linkShowVenue(show.ID, v2.ID)
	}

	// Verified venue with many shows (should NOT appear)
	v3 := suite.createVenue("Verified Venue", "Phoenix", "AZ", true, nil)
	for i := 0; i < 5; i++ {
		show := suite.createShow(fmt.Sprintf("Verified Show %d", i), models.ShowStatusApproved, nil)
		suite.linkShowVenue(show.ID, v3.ID)
	}

	items, total, err := suite.service.GetCategoryItems("venues_unverified_with_shows", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("Busy Unverified", items[0].Name)
	suite.Equal(3, items[0].ShowCount)
}

// =============================================================================
// TESTS: GetCategoryItems - Shows No Billing Order
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestShowsNoBillingOrder() {
	a1 := suite.createArtist("Band A", nil)
	a2 := suite.createArtist("Band B", nil)
	a3 := suite.createArtist("Band C", nil)

	// Future show with 2+ artists, all at position 0
	show1 := suite.createShow("No Billing Show", models.ShowStatusApproved, nil)
	suite.linkShowArtist(show1.ID, a1.ID, 0)
	suite.linkShowArtist(show1.ID, a2.ID, 0)

	// Future show with proper billing (should NOT appear)
	show2 := suite.createShow("Proper Billing Show", models.ShowStatusApproved, nil)
	suite.linkShowArtist(show2.ID, a1.ID, 0)
	suite.linkShowArtist(show2.ID, a3.ID, 1)

	// Future show with only 1 artist (should NOT appear)
	show3 := suite.createShow("Solo Show", models.ShowStatusApproved, nil)
	suite.linkShowArtist(show3.ID, a1.ID, 0)

	// Past show with no billing (should NOT appear)
	pastShow := suite.createShowWithDate("Past No Billing", models.ShowStatusApproved, time.Now().Add(-48*time.Hour))
	suite.linkShowArtist(pastShow.ID, a1.ID, 0)
	suite.linkShowArtist(pastShow.ID, a2.ID, 0)

	items, total, err := suite.service.GetCategoryItems("shows_no_billing_order", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("No Billing Show", items[0].Name)
}

// =============================================================================
// TESTS: GetCategoryItems - Shows Missing Price
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestShowsMissingPrice() {
	price := 15.0

	// Future approved show with no price
	suite.createShow("No Price Show", models.ShowStatusApproved, nil)

	// Future approved show with price (should NOT appear)
	suite.createShow("Priced Show", models.ShowStatusApproved, &price)

	// Future pending show with no price (should NOT appear)
	suite.createShow("Pending No Price", models.ShowStatusPending, nil)

	// Past show with no price (should NOT appear)
	suite.createShowWithDate("Past No Price", models.ShowStatusApproved, time.Now().Add(-48*time.Hour))

	items, total, err := suite.service.GetCategoryItems("shows_missing_price", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("No Price Show", items[0].Name)
}

// =============================================================================
// TESTS: Pagination
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestPagination() {
	// Create 5 artists with no links
	for i := 0; i < 5; i++ {
		suite.createArtist(fmt.Sprintf("No Links Band %d", i), nil)
	}

	// Page 1: limit 2, offset 0
	items, total, err := suite.service.GetCategoryItems("artists_missing_links", 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(items, 2)

	// Page 2: limit 2, offset 2
	items2, total2, err := suite.service.GetCategoryItems("artists_missing_links", 2, 2)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total2)
	suite.Len(items2, 2)

	// Page 3: limit 2, offset 4
	items3, total3, err := suite.service.GetCategoryItems("artists_missing_links", 2, 4)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total3)
	suite.Len(items3, 1)

	// Ensure no duplicates across pages
	names := make(map[string]bool)
	for _, item := range items {
		names[item.Name] = true
	}
	for _, item := range items2 {
		suite.False(names[item.Name], "duplicate item across pages: %s", item.Name)
		names[item.Name] = true
	}
	for _, item := range items3 {
		suite.False(names[item.Name], "duplicate item across pages: %s", item.Name)
	}
}

// =============================================================================
// TESTS: Unknown Category
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestUnknownCategory() {
	_, _, err := suite.service.GetCategoryItems("nonexistent", 50, 0)
	suite.Error(err)
	suite.Contains(err.Error(), "unknown category")
}

// =============================================================================
// TESTS: Max Limit
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestMaxLimit() {
	// Create 3 artists with no links
	for i := 0; i < 3; i++ {
		suite.createArtist(fmt.Sprintf("Band %d", i), nil)
	}

	// Request with limit > 200 should be capped to 200
	items, _, err := suite.service.GetCategoryItems("artists_missing_links", 500, 0)
	suite.Require().NoError(err)
	suite.Len(items, 3) // only 3 exist
}
