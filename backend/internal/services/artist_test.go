package services

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewArtistService(t *testing.T) {
	artistService := NewArtistService(nil)
	assert.NotNil(t, artistService)
}

func TestArtistService_NilDatabase(t *testing.T) {
	svc := &ArtistService{db: nil}

	t.Run("CreateArtist", func(t *testing.T) {
		resp, err := svc.CreateArtist(&CreateArtistRequest{Name: "Test"})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetArtist", func(t *testing.T) {
		resp, err := svc.GetArtist(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetArtistByName", func(t *testing.T) {
		resp, err := svc.GetArtistByName("Test")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetArtistBySlug", func(t *testing.T) {
		resp, err := svc.GetArtistBySlug("test-slug")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetArtists", func(t *testing.T) {
		resp, err := svc.GetArtists(nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("UpdateArtist", func(t *testing.T) {
		resp, err := svc.UpdateArtist(1, map[string]interface{}{"name": "x"})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("DeleteArtist", func(t *testing.T) {
		err := svc.DeleteArtist(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("SearchArtists", func(t *testing.T) {
		resp, err := svc.SearchArtists("test")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetShowsForArtist", func(t *testing.T) {
		resp, total, err := svc.GetShowsForArtist(1, "UTC", 10, "upcoming")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("GetArtistCities", func(t *testing.T) {
		resp, err := svc.GetArtistCities()
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetArtistsWithShowCounts", func(t *testing.T) {
		resp, err := svc.GetArtistsWithShowCounts(nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ArtistServiceIntegrationTestSuite struct {
	suite.Suite
	container     testcontainers.Container
	db            *gorm.DB
	artistService *ArtistService
	ctx           context.Context
}

func (suite *ArtistServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Start PostgreSQL container
	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}
	testutil.RunAllMigrations(suite.T(), sqlDB, filepath.Join("..", "..", "db", "migrations"))

	suite.artistService = &ArtistService{db: db}
}

func (suite *ArtistServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

// TearDownTest cleans up data between tests for isolation
func (suite *ArtistServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
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
	req := &CreateArtistRequest{
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
	req := &CreateArtistRequest{Name: "The National"}
	_, err := suite.artistService.CreateArtist(req)
	suite.Require().NoError(err)

	// Same name, different case
	req2 := &CreateArtistRequest{Name: "the national"}
	_, err = suite.artistService.CreateArtist(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_WithSocialFields() {
	req := &CreateArtistRequest{
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
	req := &CreateArtistRequest{
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
	created, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Get Test Artist"})
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
	created, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Slug Test Artist"})
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
	_, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "The Beatles"})
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
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "PHX Artist", City: stringPtr("Phoenix")})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "TUC Artist", City: stringPtr("Tucson")})

	resp, err := suite.artistService.GetArtists(map[string]interface{}{"city": "Phoenix"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("PHX Artist", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_MultiCityFilter() {
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "PHX Band", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Mesa Band", City: stringPtr("Mesa"), State: stringPtr("AZ")})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "LA Band", City: stringPtr("Los Angeles"), State: stringPtr("CA")})

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
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "AZ Artist", State: stringPtr("AZ")})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "CA Artist", State: stringPtr("CA")})

	resp, err := suite.artistService.GetArtists(map[string]interface{}{"state": "CA"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("CA Artist", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestGetArtists_FilterByName() {
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Crescent Band"})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Valley Group"})

	resp, err := suite.artistService.GetArtists(map[string]interface{}{"name": "crescent"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Crescent Band", resp[0].Name)
}

// =============================================================================
// Group 4: UpdateArtist
// =============================================================================

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_BasicFields() {
	created, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Original Artist"})
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
	created, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Old Name Band"})
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
	_, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Existing Artist"})
	suite.Require().NoError(err)

	other, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Other Artist"})
	suite.Require().NoError(err)

	// Try to rename "Other Artist" to "Existing Artist"
	_, err = suite.artistService.UpdateArtist(other.ID, map[string]interface{}{"name": "Existing Artist"})

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_SameNameSameArtist_OK() {
	created, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Keep My Name"})
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
	created, err := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Delete Me"})
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
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Some Artist"})

	resp, err := suite.artistService.SearchArtists("")

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_ShortQuery_PrefixMatch() {
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Valley Heat"})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Crescent Moon"})

	resp, err := suite.artistService.SearchArtists("Va")

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Valley Heat", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_LongQuery_TrigramMatch() {
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Radiohead"})
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "The Rebel Lounge Band"})

	resp, err := suite.artistService.SearchArtists("Radiohead")

	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp)
	suite.Equal("Radiohead", resp[0].Name)
}

func (suite *ArtistServiceIntegrationTestSuite) TestSearchArtists_NoMatch() {
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Real Artist"})

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
	a1, _ := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "PHX Artist 1", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	a2, _ := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "PHX Artist 2", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	a3, _ := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Mesa Artist", City: stringPtr("Mesa"), State: stringPtr("AZ")})

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
	hasCityArtist, _ := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Has City", City: stringPtr("Tempe"), State: stringPtr("AZ")})
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
	pastArtist, _ := suite.artistService.CreateArtist(&CreateArtistRequest{Name: "Past Artist", City: stringPtr("Phoenix"), State: stringPtr("AZ")})
	suite.createApprovedShowWithArtist(pastArtist.ID, venue.ID, user.ID, time.Now().UTC().AddDate(0, 0, -7))

	// Artist with city but no shows at all should not appear
	suite.artistService.CreateArtist(&CreateArtistRequest{Name: "No Show Artist", City: stringPtr("Mesa"), State: stringPtr("AZ")})

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
