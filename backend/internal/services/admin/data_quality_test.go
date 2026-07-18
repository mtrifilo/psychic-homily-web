package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

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
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM artist_aliases")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestDataQualityServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(DataQualityServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) createArtist(name string, social *catalogm.Social) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name}
	if social != nil {
		artist.Social = *social
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataQualityServiceIntegrationTestSuite) createArtistWithLocation(name string, city, state *string) *catalogm.Artist {
	artist := &catalogm.Artist{
		Name:  name,
		City:  city,
		State: state,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataQualityServiceIntegrationTestSuite) createVenue(name, city, state string, verified bool, social *catalogm.Social) *catalogm.Venue {
	venue := &catalogm.Venue{
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

func (suite *DataQualityServiceIntegrationTestSuite) createShow(title string, status catalogm.ShowStatus, price *float64) *catalogm.Show {
	show := &catalogm.Show{
		Title:     title,
		EventDate: time.Now().Add(7 * 24 * time.Hour), // future
		Status:    status,
		Source:    catalogm.ShowSourceUser,
		Price:     price,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *DataQualityServiceIntegrationTestSuite) createShowWithDate(title string, status catalogm.ShowStatus, eventDate time.Time) *catalogm.Show {
	show := &catalogm.Show{
		Title:     title,
		EventDate: eventDate,
		Status:    status,
		Source:    catalogm.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *DataQualityServiceIntegrationTestSuite) linkShowArtist(showID, artistID uint, position int) {
	sa := &catalogm.ShowArtist{
		ShowID:   showID,
		ArtistID: artistID,
		Position: position,
	}
	err := suite.db.Create(sa).Error
	suite.Require().NoError(err)
}

func (suite *DataQualityServiceIntegrationTestSuite) linkShowVenue(showID, venueID uint) {
	sv := &catalogm.ShowVenue{
		ShowID:  showID,
		VenueID: venueID,
	}
	err := suite.db.Create(sv).Error
	suite.Require().NoError(err)
}

func (suite *DataQualityServiceIntegrationTestSuite) createAlias(artistID uint, alias string) {
	a := &catalogm.ArtistAlias{
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
	suite.Len(summary.Categories, 8)

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
	suite.createArtist("Has Links Band", &catalogm.Social{Instagram: &ig})

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
	show1 := suite.createShow("Show 1", catalogm.ShowStatusApproved, nil)
	show2 := suite.createShow("Show 2", catalogm.ShowStatusApproved, nil)
	suite.linkShowArtist(show1.ID, a1.ID, 0)
	suite.linkShowArtist(show2.ID, a1.ID, 0)
	suite.linkShowArtist(show1.ID, a2.ID, 1)

	// Artist with links (should NOT appear)
	ig := "insta"
	suite.createArtist("Has Links", &catalogm.Social{Instagram: &ig})

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
		show := suite.createShow(fmt.Sprintf("Show %d", i), catalogm.ShowStatusApproved, nil)
		suite.linkShowArtist(show.ID, a1.ID, 0)
	}

	// Artist with 5+ shows AND aliases (should NOT appear)
	a2 := suite.createArtist("Aliased Band", nil)
	for i := 0; i < 5; i++ {
		show := suite.createShow(fmt.Sprintf("Aliased Show %d", i), catalogm.ShowStatusApproved, nil)
		suite.linkShowArtist(show.ID, a2.ID, 0)
	}
	suite.createAlias(a2.ID, "Alt Name")

	// Artist with <5 shows (should NOT appear)
	a3 := suite.createArtist("New Band", nil)
	show := suite.createShow("Single Show", catalogm.ShowStatusApproved, nil)
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
	suite.createVenue("Social Venue", "Phoenix", "AZ", true, &catalogm.Social{Instagram: &ig})

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
		show := suite.createShow(fmt.Sprintf("Busy Show %d", i), catalogm.ShowStatusApproved, nil)
		suite.linkShowVenue(show.ID, v1.ID)
	}

	// Unverified venue with only 2 shows (should NOT appear)
	v2 := suite.createVenue("Quiet Unverified", "Phoenix", "AZ", false, nil)
	for i := 0; i < 2; i++ {
		show := suite.createShow(fmt.Sprintf("Quiet Show %d", i), catalogm.ShowStatusApproved, nil)
		suite.linkShowVenue(show.ID, v2.ID)
	}

	// Verified venue with many shows (should NOT appear)
	v3 := suite.createVenue("Verified Venue", "Phoenix", "AZ", true, nil)
	for i := 0; i < 5; i++ {
		show := suite.createShow(fmt.Sprintf("Verified Show %d", i), catalogm.ShowStatusApproved, nil)
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
	show1 := suite.createShow("No Billing Show", catalogm.ShowStatusApproved, nil)
	suite.linkShowArtist(show1.ID, a1.ID, 0)
	suite.linkShowArtist(show1.ID, a2.ID, 0)

	// Future show with proper billing (should NOT appear)
	show2 := suite.createShow("Proper Billing Show", catalogm.ShowStatusApproved, nil)
	suite.linkShowArtist(show2.ID, a1.ID, 0)
	suite.linkShowArtist(show2.ID, a3.ID, 1)

	// Future show with only 1 artist (should NOT appear)
	show3 := suite.createShow("Solo Show", catalogm.ShowStatusApproved, nil)
	suite.linkShowArtist(show3.ID, a1.ID, 0)

	// Past show with no billing (should NOT appear)
	pastShow := suite.createShowWithDate("Past No Billing", catalogm.ShowStatusApproved, time.Now().Add(-48*time.Hour))
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
	suite.createShow("No Price Show", catalogm.ShowStatusApproved, nil)

	// Future approved show with price (should NOT appear)
	suite.createShow("Priced Show", catalogm.ShowStatusApproved, &price)

	// Future pending show with no price (should NOT appear)
	suite.createShow("Pending No Price", catalogm.ShowStatusPending, nil)

	// Past show with no price (should NOT appear)
	suite.createShowWithDate("Past No Price", catalogm.ShowStatusApproved, time.Now().Add(-48*time.Hour))

	items, total, err := suite.service.GetCategoryItems("shows_missing_price", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("No Price Show", items[0].Name)
}

// =============================================================================
// TESTS: GetCategoryItems - Releases Missing Year
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestReleasesMissingYear() {
	// Release with no year
	noYear := &catalogm.Release{Title: "Unknown Year Album", ReleaseType: catalogm.ReleaseTypeLP}
	suite.Require().NoError(suite.db.Create(noYear).Error)

	// Release with a year (should NOT appear)
	year := 2020
	withYear := &catalogm.Release{Title: "Dated Album", ReleaseType: catalogm.ReleaseTypeLP, ReleaseYear: &year}
	suite.Require().NoError(suite.db.Create(withYear).Error)

	// Release with no year but linked to an artist (test artist name in reason)
	noYearLinked := &catalogm.Release{Title: "Mystery EP", ReleaseType: catalogm.ReleaseTypeEP}
	suite.Require().NoError(suite.db.Create(noYearLinked).Error)
	artist := suite.createArtist("Cool Band", nil)
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelease{
		ArtistID:  artist.ID,
		ReleaseID: noYearLinked.ID,
		Role:      catalogm.ArtistReleaseRoleMain,
		Position:  0,
	}).Error)

	items, total, err := suite.service.GetCategoryItems("releases_missing_year", 50, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(items, 2)

	// Both items should be releases
	for _, item := range items {
		suite.Equal("release", item.EntityType)
		suite.Contains(item.Reason, "No release year set")
	}

	// Find the linked release and verify artist name is in the reason
	for _, item := range items {
		if item.Name == "Mystery EP" {
			suite.Contains(item.Reason, "Cool Band")
		}
	}
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

// =============================================================================
// HELPERS: Loose Ends (PSY-1483)
// =============================================================================

func strPtr(s string) *string { return &s }

func (suite *DataQualityServiceIntegrationTestSuite) createUser(email string) *authm.User {
	user := &authm.User{
		Email:         strPtr(email),
		FirstName:     strPtr("Test"),
		LastName:      strPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *DataQualityServiceIntegrationTestSuite) followArtist(userID, artistID uint) {
	bookmark := &engagementm.UserBookmark{
		UserID:     userID,
		EntityType: engagementm.BookmarkEntityArtist,
		EntityID:   artistID,
		Action:     engagementm.BookmarkActionFollow,
		CreatedAt:  time.Now(),
	}
	suite.Require().NoError(suite.db.Create(bookmark).Error)
}

// chartingShow links an artist to a distinct approved, non-cancelled show
// dated `daysAgo` in the past so it counts toward the charting window.
func (suite *DataQualityServiceIntegrationTestSuite) chartingShow(title string, artistID uint, daysAgo int) *catalogm.Show {
	show := suite.createShowWithDate(title, catalogm.ShowStatusApproved, time.Now().AddDate(0, 0, -daysAgo))
	suite.linkShowArtist(show.ID, artistID, 0)
	return show
}

// =============================================================================
// TESTS: Loose Ends — followed_artists_missing_links
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestFollowedArtistsMissingLinks() {
	user := suite.createUser("follower@test.com")
	other := suite.createUser("other@test.com")

	// Followed, missing bandcamp + spotify (only Instagram set) → INCLUDED.
	// This proves the NARROW definition: an artist with a non-bandcamp,
	// non-spotify link still counts as a loose end here.
	ig := "insta"
	followedGap := suite.createArtist("Followed Gap Band", &catalogm.Social{Instagram: &ig})
	suite.followArtist(user.ID, followedGap.ID)

	// Followed, but HAS a bandcamp link → EXCLUDED.
	bc := "https://band.bandcamp.com"
	followedComplete := suite.createArtist("Followed Complete Band", &catalogm.Social{Bandcamp: &bc})
	suite.followArtist(user.ID, followedComplete.ID)

	// Missing links, but NOT followed by this user → EXCLUDED.
	suite.createArtist("Unfollowed Gap Band", nil)

	// Missing links, followed by a DIFFERENT user → EXCLUDED for this viewer.
	otherGap := suite.createArtist("Other User Gap Band", nil)
	suite.followArtist(other.ID, otherGap.ID)

	viewerID := user.ID
	items, total, err := suite.service.GetContributeCategoryItems(categoryFollowedArtistsMissingLinks, &viewerID, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("Followed Gap Band", items[0].Name)
	suite.Equal("artist", items[0].EntityType)
}

func (suite *DataQualityServiceIntegrationTestSuite) TestFollowedArtistsMissingLinks_AnonReturnsEmpty() {
	user := suite.createUser("follower@test.com")
	gap := suite.createArtist("Followed Gap Band", nil)
	suite.followArtist(user.ID, gap.ID)

	items, total, err := suite.service.GetContributeCategoryItems(categoryFollowedArtistsMissingLinks, nil, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(items)
}

// =============================================================================
// TESTS: Loose Ends — charting_artists_missing_links
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestChartingArtistsMissingLinks() {
	// Charting (2 in-window shows), missing links → INCLUDED.
	charting := suite.createArtist("Charting Band", nil)
	suite.chartingShow("Charting Show 1", charting.ID, 5)
	suite.chartingShow("Charting Show 2", charting.ID, 10)

	// Only 1 in-window show → below threshold → EXCLUDED.
	oneShow := suite.createArtist("One Show Band", nil)
	suite.chartingShow("One Show", oneShow.ID, 5)

	// 2 in-window shows but HAS a spotify link → EXCLUDED.
	sp := "https://open.spotify.com/artist/x"
	complete := suite.createArtist("Complete Charting Band", &catalogm.Social{Spotify: &sp})
	suite.chartingShow("Complete Show 1", complete.ID, 5)
	suite.chartingShow("Complete Show 2", complete.ID, 10)

	// 2 shows but OUTSIDE the window (older than 90d) → EXCLUDED.
	stale := suite.createArtist("Stale Band", nil)
	suite.chartingShow("Stale Show 1", stale.ID, 120)
	suite.chartingShow("Stale Show 2", stale.ID, 150)

	// 2 shows in window but CANCELLED → EXCLUDED.
	cancelled := suite.createArtist("Cancelled Band", nil)
	c1 := suite.chartingShow("Cancelled Show 1", cancelled.ID, 5)
	c2 := suite.chartingShow("Cancelled Show 2", cancelled.ID, 10)
	suite.db.Exec("UPDATE shows SET is_cancelled = true WHERE id IN (?, ?)", c1.ID, c2.ID)

	// Charting works anonymously (nil viewer).
	items, total, err := suite.service.GetContributeCategoryItems(categoryChartingArtistsMissingLinks, nil, 20, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(items, 1)
	suite.Equal("Charting Band", items[0].Name)
	suite.Equal(2, items[0].ShowCount)
}

// =============================================================================
// TESTS: Loose Ends — contribute summary
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestContributeSummary_AuthedIncludesBothCategories() {
	user := suite.createUser("viewer@test.com")

	followed := suite.createArtist("My Band", nil)
	suite.followArtist(user.ID, followed.ID)

	charting := suite.createArtist("Charting Band", nil)
	suite.chartingShow("Show 1", charting.ID, 5)
	suite.chartingShow("Show 2", charting.ID, 10)

	viewerID := user.ID
	summary, err := suite.service.GetContributeSummary(&viewerID)
	suite.Require().NoError(err)

	counts := map[string]int{}
	for _, cat := range summary.Categories {
		counts[cat.Key] = cat.Count
	}
	// 8 global categories + 2 loose-ends.
	suite.Len(summary.Categories, 10)
	suite.Equal(1, counts[categoryFollowedArtistsMissingLinks])
	suite.Equal(1, counts[categoryChartingArtistsMissingLinks])
}

func (suite *DataQualityServiceIntegrationTestSuite) TestContributeSummary_AnonOmitsFollowedIncludesCharting() {
	charting := suite.createArtist("Charting Band", nil)
	suite.chartingShow("Show 1", charting.ID, 5)
	suite.chartingShow("Show 2", charting.ID, 10)

	summary, err := suite.service.GetContributeSummary(nil)
	suite.Require().NoError(err)

	keys := map[string]bool{}
	for _, cat := range summary.Categories {
		keys[cat.Key] = true
	}
	// Followed is authed-only → omitted; charting present.
	suite.False(keys[categoryFollowedArtistsMissingLinks], "followed category must be omitted for anon")
	suite.True(keys[categoryChartingArtistsMissingLinks], "charting category must be present for anon")
	suite.Len(summary.Categories, 9)
}

// =============================================================================
// TESTS: Loose Ends — cap + stable daily rotation
// =============================================================================

func (suite *DataQualityServiceIntegrationTestSuite) TestChartingArtistsMissingLinks_CapAndStableRotation() {
	// 30 charting artists → list must cap at looseEndsMaxItems (25) while the
	// true total stays accurate.
	for i := 0; i < 30; i++ {
		a := suite.createArtist(fmt.Sprintf("Charting Band %02d", i), nil)
		suite.chartingShow(fmt.Sprintf("Show A %02d", i), a.ID, 5)
		suite.chartingShow(fmt.Sprintf("Show B %02d", i), a.ID, 10)
	}

	// Requesting more than the cap is clamped to looseEndsMaxItems.
	items, total, err := suite.service.GetContributeCategoryItems(categoryChartingArtistsMissingLinks, nil, 100, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(30), total, "count is the true total, uncapped")
	suite.Len(items, looseEndsMaxItems)

	// Rotation is stable within a UTC day for a given viewer: two calls return
	// the same slice in the same order.
	again, _, err := suite.service.GetContributeCategoryItems(categoryChartingArtistsMissingLinks, nil, 100, 0)
	suite.Require().NoError(err)
	suite.Require().Len(again, looseEndsMaxItems)
	for i := range items {
		suite.Equal(items[i].EntityID, again[i].EntityID, "rotation order changed across calls at index %d", i)
	}
}
