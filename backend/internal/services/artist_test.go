package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Run migrations
	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000002_add_artist_search_indexes.up.sql",
		"000003_add_venue_search_indexes.up.sql",
		"000004_update_venue_constraints.up.sql",
		"000005_add_show_status.up.sql",
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000020_add_show_status_flags.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
		"000032_add_favorite_cities.up.sql",
		// 000027 handled below (CONCURRENTLY not allowed in transaction)
	}
	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", m))
		if err != nil {
			suite.T().Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			suite.T().Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	// Run migration 000027 with CONCURRENTLY stripped (not valid in test context)
	migration27, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000027_add_index_duplicate_of_show_id.up.sql"))
	if err != nil {
		suite.T().Fatalf("failed to read migration 000027: %v", err)
	}
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	if err != nil {
		suite.T().Fatalf("failed to run migration 000027: %v", err)
	}

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
