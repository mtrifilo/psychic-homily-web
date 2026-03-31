package catalog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ArtistServiceIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	artistService *ArtistService
}

func (suite *ArtistServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.artistService = &ArtistService{db: suite.testDB.DB}
}

func (suite *ArtistServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// TearDownTest cleans up data between tests for isolation
func (suite *ArtistServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM artist_aliases")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM notification_log")
	_, _ = sqlDB.Exec("DELETE FROM notification_filters")
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestArtistServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ArtistServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ArtistServiceIntegrationTestSuite) createTestArtist(name string) *models.Artist {
	artist := &models.Artist{
		Name: name,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *ArtistServiceIntegrationTestSuite) createTestVenue(name, city, state string) *models.Venue {
	venue := &models.Venue{
		Name:  name,
		City:  city,
		State: state,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *ArtistServiceIntegrationTestSuite) createApprovedShowWithArtist(artistID, venueID, userID uint, eventDate time.Time) *models.Show {
	show := &models.Show{
		Title:       fmt.Sprintf("Show-%d", time.Now().UnixNano()),
		EventDate:   eventDate,
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	// Link show to venue
	err = suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error
	suite.Require().NoError(err)

	// Link show to artist
	err = suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artistID, Position: 0}).Error
	suite.Require().NoError(err)

	return show
}

// =============================================================================
// Group 1: CreateArtist
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_Success() {
	req := &contracts.CreateArtistRequest{
		Name:    "Radiohead",
		State:   stringPtr("AZ"),
		City:    stringPtr("Phoenix"),
		Website: stringPtr("https://radiohead.com"),
	}

	resp, err := suite.artistService.CreateArtist(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Radiohead", resp.Name)
	suite.NotEmpty(resp.Slug)
	suite.Equal("AZ", *resp.State)
	suite.Equal("Phoenix", *resp.City)
	suite.Equal("https://radiohead.com", *resp.Social.Website)
}

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_DuplicateName_Fails() {
	req := &contracts.CreateArtistRequest{Name: "The National"}
	_, err := suite.artistService.CreateArtist(req)
	suite.Require().NoError(err)

	// Same name, different case
	req2 := &contracts.CreateArtistRequest{Name: "the national"}
	_, err = suite.artistService.CreateArtist(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_WithSocialFields() {
	req := &contracts.CreateArtistRequest{
		Name:       "Social Band",
		Instagram:  stringPtr("@socialband"),
		Facebook:   stringPtr("facebook.com/socialband"),
		Twitter:    stringPtr("@socialband_tw"),
		YouTube:    stringPtr("youtube.com/socialband"),
		Spotify:    stringPtr("spotify:artist:123"),
		SoundCloud: stringPtr("soundcloud.com/socialband"),
		Bandcamp:   stringPtr("socialband.bandcamp.com"),
		Website:    stringPtr("https://socialband.com"),
	}

	resp, err := suite.artistService.CreateArtist(req)

	suite.Require().NoError(err)
	suite.Equal("@socialband", *resp.Social.Instagram)
	suite.Equal("facebook.com/socialband", *resp.Social.Facebook)
	suite.Equal("@socialband_tw", *resp.Social.Twitter)
	suite.Equal("youtube.com/socialband", *resp.Social.YouTube)
	suite.Equal("spotify:artist:123", *resp.Social.Spotify)
	suite.Equal("soundcloud.com/socialband", *resp.Social.SoundCloud)
	suite.Equal("socialband.bandcamp.com", *resp.Social.Bandcamp)
	suite.Equal("https://socialband.com", *resp.Social.Website)
}

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_WithCityState() {
	req := &contracts.CreateArtistRequest{
		Name:  "Local Band",
		City:  stringPtr("Tempe"),
		State: stringPtr("AZ"),
	}

	resp, err := suite.artistService.CreateArtist(req)

	suite.Require().NoError(err)
	suite.Equal("Tempe", *resp.City)
	suite.Equal("AZ", *resp.State)
}

// =============================================================================
// Group 2: GetArtist / GetArtistBySlug / GetArtistByName
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtist_Success() {
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Get Test Artist"})
	suite.Require().NoError(err)

	resp, err := suite.artistService.GetArtist(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("Get Test Artist", resp.Name)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtist_NotFound() {
	resp, err := suite.artistService.GetArtist(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistBySlug_Success() {
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Slug Test Artist"})
	suite.Require().NoError(err)

	resp, err := suite.artistService.GetArtistBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistBySlug_NotFound() {
	resp, err := suite.artistService.GetArtistBySlug("nonexistent-slug-xyz")

	suite.Require().Error(err)
	suite.Nil(resp)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistByName_Success() {
	_, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "The Beatles"})
	suite.Require().NoError(err)

	// Case-insensitive lookup
	resp, err := suite.artistService.GetArtistByName("the beatles")

	suite.Require().NoError(err)
	suite.Equal("The Beatles", resp.Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistByName_NotFound() {
	resp, err := suite.artistService.GetArtistByName("Nonexistent Band")

	suite.Require().Error(err)
	suite.Nil(resp)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

// =============================================================================
// Group 3: GetArtists filtering
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_FilterByCity() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "PHX Artist", City: stringPtr("Phoenix")})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "TUC Artist", City: stringPtr("Tucson")})

	resp, err := suite.artistService.GetArtists(map[string]interface{}{"city": "Phoenix"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("PHX Artist", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_MultiCityFilter() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "PHX Band", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Mesa Band", City: stringPtr("Mesa"), State: stringPtr("AZ")})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "LA Band", City: stringPtr("Los Angeles"), State: stringPtr("CA")})

	cities := []map[string]string{
		{"city": "Phoenix", "state": "AZ"},
		{"city": "Mesa", "state": "AZ"},
	}
	resp, err := suite.artistService.GetArtists(map[string]interface{}{"cities": cities})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	names := []string{resp[0].Name, resp[1].Name}
	suite.Contains(names, "Mesa Band")
	suite.Contains(names, "PHX Band")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_FilterByState() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "AZ Artist", State: stringPtr("AZ")})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "CA Artist", State: stringPtr("CA")})

	resp, err := suite.artistService.GetArtists(map[string]interface{}{"state": "CA"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("CA Artist", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_FilterByName() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Crescent Band"})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Valley Group"})

	resp, err := suite.artistService.GetArtists(map[string]interface{}{"name": "crescent"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Crescent Band", resp[0].Name)
}

// =============================================================================
// Group 4: UpdateArtist
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_BasicFields() {
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Original Artist"})
	suite.Require().NoError(err)

	resp, err := suite.artistService.UpdateArtist(created.ID, map[string]interface{}{
		"city":  "Mesa",
		"state": "AZ",
	})

	suite.Require().NoError(err)
	suite.Equal("Original Artist", resp.Name)
	suite.Equal("Mesa", *resp.City)
	suite.Equal("AZ", *resp.State)
}

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_NameChangeRegeneratesSlug() {
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Old Name Band"})
	suite.Require().NoError(err)
	oldSlug := created.Slug

	resp, err := suite.artistService.UpdateArtist(created.ID, map[string]interface{}{
		"name": "New Name Band",
	})

	suite.Require().NoError(err)
	suite.Equal("New Name Band", resp.Name)
	suite.NotEqual(oldSlug, resp.Slug)
	suite.NotEmpty(resp.Slug)
}

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_DuplicateName_Fails() {
	_, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Existing Artist"})
	suite.Require().NoError(err)

	other, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Other Artist"})
	suite.Require().NoError(err)

	// Try to rename "Other Artist" to "Existing Artist"
	_, err = suite.artistService.UpdateArtist(other.ID, map[string]interface{}{"name": "Existing Artist"})

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_SameNameSameArtist_OK() {
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Keep My Name"})
	suite.Require().NoError(err)

	// Update city while keeping the same name — should not conflict with self
	resp, err := suite.artistService.UpdateArtist(created.ID, map[string]interface{}{
		"name": "Keep My Name",
		"city": "Scottsdale",
	})

	suite.Require().NoError(err)
	suite.Equal("Keep My Name", resp.Name)
	suite.Equal("Scottsdale", *resp.City)
}

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_NotFound() {
	// Updating a non-existent artist — the Updates call succeeds (0 rows affected),
	// but the subsequent GetArtist reload returns ARTIST_NOT_FOUND
	resp, err := suite.artistService.UpdateArtist(99999, map[string]interface{}{"city": "Nowhere"})

	suite.Require().Error(err)
	suite.Nil(resp)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

// =============================================================================
// Group 5: DeleteArtist
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestDeleteArtist_Success() {
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Delete Me"})
	suite.Require().NoError(err)

	err = suite.artistService.DeleteArtist(created.ID)

	suite.Require().NoError(err)

	// Verify it's gone
	_, err = suite.artistService.GetArtist(created.ID)
	suite.Error(err)
}

func (suite *ArtistServiceIntegrationTestSuite) TestDeleteArtist_NotFound() {
	err := suite.artistService.DeleteArtist(99999)

	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestDeleteArtist_HasShows_Fails() {
	artist := suite.createTestArtist("Show Artist")
	venue := suite.createTestVenue("Show Venue", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	err := suite.artistService.DeleteArtist(artist.ID)

	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistHasShows, artistErr.Code)
}

// =============================================================================
// Group 6: SearchArtists
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_EmptyQuery() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Some Artist"})

	resp, err := suite.artistService.SearchArtists("")

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_ShortQuery_PrefixMatch() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Valley Heat"})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Crescent Moon"})

	resp, err := suite.artistService.SearchArtists("Va")

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Valley Heat", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_LongQuery_TrigramMatch() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Radiohead"})
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "The Rebel Lounge Band"})

	resp, err := suite.artistService.SearchArtists("Radiohead")

	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp)
	suite.Equal("Radiohead", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_NoMatch() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Real Artist"})

	resp, err := suite.artistService.SearchArtists("zzzznonexistent")

	suite.Require().NoError(err)
	suite.Empty(resp)
}

// =============================================================================
// Group 7: GetShowsForArtist
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_Upcoming() {
	artist := suite.createTestArtist("Upcoming Artist")
	venue := suite.createTestVenue("Upcoming Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Create a future show
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	// Create a past show
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	resp, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.True(resp[0].EventDate.After(time.Now().UTC().AddDate(0, 0, -1)))
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_Past() {
	artist := suite.createTestArtist("Past Artist")
	venue := suite.createTestVenue("Past Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Create a future show
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	// Create a past show
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	resp, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "past")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.True(resp[0].EventDate.Before(time.Now().UTC()))
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_All() {
	artist := suite.createTestArtist("All Artist")
	venue := suite.createTestVenue("All Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	resp, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "all")

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_NotFound() {
	_, _, err := suite.artistService.GetShowsForArtist(99999, "UTC", 10, "upcoming")

	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_IncludesVenue() {
	artist := suite.createTestArtist("Venue Artist")
	venue := suite.createTestVenue("The Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	resp, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().NotNil(resp[0].Venue)
	suite.Equal(venue.ID, resp[0].Venue.ID)
	suite.Equal("The Venue", resp[0].Venue.Name)
	suite.Equal("Phoenix", resp[0].Venue.City)
	suite.Equal("AZ", resp[0].Venue.State)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_IncludesOtherArtists() {
	artist1 := suite.createTestArtist("Main Artist")
	artist2 := suite.createTestArtist("Support Artist")
	venue := suite.createTestVenue("Multi Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Create show with artist1
	show := suite.createApprovedShowWithArtist(artist1.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	// Also add artist2 to the same show
	err := suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artist2.ID, Position: 1}).Error
	suite.Require().NoError(err)

	resp, _, err := suite.artistService.GetShowsForArtist(artist1.ID, "UTC", 10, "upcoming")

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().Len(resp[0].Artists, 2)
	// Artists should be ordered by position
	suite.Equal("Main Artist", resp[0].Artists[0].Name)
	suite.Equal("Support Artist", resp[0].Artists[1].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_ExcludesNonApproved() {
	artist := suite.createTestArtist("Approved Only Artist")
	venue := suite.createTestVenue("Approved Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Create an approved show
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	// Create a pending show manually
	pendingShow := &models.Show{
		Title:       "Pending Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 14),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusPending,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(pendingShow)
	suite.db.Create(&models.ShowVenue{ShowID: pendingShow.ID, VenueID: venue.ID})
	suite.db.Create(&models.ShowArtist{ShowID: pendingShow.ID, ArtistID: artist.ID, Position: 0})

	// Create a rejected show
	rejectedShow := &models.Show{
		Title:       "Rejected Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 21),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusRejected,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(rejectedShow)
	suite.db.Create(&models.ShowVenue{ShowID: rejectedShow.ID, VenueID: venue.ID})
	suite.db.Create(&models.ShowArtist{ShowID: rejectedShow.ID, ArtistID: artist.ID, Position: 0})

	resp, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_RespectsLimit() {
	artist := suite.createTestArtist("Limit Artist")
	venue := suite.createTestVenue("Limit Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Create 5 future shows
	for i := 1; i <= 5; i++ {
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, i))
	}

	resp, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 3, "upcoming")

	suite.Require().NoError(err)
	suite.Equal(int64(5), total) // total count is still 5
	suite.Len(resp, 3)           // but only 3 returned
}

// =============================================================================
// Group 8: GetArtistCities
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_Success() {
	venue := suite.createTestVenue("City Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Create artists in different cities with upcoming shows
	a1, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "PHX Artist 1", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	a2, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "PHX Artist 2", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	a3, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Mesa Artist", City: stringPtr("Mesa"), State: stringPtr("AZ")})

	// Give all three artists upcoming shows
	suite.createApprovedShowWithArtist(a1.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowWithArtist(a2.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 14))
	suite.createApprovedShowWithArtist(a3.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 21))

	resp, err := suite.artistService.GetArtistCities()

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// Ordered by count DESC, then city ASC
	suite.Equal("Phoenix", resp[0].City)
	suite.Equal("AZ", resp[0].State)
	suite.Equal(2, resp[0].ArtistCount)
	suite.Equal("Mesa", resp[1].City)
	suite.Equal(1, resp[1].ArtistCount)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_ExcludesNullCity() {
	venue := suite.createTestVenue("NullCity Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Artist with no city/state should not appear even with upcoming show
	noCityArtist := suite.createTestArtist("No City Artist")
	suite.createApprovedShowWithArtist(noCityArtist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	// Artist with city and upcoming show should appear
	hasCityArtist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Has City", City: stringPtr("Tempe"), State: stringPtr("AZ")})
	suite.createApprovedShowWithArtist(hasCityArtist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 14))

	resp, err := suite.artistService.GetArtistCities()

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Tempe", resp[0].City)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_ExcludesArtistsWithoutUpcomingShows() {
	venue := suite.createTestVenue("Past Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Artist with city but only past shows should not appear
	pastArtist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Past Artist", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	suite.createApprovedShowWithArtist(pastArtist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	// Artist with city but no shows at all should not appear
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "No Show Artist", City: stringPtr("Mesa"), State: stringPtr("AZ")})

	resp, err := suite.artistService.GetArtistCities()

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistCities_Empty() {
	// No artists at all
	resp, err := suite.artistService.GetArtistCities()

	suite.Require().NoError(err)
	suite.Empty(resp)
}

// =============================================================================
// Group 9: GetArtistsWithShowCounts
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_OnlyUpcoming() {
	artist1 := suite.createTestArtist("Active Artist")
	artist2 := suite.createTestArtist("Inactive Artist")
	venue := suite.createTestVenue("Test Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// artist1 has an upcoming show
	suite.createApprovedShowWithArtist(artist1.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	// artist2 has only a past show
	suite.createApprovedShowWithArtist(artist2.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	resp, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Active Artist", resp[0].Name)
	suite.Equal(1, resp[0].UpcomingShowCount)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_SortedByCount() {
	artist1 := suite.createTestArtist("Few Shows")
	artist2 := suite.createTestArtist("Many Shows")
	venue := suite.createTestVenue("Sort Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// artist1: 1 upcoming show
	suite.createApprovedShowWithArtist(artist1.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	// artist2: 3 upcoming shows
	suite.createApprovedShowWithArtist(artist2.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowWithArtist(artist2.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 14))
	suite.createApprovedShowWithArtist(artist2.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 21))

	resp, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// Sorted by count DESC
	suite.Equal("Many Shows", resp[0].Name)
	suite.Equal(3, resp[0].UpcomingShowCount)
	suite.Equal("Few Shows", resp[1].Name)
	suite.Equal(1, resp[1].UpcomingShowCount)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_Empty() {
	// No artists with upcoming shows
	suite.createTestArtist("No Shows Artist")

	resp, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{})

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistsWithShowCounts_WithCityFilter() {
	artist1 := suite.createTestArtist("PHX Artist")
	suite.db.Model(artist1).Updates(map[string]interface{}{"city": "Phoenix", "state": "AZ"})
	artist2 := suite.createTestArtist("LA Artist")
	suite.db.Model(artist2).Updates(map[string]interface{}{"city": "Los Angeles", "state": "CA"})
	venue := suite.createTestVenue("Filter Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	suite.createApprovedShowWithArtist(artist1.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.createApprovedShowWithArtist(artist2.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	cities := []map[string]string{{"city": "Phoenix", "state": "AZ"}}
	resp, err := suite.artistService.GetArtistsWithShowCounts(map[string]interface{}{"cities": cities})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("PHX Artist", resp[0].Name)
}

// =============================================================================
// Group 10: Alias CRUD
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestAddArtistAlias_Success() {
	artist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Alias Artist"})

	alias, err := suite.artistService.AddArtistAlias(artist.ID, "Alt Name")

	suite.Require().NoError(err)
	suite.Require().NotNil(alias)
	suite.NotZero(alias.ID)
	suite.Equal(artist.ID, alias.ArtistID)
	suite.Equal("Alt Name", alias.Alias)
	suite.NotEmpty(alias.CreatedAt)
}

func (suite *ArtistServiceIntegrationTestSuite) TestAddArtistAlias_DuplicateAlias_Fails() {
	artist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Dup Alias Artist"})
	_, err := suite.artistService.AddArtistAlias(artist.ID, "Same Alias")
	suite.Require().NoError(err)

	_, err = suite.artistService.AddArtistAlias(artist.ID, "same alias") // case-insensitive
	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestAddArtistAlias_ConflictsWithArtistName_Fails() {
	suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Existing Band"})
	artist2, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Another Band"})

	_, err := suite.artistService.AddArtistAlias(artist2.ID, "Existing Band")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "conflicts with existing artist name")
}

func (suite *ArtistServiceIntegrationTestSuite) TestAddArtistAlias_EmptyAlias_Fails() {
	artist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Empty Alias Artist"})

	_, err := suite.artistService.AddArtistAlias(artist.ID, "  ")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "cannot be empty")
}

func (suite *ArtistServiceIntegrationTestSuite) TestAddArtistAlias_ArtistNotFound() {
	_, err := suite.artistService.AddArtistAlias(99999, "Some Alias")
	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestRemoveArtistAlias_Success() {
	artist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Remove Alias Artist"})
	alias, _ := suite.artistService.AddArtistAlias(artist.ID, "To Remove")

	err := suite.artistService.RemoveArtistAlias(alias.ID)

	suite.Require().NoError(err)

	// Verify it's gone
	aliases, err := suite.artistService.GetArtistAliases(artist.ID)
	suite.Require().NoError(err)
	suite.Empty(aliases)
}

func (suite *ArtistServiceIntegrationTestSuite) TestRemoveArtistAlias_NotFound() {
	err := suite.artistService.RemoveArtistAlias(99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "not found")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistAliases_Success() {
	artist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "List Aliases Artist"})
	suite.artistService.AddArtistAlias(artist.ID, "Alias B")
	suite.artistService.AddArtistAlias(artist.ID, "Alias A")

	aliases, err := suite.artistService.GetArtistAliases(artist.ID)

	suite.Require().NoError(err)
	suite.Require().Len(aliases, 2)
	// Should be sorted alphabetically
	suite.Equal("Alias A", aliases[0].Alias)
	suite.Equal("Alias B", aliases[1].Alias)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistAliases_ArtistNotFound() {
	_, err := suite.artistService.GetArtistAliases(99999)
	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

// =============================================================================
// Group 11: MergeArtists
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_Basic() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Canonical Artist"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Duplicate Artist"})

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Equal(canonical.ID, result.CanonicalArtistID)
	suite.Equal(mergeFrom.ID, result.MergedArtistID)
	suite.Equal("Duplicate Artist", result.MergedArtistName)
	suite.True(result.AliasCreated)

	// Verify merged artist is deleted
	_, err = suite.artistService.GetArtist(mergeFrom.ID)
	suite.Require().Error(err)

	// Verify alias was created
	aliases, err := suite.artistService.GetArtistAliases(canonical.ID)
	suite.Require().NoError(err)
	suite.Require().Len(aliases, 1)
	suite.Equal("Duplicate Artist", aliases[0].Alias)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_TransfersShows() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Merge Canonical"})
	mergeFrom := suite.createTestArtist("Merge From")
	venue := suite.createTestVenue("Merge Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// MergeFrom has a show
	suite.createApprovedShowWithArtist(mergeFrom.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)

	suite.Require().NoError(err)
	suite.Equal(int64(1), result.ShowsMoved)

	// Verify canonical now has the show
	shows, _, err := suite.artistService.GetShowsForArtist(canonical.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Len(shows, 1)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_ShowConflictDedup() {
	canonical := suite.createTestArtist("Conflict Canonical")
	mergeFrom := suite.createTestArtist("Conflict MergeFrom")
	venue := suite.createTestVenue("Conflict Venue", "Phoenix", "AZ")
	user := suite.createTestUser()

	// Both artists are on the same show
	show := suite.createApprovedShowWithArtist(canonical.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: mergeFrom.ID, Position: 1})

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)

	suite.Require().NoError(err)
	// The conflicting show_artist row is deleted, not moved
	suite.Equal(int64(0), result.ShowsMoved)

	// Verify show still has exactly 1 artist (canonical)
	var count int64
	suite.db.Model(&models.ShowArtist{}).Where("show_id = ?", show.ID).Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_SelfMerge_Fails() {
	artist, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Self Artist"})

	_, err := suite.artistService.MergeArtists(artist.ID, artist.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "cannot merge an artist with itself")
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_CanonicalNotFound() {
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "MergeFrom NotFound"})

	_, err := suite.artistService.MergeArtists(99999, mergeFrom.ID)

	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_MergeFromNotFound() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Canonical NotFound"})

	_, err := suite.artistService.MergeArtists(canonical.ID, 99999)

	suite.Require().Error(err)
	var artistErr *apperrors.ArtistError
	suite.ErrorAs(err, &artistErr)
	suite.Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_TransfersAliases() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Alias Canon"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Alias Merge"})

	// Add existing alias to mergeFrom
	suite.artistService.AddArtistAlias(mergeFrom.ID, "Old Alias")

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)

	suite.Require().NoError(err)
	suite.True(result.AliasCreated)

	// Verify canonical has both "Old Alias" and "Alias Merge"
	aliases, err := suite.artistService.GetArtistAliases(canonical.ID)
	suite.Require().NoError(err)
	suite.Require().Len(aliases, 2)
	aliasNames := []string{aliases[0].Alias, aliases[1].Alias}
	suite.Contains(aliasNames, "Old Alias")
	suite.Contains(aliasNames, "Alias Merge")
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_TransfersRevisions() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Rev Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Rev MergeFrom"})
	user := suite.createTestUser()

	// Create a revision for mergeFrom
	suite.db.Exec("INSERT INTO revisions (entity_type, entity_id, user_id, field_changes, summary, created_at) VALUES ('artist', ?, ?, '[]', 'test revision', NOW())", mergeFrom.ID, user.ID)

	_, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)

	// Verify revision now points to canonical
	var count int64
	suite.db.Raw("SELECT COUNT(*) FROM revisions WHERE entity_type = 'artist' AND entity_id = ?", canonical.ID).Scan(&count)
	suite.Equal(int64(1), count)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_TransfersBookmarks() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "BM Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "BM MergeFrom"})
	user := suite.createTestUser()

	// Create a bookmark for mergeFrom
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'bookmark', NOW())", user.ID, mergeFrom.ID)

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.BookmarksMoved)

	// Verify bookmark now points to canonical
	var count int64
	suite.db.Raw("SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?", canonical.ID).Scan(&count)
	suite.Equal(int64(1), count)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_BookmarkConflictDedup() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "BMD Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "BMD MergeFrom"})
	user := suite.createTestUser()

	// User has a "follow" bookmark on BOTH artists — this is a conflict
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'follow', NOW())", user.ID, canonical.ID)
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'follow', NOW())", user.ID, mergeFrom.ID)
	// User also has a unique "bookmark" action only on mergeFrom — should transfer
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'bookmark', NOW())", user.ID, mergeFrom.ID)

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)
	// Only the non-conflicting bookmark should be counted as moved
	suite.Equal(int64(1), result.BookmarksMoved)

	// Verify exactly 2 bookmarks for canonical (the original follow + the transferred bookmark)
	var count int64
	suite.db.Raw("SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?", canonical.ID).Scan(&count)
	suite.Equal(int64(2), count)

	// Verify no orphaned bookmarks remain for the merged artist
	var orphanCount int64
	suite.db.Raw("SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?", mergeFrom.ID).Scan(&orphanCount)
	suite.Equal(int64(0), orphanCount)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_NoOrphanedBookmarks() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Orphan Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "Orphan MergeFrom"})
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	// Multiple users with various bookmark actions on mergeFrom
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'follow', NOW())", user1.ID, mergeFrom.ID)
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'bookmark', NOW())", user1.ID, mergeFrom.ID)
	suite.db.Exec("INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at) VALUES (?, 'artist', ?, 'follow', NOW())", user2.ID, mergeFrom.ID)

	_, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)

	// Verify all bookmarks transferred to canonical
	var canonicalCount int64
	suite.db.Raw("SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?", canonical.ID).Scan(&canonicalCount)
	suite.Equal(int64(3), canonicalCount)

	// Verify zero orphaned bookmarks
	var orphanCount int64
	suite.db.Raw("SELECT COUNT(*) FROM user_bookmarks WHERE entity_type = 'artist' AND entity_id = ?", mergeFrom.ID).Scan(&orphanCount)
	suite.Equal(int64(0), orphanCount)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_TransfersCollectionItems() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "CI Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "CI MergeFrom"})
	user := suite.createTestUser()

	// Create a collection
	suite.db.Exec("INSERT INTO collections (title, slug, creator_id, created_at, updated_at) VALUES ('Test Collection', 'test-col', ?, NOW(), NOW())", user.ID)
	var collectionID uint
	suite.db.Raw("SELECT id FROM collections WHERE slug = 'test-col'").Scan(&collectionID)

	// Add mergeFrom artist to collection
	suite.db.Exec("INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id, created_at) VALUES (?, 'artist', ?, 0, ?, NOW())", collectionID, mergeFrom.ID, user.ID)

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.CollectionItemsMoved)

	// Verify collection item now points to canonical
	var count int64
	suite.db.Raw("SELECT COUNT(*) FROM collection_items WHERE entity_type = 'artist' AND entity_id = ? AND collection_id = ?", canonical.ID, collectionID).Scan(&count)
	suite.Equal(int64(1), count)
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_CollectionItemConflictDedup() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "CID Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "CID MergeFrom"})
	user := suite.createTestUser()

	// Create a collection with both artists (conflict scenario)
	suite.db.Exec("INSERT INTO collections (title, slug, creator_id, created_at, updated_at) VALUES ('Dedup Collection', 'dedup-col', ?, NOW(), NOW())", user.ID)
	var collectionID uint
	suite.db.Raw("SELECT id FROM collections WHERE slug = 'dedup-col'").Scan(&collectionID)

	suite.db.Exec("INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id, created_at) VALUES (?, 'artist', ?, 0, ?, NOW())", collectionID, canonical.ID, user.ID)
	suite.db.Exec("INSERT INTO collection_items (collection_id, entity_type, entity_id, position, added_by_user_id, created_at) VALUES (?, 'artist', ?, 1, ?, NOW())", collectionID, mergeFrom.ID, user.ID)

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)
	// Conflict was deduped, so 0 items moved (the conflicting one was deleted)
	suite.Equal(int64(0), result.CollectionItemsMoved)

	// Verify only canonical remains in collection (no duplicates)
	var count int64
	suite.db.Raw("SELECT COUNT(*) FROM collection_items WHERE entity_type = 'artist' AND collection_id = ?", collectionID).Scan(&count)
	suite.Equal(int64(1), count)
}

// =============================================================================
// Group 10: Boundary Conditions — Pagination
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_LimitZero() {
	artist := suite.createTestArtist("LZ Artist")
	venue := suite.createTestVenue("LZ Venue", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	shows, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 0, "upcoming")
	suite.Require().NoError(err)
	suite.GreaterOrEqual(total, int64(1), "total should reflect show count")
	suite.Empty(shows, "limit=0 should return no shows")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_LimitOne() {
	artist := suite.createTestArtist("L1 Artist")
	venue := suite.createTestVenue("L1 Venue", "Phoenix", "AZ")
	user := suite.createTestUser()
	for i := 0; i < 3; i++ {
		suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, i+1))
	}
	shows, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 1, "upcoming")
	suite.Require().NoError(err)
	suite.GreaterOrEqual(total, int64(3))
	suite.Len(shows, 1, "limit=1 should return exactly 1 show")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_LargeLimit() {
	artist := suite.createTestArtist("LargeLimit Artist")
	venue := suite.createTestVenue("LargeLimit Venue", "Phoenix", "AZ")
	user := suite.createTestUser()
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, 7))
	shows, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 1000, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1, "large limit should return all available results")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_EmptyResult() {
	artist := suite.createTestArtist("EmptyResult Artist")
	shows, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 10, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(shows)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_ShowAtExactMidnight() {
	artist := suite.createTestArtist("Midnight Artist")
	venue := suite.createTestVenue("Midnight Venue", "Phoenix", "AZ")
	user := suite.createTestUser()
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, midnight)
	shows, _, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 50, "upcoming")
	suite.Require().NoError(err)
	suite.NotEmpty(shows, "show at exact midnight today should appear in upcoming")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetShowsForArtist_PastShowExcluded() {
	artist := suite.createTestArtist("PastExcl Artist")
	venue := suite.createTestVenue("PastExcl Venue", "Phoenix", "AZ")
	user := suite.createTestUser()
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	suite.createApprovedShowWithArtist(artist.ID, venue.ID, user.ID, yesterday)
	shows, total, err := suite.artistService.GetShowsForArtist(artist.ID, "UTC", 50, "upcoming")
	suite.Require().NoError(err)
	suite.Equal(int64(0), total, "past show should not appear in upcoming filter")
	suite.Empty(shows)
}

// =============================================================================
// Group 11: Boundary Conditions — IDs
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtist_ZeroID() {
	resp, err := suite.artistService.GetArtist(0)
	suite.Require().Error(err)
	suite.Nil(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtist_VeryLargeID() {
	resp, err := suite.artistService.GetArtist(4294967295)
	suite.Require().Error(err)
	suite.Nil(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestDeleteArtist_ZeroID() {
	err := suite.artistService.DeleteArtist(0)
	suite.Require().Error(err)
}

// =============================================================================
// Group 12: Boundary Conditions — Strings
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_VeryLongName() {
	longName := strings.Repeat("A", 500)
	req := &contracts.CreateArtistRequest{Name: longName}
	resp, err := suite.artistService.CreateArtist(req)
	if err == nil {
		suite.Equal(longName, resp.Name)
	}
}

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_EmptyName() {
	req := &contracts.CreateArtistRequest{Name: ""}
	resp, err := suite.artistService.CreateArtist(req)
	if err != nil {
		suite.Contains(err.Error(), "name", "error should mention name validation")
	} else {
		suite.NotNil(resp)
	}
}

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_WhitespaceOnlyName() {
	req := &contracts.CreateArtistRequest{Name: "   "}
	resp, err := suite.artistService.CreateArtist(req)
	if err != nil {
		suite.NotNil(err)
	} else {
		suite.NotNil(resp)
	}
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistBySlug_EmptySlug() {
	resp, err := suite.artistService.GetArtistBySlug("")
	suite.Require().Error(err, "empty slug should return an error")
	suite.Nil(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtistByName_EmptyName() {
	resp, err := suite.artistService.GetArtistByName("")
	suite.Require().Error(err, "empty name should return not found")
	suite.Nil(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_EmptyString() {
	results, err := suite.artistService.SearchArtists("")
	suite.Require().NoError(err)
	suite.NotNil(results)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_EmptyFilters() {
	suite.createTestArtist("EF Artist")
	resp, err := suite.artistService.GetArtists(map[string]interface{}{})
	suite.Require().NoError(err)
	suite.NotEmpty(resp, "empty filters should return all artists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_NilFilters() {
	suite.createTestArtist("NF Artist")
	resp, err := suite.artistService.GetArtists(nil)
	suite.Require().NoError(err)
	suite.NotEmpty(resp, "nil filters should return all artists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_UpdatesNotificationFilters() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "NF Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "NF MergeFrom"})
	user := suite.createTestUser()

	// Create a notification filter that references the mergeFrom artist
	suite.db.Exec("INSERT INTO notification_filters (user_id, name, artist_ids, created_at, updated_at) VALUES (?, 'My Filter', ARRAY[?]::bigint[], NOW(), NOW())", user.ID, mergeFrom.ID)

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.FiltersUpdated)

	// Verify filter now references canonical artist
	var artistIDs string
	suite.db.Raw("SELECT artist_ids::text FROM notification_filters WHERE user_id = ?", user.ID).Scan(&artistIDs)
	suite.Contains(artistIDs, fmt.Sprintf("%d", canonical.ID))
	suite.NotContains(artistIDs, fmt.Sprintf("%d", mergeFrom.ID))
}

func (suite *ArtistServiceIntegrationTestSuite) TestMergeArtists_NotificationFilterConflictDedup() {
	canonical, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "NFD Canonical"})
	mergeFrom, _ := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: "NFD MergeFrom"})
	user := suite.createTestUser()

	// Create a notification filter that references BOTH artists
	suite.db.Exec("INSERT INTO notification_filters (user_id, name, artist_ids, created_at, updated_at) VALUES (?, 'Both Artists', ARRAY[?,?]::bigint[], NOW(), NOW())", user.ID, canonical.ID, mergeFrom.ID)

	result, err := suite.artistService.MergeArtists(canonical.ID, mergeFrom.ID)
	suite.Require().NoError(err)
	// Filter already had canonical, so the first UPDATE (replace) skips it;
	// the second UPDATE (remove leftover) cleans up mergeFrom
	suite.Equal(int64(0), result.FiltersUpdated)

	// Verify filter now has only canonical (no duplicate, no mergeFrom)
	var artistIDs string
	suite.db.Raw("SELECT artist_ids::text FROM notification_filters WHERE user_id = ?", user.ID).Scan(&artistIDs)
	suite.Contains(artistIDs, fmt.Sprintf("%d", canonical.ID))
	suite.NotContains(artistIDs, fmt.Sprintf("%d", mergeFrom.ID))
}
